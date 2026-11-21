package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gorilla/mux"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/courseenv"
	"github.com/kodylow/base58-website/internal/coursehistory"
	"github.com/kodylow/base58-website/internal/types"
)

func commandTestServer(t *testing.T, courseDir string) (*httptest.Server, string) {
	t.Helper()
	dsn := os.Getenv("BASE58_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("BASE58_TEST_DATABASE_URL is required for isolated PostgreSQL integration tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("course_history_test_%d", time.Now().UnixNano())
	if _, err := db.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); db.Close() })
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	testDB, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testDB.Close() })
	paths, _ := filepath.Glob("../../migrations/postgres/*.up.sql")
	sort.Strings(paths)
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = testDB.Exec(string(content)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	files := map[string]string{"01_intro.md": "# Intro", "flake.nix": "{}", "flake.lock": "{}"}
	if courseDir != "" {
		if err := filepath.WalkDir(courseDir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			name, _ := filepath.Rel(courseDir, path)
			name = filepath.ToSlash(name)
			if !isCourseSourcePath(name) {
				return nil
			}
			content, err := os.ReadFile(path)
			if err == nil {
				files[name] = string(content)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	} else {
		store := "/nix/store/00000000000000000000000000000000-bash"
		hash := courseenv.SourceHash(files)
		m := courseenv.Manifest{Format: 1, System: courseenv.System, SourceHash: hash, Shell: store + "/bin/bash", Archives: []courseenv.Archive{{StorePath: store, Path: "runtime/" + hash + "/" + filepath.Base(store) + ".nar.gz", SHA256: strings.Repeat("0", 64), Size: 1}}}
		content, _ := json.Marshal(m)
		files[courseenv.ManifestFile] = string(content)
	}
	manifest, err := courseenv.ForFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	_, err = testDB.Exec(`INSERT INTO people(id) VALUES(1),(2); INSERT INTO courses(slug,title) VALUES('history-test','History test'); INSERT INTO course_versions(id,course_slug,version_number) VALUES(1,'history-test',1); INSERT INTO course_attempts(id,person_id,course_slug,course_version_id,attempt_number) VALUES(1,1,'history-test',1,1),(2,2,'history-test',1,1); INSERT INTO course_entitlements(person_id,course_slug) VALUES(1,'history-test'),(2,'history-test')`)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if _, err := testDB.Exec(`INSERT INTO course_version_files(course_version_id,path,content_hash,content) VALUES(1,$1,$2,$3)`, name, courseenv.Hash([]byte(content)), content); err != nil {
			t.Fatal(err)
		}
	}
	session := scs.New()
	session.Cookie.Secure = false
	ctx := &config.AppContext{DB: testDB, Session: session, Err: log.New(io.Discard, "", 0), Env: &types.EnvConfig{DBDriver: "postgres", CourseAssetBucket: "test", CourseAssetRegion: "nyc3"}}
	r := mux.NewRouter()
	r.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		person := int64(1)
		if r.URL.Query().Get("person") == "2" {
			person = 2
		}
		session.Put(r.Context(), "person_id", person)
		session.Put(r.Context(), "student_csrf", "test-token")
		http.Redirect(w, r, "/", 302)
	})
	r.HandleFunc("/api/courses/{course}/commands", func(w http.ResponseWriter, r *http.Request) { CourseCommands(w, r, ctx) }).Methods("GET", "POST")
	r.HandleFunc("/api/courses/{course}/commands/{resource:lease|chunks|checkpoints}/{key:[A-Za-z0-9_-]+}", func(w http.ResponseWriter, r *http.Request) { CourseCommands(w, r, ctx) }).Methods("GET", "PUT", "POST", "DELETE")
	r.HandleFunc("/environment.json", func(w http.ResponseWriter, r *http.Request) {
		r = mux.SetURLVars(r, map[string]string{"course": "history-test"})
		recorder := httptest.NewRecorder()
		CourseBrowserEnvironment(recorder, r, ctx)
		if recorder.Code != 200 {
			w.WriteHeader(recorder.Code)
			w.Write(recorder.Body.Bytes())
			return
		}
		var env map[string]any
		json.Unmarshal(recorder.Body.Bytes(), &env)
		env["assetBaseURL"] = "http://" + r.Host + "/"
		env["csrfToken"] = "test-token"
		writeCourseProgressJSON(w, 200, env)
	})
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("../../static"))))
	if courseDir != "" {
		r.PathPrefix("/runtime/").Handler(http.StripPrefix("/runtime/", http.FileServer(http.Dir(filepath.Join(courseDir, "runtime")))))
	}
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../../static/course-vm-preview.html")
	})
	server := httptest.NewUnstartedServer(session.LoadAndSave(r))
	if courseDir != "" {
		server.Listener.Close()
		server.Listener, err = net.Listen("tcp", "127.0.0.1:8098")
		if err != nil {
			t.Fatal(err)
		}
	}
	server.Start()
	t.Cleanup(server.Close)
	return server, manifest.SourceHash
}

func TestCourseCommandsPostgres(t *testing.T) {
	server, environment := commandTestServer(t, "")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	resp, err := client.Get(server.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	base := "/api/courses/history-test/commands"
	call := func(method, path, owner string, body []byte, want int) []byte {
		t.Helper()
		req, _ := http.NewRequest(method, server.URL+path+"?environment="+environment, bytes.NewReader(body))
		req.Header.Set("X-CSRF-Token", "test-token")
		req.Header.Set("X-Course-Session", owner)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != want {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, resp.StatusCode, want, data)
		}
		return data
	}
	call("POST", base+"/lease/session", "device-a", nil, 204)
	call("POST", base+"/lease/session", "device-b", nil, 409)
	chunk := []byte("a private compressed checkpoint fixture")
	hash := courseenv.Hash(chunk)
	call("PUT", base+"/chunks/"+hash, "device-a", []byte("corrupt"), 400)
	call("PUT", base+"/chunks/"+hash, "device-a", chunk, 204)
	checkpoint, _ := json.Marshal(vmCheckpoint{Format: 1, Chunks: []string{hash}, State: json.RawMessage(`{"sdk":"fixture"}`)})
	call("PUT", base+"/checkpoints/initial", "device-a", checkpoint, 204)
	event := coursehistory.Event{ID: "cmd-1", Type: "command", Command: &coursehistory.Command{Program: "bitcoin-cli", Args: []string{"createwallet", "course"}}}
	appendEvent := func(revision int, e coursehistory.Event, want int) {
		body, _ := json.Marshal(map[string]any{"expectedRevision": revision, "event": e})
		call("POST", base, "device-a", body, want)
	}
	appendEvent(0, event, 200)
	appendEvent(0, event, 200) // response-loss retry
	result := coursehistory.Event{ID: "res-1", Type: "result", CommandID: "cmd-1", Result: &coursehistory.Result{Stdout: "wallet created"}}
	appendEvent(1, result, 409) // history cannot outrun its checkpoint
	call("PUT", base+"/checkpoints/cmd-1", "device-a", checkpoint, 204)
	appendEvent(1, result, 200)
	appendEvent(1, coursehistory.Event{ID: "stale", Type: "checkout"}, 409)
	appendEvent(2, coursehistory.Event{ID: "undo", Type: "checkout"}, 200)
	call("DELETE", base+"/lease/session", "device-a", nil, 204)
	call("POST", base+"/lease/session", "device-b", nil, 204)
	if got := call("GET", base+"/chunks/"+hash, "device-b", nil, 200); !bytes.Equal(got, chunk) {
		t.Fatal("cross-device chunk differs")
	}
	var journal commandJournalResponse
	json.Unmarshal(call("GET", base, "device-b", nil, 200), &journal)
	if journal.Revision != 3 {
		t.Fatalf("lost or duplicated events: %d", journal.Revision)
	}
	resp, err = client.Get(server.URL + "/login?person=2")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	call("GET", base+"/chunks/"+hash, "device-b", nil, 404)
	var other commandJournalResponse
	json.Unmarshal(call("GET", base, "device-b", nil, 200), &other)
	if len(other.Events) != 0 {
		t.Fatal("another student's journal was exposed")
	}
}

// An opt-in local fixture for testing two independent browser profiles against
// the real account/checkpoint handlers. It never connects to a production DB.
func TestCourseCommandBrowserPreview(t *testing.T) {
	dir := os.Getenv("BASE58_VM_TEST_COURSE_DIR")
	if dir == "" {
		t.Skip("browser fixture not requested")
	}
	server, _ := commandTestServer(t, dir)
	t.Log("Browser checkpoint fixture: " + server.URL + "/login")
	select {
	case <-time.After(45 * time.Minute):
	}
}
