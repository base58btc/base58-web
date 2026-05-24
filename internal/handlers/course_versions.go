package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kodylow/base58-website/internal/config"
)

type CourseVersion struct {
	ID            int64
	CourseSlug    string
	VersionNumber int
	Status        string
	ContentHash   string
}

type courseSnapshot struct {
	CourseSlug    string
	ContentHash   string
	StoragePrefix string
	Files         []courseSnapshotFile
}

type courseSnapshotFile struct {
	Path        string
	Content     string
	ContentHash string
}

func SyncLocalCourseVersions(ctx *config.AppContext) error {
	if ctx == nil || ctx.DB == nil {
		return nil
	}
	entries, err := os.ReadDir(localCoursesRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !isSafeCourseSlug(entry.Name()) {
			continue
		}
		if _, err := syncLocalCourseVersion(ctx, entry.Name()); err != nil {
			return err
		}
	}
	return nil
}

func syncLocalCourseVersion(ctx *config.AppContext, courseSlug string) (CourseVersion, error) {
	snapshot, err := buildLocalCourseSnapshot(courseSlug)
	if err != nil {
		return CourseVersion{}, err
	}
	if err := ensureCourseRowForVersion(ctx, courseSlug); err != nil {
		return CourseVersion{}, err
	}
	latest, err := latestCourseVersion(ctx, courseSlug)
	if err == nil && latest.ContentHash == snapshot.ContentHash {
		if err := ensureCourseVersionFiles(ctx, latest.ID, snapshot.Files); err != nil {
			return CourseVersion{}, err
		}
		return latest, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return CourseVersion{}, err
	}
	if err == nil && latest.VersionNumber == 1 && latest.ContentHash == "" {
		_, updateErr := ctx.DB.Exec(`UPDATE course_versions
SET content_hash=`+ph(ctx, 1)+`, storage_prefix=`+ph(ctx, 2)+`, published_at=COALESCE(published_at, CURRENT_TIMESTAMP), updated_at=CURRENT_TIMESTAMP
WHERE id=`+ph(ctx, 3), snapshot.ContentHash, snapshot.StoragePrefix, latest.ID)
		if updateErr != nil {
			return CourseVersion{}, updateErr
		}
		latest.ContentHash = snapshot.ContentHash
		if err := ensureCourseVersionFiles(ctx, latest.ID, snapshot.Files); err != nil {
			return CourseVersion{}, err
		}
		return latest, nil
	}

	nextNumber := 1
	diff := ""
	if err == nil {
		nextNumber = latest.VersionNumber + 1
		previousFiles, _ := listCourseVersionFileContents(ctx, latest.ID)
		diff = buildCourseVersionDiff(previousFiles, snapshot.Files)
	}

	var version CourseVersion
	err = ctx.DB.QueryRow(`INSERT INTO course_versions (course_slug, version_number, status, source, content_hash, storage_prefix, diff_from_previous, published_at)
VALUES (`+ph(ctx, 1)+`, `+ph(ctx, 2)+`, 'published', 'local_md', `+ph(ctx, 3)+`, `+ph(ctx, 4)+`, `+ph(ctx, 5)+`, CURRENT_TIMESTAMP)
RETURNING id, course_slug, version_number, status, content_hash`, courseSlug, nextNumber, snapshot.ContentHash, snapshot.StoragePrefix, diff).
		Scan(&version.ID, &version.CourseSlug, &version.VersionNumber, &version.Status, &version.ContentHash)
	if err != nil {
		return CourseVersion{}, err
	}
	if err := ensureCourseVersionFiles(ctx, version.ID, snapshot.Files); err != nil {
		return CourseVersion{}, err
	}
	return version, nil
}

func ensureCourseRowForVersion(ctx *config.AppContext, courseSlug string) error {
	_, err := ctx.DB.Exec(`INSERT INTO courses (slug, title, status)
VALUES (`+ph(ctx, 1)+`, `+ph(ctx, 2)+`, 'published')
ON CONFLICT (slug) DO NOTHING`, courseSlug, titleFromSlug(courseSlug))
	return err
}

func latestCourseVersion(ctx *config.AppContext, courseSlug string) (CourseVersion, error) {
	var version CourseVersion
	err := ctx.DB.QueryRow(`SELECT id, course_slug, version_number, status, content_hash
FROM course_versions
WHERE course_slug=`+ph(ctx, 1)+` AND status='published'
ORDER BY version_number DESC
LIMIT 1`, courseSlug).Scan(&version.ID, &version.CourseSlug, &version.VersionNumber, &version.Status, &version.ContentHash)
	return version, err
}

func buildLocalCourseSnapshot(courseSlug string) (courseSnapshot, error) {
	if !isSafeCourseSlug(courseSlug) {
		return courseSnapshot{}, errLocalCourseNotFound
	}
	courseDir := filepath.Join(localCoursesRoot, courseSlug)
	var files []courseSnapshotFile
	err := filepath.WalkDir(courseDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(courseDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		files = append(files, courseSnapshotFile{
			Path:        rel,
			Content:     string(content),
			ContentHash: hashString(string(content)),
		})
		return nil
	})
	if err != nil {
		return courseSnapshot{}, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	manifest := strings.Builder{}
	for _, file := range files {
		manifest.WriteString(file.Path)
		manifest.WriteString(":")
		manifest.WriteString(file.ContentHash)
		manifest.WriteString("\n")
	}
	return courseSnapshot{
		CourseSlug:    courseSlug,
		ContentHash:   hashString(manifest.String()),
		StoragePrefix: "local:" + filepath.ToSlash(courseDir),
		Files:         files,
	}, nil
}

func ensureCourseVersionFiles(ctx *config.AppContext, versionID int64, files []courseSnapshotFile) error {
	for _, file := range files {
		_, err := ctx.DB.Exec(`INSERT INTO course_version_files (course_version_id, path, content_hash, content)
VALUES (`+ph(ctx, 1)+`, `+ph(ctx, 2)+`, `+ph(ctx, 3)+`, `+ph(ctx, 4)+`)
ON CONFLICT (course_version_id, path) DO UPDATE SET
  content_hash = EXCLUDED.content_hash,
  content = EXCLUDED.content`, versionID, file.Path, file.ContentHash, file.Content)
		if err != nil {
			return err
		}
	}
	return nil
}

func listCourseVersionFileContents(ctx *config.AppContext, versionID int64) (map[string]string, error) {
	rows, err := ctx.DB.Query(`SELECT path, content FROM course_version_files WHERE course_version_id=`+ph(ctx, 1), versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := make(map[string]string)
	for rows.Next() {
		var path, content string
		if err := rows.Scan(&path, &content); err != nil {
			return nil, err
		}
		files[path] = content
	}
	return files, rows.Err()
}

func loadCourseVersion(ctx *config.AppContext, versionID int64, courseSlug, currentPath string) (*localCourseOutline, error) {
	files, err := listCourseVersionFileContents(ctx, versionID)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errLocalCourseNotFound
	}
	return buildCourseOutlineFromFiles(courseSlug, currentPath, files)
}

func buildCourseOutlineFromFiles(courseSlug, currentPath string, files map[string]string) (*localCourseOutline, error) {
	if !isSafeCourseSlug(courseSlug) {
		return nil, errLocalCourseNotFound
	}
	cleanCurrentPath, ok := cleanLessonPath(currentPath)
	if !ok {
		return nil, errLocalCoursePageNotFound
	}

	outline := &localCourseOutline{
		CourseSlug:  courseSlug,
		CourseTitle: titleFromSlug(courseSlug),
	}
	itemsByNumber := make(map[string]*CourseNavItem)
	childFilesByDir := make(map[string][]string)
	var topItems []*CourseNavItem

	var paths []string
	for path := range files {
		if strings.HasSuffix(path, ".md") {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		dir, filename := filepath.Split(path)
		dir = strings.TrimSuffix(filepath.ToSlash(dir), "/")
		if dir != "" {
			childFilesByDir[dir] = append(childFilesByDir[dir], filename)
			continue
		}
		item, ok := navItemFromVersionFile(courseSlug, "", filename, files[path])
		if !ok {
			continue
		}
		itemsByNumber[item.Number] = item
		topItems = append(topItems, item)
	}

	var dirs []string
	for dir := range childFilesByDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		number, label, ok := parseNumberedName(filepath.Base(dir))
		if !ok {
			continue
		}
		parent, exists := itemsByNumber[number]
		if !exists {
			parent = &CourseNavItem{
				Number:        number,
				DisplayNumber: number,
				Title:         titleFromSlug(label),
				Path:          dir,
				order:         orderFromNumber(number),
			}
		}
		children := make([]*CourseNavItem, 0)
		for _, filename := range childFilesByDir[dir] {
			path := dir + "/" + filename
			child, ok := navItemFromVersionFile(courseSlug, dir, filename, files[path])
			if ok {
				child.DisplayNumber = number + "." + child.Number
				children = append(children, child)
			}
		}
		sortNavItems(children)
		if parent.filePath == "" && len(children) == 0 {
			continue
		}
		if !exists {
			topItems = append(topItems, parent)
			itemsByNumber[number] = parent
		}
		parent.Children = children
		if parent.filePath == "" && len(children) > 0 {
			parent.URL = children[0].URL
		}
	}
	sortNavItems(topItems)

	var pages []*CourseNavItem
	for _, item := range topItems {
		if item.filePath != "" {
			pages = append(pages, item)
		}
		pages = append(pages, item.Children...)
	}
	if len(pages) == 0 {
		return nil, errLocalCourseEmpty
	}

	outline.Items = topItems
	outline.Pages = pages
	outline.Current = selectCurrentPage(topItems, pages, cleanCurrentPath)
	if outline.Current == nil {
		return nil, errLocalCoursePageNotFound
	}
	markdown, ok := files[outline.Current.Path+".md"]
	if !ok {
		return nil, errLocalCoursePageNotFound
	}
	outline.Markdown = []byte(markdown)
	for i, page := range pages {
		page.Current = page == outline.Current
		page.Active = page.Current
		if page.Current {
			if i > 0 {
				outline.Previous = pages[i-1]
			}
			if i < len(pages)-1 {
				outline.Next = pages[i+1]
			}
		}
	}
	markActiveParents(topItems)
	return outline, nil
}

func navItemFromVersionFile(courseSlug, sectionDir, filename string, content string) (*CourseNavItem, bool) {
	slug := strings.TrimSuffix(filename, ".md")
	number, label, ok := parseNumberedName(slug)
	if !ok {
		return nil, false
	}
	lessonPath := slug
	if sectionDir != "" {
		lessonPath = sectionDir + "/" + slug
	}
	return &CourseNavItem{
		Number:         number,
		DisplayNumber:  number,
		Title:          titleFromMarkdown([]byte(content), titleFromSlug(label)),
		Path:           lessonPath,
		URL:            courseLessonURL(courseSlug, lessonPath),
		ChallengeCount: countCourseChallengeBlocks([]byte(content)),
		order:          orderFromNumber(number),
		filePath:       lessonPath + ".md",
	}, true
}

func buildCourseVersionDiff(previous map[string]string, current []courseSnapshotFile) string {
	currentByPath := make(map[string]string)
	for _, file := range current {
		currentByPath[file.Path] = file.Content
	}
	var paths []string
	for path := range previous {
		paths = append(paths, path)
	}
	for path := range currentByPath {
		if _, ok := previous[path]; !ok {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)

	var b strings.Builder
	for _, path := range paths {
		oldContent, oldOK := previous[path]
		newContent, newOK := currentByPath[path]
		if oldOK && newOK && oldContent == newContent {
			continue
		}
		fmt.Fprintf(&b, "diff -- %s\n", path)
		if oldOK {
			fmt.Fprintf(&b, "--- a/%s\n", path)
		} else {
			fmt.Fprintln(&b, "--- /dev/null")
		}
		if newOK {
			fmt.Fprintf(&b, "+++ b/%s\n", path)
		} else {
			fmt.Fprintln(&b, "+++ /dev/null")
		}
		fmt.Fprintln(&b, "@@")
		if oldOK {
			for _, line := range strings.Split(strings.TrimRight(oldContent, "\n"), "\n") {
				fmt.Fprintf(&b, "-%s\n", line)
			}
		}
		if newOK {
			for _, line := range strings.Split(strings.TrimRight(newContent, "\n"), "\n") {
				fmt.Fprintf(&b, "+%s\n", line)
			}
		}
	}
	return b.String()
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
