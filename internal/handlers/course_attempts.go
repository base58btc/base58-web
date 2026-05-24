package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/kodylow/base58-website/internal/config"
)

type CourseAttempt struct {
	ID              int64
	PersonID        int64
	CourseSlug      string
	CourseVersionID int64
	AttemptNumber   int
	Status          string
}

func activeCourseAttempt(ctx *config.AppContext, personID int64, courseSlug string) (CourseAttempt, error) {
	var attempt CourseAttempt
	err := ctx.DB.QueryRow(`SELECT id, person_id, course_slug, course_version_id, attempt_number, status
FROM course_attempts
WHERE person_id=`+ph(ctx, 1)+` AND course_slug=`+ph(ctx, 2)+` AND status='active'
LIMIT 1`, personID, courseSlug).
		Scan(&attempt.ID, &attempt.PersonID, &attempt.CourseSlug, &attempt.CourseVersionID, &attempt.AttemptNumber, &attempt.Status)
	return attempt, err
}

func ensureActiveCourseAttempt(ctx *config.AppContext, personID int64, courseSlug, createdBy string) (CourseAttempt, error) {
	attempt, err := activeCourseAttempt(ctx, personID, courseSlug)
	if err == nil {
		return attempt, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return CourseAttempt{}, err
	}
	courseVersionID, err := latestCourseVersionID(ctx, courseSlug)
	if err != nil {
		return CourseAttempt{}, err
	}

	if strings.TrimSpace(createdBy) == "" {
		createdBy = "student"
	}
	err = ctx.DB.QueryRow(`INSERT INTO course_attempts (person_id, course_slug, course_version_id, attempt_number, status, created_by)
SELECT `+ph(ctx, 1)+`, `+ph(ctx, 2)+`, `+ph(ctx, 3)+`, COALESCE(MAX(attempt_number), 0) + 1, 'active', `+ph(ctx, 4)+`
FROM course_attempts
WHERE person_id=`+ph(ctx, 1)+` AND course_slug=`+ph(ctx, 2)+`
RETURNING id, person_id, course_slug, course_version_id, attempt_number, status`, personID, courseSlug, courseVersionID, createdBy).
		Scan(&attempt.ID, &attempt.PersonID, &attempt.CourseSlug, &attempt.CourseVersionID, &attempt.AttemptNumber, &attempt.Status)
	if err == nil {
		return attempt, nil
	}

	// If another request created the active attempt first, use that one.
	if retry, retryErr := activeCourseAttempt(ctx, personID, courseSlug); retryErr == nil {
		return retry, nil
	}
	return CourseAttempt{}, err
}

func resetActiveCourseAttempt(ctx *config.AppContext, personID int64, courseSlug, reason string) (CourseAttempt, error) {
	courseVersionID, err := latestCourseVersionID(ctx, courseSlug)
	if err != nil {
		return CourseAttempt{}, err
	}
	tx, err := ctx.DB.Begin()
	if err != nil {
		return CourseAttempt{}, err
	}
	defer tx.Rollback()

	var current CourseAttempt
	err = tx.QueryRow(`SELECT id, person_id, course_slug, course_version_id, attempt_number, status
FROM course_attempts
WHERE person_id=$1 AND course_slug=$2 AND status='active'
FOR UPDATE`, personID, courseSlug).
		Scan(&current.ID, &current.PersonID, &current.CourseSlug, &current.CourseVersionID, &current.AttemptNumber, &current.Status)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRow(`INSERT INTO course_attempts (person_id, course_slug, course_version_id, attempt_number, status, created_by)
VALUES ($1, $2, $3, 1, 'active', 'student')
RETURNING id, person_id, course_slug, course_version_id, attempt_number, status`, personID, courseSlug, courseVersionID).
			Scan(&current.ID, &current.PersonID, &current.CourseSlug, &current.CourseVersionID, &current.AttemptNumber, &current.Status)
	}
	if err != nil {
		return CourseAttempt{}, err
	}

	_, err = tx.Exec(`UPDATE course_attempts
SET status='reset', reset_at=CURRENT_TIMESTAMP, reset_reason=$1, updated_at=CURRENT_TIMESTAMP
WHERE id=$2`, strings.TrimSpace(reason), current.ID)
	if err != nil {
		return CourseAttempt{}, err
	}

	var next CourseAttempt
	err = tx.QueryRow(`INSERT INTO course_attempts (person_id, course_slug, course_version_id, attempt_number, status, created_by)
VALUES ($1, $2, $3, $4, 'active', 'student')
RETURNING id, person_id, course_slug, course_version_id, attempt_number, status`, personID, courseSlug, courseVersionID, current.AttemptNumber+1).
		Scan(&next.ID, &next.PersonID, &next.CourseSlug, &next.CourseVersionID, &next.AttemptNumber, &next.Status)
	if err != nil {
		return CourseAttempt{}, fmt.Errorf("create next course attempt: %w", err)
	}

	return next, tx.Commit()
}

func startLatestCourseAttempt(ctx *config.AppContext, personID int64, courseSlug string) (CourseAttempt, error) {
	courseVersionID, err := latestCourseVersionID(ctx, courseSlug)
	if err != nil {
		return CourseAttempt{}, err
	}
	tx, err := ctx.DB.Begin()
	if err != nil {
		return CourseAttempt{}, err
	}
	defer tx.Rollback()

	var current CourseAttempt
	err = tx.QueryRow(`SELECT id, person_id, course_slug, course_version_id, attempt_number, status
FROM course_attempts
WHERE person_id=$1 AND course_slug=$2 AND status='active'
FOR UPDATE`, personID, courseSlug).
		Scan(&current.ID, &current.PersonID, &current.CourseSlug, &current.CourseVersionID, &current.AttemptNumber, &current.Status)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return CourseAttempt{}, err
	}
	if err == nil && current.CourseVersionID == courseVersionID {
		return current, tx.Commit()
	}
	if err == nil {
		_, err = tx.Exec(`UPDATE course_attempts
SET status='superseded', reset_at=CURRENT_TIMESTAMP, reset_reason='Started newer course version', updated_at=CURRENT_TIMESTAMP
WHERE id=$1`, current.ID)
		if err != nil {
			return CourseAttempt{}, err
		}
	}

	nextNumber := current.AttemptNumber + 1
	if nextNumber <= 0 {
		nextNumber = 1
	}
	var next CourseAttempt
	err = tx.QueryRow(`INSERT INTO course_attempts (person_id, course_slug, course_version_id, attempt_number, status, created_by)
VALUES ($1, $2, $3, $4, 'active', 'student')
RETURNING id, person_id, course_slug, course_version_id, attempt_number, status`, personID, courseSlug, courseVersionID, nextNumber).
		Scan(&next.ID, &next.PersonID, &next.CourseSlug, &next.CourseVersionID, &next.AttemptNumber, &next.Status)
	if err != nil {
		return CourseAttempt{}, fmt.Errorf("create latest course attempt: %w", err)
	}
	return next, tx.Commit()
}

func courseAttemptVersionNumbers(ctx *config.AppContext, attempt CourseAttempt) (int, int) {
	if ctx == nil || ctx.DB == nil || attempt.CourseSlug == "" {
		return 0, 0
	}
	var current, latest int
	if attempt.CourseVersionID > 0 {
		_ = ctx.DB.QueryRow(`SELECT version_number FROM course_versions WHERE id=`+ph(ctx, 1), attempt.CourseVersionID).Scan(&current)
	}
	_ = ctx.DB.QueryRow(`SELECT version_number
FROM course_versions
WHERE course_slug=`+ph(ctx, 1)+` AND status='published'
ORDER BY version_number DESC
LIMIT 1`, attempt.CourseSlug).Scan(&latest)
	return current, latest
}

func latestCourseVersionID(ctx *config.AppContext, courseSlug string) (int64, error) {
	var id int64
	err := ctx.DB.QueryRow(`SELECT id
FROM course_versions
WHERE course_slug=`+ph(ctx, 1)+` AND status='published'
ORDER BY version_number DESC
LIMIT 1`, courseSlug).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	err = ctx.DB.QueryRow(`INSERT INTO course_versions (course_slug, version_number, status, source, published_at)
VALUES (`+ph(ctx, 1)+`, 1, 'published', 'local_md', CURRENT_TIMESTAMP)
ON CONFLICT (course_slug, version_number) DO UPDATE SET updated_at=CURRENT_TIMESTAMP
RETURNING id`, courseSlug).Scan(&id)
	return id, err
}
