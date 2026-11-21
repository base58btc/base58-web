package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/emails"
)

type LoginData struct {
	Page         Page
	Email        string
	Next         string
	Error        string
	Flash        string
	LoginLink    string
	CSRFField    template.HTML
	SignupCourse string
	DisplayName  string
}

type studentLoginRequest struct {
	Email       string
	NextPath    string
	CourseSlug  string
	DisplayName string
}

var errInvalidStudentEmail = errors.New("Enter a valid email address.")

func StudentLogin(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if ctx == nil || ctx.DB == nil {
		http.Error(w, "Database is not configured", http.StatusServiceUnavailable)
		return
	}

	data := LoginData{
		Page:      getPage(ctx, "Login", defaultCard(ctx, r, "Login")),
		Next:      "/dashboard",
		CSRFField: studentCSRFField(r, ctx),
	}
	if r.Method == http.MethodPost {
		if !validateStudentCSRF(w, r, ctx) {
			return
		}
		if err := r.ParseForm(); err != nil {
			data.Error = "Unable to read login form."
			renderLogin(w, r, ctx, data)
			return
		}
		requestStudentLogin(w, r, ctx, studentLoginRequest{
			Email: normalizeEmail(r.Form.Get("email")), NextPath: "/dashboard",
		})
		return
	}
	renderLogin(w, r, ctx, data)
}

func requestStudentLogin(w http.ResponseWriter, r *http.Request, ctx *config.AppContext, request studentLoginRequest) {
	data := LoginData{
		Page:  getPage(ctx, "Login", defaultCard(ctx, r, "Login")),
		Email: request.Email, Next: request.NextPath,
		SignupCourse: request.CourseSlug, DisplayName: request.DisplayName,
	}
	token, err := createStudentLoginToken(ctx, request)
	if err != nil {
		data.Error = "Unable to send a login link. Please try again."
		if errors.Is(err, errInvalidStudentEmail) {
			data.Error = err.Error()
		} else {
			ctx.Err.Printf("student login token creation failed: %s", err)
		}
	} else {
		link := ctx.SitePath() + "/login/" + token
		if err := sendStudentLoginEmail(ctx, request.Email, link); err != nil {
			// A delivery failure must never turn into an authenticated session.
			_, revokeErr := ctx.DB.Exec(`DELETE FROM student_login_tokens WHERE token=`+ph(ctx, 1), token)
			if revokeErr != nil {
				ctx.Err.Printf("student login token revocation failed: %s", revokeErr)
			}
			ctx.Err.Printf("student login email failed: %s", err)
			data.Error = "Unable to send your email. Please try again."
		} else {
			data.Flash = "Check your email for a login link."
			if request.CourseSlug != "" {
				data.Flash = "Check your email for a link to verify your address and start the course."
			}
			if !ctx.IsProd {
				data.LoginLink = link
			}
		}
	}
	renderLogin(w, r, ctx, data)
}

func StudentAuthToken(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if ctx == nil || ctx.DB == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	token := mux.Vars(r)["token"]
	request, err := consumeStudentLoginToken(ctx, token)
	if err != nil {
		data := LoginData{
			Page:  getPage(ctx, "Login", defaultCard(ctx, r, "Login")),
			Error: err.Error(),
			Next:  "/dashboard",
		}
		renderLogin(w, r, ctx, data)
		return
	}

	if request.CourseSlug != "" {
		canonical, ok := canonicalLocalCourseSlug(request.CourseSlug)
		if !ok {
			http.Error(w, "Course registration is not available", http.StatusNotFound)
			return
		}
		request.CourseSlug = canonical
		request.NextPath = localCourseStartURL(canonical)
	}
	personID, err := ensurePersonForEmailWithName(ctx, request.Email, request.DisplayName)
	if err != nil {
		http.Error(w, "Unable to sign in", http.StatusInternalServerError)
		ctx.Err.Printf("student login person failed: %s", err.Error())
		return
	}
	if _, err := ctx.DB.Exec(`UPDATE person_emails SET verified_at=COALESCE(verified_at, CURRENT_TIMESTAMP) WHERE person_id=`+ph(ctx, 1)+` AND email=`+ph(ctx, 2), personID, request.Email); err != nil {
		http.Error(w, "Unable to verify email", http.StatusInternalServerError)
		ctx.Err.Printf("student email verification failed: %s", err)
		return
	}
	if request.CourseSlug != "" {
		if _, err := grantEntitlement(ctx, personID, request.CourseSlug, "self_signup", "Self-registered with verified email"); err != nil {
			http.Error(w, "Unable to grant course access", http.StatusInternalServerError)
			ctx.Err.Printf("verified course signup entitlement failed: %s", err)
			return
		}
	}
	// Rotate only this browser's session after authentication. Other students'
	// existing sessions remain valid until their normal expiry.
	if err := ctx.Session.RenewToken(r.Context()); err != nil {
		http.Error(w, "Unable to sign in", http.StatusInternalServerError)
		ctx.Err.Printf("student session renewal failed: %s", err)
		return
	}
	ctx.Session.Clear(r.Context())

	ctx.Session.Put(r.Context(), "person_id", personID)
	ctx.Session.Put(r.Context(), "person_email", request.Email)
	if admin, err := adminForLogin(ctx, request.Email); err == nil && admin.ID > 0 {
		putAdminSession(ctx, r, admin)
		writeAudit(ctx, admin.Email, "admin.login", "admin_user", strconv.FormatInt(admin.ID, 10), "via student login")
	}
	http.Redirect(w, r, safeNextPath(request.NextPath, "/dashboard"), http.StatusSeeOther)
}

func StudentLogout(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if !validateStudentCSRF(w, r, ctx) {
		return
	}
	if ctx != nil && ctx.Session != nil {
		ctx.Session.Remove(r.Context(), "person_id")
		ctx.Session.Remove(r.Context(), "person_email")
		ctx.Session.Remove(r.Context(), "admin_user_id")
		ctx.Session.Remove(r.Context(), "admin_person_id")
		ctx.Session.Remove(r.Context(), "admin_role")
		ctx.Session.Remove(r.Context(), "admin_email")
		ctx.Session.Remove(r.Context(), "admin_csrf")
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func renderLogin(w http.ResponseWriter, r *http.Request, ctx *config.AppContext, data LoginData) {
	if data.CSRFField == "" {
		data.CSRFField = studentCSRFField(r, ctx)
	}
	if err := ctx.TemplateCache.ExecuteTemplate(w, "login.tmpl", data); err != nil {
		http.Error(w, "Unable to load login page", http.StatusInternalServerError)
		ctx.Err.Printf("login.tmpl exec failed: %s", err.Error())
	}
}

func studentCSRFToken(r *http.Request, ctx *config.AppContext) string {
	if ctx == nil || ctx.Session == nil {
		return ""
	}
	token := ctx.Session.GetString(r.Context(), "student_csrf")
	if token == "" {
		token = randomToken()
		ctx.Session.Put(r.Context(), "student_csrf", token)
	}
	return token
}

func studentCSRFField(r *http.Request, ctx *config.AppContext) template.HTML {
	return template.HTML(fmt.Sprintf(`<input type="hidden" name="csrf_token" value="%s">`, template.HTMLEscapeString(studentCSRFToken(r, ctx))))
}

func validateStudentCSRF(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) bool {
	expected := ""
	if ctx != nil && ctx.Session != nil {
		expected = ctx.Session.GetString(r.Context(), "student_csrf")
	}
	actual := r.Header.Get("X-CSRF-Token")
	if actual == "" {
		_ = r.ParseForm()
		actual = r.Form.Get("csrf_token")
	}
	if expected == "" || actual == "" || actual != expected {
		http.Error(w, "Invalid form token", http.StatusForbidden)
		return false
	}
	return true
}

func createStudentLoginToken(ctx *config.AppContext, request studentLoginRequest) (string, error) {
	address, err := mail.ParseAddress(request.Email)
	if err != nil || address.Address != request.Email || len(request.Email) > 254 {
		return "", errInvalidStudentEmail
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(secret[:])
	expires := time.Now().Add(30 * time.Minute)
	_, err = ctx.DB.Exec(`INSERT INTO student_login_tokens (token, email, next_path, expires_at, course_slug, display_name)
VALUES (`+ph(ctx, 1)+`, `+ph(ctx, 2)+`, `+ph(ctx, 3)+`, `+ph(ctx, 4)+`, `+ph(ctx, 5)+`, `+ph(ctx, 6)+`)`, token, request.Email, safeNextPath(request.NextPath, "/dashboard"), expires, request.CourseSlug, request.DisplayName)
	return token, err
}

func consumeStudentLoginToken(ctx *config.AppContext, token string) (studentLoginRequest, error) {
	var request studentLoginRequest
	// Claim the token atomically: concurrent requests must not reuse a link.
	err := ctx.DB.QueryRow(`UPDATE student_login_tokens SET used_at=CURRENT_TIMESTAMP
WHERE token=`+ph(ctx, 1)+` AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP
RETURNING email, next_path, course_slug, display_name`, token).
		Scan(&request.Email, &request.NextPath, &request.CourseSlug, &request.DisplayName)
	if err != nil {
		return studentLoginRequest{}, errors.New("login link is invalid or expired")
	}
	return request, nil
}

func adminForLogin(ctx *config.AppContext, email string) (AdminUser, error) {
	admin, err := adminByEmail(ctx, email)
	if err == nil {
		return admin, nil
	}
	if isBootstrapAdminEmail(ctx, email) {
		return bootstrapAdminUser(ctx, email)
	}
	return AdminUser{}, err
}

func putAdminSession(ctx *config.AppContext, r *http.Request, admin AdminUser) {
	ctx.Session.Put(r.Context(), "admin_user_id", admin.ID)
	ctx.Session.Put(r.Context(), "admin_person_id", admin.PersonID)
	ctx.Session.Put(r.Context(), "admin_role", admin.Role)
	ctx.Session.Put(r.Context(), "admin_email", admin.Email)
}

func sendStudentLoginEmail(ctx *config.AppContext, email, link string) error {
	if ctx.Env == nil || ctx.Env.MailEndpoint == "" || ctx.Env.MailDomain == "" {
		if ctx.IsProd {
			return errors.New("login email is not configured")
		}
		return nil
	}
	body := fmt.Sprintf(`<p>Use this link to sign in to Base58:</p><p><a href="%s">%s</a></p><p>This link expires in 30 minutes.</p>`, link, link)
	return emails.ComposeAndSendMail(ctx, &emails.Mail{
		JobKey:   "student-login-" + randomToken(),
		Email:    email,
		Title:    "Base58 login",
		SendAt:   time.Now(),
		HTMLBody: []byte(body),
		TextBody: []byte("Use this link to sign in to Base58: " + link),
	})
}

func safeNextPath(nextPath, fallback string) string {
	nextPath = strings.TrimSpace(nextPath)
	if fallback == "" {
		fallback = "/dashboard"
	}
	if nextPath == "" {
		return fallback
	}
	parsed, err := url.Parse(nextPath)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") || strings.HasPrefix(parsed.Path, "//") {
		return fallback
	}
	return parsed.RequestURI()
}

func loginPathWithNext(nextPath string) string {
	nextPath = safeNextPath(nextPath, "/dashboard")
	return "/login?next=" + url.QueryEscape(nextPath)
}

func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
