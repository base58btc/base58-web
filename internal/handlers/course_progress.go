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
	AttemptID     int64                      `json:"attemptId,omitempty"`
	AttemptNumber int                        `json:"attemptNumber,omitempty"`
	Blocks        []CourseProgressBlockState `json:"blocks"`
	CodeBlocks    []CourseCodeBlockState     `json:"codeBlocks"`
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
	CodeText        *string  `json:"codeText"`
	OutputText      *string  `json:"outputText"`
	OutputOK        *bool    `json:"outputOk"`
	ExecutionCount  *int     `json:"executionCount"`
}

type CourseCodeBlockState struct {
	LessonPath     string `json:"lessonPath"`
	BlockID        string `json:"blockId"`
	BlockType      string `json:"blockType"`
	CodeText       string `json:"codeText"`
	OutputText     string `json:"outputText"`
	OutputOK       *bool  `json:"outputOk,omitempty"`
	ExecutionCount *int   `json:"executionCount,omitempty"`
	UpdatedAt      string `json:"updatedAt"`
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
	if !hasActiveCourseEntitlement(ctx, personID, courseSlug) {
		http.Error(w, "Course access required", http.StatusForbidden)
		return
	}

	attempt, err := ensureActiveCourseAttempt(ctx, personID, courseSlug, "student")
	if err != nil {
		http.Error(w, "Unable to load course attempt", http.StatusInternalServerError)
		ctx.Err.Printf("course attempt load failed: %s", err.Error())
		return
	}

	switch r.Method {
	case http.MethodGet:
		blocks, err := listCourseProgress(ctx, attempt.ID)
		if err != nil {
			http.Error(w, "Unable to load course progress", http.StatusInternalServerError)
			ctx.Err.Printf("course progress load failed: %s", err.Error())
			return
		}
		codeBlocks, err := listCourseCodeBlocks(ctx, attempt.ID)
		if err != nil {
			http.Error(w, "Unable to load course code work", http.StatusInternalServerError)
			ctx.Err.Printf("course code work load failed: %s", err.Error())
			return
		}
		writeCourseProgressJSON(w, http.StatusOK, CourseProgressResponse{
			Authenticated: true,
			CourseSlug:    courseSlug,
			AttemptID:     attempt.ID,
			AttemptNumber: attempt.AttemptNumber,
			Blocks:        blocks,
			CodeBlocks:    codeBlocks,
		})
	case http.MethodPost:
		var req CourseProgressSaveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid progress payload", http.StatusBadRequest)
			return
		}
		if err := saveCourseProgress(ctx, attempt.ID, personID, courseSlug, req); err != nil {
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

func listCourseProgress(ctx *config.AppContext, attemptID int64) ([]CourseProgressBlockState, error) {
	rows, err := ctx.DB.Query(`SELECT lesson_path, block_id, block_type, correct, selected_option, selected_options_json, CAST(answered_at AS TEXT), CAST(updated_at AS TEXT)
FROM course_progress
WHERE attempt_id=`+ph(ctx, 1), attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	blocks := make([]CourseProgressBlockState, 0)
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

func saveCourseProgress(ctx *config.AppContext, attemptID int64, personID int64, courseSlug string, req CourseProgressSaveRequest) error {
	req.LessonPath = strings.TrimSpace(req.LessonPath)
	req.BlockID = strings.TrimSpace(req.BlockID)
	req.BlockType = strings.TrimSpace(req.BlockType)
	if req.LessonPath == "" || req.BlockID == "" || req.BlockType == "" {
		return errInvalidCourseProgress
	}
	if !isSafeCourseProgressPath(req.LessonPath) || !isSafeCourseBlockID(req.BlockID) {
		return errInvalidCourseProgress
	}

	if req.CodeText != nil {
		if err := saveCourseCodeBlock(ctx, attemptID, personID, courseSlug, req); err != nil {
			return err
		}
	}
	if req.BlockType == "code-cell" {
		return nil
	}

	selectedJSON, err := json.Marshal(req.SelectedOptions)
	if err != nil {
		return err
	}

	_, err = ctx.DB.Exec(`INSERT INTO course_progress (person_id, course_slug, attempt_id, lesson_path, block_id, block_type, correct, selected_option, selected_options_json)
VALUES (`+ph(ctx, 1)+`, `+ph(ctx, 2)+`, `+ph(ctx, 3)+`, `+ph(ctx, 4)+`, `+ph(ctx, 5)+`, `+ph(ctx, 6)+`, `+ph(ctx, 7)+`, `+ph(ctx, 8)+`, `+ph(ctx, 9)+`)
ON CONFLICT (attempt_id, lesson_path, block_id) DO UPDATE SET
  block_type = EXCLUDED.block_type,
  correct = EXCLUDED.correct,
  selected_option = EXCLUDED.selected_option,
  selected_options_json = EXCLUDED.selected_options_json,
  answered_at = CURRENT_TIMESTAMP,
  updated_at = CURRENT_TIMESTAMP`,
		personID, courseSlug, attemptID, req.LessonPath, req.BlockID, req.BlockType, req.Correct, req.SelectedOption, string(selectedJSON))
	return err
}

func listCourseCodeBlocks(ctx *config.AppContext, attemptID int64) ([]CourseCodeBlockState, error) {
	rows, err := ctx.DB.Query(`SELECT lesson_path, block_id, block_type, code_text, output_text, output_ok, execution_count, CAST(updated_at AS TEXT)
FROM course_code_blocks
WHERE attempt_id=`+ph(ctx, 1), attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	blocks := make([]CourseCodeBlockState, 0)
	for rows.Next() {
		var block CourseCodeBlockState
		var outputOK sql.NullBool
		var executionCount sql.NullInt64
		if err := rows.Scan(&block.LessonPath, &block.BlockID, &block.BlockType, &block.CodeText, &block.OutputText, &outputOK, &executionCount, &block.UpdatedAt); err != nil {
			return nil, err
		}
		if outputOK.Valid {
			value := outputOK.Bool
			block.OutputOK = &value
		}
		if executionCount.Valid {
			value := int(executionCount.Int64)
			block.ExecutionCount = &value
		}
		blocks = append(blocks, block)
	}
	return blocks, rows.Err()
}

func saveCourseCodeBlock(ctx *config.AppContext, attemptID int64, personID int64, courseSlug string, req CourseProgressSaveRequest) error {
	codeText := ""
	if req.CodeText != nil {
		codeText = *req.CodeText
	}
	outputText := ""
	if req.OutputText != nil {
		outputText = *req.OutputText
	}

	_, err := ctx.DB.Exec(`INSERT INTO course_code_blocks (person_id, course_slug, attempt_id, lesson_path, block_id, block_type, code_text, output_text, output_ok, execution_count)
VALUES (`+ph(ctx, 1)+`, `+ph(ctx, 2)+`, `+ph(ctx, 3)+`, `+ph(ctx, 4)+`, `+ph(ctx, 5)+`, `+ph(ctx, 6)+`, `+ph(ctx, 7)+`, `+ph(ctx, 8)+`, `+ph(ctx, 9)+`, `+ph(ctx, 10)+`)
ON CONFLICT (attempt_id, lesson_path, block_id) DO UPDATE SET
  block_type = EXCLUDED.block_type,
  code_text = EXCLUDED.code_text,
  output_text = EXCLUDED.output_text,
  output_ok = EXCLUDED.output_ok,
  execution_count = EXCLUDED.execution_count,
  updated_at = CURRENT_TIMESTAMP`,
		personID, courseSlug, attemptID, req.LessonPath, req.BlockID, req.BlockType, codeText, outputText, req.OutputOK, req.ExecutionCount)
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
