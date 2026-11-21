package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"

	"github.com/gorilla/mux"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/courseenv"
)

var vmHash = regexp.MustCompile(`^[a-f0-9]{64}$`)
var vmID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

type vmCheckpoint struct {
	Format int             `json:"format"`
	Chunks []string        `json:"chunks"`
	State  json.RawMessage `json:"state"`
}
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
	resource, key := mux.Vars(r)["resource"], mux.Vars(r)["key"]
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
	if !vmID.MatchString(key) {
		http.Error(w, "Invalid checkpoint", 400)
		return
	}
	if r.Method == "GET" {
		if resource == "chunks" {
			if !vmHash.MatchString(key) {
				http.Error(w, "Invalid chunk", 400)
				return
			}
			var data []byte
			err := ctx.DB.QueryRow(`SELECT data FROM course_vm_chunks WHERE attempt_id=$1 AND environment_hash=$2 AND hash=$3`, attempt, environment, key).Scan(&data)
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			if err != nil {
				http.Error(w, "Unable to read checkpoint", 500)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(data)
			return
		}
		var descriptor string
		err := ctx.DB.QueryRow(`SELECT descriptor FROM course_vm_checkpoints WHERE attempt_id=$1 AND environment_hash=$2 AND checkpoint_id=$3`, attempt, environment, key).Scan(&descriptor)
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Unable to read checkpoint", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, descriptor)
		return
	}
	if r.Method != "PUT" {
		w.WriteHeader(405)
		return
	}
	if !ownsCourseVM(ctx.DB, attempt, environment, owner) {
		http.Error(w, "Course session expired; reopen the environment", 409)
		return
	}
	if resource == "chunks" {
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
		if err != nil || !vmHash.MatchString(key) || courseenv.Hash(data) != key {
			http.Error(w, "Invalid checkpoint chunk", 400)
			return
		}
		_, err = ctx.DB.Exec(`INSERT INTO course_vm_chunks (attempt_id,environment_hash,hash,data) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, attempt, environment, key, data)
		if err != nil {
			http.Error(w, "Unable to save checkpoint chunk", 500)
			return
		}
		w.WriteHeader(204)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	var checkpoint vmCheckpoint
	if err != nil || json.Unmarshal(data, &checkpoint) != nil || checkpoint.Format != 1 || len(checkpoint.Chunks) == 0 || len(checkpoint.Chunks) > 8192 || len(checkpoint.State) == 0 {
		http.Error(w, "Invalid VM checkpoint", 400)
		return
	}
	unique := map[string]bool{}
	for _, hash := range checkpoint.Chunks {
		if !vmHash.MatchString(hash) {
			http.Error(w, "Invalid checkpoint hash", 400)
			return
		}
		unique[hash] = true
	}
	hashes, _ := json.Marshal(checkpoint.Chunks)
	var count int
	err = ctx.DB.QueryRow(`SELECT count(*) FROM course_vm_chunks WHERE attempt_id=$1 AND environment_hash=$2 AND hash IN (SELECT jsonb_array_elements_text($3::jsonb))`, attempt, environment, string(hashes)).Scan(&count)
	if err != nil || count != len(unique) {
		http.Error(w, "Checkpoint upload is incomplete", 409)
		return
	}
	// Immutable checkpoints allow retries and protect the baseline from a second
	// device trying to initialize a different random state under the same name.
	var stored string
	err = ctx.DB.QueryRow(`INSERT INTO course_vm_checkpoints (attempt_id,environment_hash,checkpoint_id,descriptor) VALUES ($1,$2,$3,$4) ON CONFLICT (attempt_id,environment_hash,checkpoint_id) DO UPDATE SET descriptor=course_vm_checkpoints.descriptor RETURNING descriptor`, attempt, environment, key, string(data)).Scan(&stored)
	if err != nil {
		http.Error(w, "Unable to save checkpoint", 500)
		return
	}
	if stored != string(data) {
		http.Error(w, "Checkpoint already exists; reload the environment", 409)
		return
	}
	w.WriteHeader(204)
}
