package database

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/kodylow/base58-website/external/getters"
	"github.com/kodylow/base58-website/internal/types"
)

type CourseSyncResult struct {
	Seen    int
	Upserts int
	Skipped int
}

func SyncCoursesFromNotion(db *sql.DB, dbDriver string, notion *types.Notion) (CourseSyncResult, error) {
	var result CourseSyncResult
	if db == nil {
		return result, nil
	}
	if notion == nil || notion.Client == nil || notion.Config.CoursesDb == "" {
		return result, nil
	}

	courses, err := getters.ListCourses(notion)
	if err != nil {
		return result, err
	}

	for _, course := range courses {
		result.Seen++
		if course == nil || strings.TrimSpace(course.Tag) == "" {
			result.Skipped++
			continue
		}
		if err := upsertCourse(db, dbDriver, course); err != nil {
			return result, err
		}
		result.Upserts++
	}
	return result, nil
}

func upsertCourse(db *sql.DB, dbDriver string, course *types.Course) error {
	slug := strings.TrimSpace(course.Tag)
	title := strings.TrimSpace(course.Title)
	if title == "" {
		title = titleFromSlug(slug)
	}
	description := strings.TrimSpace(course.ShortDesc)
	if description == "" {
		description = strings.TrimSpace(course.LongDesc)
	}
	status := "draft"
	if course.Visible {
		status = "published"
	}

	if isPostgresDriver(dbDriver) {
		_, err := db.Exec(`INSERT INTO courses (slug, title, description, header_img, status)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (slug) DO UPDATE SET
  title = EXCLUDED.title,
  description = EXCLUDED.description,
  header_img = EXCLUDED.header_img,
  status = EXCLUDED.status,
  updated_at = CURRENT_TIMESTAMP`,
			slug, title, description, strings.TrimSpace(course.HeaderImg), status)
		return err
	}

	_, err := db.Exec(`INSERT INTO courses (slug, title, description, header_img, status)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(slug) DO UPDATE SET
  title = excluded.title,
  description = excluded.description,
  header_img = excluded.header_img,
  status = excluded.status,
  updated_at = CURRENT_TIMESTAMP`,
		slug, title, description, strings.TrimSpace(course.HeaderImg), status)
	return err
}

func isPostgresDriver(driver string) bool {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "postgres", "postgresql", "pgx":
		return true
	default:
		return false
	}
}

func titleFromSlug(slug string) string {
	parts := strings.FieldsFunc(slug, func(r rune) bool {
		return r == '-' || r == '_'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	title := strings.Join(parts, " ")
	if title == "" {
		return fmt.Sprintf("Course %s", slug)
	}
	return title
}
