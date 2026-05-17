package handlers

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/kodylow/base58-website/internal/config"
)

const localCoursesRoot = "courses"

var numberedNamePattern = regexp.MustCompile(`^([0-9]+)[_-](.+)$`)
var safeCourseSlugPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

type CourseNavItem struct {
	Number        string
	DisplayNumber string
	Title         string
	Path          string
	URL           string
	Current       bool
	Active        bool
	Children      []*CourseNavItem

	order    int
	filePath string
}

type CourseLessonData struct {
	Page        Page
	CourseSlug  string
	CourseTitle string
	Current     *CourseNavItem
	Previous    *CourseNavItem
	Next        *CourseNavItem
	Sidebar     []*CourseNavItem
	ContentHTML template.HTML
	HasCode     bool
}

type localCourseOutline struct {
	CourseSlug  string
	CourseTitle string
	Items       []*CourseNavItem
	Pages       []*CourseNavItem
	Current     *CourseNavItem
	Previous    *CourseNavItem
	Next        *CourseNavItem
	Markdown    []byte
}

var errLocalCourseNotFound = errors.New("local course not found")
var errLocalCoursePageNotFound = errors.New("local course page not found")
var errLocalCourseEmpty = errors.New("local course has no pages")

func CourseLesson(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	params := mux.Vars(r)
	courseSlug := params["course"]
	pagePath := params["page"]

	outline, err := loadLocalCourse(localCoursesRoot, courseSlug, pagePath)
	if err != nil {
		switch {
		case errors.Is(err, errLocalCourseNotFound), errors.Is(err, errLocalCoursePageNotFound):
			http.NotFound(w, r)
		case errors.Is(err, errLocalCourseEmpty):
			http.Error(w, "Course has no pages", http.StatusInternalServerError)
		default:
			http.Error(w, "Unable to load course page", http.StatusInternalServerError)
		}
		ctx.Err.Printf("/courses/%s/%s local course load failed %s\n", courseSlug, pagePath, err.Error())
		return
	}

	title := outline.Current.Title
	if outline.CourseTitle != "" {
		title = fmt.Sprintf("%s: %s", outline.CourseTitle, outline.Current.Title)
	}

	furlCard := defaultCard(ctx, r, title)
	err = ctx.TemplateCache.ExecuteTemplate(w, "courses/lesson.tmpl", CourseLessonData{
		Page:        getPage(ctx, title, furlCard),
		CourseSlug:  outline.CourseSlug,
		CourseTitle: outline.CourseTitle,
		Current:     outline.Current,
		Previous:    outline.Previous,
		Next:        outline.Next,
		Sidebar:     outline.Items,
		ContentHTML: template.HTML(courseMarkdownToHTML(outline.Markdown)),
		HasCode:     markdownHasCourseCode(outline.Markdown),
	})
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("courses/lesson.tmpl exec failed %s\n", err.Error())
		return
	}
}

func localCourseExists(courseSlug string) bool {
	if !isSafeCourseSlug(courseSlug) {
		return false
	}

	info, err := os.Stat(filepath.Join(localCoursesRoot, courseSlug))
	return err == nil && info.IsDir()
}

func loadLocalCourse(rootDir, courseSlug, currentPath string) (*localCourseOutline, error) {
	if !isSafeCourseSlug(courseSlug) {
		return nil, errLocalCourseNotFound
	}

	cleanCurrentPath, ok := cleanLessonPath(currentPath)
	if !ok {
		return nil, errLocalCoursePageNotFound
	}

	courseDir := filepath.Join(rootDir, courseSlug)
	info, err := os.Stat(courseDir)
	if err != nil || !info.IsDir() {
		return nil, errLocalCourseNotFound
	}

	outline := &localCourseOutline{
		CourseSlug:  courseSlug,
		CourseTitle: titleFromSlug(courseSlug),
	}

	topItems, pages, err := readTopLevelCourseItems(courseDir, courseSlug)
	if err != nil {
		return nil, err
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

	markdown, err := os.ReadFile(outline.Current.filePath)
	if err != nil {
		return nil, err
	}
	outline.Markdown = markdown

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

func readTopLevelCourseItems(courseDir, courseSlug string) ([]*CourseNavItem, []*CourseNavItem, error) {
	entries, err := os.ReadDir(courseDir)
	if err != nil {
		return nil, nil, err
	}

	itemsByNumber := make(map[string]*CourseNavItem)
	var topItems []*CourseNavItem

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		item, ok, err := navItemFromFile(courseDir, courseSlug, "", entry.Name())
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			continue
		}

		itemsByNumber[item.Number] = item
		topItems = append(topItems, item)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		number, label, ok := parseNumberedName(entry.Name())
		if !ok {
			continue
		}

		parent, ok := itemsByNumber[number]
		if !ok {
			parent = &CourseNavItem{
				Number:        number,
				DisplayNumber: number,
				Title:         titleFromSlug(label),
				Path:          entry.Name(),
				order:         orderFromNumber(number),
			}
		}

		children, err := readCourseSubpages(courseDir, courseSlug, entry.Name(), number)
		if err != nil {
			return nil, nil, err
		}
		if parent.filePath == "" && len(children) == 0 {
			continue
		}

		if !ok {
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

	return topItems, pages, nil
}

func readCourseSubpages(courseDir, courseSlug, sectionDir, parentNumber string) ([]*CourseNavItem, error) {
	entries, err := os.ReadDir(filepath.Join(courseDir, sectionDir))
	if err != nil {
		return nil, err
	}

	children := make([]*CourseNavItem, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		item, ok, err := navItemFromFile(courseDir, courseSlug, sectionDir, entry.Name())
		if err != nil {
			return nil, err
		}
		if ok {
			item.DisplayNumber = parentNumber + "." + item.Number
			children = append(children, item)
		}
	}

	sortNavItems(children)
	return children, nil
}

func navItemFromFile(courseDir, courseSlug, sectionDir, filename string) (*CourseNavItem, bool, error) {
	slug := strings.TrimSuffix(filename, ".md")
	number, label, ok := parseNumberedName(slug)
	if !ok {
		return nil, false, nil
	}

	lessonPath := slug
	if sectionDir != "" {
		lessonPath = sectionDir + "/" + slug
	}

	filePath := filepath.Join(courseDir, filepath.FromSlash(lessonPath)+".md")
	markdown, err := os.ReadFile(filePath)
	if err != nil {
		return nil, false, err
	}

	return &CourseNavItem{
		Number:        number,
		DisplayNumber: number,
		Title:         titleFromMarkdown(markdown, titleFromSlug(label)),
		Path:          lessonPath,
		URL:           courseLessonURL(courseSlug, lessonPath),
		order:         orderFromNumber(number),
		filePath:      filePath,
	}, true, nil
}

func selectCurrentPage(topItems, pages []*CourseNavItem, currentPath string) *CourseNavItem {
	if currentPath == "" {
		return pages[0]
	}

	for _, page := range pages {
		if page.Path == currentPath {
			return page
		}
	}

	for _, item := range topItems {
		if item.Path == currentPath && item.filePath == "" && len(item.Children) > 0 {
			return item.Children[0]
		}
	}

	return nil
}

func markActiveParents(items []*CourseNavItem) {
	for _, item := range items {
		for _, child := range item.Children {
			if child.Active {
				item.Active = true
				return
			}
		}
	}
}

func sortNavItems(items []*CourseNavItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].order == items[j].order {
			return items[i].Path < items[j].Path
		}
		return items[i].order < items[j].order
	})
}

func parseNumberedName(name string) (string, string, bool) {
	matches := numberedNamePattern.FindStringSubmatch(name)
	if len(matches) != 3 {
		return "", "", false
	}
	return matches[1], matches[2], true
}

func orderFromNumber(number string) int {
	order, err := strconv.Atoi(number)
	if err != nil {
		return 0
	}
	return order
}

func cleanLessonPath(rawPath string) (string, bool) {
	rawPath = strings.TrimSpace(strings.Trim(rawPath, "/"))
	if rawPath == "" {
		return "", true
	}
	rawPath = strings.TrimSuffix(rawPath, ".md")
	if strings.Contains(rawPath, "\\") {
		return "", false
	}

	parts := strings.Split(rawPath, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, ".") {
			return "", false
		}
	}

	return strings.Join(parts, "/"), true
}

func isSafeCourseSlug(courseSlug string) bool {
	return safeCourseSlugPattern.MatchString(courseSlug)
}

func titleFromMarkdown(markdown []byte, fallback string) string {
	scanner := bufio.NewScanner(bytes.NewReader(markdown))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "#") {
			continue
		}

		level := 0
		for level < len(line) && line[level] == '#' {
			level++
		}
		if level < len(line) && line[level] == ' ' {
			title := strings.TrimSpace(line[level+1:])
			if title != "" {
				return title
			}
		}
	}

	return fallback
}

func titleFromSlug(slug string) string {
	words := strings.FieldsFunc(slug, func(r rune) bool {
		return r == '-' || r == '_' || r == ' '
	})
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

func courseLessonURL(courseSlug, lessonPath string) string {
	return fmt.Sprintf("/courses/%s/%s", courseSlug, lessonPath)
}
