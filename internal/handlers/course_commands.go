package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"

	"github.com/gorilla/mux"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/courseenv"
	"github.com/kodylow/base58-website/internal/coursehistory"
)

type commandJournalResponse struct {
	Revision int                   `json:"revision"`
	Events   []coursehistory.Event `json:"events"`
}

// This endpoint records browser executions; it never runs a command.
func CourseCommands(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	w.Header().Set("Cache-Control", "no-store")
	if ctx == nil || ctx.DB == nil {
		http.Error(w, "Course history unavailable", 503)
		return
	}
	slug := mux.Vars(r)["course"]
	personID := currentProgressPersonID(r, ctx)
	if !isSafeCourseSlug(slug) || personID == 0 || !hasActiveCourseEntitlement(ctx, personID, slug) {
		http.Error(w, "Course access required", 403)
		return
	}
	attempt, err := activeCourseAttempt(ctx, personID, slug)
	if err != nil {
		http.Error(w, "Active course attempt required", 403)
		return
	}
	files, err := listCourseVersionFileContents(ctx, attempt.CourseVersionID)
	if err != nil {
		http.Error(w, "Course environment unavailable", 503)
		return
	}
	manifest, err := courseenv.ForFiles(files)
	if err != nil || manifest.SourceHash != r.URL.Query().Get("environment") {
		http.Error(w, "Course environment changed; reload this lesson", 409)
		return
	}
	if mux.Vars(r)["resource"] != "" {
		courseVMStorage(w, r, ctx, attempt.ID, manifest.SourceHash)
		return
	}
	if r.Method == http.MethodGet {
		journal, err := readCommandJournal(ctx.DB, attempt.ID, manifest.SourceHash)
		if err != nil {
			http.Error(w, "Unable to read command history", 500)
			return
		}
		writeCourseProgressJSON(w, 200, journal)
		return
	}
	if !validateStudentCSRF(w, r, ctx) {
		return
	}
	var request struct {
		ExpectedRevision int                 `json:"expectedRevision"`
		Event            coursehistory.Event `json:"event"`
	}
	// JSON escaping can expand two 1 MiB output streams by up to six times.
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "Invalid command event", 400)
		return
	}
	tx, err := ctx.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Unable to save command history", 500)
		return
	}
	defer tx.Rollback()
	// Serialize journal mutations and attempt resets across browsers/devices.
	var activeID int64
	err = tx.QueryRowContext(r.Context(), `SELECT id FROM course_attempts WHERE id=$1 AND status='active' FOR UPDATE`, attempt.ID).Scan(&activeID)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Course attempt changed; reload", 409)
		return
	}
	if err != nil {
		http.Error(w, "Unable to save command history", 500)
		return
	}
	if !ownsCourseVM(tx, attempt.ID, manifest.SourceHash, r.Header.Get("X-Course-Session")) {
		http.Error(w, "Course environment is open on another device; close it there first", 409)
		return
	}
	journal, err := readCommandJournal(tx, attempt.ID, manifest.SourceHash)
	if err != nil {
		http.Error(w, "Unable to read command history", 500)
		return
	}
	state := coursehistory.New()
	for _, event := range journal.Events {
		if event.ID == request.Event.ID {
			// Retrying a request whose response was lost is safe and idempotent.
			if !reflect.DeepEqual(event, request.Event) {
				http.Error(w, "Event ID already used", 409)
				return
			}
			writeCourseProgressJSON(w, 200, journal)
			return
		}
		if err := state.Apply(event); err != nil {
			http.Error(w, "Invalid stored command history", 500)
			return
		}
	}
	if request.ExpectedRevision != journal.Revision {
		http.Error(w, "Command history changed in another session; reload", 409)
		return
	}
	if err := state.Apply(request.Event); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	checkpoint := "initial"
	if request.Event.Type == "result" {
		checkpoint = request.Event.CommandID
	}
	if request.Event.Type == "checkout" && request.Event.Head != "" {
		checkpoint = request.Event.Head
	}
	if request.Event.Type == "result" || request.Event.Type == "checkout" || request.Event.Type == "command" {
		if request.Event.Type == "command" && request.Event.Parent != "" {
			checkpoint = request.Event.Parent
		}
		var present bool
		err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM course_vm_checkpoints WHERE attempt_id=$1 AND environment_hash=$2 AND checkpoint_id=$3)`, attempt.ID, manifest.SourceHash, checkpoint).Scan(&present)
		if err != nil || !present {
			http.Error(w, "Save the VM checkpoint before advancing history", 409)
			return
		}
	}
	body, err := json.Marshal(request.Event)
	if err != nil {
		http.Error(w, "Invalid command event", 400)
		return
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO course_command_events (attempt_id, environment_hash, revision, event_id, event_json) VALUES ($1,$2,$3,$4,$5)`, attempt.ID, manifest.SourceHash, journal.Revision+1, request.Event.ID, string(body))
	if err != nil {
		http.Error(w, "Unable to save command history", 500)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Unable to save command history", 500)
		return
	}
	journal.Events = append(journal.Events, request.Event)
	journal.Revision++
	writeCourseProgressJSON(w, 200, journal)
}

type commandJournalReader interface {
	Query(string, ...any) (*sql.Rows, error)
}

func readCommandJournal(db commandJournalReader, attemptID int64, environment string) (commandJournalResponse, error) {
	journal := commandJournalResponse{Events: []coursehistory.Event{}}
	rows, err := db.Query(`SELECT event_json FROM course_command_events WHERE attempt_id=$1 AND environment_hash=$2 ORDER BY revision`, attemptID, environment)
	if err != nil {
		return journal, err
	}
	defer rows.Close()
	for rows.Next() {
		var content string
		var event coursehistory.Event
		if err := rows.Scan(&content); err != nil {
			return journal, err
		}
		if err := json.Unmarshal([]byte(content), &event); err != nil {
			return journal, err
		}
		journal.Events = append(journal.Events, event)
	}
	journal.Revision = len(journal.Events)
	return journal, rows.Err()
}
