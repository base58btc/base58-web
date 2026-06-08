package handlers

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gorilla/mux"
	"github.com/kodylow/base58-website/internal/config"
)

type CourseEditorSyncRequest struct {
	Files []CourseEditorSyncFile `json:"files"`
}

type CourseEditorSyncFile struct {
	Path        string `json:"path"`
	Content     string `json:"content"`
	ContentHash string `json:"sha256,omitempty"`
}

type CourseEditorSyncResponse struct {
	CourseSlug    string `json:"courseSlug"`
	VersionID     int64  `json:"versionId"`
	VersionNumber int    `json:"versionNumber"`
	Status        string `json:"status"`
	ContentHash   string `json:"contentHash"`
	FileCount     int    `json:"fileCount"`
}

type CourseEditorPublishRequest struct {
	VersionID int64 `json:"versionId,omitempty"`
}

func CourseEditorSync(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if ctx == nil || ctx.DB == nil {
		writeCourseEditorJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database is not configured"})
		return
	}
	if !authorizeCourseEditorSync(w, r, ctx) {
		return
	}

	courseSlug := mux.Vars(r)["course"]
	if !isSafeCourseSlug(courseSlug) {
		writeCourseEditorJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid course slug"})
		return
	}

	var req CourseEditorSyncRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20)).Decode(&req); err != nil {
		writeCourseEditorJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sync payload"})
		return
	}

	snapshot, err := buildUploadedCourseSnapshot(courseSlug, req.Files)
	if err != nil {
		writeCourseEditorJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	version, err := syncDraftCourseVersion(ctx, snapshot)
	if err != nil {
		ctx.Err.Printf("course editor sync failed: %s", err.Error())
		writeCourseEditorJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to sync course"})
		return
	}

	writeCourseEditorJSON(w, http.StatusOK, CourseEditorSyncResponse{
		CourseSlug:    version.CourseSlug,
		VersionID:     version.ID,
		VersionNumber: version.VersionNumber,
		Status:        version.Status,
		ContentHash:   version.ContentHash,
		FileCount:     len(snapshot.Files),
	})
}

func CourseEditorPublish(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if ctx == nil || ctx.DB == nil {
		writeCourseEditorJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database is not configured"})
		return
	}
	if !authorizeCourseEditorSync(w, r, ctx) {
		return
	}

	courseSlug := mux.Vars(r)["course"]
	if !isSafeCourseSlug(courseSlug) {
		writeCourseEditorJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid course slug"})
		return
	}

	var req CourseEditorPublishRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeCourseEditorJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid publish payload"})
		return
	}

	version, fileCount, err := publishDraftCourseVersion(ctx, courseSlug, req.VersionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeCourseEditorJSON(w, http.StatusNotFound, map[string]string{"error": "no matching draft version to publish"})
			return
		}
		ctx.Err.Printf("course editor publish failed: %s", err.Error())
		writeCourseEditorJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to publish course"})
		return
	}

	writeCourseEditorJSON(w, http.StatusOK, CourseEditorSyncResponse{
		CourseSlug:    version.CourseSlug,
		VersionID:     version.ID,
		VersionNumber: version.VersionNumber,
		Status:        version.Status,
		ContentHash:   version.ContentHash,
		FileCount:     fileCount,
	})
}

func authorizeCourseEditorSync(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) bool {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token != "" {
		if ctx.Env != nil && ctx.Env.EditorSyncToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(ctx.Env.EditorSyncToken)) == 1 {
			return true
		}

		var id int64
		err := ctx.DB.QueryRow(`SELECT id FROM editor_api_tokens
WHERE token_hash=`+ph(ctx, 1)+`
  AND status='active'
  AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)`, hashString(token)).Scan(&id)
		if err != nil {
			writeCourseEditorJSON(w, http.StatusForbidden, map[string]string{"error": "invalid editor token"})
			return false
		}
		_, _ = ctx.DB.Exec(`UPDATE editor_api_tokens SET last_used_at=CURRENT_TIMESTAMP WHERE id=`+ph(ctx, 1), id)
		return true
	}

	if admin := currentAdminFromSession(r, ctx); admin.ID > 0 {
		return true
	}
	writeCourseEditorJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing editor token"})
	return false
}

func buildUploadedCourseSnapshot(courseSlug string, files []CourseEditorSyncFile) (courseSnapshot, error) {
	if len(files) == 0 {
		return courseSnapshot{}, errors.New("course sync needs at least one markdown file")
	}

	seen := make(map[string]bool)
	out := make([]courseSnapshotFile, 0, len(files))
	for _, file := range files {
		path, ok := cleanUploadedCoursePath(file.Path)
		if !ok {
			return courseSnapshot{}, errors.New("course sync contains an invalid file path")
		}
		if seen[path] {
			return courseSnapshot{}, errors.New("course sync contains duplicate file paths")
		}
		seen[path] = true
		contentHash := hashString(file.Content)
		if file.ContentHash != "" && file.ContentHash != contentHash {
			return courseSnapshot{}, errors.New("course sync contains a file hash mismatch")
		}
		out = append(out, courseSnapshotFile{
			Path:        path,
			Content:     file.Content,
			ContentHash: contentHash,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })

	manifest := strings.Builder{}
	for _, file := range out {
		manifest.WriteString(file.Path)
		manifest.WriteString(":")
		manifest.WriteString(file.ContentHash)
		manifest.WriteString("\n")
	}
	return courseSnapshot{
		CourseSlug:    courseSlug,
		ContentHash:   hashString(manifest.String()),
		StoragePrefix: "editor-sync:" + courseSlug,
		Files:         out,
	}, nil
}

func cleanUploadedCoursePath(path string) (string, bool) {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, "\x00") {
		return "", false
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." || !strings.HasSuffix(clean, ".md") {
		return "", false
	}
	return clean, true
}

func syncDraftCourseVersion(ctx *config.AppContext, snapshot courseSnapshot) (CourseVersion, error) {
	if err := ensureCourseRowForVersion(ctx, snapshot.CourseSlug); err != nil {
		return CourseVersion{}, err
	}

	var version CourseVersion
	err := ctx.DB.QueryRow(`SELECT id, course_slug, version_number, status, content_hash
FROM course_versions
WHERE course_slug=`+ph(ctx, 1)+` AND status='draft' AND source='cli_sync'
ORDER BY version_number DESC
LIMIT 1`, snapshot.CourseSlug).Scan(&version.ID, &version.CourseSlug, &version.VersionNumber, &version.Status, &version.ContentHash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return CourseVersion{}, err
	}

	if errors.Is(err, sql.ErrNoRows) {
		nextNumber := 1
		var latestNumber sql.NullInt64
		if scanErr := ctx.DB.QueryRow(`SELECT MAX(version_number) FROM course_versions WHERE course_slug=`+ph(ctx, 1), snapshot.CourseSlug).Scan(&latestNumber); scanErr != nil {
			return CourseVersion{}, scanErr
		}
		if latestNumber.Valid {
			nextNumber = int(latestNumber.Int64) + 1
		}

		previousFiles := map[string]string{}
		if latest, latestErr := latestCourseVersion(ctx, snapshot.CourseSlug); latestErr == nil {
			previousFiles, _ = listCourseVersionFileContents(ctx, latest.ID)
		}
		diff := buildCourseVersionDiff(previousFiles, snapshot.Files)

		err = ctx.DB.QueryRow(`INSERT INTO course_versions (course_slug, version_number, status, source, content_hash, storage_prefix, diff_from_previous)
VALUES (`+ph(ctx, 1)+`, `+ph(ctx, 2)+`, 'draft', 'cli_sync', `+ph(ctx, 3)+`, `+ph(ctx, 4)+`, `+ph(ctx, 5)+`)
RETURNING id, course_slug, version_number, status, content_hash`, snapshot.CourseSlug, nextNumber, snapshot.ContentHash, snapshot.StoragePrefix, diff).
			Scan(&version.ID, &version.CourseSlug, &version.VersionNumber, &version.Status, &version.ContentHash)
		if err != nil {
			return CourseVersion{}, err
		}
	} else {
		previousFiles := map[string]string{}
		if latest, latestErr := latestCourseVersion(ctx, snapshot.CourseSlug); latestErr == nil {
			previousFiles, _ = listCourseVersionFileContents(ctx, latest.ID)
		}
		diff := buildCourseVersionDiff(previousFiles, snapshot.Files)

		_, err = ctx.DB.Exec(`UPDATE course_versions
SET content_hash=`+ph(ctx, 1)+`, storage_prefix=`+ph(ctx, 2)+`, diff_from_previous=`+ph(ctx, 3)+`, updated_at=CURRENT_TIMESTAMP
WHERE id=`+ph(ctx, 4), snapshot.ContentHash, snapshot.StoragePrefix, diff, version.ID)
		if err != nil {
			return CourseVersion{}, err
		}
		version.ContentHash = snapshot.ContentHash
	}

	if err := replaceCourseVersionFiles(ctx, version.ID, snapshot.Files); err != nil {
		return CourseVersion{}, err
	}
	return version, nil
}

func replaceCourseVersionFiles(ctx *config.AppContext, versionID int64, files []courseSnapshotFile) error {
	tx, err := ctx.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM course_version_files WHERE course_version_id=`+ph(ctx, 1), versionID); err != nil {
		return err
	}
	for _, file := range files {
		_, err := tx.Exec(`INSERT INTO course_version_files (course_version_id, path, content_hash, content)
VALUES (`+ph(ctx, 1)+`, `+ph(ctx, 2)+`, `+ph(ctx, 3)+`, `+ph(ctx, 4)+`)`, versionID, file.Path, file.ContentHash, file.Content)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func publishDraftCourseVersion(ctx *config.AppContext, courseSlug string, versionID int64) (CourseVersion, int, error) {
	var draft CourseVersion
	var err error
	if versionID > 0 {
		err = ctx.DB.QueryRow(`SELECT id, course_slug, version_number, status, content_hash
FROM course_versions
WHERE id=`+ph(ctx, 1)+` AND course_slug=`+ph(ctx, 2)+` AND status='draft' AND source='cli_sync'`, versionID, courseSlug).
			Scan(&draft.ID, &draft.CourseSlug, &draft.VersionNumber, &draft.Status, &draft.ContentHash)
	} else {
		err = ctx.DB.QueryRow(`SELECT id, course_slug, version_number, status, content_hash
FROM course_versions
WHERE course_slug=`+ph(ctx, 1)+` AND status='draft' AND source='cli_sync'
ORDER BY version_number DESC
LIMIT 1`, courseSlug).
			Scan(&draft.ID, &draft.CourseSlug, &draft.VersionNumber, &draft.Status, &draft.ContentHash)
	}
	if err != nil {
		return CourseVersion{}, 0, err
	}

	existing, err := publishedCourseVersionByHash(ctx, courseSlug, draft.ContentHash)
	if err == nil {
		_, deleteErr := ctx.DB.Exec(`DELETE FROM course_versions WHERE id=`+ph(ctx, 1), draft.ID)
		if deleteErr != nil {
			return CourseVersion{}, 0, deleteErr
		}
		count, countErr := countCourseVersionFiles(ctx, existing.ID)
		return existing, count, countErr
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return CourseVersion{}, 0, err
	}

	err = ctx.DB.QueryRow(`UPDATE course_versions
SET status='published', published_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP
WHERE id=`+ph(ctx, 1)+`
RETURNING id, course_slug, version_number, status, content_hash`, draft.ID).
		Scan(&draft.ID, &draft.CourseSlug, &draft.VersionNumber, &draft.Status, &draft.ContentHash)
	if err != nil {
		return CourseVersion{}, 0, err
	}
	count, err := countCourseVersionFiles(ctx, draft.ID)
	return draft, count, err
}

func countCourseVersionFiles(ctx *config.AppContext, versionID int64) (int, error) {
	var count int
	err := ctx.DB.QueryRow(`SELECT COUNT(*) FROM course_version_files WHERE course_version_id=`+ph(ctx, 1), versionID).Scan(&count)
	return count, err
}

func writeCourseEditorJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
