package handlers

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mailer "github.com/base58btc/mailer/mail"
	"github.com/gorilla/mux"
	"github.com/kodylow/base58-website/checkout"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/emails"
	"github.com/kodylow/base58-website/internal/helpers"
	"github.com/kodylow/base58-website/internal/types"
)

func retirementContext() *config.AppContext {
	return &config.AppContext{
		Infos: log.New(io.Discard, "", 0), Err: log.New(io.Discard, "", 0),
		Env: &types.EnvConfig{Secret: strings.Repeat("12", 32), MailerSecret: "test-secret", MailDomain: "example.com"},
	}
}

func TestRetiredFeatureRoutesAndCourseForms(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	ctx := retirementContext() // No Notion or payment clients: retired URLs must never call them.
	router, err := Routes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/register?c=old", "/reserve", "/summary", "/success", "/waitlist", "/newsletter/subscribe", "/lightning/subscribe", "/confirm/old", "/newsletter/schedule", "/old-missive/unschedule", "/check-email/student@example.com"} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(method, path, nil))
			if w.Code != http.StatusGone {
				t.Errorf("%s %s: status %d", method, path, w.Code)
			}
		}
	}
	for _, path := range []string{"/stripe-hook", "/opennode-hook"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader("old callback")))
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s acknowledged an unprocessed payment: %d", path, w.Code)
		}
	}
	for _, local := range []bool{false, true} {
		var body bytes.Buffer
		err := ctx.TemplateCache.ExecuteTemplate(&body, "courses/course.tmpl", CourseData{
			Course:      &types.Course{Tag: "lightning", Title: "Lightning", Availability: "Coming Soon"},
			LocalCourse: local, LoggedIn: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(body.String(), "/subscribe") || strings.Contains(body.String(), "newsletter-email") {
			t.Fatal("course still collects newsletter addresses")
		}
		if local && !strings.Contains(body.String(), `action="/courses/lightning/signup"`) {
			t.Fatal("course enrollment form was removed")
		}
	}
}

func TestLegacyCartSessionDecodesWithoutCheckout(t *testing.T) {
	RegisterLegacyCartTypes()
	fixture, err := os.ReadFile("testdata/legacy-cart-session.b64")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(fixture)))
	if err != nil {
		t.Fatal(err)
	}
	var session map[string]any
	if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&session); err != nil {
		t.Fatal(err)
	}
	if session["person_id"] != int64(42) {
		t.Fatal("lost existing signed-in person")
	}
	cart := session["cart"].(checkout.Cart)
	item := cart.Items[0].(CourseItem)
	if cart.LookupID != "legacy-cart" || cart.Infos.Email != "student@example.com" || item.Qty != 2 || item.Price.Value != 10000 {
		t.Fatalf("lost historical cart data: %#v", cart)
	}
}

func TestLegacyUnsubscribeTokenValidation(t *testing.T) {
	secret := []byte("test-secret")
	_, valid := helpers.GetSubscribeToken(secret, "Student@example.com", "lightning", 1234)
	gotEmail, gotList, err := legacySubscriptionToken(secret, valid)
	if err != nil || gotEmail != "Student@example.com" || gotList != "lightning" {
		t.Fatalf("old token failed: %v", err)
	}
	parts := strings.Split(valid, "-")
	for _, token := range []string{"", "a-b-c-d", strings.Join(parts[:3], "-") + "-00", valid + "00", "x" + valid[1:], strings.Repeat("a", 4097)} {
		if _, _, err := legacySubscriptionToken(secret, token); err == nil {
			t.Errorf("accepted malformed token %q", token)
		}
	}
	if _, _, err := legacySubscriptionToken(nil, valid); err == nil {
		t.Fatal("accepted missing secret")
	}
	if _, _, err := legacySubscriptionToken([]byte("wrong-secret"), valid); err == nil {
		t.Fatal("accepted wrong signature")
	}
}

func TestAccountAndContactEmailSurvivesRetirement(t *testing.T) {
	var jobs []mailer.MailRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/job" || r.Header.Get("Authorization") == "" {
			t.Errorf("invalid mailer request: %s %s", r.Method, r.URL.Path)
		}
		var job mailer.MailRequest
		if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
			t.Error(err)
		}
		jobs = append(jobs, job)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()
	ctx := retirementContext()
	ctx.Env.MailEndpoint = server.URL
	ctx.TemplateCache = template.Must(template.New("").Parse(`{{define "emails/tmp.tmpl"}}{{.Content}}{{end}}`))
	for _, send := range []func(*config.AppContext, string, string) error{sendStudentLoginEmail, sendAdminLoginEmail} {
		if err := send(ctx, "student@example.com", "https://example.com/login/test"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := emails.SendContactEmail(ctx, "hello@example.com", "A question", "student@example.com", "contact"); err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 || jobs[0].Title != "Base58 login" || jobs[1].Title != "Base58 admin login" || jobs[2].TextBody == "" {
		t.Fatalf("unexpected account/contact jobs: %#v", jobs)
	}
}

func TestLegacyUnsubscribePostgres(t *testing.T) {
	dsn := os.Getenv("BASE58_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("BASE58_TEST_DATABASE_URL is required for isolated PostgreSQL integration tests")
	}
	rootDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("retirement_test_%d", time.Now().UnixNano())
	if _, err := rootDB.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rootDB.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); rootDB.Close() })
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	up, err := os.ReadFile("../../migrations/postgres/000014_create_email_opt_outs.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(up)); err != nil {
		t.Fatal(err)
	}
	ctx := retirementContext()
	ctx.DB = db
	var cancellations int
	fail := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/sub" {
			t.Errorf("unsubscribe attempted %s %s", r.Method, r.URL.Path)
		}
		var request mailer.SubDelete
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.SubKey == "" {
			t.Errorf("missing subscription key: %v", err)
		}
		cancellations++
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
		}
		_, _ = w.Write([]byte(`{"success":true}`)) // HTTP failure must win over this body.
	}))
	defer server.Close()
	ctx.Env.MailEndpoint = server.URL
	router := mux.NewRouter()
	registerRetiredRoutes(router, ctx)
	_, token := helpers.GetSubscribeToken(ctx.Env.SecretBytes(), "Student@example.com", "lightning", 1234)
	visit := func(want int) {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/newsletter/unsubscribe/"+token, nil))
		if w.Code != want {
			t.Fatalf("unsubscribe status=%d body=%s", w.Code, w.Body.String())
		}
	}
	visit(http.StatusServiceUnavailable)
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM email_opt_outs WHERE email='student@example.com' AND original_email='Student@example.com' AND mailer_cancelled_at IS NULL`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("opt-out was lost on mailer failure: %d %v", count, err)
	}
	fail = false
	visit(http.StatusOK)
	visit(http.StatusOK)
	if err := db.QueryRow(`SELECT count(*) FROM email_opt_outs WHERE mailer_cancelled_at IS NOT NULL`).Scan(&count); err != nil || count != 1 || cancellations != 3 {
		t.Fatalf("retry/repeat failed: rows=%d requests=%d err=%v", count, cancellations, err)
	}
	// Rolling back application code and applying the migration again retains consent.
	down, err := os.ReadFile("../../migrations/postgres/000014_create_email_opt_outs.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(down)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(up)); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM email_opt_outs`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("rollback lost email preference: %v", err)
	}
}
