package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/kodylow/base58-website/internal/config"
)

type CourseAttempt struct {
	ID            int64
	PersonID      int64
	CourseSlug    string
	AttemptNumber int
	Status        string
}

func activeCourseAttempt(ctx *config.AppContext, personID int64, courseSlug string) (CourseAttempt, error) {
	var attempt CourseAttempt
	err := ctx.DB.QueryRow(`SELECT id, person_id, course_slug, attempt_number, status
FROM course_attempts
WHERE person_id=`+ph(ctx, 1)+` AND course_slug=`+ph(ctx, 2)+` AND status='active'
LIMIT 1`, personID, courseSlug).
		Scan(&attempt.ID, &attempt.PersonID, &attempt.CourseSlug, &attempt.AttemptNumber, &attempt.Status)
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

	if strings.TrimSpace(createdBy) == "" {
		createdBy = "student"
	}
	err = ctx.DB.QueryRow(`INSERT INTO course_attempts (person_id, course_slug, attempt_number, status, created_by)
SELECT `+ph(ctx, 1)+`, `+ph(ctx, 2)+`, COALESCE(MAX(attempt_number), 0) + 1, 'active', `+ph(ctx, 3)+`
FROM course_attempts
WHERE person_id=`+ph(ctx, 1)+` AND course_slug=`+ph(ctx, 2)+`
RETURNING id, person_id, course_slug, attempt_number, status`, personID, courseSlug, createdBy).
		Scan(&attempt.ID, &attempt.PersonID, &attempt.CourseSlug, &attempt.AttemptNumber, &attempt.Status)
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
	tx, err := ctx.DB.Begin()
	if err != nil {
		return CourseAttempt{}, err
	}
	defer tx.Rollback()

	var current CourseAttempt
	err = tx.QueryRow(`SELECT id, person_id, course_slug, attempt_number, status
FROM course_attempts
WHERE person_id=$1 AND course_slug=$2 AND status='active'
FOR UPDATE`, personID, courseSlug).
		Scan(&current.ID, &current.PersonID, &current.CourseSlug, &current.AttemptNumber, &current.Status)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRow(`INSERT INTO course_attempts (person_id, course_slug, attempt_number, status, created_by)
VALUES ($1, $2, 1, 'active', 'student')
RETURNING id, person_id, course_slug, attempt_number, status`, personID, courseSlug).
			Scan(&current.ID, &current.PersonID, &current.CourseSlug, &current.AttemptNumber, &current.Status)
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
	err = tx.QueryRow(`INSERT INTO course_attempts (person_id, course_slug, attempt_number, status, created_by)
VALUES ($1, $2, $3, 'active', 'student')
RETURNING id, person_id, course_slug, attempt_number, status`, personID, courseSlug, current.AttemptNumber+1).
		Scan(&next.ID, &next.PersonID, &next.CourseSlug, &next.AttemptNumber, &next.Status)
	if err != nil {
		return CourseAttempt{}, fmt.Errorf("create next course attempt: %w", err)
	}

	return next, tx.Commit()
}
