package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/kodylow/base58-website/internal/config"
)

type DashboardData struct {
	Page       Page
	Person     DashboardPerson
	Courses    []DashboardCourse
	AdminPanel bool
}

type DashboardPerson struct {
	ID          int64
	DisplayName string
	Email       string
}

type DashboardCourse struct {
	Slug              string
	Title             string
	Description       string
	HeaderImg         string
	Status            string
	GrantedAt         string
	ResumeURL         string
	CourseURL         string
	PagesViewed       int
	TotalPages        int
	QuestionsAnswered int
	TotalQuestions    int
	QuestionsPassed   int
	QuestionsFailed   int
	AttemptNumber     int
	AllowReset        bool
}

func Dashboard(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if ctx == nil || ctx.DB == nil {
		http.Error(w, "Database is not configured", http.StatusServiceUnavailable)
		return
	}

	personID := currentProgressPersonID(r, ctx)
	if personID == 0 {
		http.Redirect(w, r, loginPathWithNext(r.URL.RequestURI()), http.StatusSeeOther)
		return
	}

	person, err := dashboardPerson(ctx, personID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			ctx.Session.Remove(r.Context(), "person_id")
			ctx.Session.Remove(r.Context(), "person_email")
			http.Redirect(w, r, loginPathWithNext(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}
		http.Error(w, "Unable to load dashboard", http.StatusInternalServerError)
		ctx.Err.Printf("dashboard person load failed: %s", err.Error())
		return
	}

	courses, err := dashboardCourses(ctx, personID)
	if err != nil {
		http.Error(w, "Unable to load dashboard", http.StatusInternalServerError)
		ctx.Err.Printf("dashboard courses load failed: %s", err.Error())
		return
	}

	card := defaultCard(ctx, r, "Dashboard")
	err = ctx.TemplateCache.ExecuteTemplate(w, "dashboard.tmpl", DashboardData{
		Page:       getPage(ctx, "Dashboard", card),
		Person:     person,
		Courses:    courses,
		AdminPanel: dashboardHasAdminPanel(ctx, personID),
	})
	if err != nil {
		http.Error(w, "Unable to load dashboard", http.StatusInternalServerError)
		ctx.Err.Printf("dashboard.tmpl exec failed: %s", err.Error())
	}
}

func dashboardHasAdminPanel(ctx *config.AppContext, personID int64) bool {
	if ctx == nil || ctx.DB == nil || personID == 0 {
		return false
	}
	var count int
	err := ctx.DB.QueryRow(`SELECT COUNT(1)
FROM admin_users
WHERE person_id=`+ph(ctx, 1)+` AND status='active'`, personID).Scan(&count)
	return err == nil && count > 0
}

func DashboardResetCourse(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if ctx == nil || ctx.DB == nil {
		http.Error(w, "Database is not configured", http.StatusServiceUnavailable)
		return
	}

	personID := currentProgressPersonID(r, ctx)
	if personID == 0 {
		http.Redirect(w, r, loginPathWithNext("/dashboard"), http.StatusSeeOther)
		return
	}

	courseSlug := mux.Vars(r)["course"]
	if !isSafeCourseSlug(courseSlug) || !hasActiveCourseEntitlement(ctx, personID, courseSlug) {
		http.Redirect(w, r, "/dashboard?reset=denied", http.StatusSeeOther)
		return
	}

	if _, err := resetActiveCourseAttempt(ctx, personID, courseSlug, "Student reset from dashboard"); err != nil {
		http.Error(w, "Unable to reset course", http.StatusInternalServerError)
		ctx.Err.Printf("course reset failed: %s", err.Error())
		return
	}
	http.Redirect(w, r, "/dashboard?reset=1", http.StatusSeeOther)
}

func dashboardPerson(ctx *config.AppContext, personID int64) (DashboardPerson, error) {
	var person DashboardPerson
	err := ctx.DB.QueryRow(`SELECT p.id, p.display_name, COALESCE(pe.email, '')
FROM people p
LEFT JOIN person_emails pe ON pe.person_id = p.id AND pe.is_primary = `+primarySQL(ctx)+`
WHERE p.id=`+ph(ctx, 1), personID).Scan(&person.ID, &person.DisplayName, &person.Email)
	return person, err
}

func dashboardCourses(ctx *config.AppContext, personID int64) ([]DashboardCourse, error) {
	rows, err := ctx.DB.Query(`SELECT ce.course_slug, c.title, c.description, c.header_img, c.status, CAST(ce.granted_at AS TEXT)
FROM course_entitlements ce
JOIN courses c ON c.slug = ce.course_slug
WHERE ce.person_id=`+ph(ctx, 1)+`
  AND ce.status='active'
  AND (ce.starts_at IS NULL OR ce.starts_at <= CURRENT_TIMESTAMP)
  AND (ce.expires_at IS NULL OR ce.expires_at > CURRENT_TIMESTAMP)
ORDER BY c.title`, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var courses []DashboardCourse
	for rows.Next() {
		var course DashboardCourse
		if err := rows.Scan(&course.Slug, &course.Title, &course.Description, &course.HeaderImg, &course.Status, &course.GrantedAt); err != nil {
			return nil, err
		}
		course.CourseURL = "/courses/" + course.Slug
		course.ResumeURL = course.CourseURL
		course.AllowReset = true
		if course.Description == "" {
			course.Description = "Continue your course from the latest saved progress."
		}

		if outline, err := loadLocalCourse(localCoursesRoot, course.Slug, ""); err == nil {
			course.TotalPages = len(outline.Pages)
			course.TotalQuestions = totalChallengeCount(outline.Pages)
			if outline.Current != nil {
				course.ResumeURL = outline.Current.URL
			}
		}

		attempt, err := ensureActiveCourseAttempt(ctx, personID, course.Slug, "student")
		if err != nil {
			return nil, err
		}
		course.AttemptNumber = attempt.AttemptNumber

		stats, err := dashboardCourseStats(ctx, attempt.ID)
		if err != nil {
			return nil, err
		}
		course.PagesViewed = stats.PagesViewed
		course.QuestionsAnswered = stats.QuestionsAnswered
		course.QuestionsPassed = stats.QuestionsPassed
		course.QuestionsFailed = stats.QuestionsFailed
		if stats.LastLessonPath != "" {
			course.ResumeURL = courseLessonURL(course.Slug, stats.LastLessonPath)
		}

		courses = append(courses, course)
	}
	return courses, rows.Err()
}

type dashboardStats struct {
	PagesViewed       int
	QuestionsAnswered int
	QuestionsPassed   int
	QuestionsFailed   int
	LastLessonPath    string
}

func dashboardCourseStats(ctx *config.AppContext, attemptID int64) (dashboardStats, error) {
	var stats dashboardStats
	err := ctx.DB.QueryRow(`SELECT COUNT(*) FROM course_page_views WHERE attempt_id=`+ph(ctx, 1), attemptID).Scan(&stats.PagesViewed)
	if err != nil {
		return stats, err
	}

	var last sql.NullString
	err = ctx.DB.QueryRow(`SELECT lesson_path
FROM course_page_views
WHERE attempt_id=`+ph(ctx, 1)+`
ORDER BY last_viewed_at DESC
LIMIT 1`, attemptID).Scan(&last)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return stats, err
	}
	if last.Valid {
		stats.LastLessonPath = last.String
	}

	err = ctx.DB.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN correct THEN 1 ELSE 0 END), 0), COALESCE(SUM(CASE WHEN correct THEN 0 ELSE 1 END), 0)
FROM course_progress
WHERE attempt_id=`+ph(ctx, 1), attemptID).
		Scan(&stats.QuestionsAnswered, &stats.QuestionsPassed, &stats.QuestionsFailed)
	return stats, err
}

func totalChallengeCount(pages []*CourseNavItem) int {
	total := 0
	for _, page := range pages {
		total += page.ChallengeCount
	}
	return total
}

func dashboardProgressLabel(done, total int) string {
	if total <= 0 {
		return fmt.Sprintf("%d", done)
	}
	return fmt.Sprintf("%d / %d", done, total)
}

func dashboardInitials(person DashboardPerson) string {
	name := strings.TrimSpace(person.DisplayName)
	if name == "" {
		name = strings.TrimSpace(person.Email)
	}
	if name == "" {
		return "B"
	}
	return strings.ToUpper(name[:1])
}
