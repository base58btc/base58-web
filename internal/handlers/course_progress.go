package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/kodylow/base58-website/internal/config"
)

type CourseProgressResponse struct {
	Authenticated bool                       `json:"authenticated"`
	CourseSlug    string                     `json:"courseSlug"`
	Blocks        []CourseProgressBlockState `json:"blocks"`
}

type CourseProgressBlockState struct {
	LessonPath          string   `json:"lessonPath"`
	BlockID             string   `json:"blockId"`
	BlockType           string   `json:"blockType"`
	Correct             bool     `json:"correct"`
	SelectedOption      *string  `json:"selectedOption,omitempty"`
	SelectedOptions     []string `json:"selectedOptions"`
	SelectedOptionsJSON string   `json:"-"`
	AnsweredAt          string   `json:"answeredAt"`
	UpdatedAt           string   `json:"updatedAt"`
}

type CourseProgressSaveRequest struct {
	LessonPath      string   `json:"lessonPath"`
	BlockID         string   `json:"blockId"`
	BlockType       string   `json:"blockType"`
	Correct         bool     `json:"correct"`
	SelectedOption  *string  `json:"selectedOption"`
	SelectedOptions []string `json:"selectedOptions"`
}

func CourseProgress(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if ctx.DB == nil {
		writeCourseProgressJSON(w, http.StatusOK, CourseProgressResponse{Authenticated: false})
		return
	}

	personID := currentProgressPersonID(r, ctx)
	if personID == 0 {
		writeCourseProgressJSON(w, http.StatusOK, CourseProgressResponse{Authenticated: false})
		return
	}

	courseSlug := mux.Vars(r)["course"]
	if !isSafeCourseSlug(courseSlug) {
		http.Error(w, "Invalid course", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		blocks, err := listCourseProgress(ctx, personID, courseSlug)
		if err != nil {
			http.Error(w, "Unable to load course progress", http.StatusInternalServerError)
			ctx.Err.Printf("course progress load failed: %s", err.Error())
			return
		}
		writeCourseProgressJSON(w, http.StatusOK, CourseProgressResponse{
			Authenticated: true,
			CourseSlug:    courseSlug,
			Blocks:        blocks,
		})
	case http.MethodPost:
		var req CourseProgressSaveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid progress payload", http.StatusBadRequest)
			return
		}
		if err := saveCourseProgress(ctx, personID, courseSlug, req); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, errInvalidCourseProgress) {
				status = http.StatusBadRequest
			}
			http.Error(w, "Unable to save course progress", status)
			ctx.Err.Printf("course progress save failed: %s", err.Error())
			return
		}
		writeCourseProgressJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

var errInvalidCourseProgress = errors.New("invalid course progress")

func currentProgressPersonID(r *http.Request, ctx *config.AppContext) int64 {
	for _, key := range []string{"person_id", "admin_person_id"} {
		switch value := ctx.Session.Get(r.Context(), key).(type) {
		case int64:
			return value
		case int:
			return int64(value)
		case string:
			id, _ := strconv.ParseInt(value, 10, 64)
			if id > 0 {
				return id
			}
		}
	}
	return 0
}

func listCourseProgress(ctx *config.AppContext, personID int64, courseSlug string) ([]CourseProgressBlockState, error) {
	rows, err := ctx.DB.Query(`SELECT lesson_path, block_id, block_type, correct, selected_option, selected_options_json, CAST(answered_at AS TEXT), CAST(updated_at AS TEXT)
FROM course_progress
WHERE person_id=`+ph(ctx, 1)+` AND course_slug=`+ph(ctx, 2), personID, courseSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocks []CourseProgressBlockState
	for rows.Next() {
		var block CourseProgressBlockState
		var selected sql.NullString
		if err := rows.Scan(&block.LessonPath, &block.BlockID, &block.BlockType, &block.Correct, &selected, &block.SelectedOptionsJSON, &block.AnsweredAt, &block.UpdatedAt); err != nil {
			return nil, err
		}
		if selected.Valid {
			block.SelectedOption = &selected.String
		}
		if block.SelectedOptionsJSON != "" {
			_ = json.Unmarshal([]byte(block.SelectedOptionsJSON), &block.SelectedOptions)
		}
		blocks = append(blocks, block)
	}
	return blocks, rows.Err()
}

func saveCourseProgress(ctx *config.AppContext, personID int64, courseSlug string, req CourseProgressSaveRequest) error {
	req.LessonPath = strings.TrimSpace(req.LessonPath)
	req.BlockID = strings.TrimSpace(req.BlockID)
	req.BlockType = strings.TrimSpace(req.BlockType)
	if req.LessonPath == "" || req.BlockID == "" || req.BlockType == "" {
		return errInvalidCourseProgress
	}
	if !isSafeCourseProgressPath(req.LessonPath) || !isSafeCourseBlockID(req.BlockID) {
		return errInvalidCourseProgress
	}

	selectedJSON, err := json.Marshal(req.SelectedOptions)
	if err != nil {
		return err
	}

	if adminPostgres(ctx) {
		_, err = ctx.DB.Exec(`INSERT INTO course_progress (person_id, course_slug, lesson_path, block_id, block_type, correct, selected_option, selected_options_json)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (person_id, course_slug, lesson_path, block_id) DO UPDATE SET
  block_type = EXCLUDED.block_type,
  correct = EXCLUDED.correct,
  selected_option = EXCLUDED.selected_option,
  selected_options_json = EXCLUDED.selected_options_json,
  answered_at = CURRENT_TIMESTAMP,
  updated_at = CURRENT_TIMESTAMP`,
			personID, courseSlug, req.LessonPath, req.BlockID, req.BlockType, req.Correct, req.SelectedOption, string(selectedJSON))
		return err
	}

	_, err = ctx.DB.Exec(`INSERT INTO course_progress (person_id, course_slug, lesson_path, block_id, block_type, correct, selected_option, selected_options_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(person_id, course_slug, lesson_path, block_id) DO UPDATE SET
  block_type = excluded.block_type,
  correct = excluded.correct,
  selected_option = excluded.selected_option,
  selected_options_json = excluded.selected_options_json,
  answered_at = CURRENT_TIMESTAMP,
  updated_at = CURRENT_TIMESTAMP`,
		personID, courseSlug, req.LessonPath, req.BlockID, req.BlockType, req.Correct, req.SelectedOption, string(selectedJSON))
	return err
}

func writeCourseProgressJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func isSafeCourseProgressPath(path string) bool {
	if path == "" || strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || !safeCourseSlugPattern.MatchString(part) {
			return false
		}
	}
	return true
}

func isSafeCourseBlockID(blockID string) bool {
	return safeCourseSlugPattern.MatchString(blockID)
}
