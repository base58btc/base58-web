package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gorilla/mux"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/courseenv"
)

// Environment readiness is a property of uploaded content, not a queued build.
func courseEnvironmentState(ctx *config.AppContext, versionID int64) (string, string) {
	files, err := listCourseVersionFileContents(ctx, versionID)
	if err != nil {
		return "unavailable", "unable to read course environment"
	}
	if _, ok := files[courseFlakeFile]; !ok {
		if _, hasRuntime := files[courseenv.ManifestFile]; hasRuntime {
			return "unprepared", "course runtime requires flake.nix and flake.lock"
		}
		return "", ""
	}
	if _, err := courseenv.ForFiles(files); err != nil {
		return "unprepared", err.Error()
	}
	return "ready", ""
}

// Serve only the manifest and sources pinned to this student's attempt. Commands
// never pass through this endpoint; the browser executes them in its own guest.
func CourseBrowserEnvironment(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	w.Header().Set("Cache-Control", "no-store")
	courseSlug := mux.Vars(r)["course"]
	if ctx == nil || ctx.DB == nil || !isSafeCourseSlug(courseSlug) {
		http.Error(w, "Course environment is unavailable", http.StatusServiceUnavailable)
		return
	}
	personID := currentProgressPersonID(r, ctx)
	var versionID int64
	var workspace string
	if r.URL.Query().Get("preview") == "draft" && canPreviewCourseDraft(r, ctx, personID, courseSlug) {
		version, err := latestDraftCourseVersion(ctx, courseSlug)
		if err == nil {
			versionID = version.ID
			workspace = fmt.Sprintf("draft-%d-%d", personID, versionID)
		}
	} else if personID > 0 && hasActiveCourseEntitlement(ctx, personID, courseSlug) {
		attempt, err := activeCourseAttempt(ctx, personID, courseSlug)
		if err == nil {
			versionID = attempt.CourseVersionID
			workspace = fmt.Sprintf("attempt-%d", attempt.ID)
		}
	}
	if versionID == 0 {
		http.Error(w, "Course access and an active attempt are required", http.StatusForbidden)
		return
	}
	files, err := listCourseVersionFileContents(ctx, versionID)
	if err != nil {
		http.Error(w, "Unable to read course environment", http.StatusInternalServerError)
		return
	}
	manifest, err := courseenv.ForFiles(files)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	delete(files, courseenv.ManifestFile)
	contentHash := courseenv.ContentHash(files)
	assetBase := courseAssetBaseURL(ctx)
	assetURL := strings.TrimRight(assetBase, "/") + "/" + url.PathEscape(courseSlug) + "/"
	if !ctx.IsProd && localRuntimeAvailable(courseSlug, manifest) {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		assetBase = scheme + "://" + r.Host + "/_course-runtime"
		assetURL = assetBase + "/" + url.PathEscape(courseSlug) + "/"
	}
	if assetBase == "" {
		http.Error(w, "Course environment assets are not configured", http.StatusServiceUnavailable)
		return
	}
	journalURL := ""
	if strings.HasPrefix(workspace, "attempt-") {
		journalURL = "/api/courses/" + url.PathEscape(courseSlug) + "/commands?environment=" + contentHash
	}
	writeCourseEditorJSON(w, http.StatusOK, map[string]any{
		"manifest": manifest, "files": files,
		"contentHash":  contentHash,
		"journalURL":   journalURL,
		"workspace":    workspace + "-" + contentHash,
		"assetBaseURL": assetURL,
	})
}

var localRuntimePath = regexp.MustCompile(`^runtime/[a-f0-9]{64}/[a-z0-9]{32}-[A-Za-z0-9+._?=-]+\.nar\.gz$`)

func localRuntimeFile(slug, archive string) (string, bool) {
	if !isSafeCourseSlug(slug) || !localRuntimePath.MatchString(archive) {
		return "", false
	}
	root, err := filepath.Abs(localCoursesRoot)
	if err != nil {
		return "", false
	}
	path, err := filepath.EvalSymlinks(filepath.Join(root, slug, filepath.FromSlash(archive)))
	if err != nil || !strings.HasPrefix(path, root+string(filepath.Separator)) {
		return "", false
	}
	info, err := os.Stat(path)
	return path, err == nil && info.Mode().IsRegular()
}

func localRuntimeAvailable(slug string, manifest *courseenv.Manifest) bool {
	for _, archive := range manifest.Archives {
		path, ok := localRuntimeFile(slug, archive.Path)
		if !ok {
			return false
		}
		info, err := os.Stat(path)
		if err != nil || info.Size() != archive.Size {
			return false
		}
	}
	return len(manifest.Archives) > 0
}

// Development only: use an already-built bundle without uploading or rebuilding it.
func LocalCourseRuntimeAsset(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if ctx.IsProd {
		http.NotFound(w, r)
		return
	}
	vars := mux.Vars(r)
	path, ok := localRuntimeFile(vars["course"], vars["archive"])
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, path)
}
