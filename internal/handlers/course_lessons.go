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
	"github.com/kodylow/base58-website/external/getters"
	"github.com/kodylow/base58-website/internal/config"
)

const localCoursesRoot = "courses"

var numberedNamePattern = regexp.MustCompile(`^([0-9]+)[_-](.+)$`)
var safeCourseSlugPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

type CourseNavItem struct {
	Number              string
	DisplayNumber       string
	Title               string
	Path                string
	URL                 string
	Current             bool
	Active              bool
	ChallengeCount      int
	CodeCheckCount      int
	MultipleChoiceCount int
	Children            []*CourseNavItem

	order    int
	filePath string
}

type CourseLessonData struct {
	Page                Page
	CourseSlug          string
	CourseTitle         string
	Current             *CourseNavItem
	Previous            *CourseNavItem
	Next                *CourseNavItem
	Sidebar             []*CourseNavItem
	ContentHTML         template.HTML
	HasCode             bool
	HeroURL             string
	HeroAlt             string
	CSRFToken           string
	VersionNumber       int
	LatestVersionNumber int
	HasNewVersion       bool
	CanPreviewDraft     bool
	IsDraftPreview      bool
	DraftVersionNumber  int
	DraftPreviewOnURL   string
	DraftPreviewOffURL  string
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

	if canonical, ok := canonicalLocalCourseSlug(courseSlug); ok && canonical != courseSlug {
		http.Redirect(w, r, courseLessonURL(canonical, pagePath), http.StatusMovedPermanently)
		return
	}

	personID := currentProgressPersonID(r, ctx)
	canPreviewDraft := canPreviewCourseDraft(r, ctx, personID, courseSlug)
	if handled := handleCourseDraftPreviewToggle(w, r, ctx, courseSlug, canPreviewDraft); handled {
		return
	}
	isDraftPreview := courseDraftPreviewEnabled(r, ctx, courseSlug) && canPreviewDraft

	if access := checkCourseContentAccess(r, ctx, courseSlug); !access.Allowed {
		if !canPreviewDraft {
			redirectCourseAccessDenied(w, r, courseSlug, access.Reason)
			return
		}
	}

	var attempt CourseAttempt
	if !isDraftPreview && ctx != nil && ctx.DB != nil && personID > 0 && hasActiveCourseEntitlement(ctx, personID, courseSlug) {
		attempt, _ = ensureActiveCourseAttempt(ctx, personID, courseSlug, "student")
	}

	var outline *localCourseOutline
	var err error
	var draftVersion CourseVersion
	if isDraftPreview {
		draftVersion, err = latestDraftCourseVersion(ctx, courseSlug)
		if err == nil && draftVersion.ID > 0 {
			outline, err = loadCourseVersion(ctx, draftVersion.ID, courseSlug, pagePath)
		}
	}
	if outline == nil && attempt.CourseVersionID > 0 {
		outline, err = loadCourseVersion(ctx, attempt.CourseVersionID, courseSlug, pagePath)
	}
	if outline == nil || err != nil {
		outline, err = loadLocalCourse(localCoursesRoot, courseSlug, pagePath)
	}
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
	heroURL, heroAlt := localCourseHero(ctx, outline.CourseSlug, outline.CourseTitle)
	if !isDraftPreview {
		recordCoursePageView(r, ctx, outline.CourseSlug, outline.Current.Path)
	}
	versionNumber, latestVersionNumber := courseAttemptVersionNumbers(ctx, attempt)
	err = ctx.TemplateCache.ExecuteTemplate(w, "courses/lesson.tmpl", CourseLessonData{
		Page:                getPage(ctx, title, furlCard),
		CourseSlug:          outline.CourseSlug,
		CourseTitle:         outline.CourseTitle,
		Current:             outline.Current,
		Previous:            outline.Previous,
		Next:                outline.Next,
		Sidebar:             outline.Items,
		ContentHTML:         template.HTML(courseMarkdownToHTMLForCourse(ctx, outline.CourseSlug, outline.Markdown)),
		HasCode:             markdownHasCourseCode(outline.Markdown),
		HeroURL:             heroURL,
		HeroAlt:             heroAlt,
		CSRFToken:           studentCSRFToken(r, ctx),
		VersionNumber:       versionNumber,
		LatestVersionNumber: latestVersionNumber,
		HasNewVersion:       latestVersionNumber > versionNumber && versionNumber > 0,
		CanPreviewDraft:     canPreviewDraft,
		IsDraftPreview:      isDraftPreview,
		DraftVersionNumber:  draftVersion.VersionNumber,
		DraftPreviewOnURL:   coursePreviewURL(r, "draft"),
		DraftPreviewOffURL:  coursePreviewURL(r, "off"),
	})
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("courses/lesson.tmpl exec failed %s\n", err.Error())
		return
	}
}

func recordCoursePageView(r *http.Request, ctx *config.AppContext, courseSlug, lessonPath string) {
	if ctx == nil || ctx.DB == nil || courseSlug == "" || lessonPath == "" {
		return
	}
	personID := currentProgressPersonID(r, ctx)
	if personID == 0 {
		return
	}
	if !hasActiveCourseEntitlement(ctx, personID, courseSlug) {
		return
	}
	attempt, err := ensureActiveCourseAttempt(ctx, personID, courseSlug, "student")
	if err != nil {
		ctx.Err.Printf("course attempt load failed: %s", err.Error())
		return
	}

	_, err = ctx.DB.Exec(`INSERT INTO course_page_views (person_id, course_slug, attempt_id, lesson_path)
VALUES (`+ph(ctx, 1)+`, `+ph(ctx, 2)+`, `+ph(ctx, 3)+`, `+ph(ctx, 4)+`)
ON CONFLICT (attempt_id, lesson_path) DO UPDATE SET
  last_viewed_at = CURRENT_TIMESTAMP,
  view_count = course_page_views.view_count + 1`, personID, courseSlug, attempt.ID, lessonPath)
	if err != nil {
		ctx.Err.Printf("course page view save failed: %s", err.Error())
	}
}

func canPreviewCourseDraft(r *http.Request, ctx *config.AppContext, personID int64, courseSlug string) bool {
	if ctx == nil || ctx.DB == nil || !isSafeCourseSlug(courseSlug) {
		return false
	}
	if admin := currentAdminFromSession(r, ctx); admin.ID > 0 {
		return true
	}
	if personID <= 0 {
		return false
	}
	var id int64
	err := ctx.DB.QueryRow(`SELECT id
FROM course_editors
WHERE person_id=`+ph(ctx, 1)+`
  AND course_slug=`+ph(ctx, 2)+`
  AND status='active'
  AND role IN ('owner', 'editor')
LIMIT 1`, personID, courseSlug).Scan(&id)
	return err == nil && id > 0
}

func handleCourseDraftPreviewToggle(w http.ResponseWriter, r *http.Request, ctx *config.AppContext, courseSlug string, allowed bool) bool {
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("preview")))
	if mode == "" {
		return false
	}
	switch mode {
	case "draft", "latest":
		if !allowed {
			http.Error(w, "Course editor access required", http.StatusForbidden)
			return true
		}
		if ctx != nil && ctx.Session != nil {
			ctx.Session.Put(r.Context(), courseDraftPreviewSessionKey(courseSlug), true)
		}
		http.Redirect(w, r, coursePreviewURL(r, ""), http.StatusSeeOther)
		return true
	case "off", "published":
		if ctx != nil && ctx.Session != nil {
			ctx.Session.Remove(r.Context(), courseDraftPreviewSessionKey(courseSlug))
		}
		http.Redirect(w, r, coursePreviewURL(r, ""), http.StatusSeeOther)
		return true
	default:
		return false
	}
}

func courseDraftPreviewEnabled(r *http.Request, ctx *config.AppContext, courseSlug string) bool {
	if ctx == nil || ctx.Session == nil {
		return false
	}
	value, _ := ctx.Session.Get(r.Context(), courseDraftPreviewSessionKey(courseSlug)).(bool)
	return value
}

func courseDraftPreviewSessionKey(courseSlug string) string {
	return "course_draft_preview_" + courseSlug
}

func coursePreviewURL(r *http.Request, mode string) string {
	values := r.URL.Query()
	values.Del("preview")
	if mode != "" {
		values.Set("preview", mode)
	}
	if encoded := values.Encode(); encoded != "" {
		return r.URL.Path + "?" + encoded
	}
	return r.URL.Path
}

func localCourseHero(ctx *config.AppContext, courseSlug, fallbackTitle string) (string, string) {
	altTitle := fallbackTitle
	if ctx == nil || ctx.Notion == nil {
		return localCourseStaticHero(courseSlug, altTitle)
	}

	course, err := getters.GetCourse(ctx.Notion, courseSlug)
	if err == nil && course != nil && course.Title != "" {
		altTitle = course.Title
	}
	if err == nil && course != nil && course.HeaderImg != "" {
		return course.HeaderImg, fmt.Sprintf("%s course header", altTitle)
	}
	return localCourseStaticHero(courseSlug, altTitle)
}

func LocalCourseLanding(w http.ResponseWriter, r *http.Request, ctx *config.AppContext, courseSlug string) {
	outline, err := loadLocalCourse(localCoursesRoot, courseSlug, "")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	course, err := getters.GetCourse(ctx.Notion, courseSlug)
	if err != nil {
		http.Error(w, "Unable to load page, course not found", http.StatusInternalServerError)
		ctx.Err.Printf("/courses local course unable to find notion course %s\n", err.Error())
		return
	}
	title, description, heroURL := localCourseLandingMetadata(ctx, courseSlug, course.Title)
	if title != "" {
		course.Title = title
	}
	if description != "" {
		course.ShortDesc = description
		if course.LongDesc == "" {
			course.LongDesc = description
		}
	}
	if heroURL != "" {
		course.HeaderImg = heroURL
	}
	accessError := localCourseAccessError(r.URL.Query().Get("access"))
	notice := localCourseNotice(r.URL.Query().Get("registered"))
	startURL := ""
	if outline.Current != nil {
		startURL = outline.Current.URL
	}
	enrolled := false
	resumeURL := startURL
	personID := currentProgressPersonID(r, ctx)
	if personID > 0 && hasActiveCourseEntitlement(ctx, personID, courseSlug) {
		enrolled = true
		if attempt, err := ensureActiveCourseAttempt(ctx, personID, courseSlug, "student"); err == nil {
			if stats, err := dashboardCourseStats(ctx, attempt.ID); err == nil && stats.LastLessonPath != "" {
				resumeURL = courseLessonURL(courseSlug, stats.LastLessonPath)
			}
		}
	}

	course.CourseURL = startURL
	course.CourseHost = "Base58"
	course.Includes = localCourseIncludes(outline, course.Includes)
	if course.ShortDesc == "" {
		course.ShortDesc = "Work through the course lessons and exercises at your own pace."
	}
	if course.LongDesc == "" {
		course.LongDesc = course.ShortDesc
	}

	furlCard := courseCard(ctx, r, course)
	err = ctx.TemplateCache.ExecuteTemplate(w, "courses/course.tmpl", CourseData{
		Course:       course,
		Page:         getPage(ctx, title, furlCard),
		LocalCourse:  true,
		LoggedIn:     personID > 0,
		Enrolled:     enrolled,
		AccessError:  accessError,
		Notice:       notice,
		StartURL:     startURL,
		ResumeURL:    resumeURL,
		LocalOutline: outline.Items,
		CSRFField:    studentCSRFField(r, ctx),
	})
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("courses/course.tmpl local course exec failed %s\n", err.Error())
		return
	}
}

func LocalCourseSignup(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if ctx == nil || ctx.DB == nil {
		http.Error(w, "Course registration is not available", http.StatusServiceUnavailable)
		return
	}

	courseSlug := mux.Vars(r)["course"]
	if canonical, ok := canonicalLocalCourseSlug(courseSlug); ok {
		courseSlug = canonical
	} else {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Unable to read registration", http.StatusBadRequest)
		return
	}
	if !validateStudentCSRF(w, r, ctx) {
		return
	}
	email := normalizeEmail(r.Form.Get("email"))
	personID := currentProgressPersonID(r, ctx)
	if personID == 0 {
		name := strings.TrimSpace(r.Form.Get("display_name"))
		if email == "" {
			http.Redirect(w, r, "/courses/"+courseSlug+"?access=signin", http.StatusSeeOther)
			return
		}

		var err error
		personID, err = ensurePersonForEmail(ctx, email)
		if err != nil {
			http.Error(w, "Unable to create account", http.StatusInternalServerError)
			ctx.Err.Printf("course signup person failed: %s", err.Error())
			return
		}
		if name != "" {
			_ = updatePersonDisplayName(ctx, personID, name)
		}

		ctx.Session.Put(r.Context(), "person_id", personID)
		ctx.Session.Put(r.Context(), "person_email", email)
	}
	if _, err := grantEntitlement(ctx, personID, courseSlug, "self_signup", "Self-registered from course landing page"); err != nil {
		http.Error(w, "Unable to grant course access", http.StatusInternalServerError)
		ctx.Err.Printf("course signup entitlement failed: %s", err.Error())
		return
	}

	startURL := "/courses/" + courseSlug
	if outline, err := loadLocalCourse(localCoursesRoot, courseSlug, ""); err == nil && outline.Current != nil {
		startURL = outline.Current.URL + "?registered=1"
	}
	http.Redirect(w, r, startURL, http.StatusSeeOther)
}

func StartLatestCourseVersion(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if ctx == nil || ctx.DB == nil {
		http.Error(w, "Database is not configured", http.StatusServiceUnavailable)
		return
	}
	if !validateStudentCSRF(w, r, ctx) {
		return
	}
	courseSlug := mux.Vars(r)["course"]
	if canonical, ok := canonicalLocalCourseSlug(courseSlug); ok {
		courseSlug = canonical
	} else {
		http.NotFound(w, r)
		return
	}
	personID := currentProgressPersonID(r, ctx)
	if personID == 0 {
		http.Redirect(w, r, loginPathWithNext("/dashboard"), http.StatusSeeOther)
		return
	}
	if !hasActiveCourseEntitlement(ctx, personID, courseSlug) {
		http.Redirect(w, r, "/courses/"+courseSlug+"?access=entitlement", http.StatusSeeOther)
		return
	}
	if _, err := startLatestCourseAttempt(ctx, personID, courseSlug); err != nil {
		http.Error(w, "Unable to start latest course version", http.StatusInternalServerError)
		ctx.Err.Printf("start latest course version failed: %s", err.Error())
		return
	}
	startURL := "/courses/" + courseSlug
	if outline, err := loadLocalCourse(localCoursesRoot, courseSlug, ""); err == nil && outline.Current != nil {
		startURL = outline.Current.URL
	}
	http.Redirect(w, r, startURL, http.StatusSeeOther)
}

func PublishCourseDraft(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if ctx == nil || ctx.DB == nil {
		http.Error(w, "Database is not configured", http.StatusServiceUnavailable)
		return
	}
	if !validateStudentCSRF(w, r, ctx) {
		return
	}

	courseSlug := mux.Vars(r)["course"]
	if !isSafeCourseSlug(courseSlug) {
		http.NotFound(w, r)
		return
	}

	personID := currentProgressPersonID(r, ctx)
	if !canPreviewCourseDraft(r, ctx, personID, courseSlug) {
		http.Error(w, "Course editor access required", http.StatusForbidden)
		return
	}

	version, fileCount, err := publishDraftCourseVersion(ctx, courseSlug, 0)
	if err != nil {
		http.Error(w, "Unable to publish draft", http.StatusInternalServerError)
		ctx.Err.Printf("course draft publish failed for %s: %s", courseSlug, err.Error())
		return
	}
	ctx.Session.Remove(r.Context(), courseDraftPreviewSessionKey(courseSlug))
	writeAudit(ctx, adminActor(r, ctx), "course.publish_draft", "course", courseSlug, fmt.Sprintf("v%d files=%d", version.VersionNumber, fileCount))

	redirectTo := r.Referer()
	if redirectTo == "" {
		redirectTo = "/courses/" + courseSlug
	}
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

func localCourseLandingMetadata(ctx *config.AppContext, courseSlug, fallbackTitle string) (string, string, string) {
	if ctx != nil && ctx.DB != nil {
		if course, err := getAdminCourse(ctx, courseSlug); err == nil {
			return firstNonEmpty(course.Title, fallbackTitle), course.Description, firstNonEmpty(course.HeaderImg, localCourseStaticHeroURL(courseSlug))
		}
	}

	if ctx != nil && ctx.Notion != nil {
		if course, err := getters.GetCourse(ctx.Notion, courseSlug); err == nil && course != nil {
			return firstNonEmpty(course.Title, fallbackTitle), firstNonEmpty(course.ShortDesc, course.LongDesc), firstNonEmpty(course.HeaderImg, localCourseStaticHeroURL(courseSlug))
		}
	}

	return fallbackTitle, "", localCourseStaticHeroURL(courseSlug)
}

func localCourseStaticHero(courseSlug, fallbackTitle string) (string, string) {
	url := localCourseStaticHeroURL(courseSlug)
	if url == "" {
		return "", ""
	}
	return url, fmt.Sprintf("%s course header", fallbackTitle)
}

func localCourseStaticHeroURL(courseSlug string) string {
	if !isSafeCourseSlug(courseSlug) {
		return ""
	}
	path := filepath.Join("static", "img", "courses", courseSlug+".png")
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return "/static/img/courses/" + courseSlug + ".png"
}

func updatePersonDisplayName(ctx *config.AppContext, personID int64, name string) error {
	if ctx == nil || ctx.DB == nil || personID == 0 || strings.TrimSpace(name) == "" {
		return nil
	}
	_, err := ctx.DB.Exec(`UPDATE people SET display_name=`+ph(ctx, 1)+`, updated_at=CURRENT_TIMESTAMP WHERE id=`+ph(ctx, 2), strings.TrimSpace(name), personID)
	return err
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

type courseContentAccess struct {
	Allowed bool
	Reason  string
}

func checkCourseContentAccess(r *http.Request, ctx *config.AppContext, courseSlug string) courseContentAccess {
	if !localCourseExists(courseSlug) {
		return courseContentAccess{Allowed: true}
	}
	if ctx == nil || ctx.DB == nil {
		return courseContentAccess{Reason: "signin"}
	}

	personID := currentProgressPersonID(r, ctx)
	if personID == 0 {
		return courseContentAccess{Reason: "signin"}
	}
	if hasActiveCourseEntitlement(ctx, personID, courseSlug) {
		return courseContentAccess{Allowed: true}
	}
	return courseContentAccess{Reason: "entitlement"}
}

func redirectCourseAccessDenied(w http.ResponseWriter, r *http.Request, courseSlug, reason string) {
	if reason == "" {
		reason = "entitlement"
	}
	if reason == "signin" {
		http.Redirect(w, r, loginPathWithNext(r.URL.RequestURI()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/courses/%s?access=%s", courseSlug, reason), http.StatusSeeOther)
}

func localCourseAccessError(reason string) string {
	switch reason {
	case "signin":
		return "Sign in to access the course lessons."
	case "entitlement":
		return "You do not have access to this course yet."
	default:
		return ""
	}
}

func localCourseNotice(value string) string {
	if value == "1" {
		return "Registration complete. You now have access to this course."
	}
	return ""
}

func hasActiveCourseEntitlement(ctx *config.AppContext, personID int64, courseSlug string) bool {
	if ctx == nil || ctx.DB == nil || personID == 0 || courseSlug == "" {
		return false
	}

	var count int
	err := ctx.DB.QueryRow(`SELECT COUNT(1)
FROM course_entitlements
WHERE person_id=`+ph(ctx, 1)+`
  AND course_slug=`+ph(ctx, 2)+`
  AND status='active'
  AND (starts_at IS NULL OR starts_at <= CURRENT_TIMESTAMP)
  AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)`, personID, courseSlug).Scan(&count)
	return err == nil && count > 0
}

func localCourseExists(courseSlug string) bool {
	if !isSafeCourseSlug(courseSlug) {
		return false
	}

	info, err := os.Stat(filepath.Join(localCoursesRoot, courseSlug))
	return err == nil && info.IsDir()
}

func canonicalLocalCourseSlug(courseSlug string) (string, bool) {
	if localCourseExists(courseSlug) {
		return courseSlug, true
	}
	if strings.Contains(courseSlug, "_") {
		canonical := strings.ReplaceAll(courseSlug, "_", "-")
		if localCourseExists(canonical) {
			return canonical, true
		}
	}
	return courseSlug, false
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
		Number:              number,
		DisplayNumber:       number,
		Title:               titleFromMarkdown(markdown, titleFromSlug(label)),
		Path:                lessonPath,
		URL:                 courseLessonURL(courseSlug, lessonPath),
		ChallengeCount:      countCourseChallengeBlocks(markdown),
		CodeCheckCount:      countCourseFenceBlocks(markdown, courseCodeChallengeFence),
		MultipleChoiceCount: countCourseFenceBlocks(markdown, courseMultipleChoiceFence),
		order:               orderFromNumber(number),
		filePath:            filePath,
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

func localCourseIncludes(outline *localCourseOutline, notionIncludes []string) []string {
	codeChecks, multipleChoice := totalLocalCourseExerciseCounts(outline)
	includes := make([]string, 0, len(notionIncludes)+2)
	if codeChecks > 0 {
		includes = append(includes, pluralizeCount(codeChecks, "code exercise", "code exercises"))
	}
	if multipleChoice > 0 {
		includes = append(includes, pluralizeCount(multipleChoice, "multiple choice question", "multiple choice questions"))
	}
	for _, include := range notionIncludes {
		include = strings.TrimSpace(include)
		if include != "" {
			includes = append(includes, include)
		}
	}
	return includes
}

func totalLocalCourseExerciseCounts(outline *localCourseOutline) (int, int) {
	if outline == nil {
		return 0, 0
	}
	codeChecks := 0
	multipleChoice := 0
	for _, page := range outline.Pages {
		codeChecks += page.CodeCheckCount
		multipleChoice += page.MultipleChoiceCount
	}
	return codeChecks, multipleChoice
}

func pluralizeCount(count int, singular, plural string) string {
	label := plural
	if count == 1 {
		label = singular
	}
	return fmt.Sprintf("%d %s", count, label)
}

func countCourseChallengeBlocks(markdown []byte) int {
	return countCourseFenceBlocks(markdown, courseMultipleChoiceFence) + countCourseFenceBlocks(markdown, courseCodeChallengeFence)
}

func countCourseFenceBlocks(markdown, fence []byte) int {
	count := 0
	scanner := bufio.NewScanner(bytes.NewReader(markdown))
	for scanner.Scan() {
		if isCourseFenceLine(scanner.Bytes(), fence) {
			count++
		}
	}
	return count / 2
}
