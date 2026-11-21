package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"

	"github.com/gorilla/mux"
	"github.com/kodylow/base58-website/internal/config"
)

var vmID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

type vmRowReader interface{ QueryRow(string, ...any) *sql.Row }

func ownsCourseVM(db vmRowReader, attempt int64, environment, owner string) bool {
	if !vmID.MatchString(owner) {
		return false
	}
	var yes bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM course_vm_sessions WHERE attempt_id=$1 AND environment_hash=$2 AND owner=$3 AND expires_at > now())`, attempt, environment, owner).Scan(&yes)
	return err == nil && yes
}

func courseVMStorage(w http.ResponseWriter, r *http.Request, ctx *config.AppContext, attempt int64, environment string) {
	resource := mux.Vars(r)["resource"]
	owner := r.Header.Get("X-Course-Session")
	if r.Method != "GET" && !validateStudentCSRF(w, r, ctx) {
		return
	}
	if resource == "lease" {
		if !vmID.MatchString(owner) {
			http.Error(w, "Invalid course session", 400)
			return
		}
		if r.Method == "DELETE" {
			_, err := ctx.DB.Exec(`DELETE FROM course_vm_sessions WHERE attempt_id=$1 AND environment_hash=$2 AND owner=$3`, attempt, environment, owner)
			if err != nil {
				http.Error(w, "Unable to close course session", 500)
				return
			}
			w.WriteHeader(204)
			return
		}
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var acquired string
		err := ctx.DB.QueryRow(`INSERT INTO course_vm_sessions (attempt_id, environment_hash, owner, expires_at) VALUES ($1,$2,$3,now()+interval '2 minutes') ON CONFLICT (attempt_id, environment_hash) DO UPDATE SET owner=EXCLUDED.owner, expires_at=EXCLUDED.expires_at WHERE course_vm_sessions.owner=EXCLUDED.owner OR course_vm_sessions.expires_at <= now() RETURNING owner`, attempt, environment, owner).Scan(&acquired)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "This course is open on another device. Save and stop it there, or wait two minutes after closing it.", 409)
			return
		}
		if err != nil {
			http.Error(w, "Unable to open course session", 500)
			return
		}
		w.WriteHeader(204)
		return
	}
	// Keep historical snapshot data, but reject all further reads and writes.
	http.Error(w, "VM snapshots are disabled. Reload to use command replay.", http.StatusGone)
}
