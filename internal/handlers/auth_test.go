package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	mailer "github.com/base58btc/mailer/mail"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/database"
	"github.com/kodylow/base58-website/internal/types"
)

func signupTestServer(t *testing.T) (*httptest.Server, *config.AppContext, <-chan mailer.MailRequest) {
	t.Helper()
	_, _, db := commandTestServer(t, "")
	if _, err := db.Exec(`SELECT setval('people_id_seq',2); SELECT setval('course_versions_id_seq',1); SELECT setval('course_attempts_id_seq',2)`); err != nil {
		t.Fatal(err)
	}
	t.Chdir("../..")
	if _, err := database.ImportCatalog(context.Background(), db, catalogFixture()); err != nil {
		t.Fatal(err)
	}
	if err := database.ActivateLightning(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	mailbox := make(chan mailer.MailRequest, 10)
	mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var message mailer.MailRequest
		if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
			t.Errorf("decode test email: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mailbox <- message
		json.NewEncoder(w).Encode(mailer.ReturnVal{Success: true})
	}))
	t.Cleanup(mailServer.Close)
	ctx := &config.AppContext{
		IsProd: true, DB: db, Session: scs.New(),
		Infos: log.New(io.Discard, "", 0), Err: log.New(io.Discard, "", 0),
		Env: &types.EnvConfig{DBDriver: "postgres", Domain: "base58.example.invalid", CourseAssetBucket: "test", CourseAssetRegion: "nyc3", MailDomain: "example.invalid", MailEndpoint: mailServer.URL, MailerSecret: "test-secret"},
	}
	if err := SyncLocalCourseVersions(ctx); err != nil {
		t.Fatal(err)
	}
	routes, err := Routes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Reproduce a pre-fix session without sending an email or changing production.
	server := httptest.NewServer(ctx.Session.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test/legacy-session" {
			ctx.Session.Put(r.Context(), "person_id", int64(1))
			ctx.Session.Put(r.Context(), "person_email", "legacy@example.invalid")
			ctx.Session.Put(r.Context(), "student_csrf", "legacy-csrf")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		routes.ServeHTTP(w, r)
	})))
	t.Cleanup(server.Close)
	return server, ctx, mailbox
}

func signupClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func authResponse(t *testing.T, response *http.Response, err error) string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func signupCSRF(t *testing.T, client *http.Client, serverURL, path string) string {
	t.Helper()
	response, err := client.Get(serverURL + path)
	body := authResponse(t, response, err)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %d", path, response.StatusCode)
	}
	match := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("missing CSRF in %s", path)
	}
	return match[1]
}

func verificationToken(t *testing.T, mailbox <-chan mailer.MailRequest, email string) string {
	t.Helper()
	select {
	case message := <-mailbox:
		if message.ToAddr != email {
			t.Fatalf("email addressed to %q", message.ToAddr)
		}
		match := regexp.MustCompile(`https://base58.example.invalid/login/([a-f0-9]{64})`).FindStringSubmatch(message.TextBody)
		if len(match) != 2 || !strings.Contains(message.HTMLBody, match[0]) {
			t.Fatal("missing verification link in email")
		}
		return match[1]
	case <-time.After(time.Second):
		t.Fatal("no verification email was sent to the test mailer")
		return ""
	}
}

func assertSignedOut(t *testing.T, client *http.Client, serverURL string) {
	t.Helper()
	for path, status := range map[string]int{
		"/dashboard":                         http.StatusSeeOther,
		"/courses/lightning/00-welcome":      http.StatusSeeOther,
		"/api/courses/lightning/environment": http.StatusForbidden,
		"/api/courses/lightning/commands":    http.StatusForbidden,
	} {
		response, err := client.Get(serverURL + path)
		authResponse(t, response, err)
		if response.StatusCode != status {
			t.Fatalf("anonymous GET %s: %d, want %d", path, response.StatusCode, status)
		}
		if status == http.StatusSeeOther && !strings.HasPrefix(response.Header.Get("Location"), "/login") {
			t.Fatalf("anonymous GET %s did not require login", path)
		}
	}
}

func TestCourseSignupRequiresVerificationPostgres(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "new account"
		if existing {
			name = "existing account"
		}
		t.Run(name, func(t *testing.T) {
			server, ctx, mailbox := signupTestServer(t)
			email := "student@example.invalid"
			if existing {
				if _, err := ctx.DB.Exec(`UPDATE people SET display_name='Original name' WHERE id=1; INSERT INTO person_emails(person_id,email,is_primary) VALUES(1,'student@example.invalid',true)`); err != nil {
					t.Fatal(err)
				}
			}
			client := signupClient()
			csrf := signupCSRF(t, client, server.URL, "/courses/lightning")
			response, err := client.PostForm(server.URL+"/courses/lightning/signup", url.Values{"csrf_token": {csrf}, "email": {" Student@Example.Invalid "}, "display_name": {"Submitted name"}, "next": {"https://attacker.example.invalid"}})
			body := authResponse(t, response, err)
			if response.StatusCode != 200 || !strings.Contains(body, "Check your email") {
				t.Fatalf("verification prompt: %d", response.StatusCode)
			}
			token := verificationToken(t, mailbox, email)
			if strings.Contains(body, token) || strings.Contains(body, "Open dev login link") {
				t.Fatal("production response exposed the verification link")
			}
			assertSignedOut(t, client, server.URL)
			var count int
			if err := ctx.DB.QueryRow(`SELECT count(*) FROM person_emails WHERE email=$1`, email).Scan(&count); err != nil {
				t.Fatal(err)
			}
			wantCount := 0
			if existing {
				wantCount = 1
			}
			if count != wantCount {
				t.Fatal("signup created an account before verification")
			}
			if err := ctx.DB.QueryRow(`SELECT count(*) FROM course_entitlements WHERE course_slug='lightning'`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("enrolled before verification: %d %v", count, err)
			}
			if existing {
				var got string
				if err := ctx.DB.QueryRow(`SELECT display_name FROM people WHERE id=1`).Scan(&got); err != nil || got != "Original name" {
					t.Fatalf("unverified signup changed existing profile: %q %v", got, err)
				}
			}

			// The recipient can verify in a different browser; the submitting browser stays anonymous.
			verifier := signupClient()
			signupCSRF(t, verifier, server.URL, "/login")
			serverURL, _ := url.Parse(server.URL)
			oldCookie := verifier.Jar.Cookies(serverURL)[0].Value
			response, err = verifier.Get(server.URL + "/login/" + token)
			authResponse(t, response, err)
			if response.StatusCode != 303 || response.Header.Get("Location") != "/courses/lightning/00-welcome?registered=1" {
				t.Fatalf("verification redirect: %d %q", response.StatusCode, response.Header.Get("Location"))
			}
			if verifier.Jar.Cookies(serverURL)[0].Value == oldCookie {
				t.Fatal("verification did not rotate the session cookie")
			}
			var personID int64
			var displayName string
			var verified bool
			if err := ctx.DB.QueryRow(`SELECT p.id,p.display_name,pe.verified_at IS NOT NULL FROM people p JOIN person_emails pe ON p.id=pe.person_id WHERE pe.email=$1`, email).Scan(&personID, &displayName, &verified); err != nil {
				t.Fatal(err)
			}
			wantName := "Submitted name"
			if existing {
				wantName = "Original name"
				if personID != 1 {
					t.Fatal("verification duplicated the existing account")
				}
			}
			if !verified || displayName != wantName {
				t.Fatalf("verified account: name=%q verified=%v", displayName, verified)
			}
			if !hasActiveCourseEntitlement(ctx, personID, "lightning") {
				t.Fatal("verified student was not enrolled")
			}
			response, err = verifier.Get(server.URL + "/courses/lightning/00-welcome")
			authResponse(t, response, err)
			if response.StatusCode != 200 {
				t.Fatalf("verified lesson access: %d", response.StatusCode)
			}
			assertSignedOut(t, client, server.URL)
			replay := signupClient()
			response, err = replay.Get(server.URL + "/login/" + token)
			body = authResponse(t, response, err)
			if !strings.Contains(body, "invalid or expired") {
				t.Fatal("used verification link was accepted")
			}
			assertSignedOut(t, replay, server.URL)
		})
	}
}

func TestCourseVerificationRejectsExpiredAndInvalidTokensPostgres(t *testing.T) {
	server, ctx, mailbox := signupTestServer(t)
	client := signupClient()
	csrf := signupCSRF(t, client, server.URL, "/courses/lightning")
	response, err := client.PostForm(server.URL+"/courses/lightning/signup", url.Values{"csrf_token": {csrf}, "email": {"expired@example.invalid"}})
	authResponse(t, response, err)
	token := verificationToken(t, mailbox, "expired@example.invalid")
	if _, err := ctx.DB.Exec(`UPDATE student_login_tokens SET expires_at=CURRENT_TIMESTAMP-INTERVAL '1 minute' WHERE token=$1`, token); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{token, strings.Repeat("0", 64)} {
		response, err = client.Get(server.URL + "/login/" + value)
		if !strings.Contains(authResponse(t, response, err), "invalid or expired") {
			t.Fatal("invalid verification link was accepted")
		}
		assertSignedOut(t, client, server.URL)
	}
	var count int
	if err := ctx.DB.QueryRow(`SELECT count(*) FROM person_emails WHERE email='expired@example.invalid'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("expired token created account: %d %v", count, err)
	}
}

func TestCourseVerificationDeliveryFailurePostgres(t *testing.T) {
	server, ctx, _ := signupTestServer(t)
	failedMailer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer failedMailer.Close()
	for _, endpoint := range []string{failedMailer.URL, ""} {
		ctx.Env.MailEndpoint = endpoint
		client := signupClient()
		csrf := signupCSRF(t, client, server.URL, "/courses/lightning")
		response, err := client.PostForm(server.URL+"/courses/lightning/signup", url.Values{"csrf_token": {csrf}, "email": {"failed@example.invalid"}})
		body := authResponse(t, response, err)
		if !strings.Contains(body, "Unable to send your email") || strings.Contains(body, "Check your email") {
			t.Fatal("mail failure reported success")
		}
		if !strings.Contains(body, `action="/courses/lightning/signup"`) {
			t.Fatal("retry lost course enrollment context")
		}
		assertSignedOut(t, client, server.URL)
		var count int
		if err := ctx.DB.QueryRow(`SELECT count(*) FROM student_login_tokens`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("mail failure left an active token: %d %v", count, err)
		}
		if err := ctx.DB.QueryRow(`SELECT count(*) FROM person_emails WHERE email='failed@example.invalid'`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("mail failure created account: %d %v", count, err)
		}
	}
}

func TestStudentLoginTokenSingleUsePostgres(t *testing.T) {
	_, ctx, _ := signupTestServer(t)
	token, err := createStudentLoginToken(ctx, studentLoginRequest{Email: "race@example.invalid", NextPath: "/dashboard"})
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 12)
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); _, err := consumeStudentLoginToken(ctx, token); results <- err }()
	}
	workers.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("one-time link accepted %d times", successes)
	}
}

func TestExistingStudentSessionsStillEnrollPostgres(t *testing.T) {
	server, ctx, mailbox := signupTestServer(t)
	client := signupClient()
	response, err := client.Get(server.URL + "/test/legacy-session")
	authResponse(t, response, err)
	response, err = client.PostForm(server.URL+"/courses/lightning/signup", url.Values{"csrf_token": {"legacy-csrf"}, "email": {"someone-else@example.invalid"}})
	authResponse(t, response, err)
	if response.StatusCode != 303 || !hasActiveCourseEntitlement(ctx, 1, "lightning") {
		t.Fatalf("existing session could not enroll: %d", response.StatusCode)
	}
	select {
	case <-mailbox:
		t.Fatal("existing session was forced to verify again")
	default:
	}
	var count int
	if err := ctx.DB.QueryRow(`SELECT count(*) FROM person_emails WHERE email='someone-else@example.invalid'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("signed-in signup switched identities")
	}
}

func TestStudentLoginStillUsesEmailVerificationPostgres(t *testing.T) {
	server, ctx, mailbox := signupTestServer(t)
	client := signupClient()
	csrf := signupCSRF(t, client, server.URL, "/login")
	response, err := client.PostForm(server.URL+"/login", url.Values{"csrf_token": {csrf}, "email": {"login@example.invalid"}})
	body := authResponse(t, response, err)
	if !strings.Contains(body, "Check your email") {
		t.Fatal("missing login confirmation")
	}
	assertSignedOut(t, client, server.URL)
	token := verificationToken(t, mailbox, "login@example.invalid")
	response, err = client.Get(server.URL + "/login/" + token)
	authResponse(t, response, err)
	if response.StatusCode != 303 || response.Header.Get("Location") != "/dashboard" {
		t.Fatal("regular login redirect changed")
	}
	var count int
	if err := ctx.DB.QueryRow(`SELECT count(*) FROM course_entitlements WHERE course_slug='lightning'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("regular login unexpectedly enrolled in a course")
	}
}
