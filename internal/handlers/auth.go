package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/emails"
)

type LoginData struct {
	Page      Page
	Email     string
	Next      string
	Error     string
	Flash     string
	LoginLink string
}

func StudentLogin(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if ctx == nil || ctx.DB == nil {
		http.Error(w, "Database is not configured", http.StatusServiceUnavailable)
		return
	}

	data := LoginData{
		Page: getPage(ctx, "Login", defaultCard(ctx, r, "Login")),
		Next: "/dashboard",
	}
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			data.Error = "Unable to read login form."
			renderLogin(w, r, ctx, data)
			return
		}
		email := normalizeEmail(r.Form.Get("email"))
		data.Email = email
		data.Next = "/dashboard"
		token, err := createStudentLoginToken(ctx, email, data.Next)
		if err != nil {
			data.Error = err.Error()
		} else {
			link := ctx.SitePath() + "/login/" + token
			data.Flash = "Check your email for a login link."
			if !ctx.IsProd {
				data.LoginLink = link
				ctx.Infos.Printf("student login link for %s: %s", email, link)
			}
			_ = sendStudentLoginEmail(ctx, email, link)
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
	email, _, err := consumeStudentLoginToken(ctx, token)
	if err != nil {
		data := LoginData{
			Page:  getPage(ctx, "Login", defaultCard(ctx, r, "Login")),
			Error: err.Error(),
			Next:  "/dashboard",
		}
		renderLogin(w, r, ctx, data)
		return
	}

	personID, err := ensurePersonForEmail(ctx, email)
	if err != nil {
		http.Error(w, "Unable to sign in", http.StatusInternalServerError)
		ctx.Err.Printf("student login person failed: %s", err.Error())
		return
	}

	ctx.Session.Put(r.Context(), "person_id", personID)
	ctx.Session.Put(r.Context(), "person_email", email)
	if admin, err := adminForLogin(ctx, email); err == nil && admin.ID > 0 {
		putAdminSession(ctx, r, admin)
		writeAudit(ctx, admin.Email, "admin.login", "admin_user", strconv.FormatInt(admin.ID, 10), "via student login")
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func StudentLogout(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
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
	if err := ctx.TemplateCache.ExecuteTemplate(w, "login.tmpl", data); err != nil {
		http.Error(w, "Unable to load login page", http.StatusInternalServerError)
		ctx.Err.Printf("login.tmpl exec failed: %s", err.Error())
	}
}

func createStudentLoginToken(ctx *config.AppContext, email, nextPath string) (string, error) {
	if email == "" {
		return "", errors.New("email is required")
	}
	token := randomToken()
	expires := time.Now().Add(30 * time.Minute)
	_, err := ctx.DB.Exec(`INSERT INTO student_login_tokens (token, email, next_path, expires_at)
VALUES (`+ph(ctx, 1)+`, `+ph(ctx, 2)+`, `+ph(ctx, 3)+`, `+ph(ctx, 4)+`)`, token, email, safeNextPath(nextPath, "/dashboard"), expires)
	return token, err
}

func consumeStudentLoginToken(ctx *config.AppContext, token string) (string, string, error) {
	var email, nextPath string
	err := ctx.DB.QueryRow(`SELECT email, next_path
FROM student_login_tokens
WHERE token=`+ph(ctx, 1)+` AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP`, token).Scan(&email, &nextPath)
	if err != nil {
		return "", "", errors.New("login link is invalid or expired")
	}
	if _, err := ctx.DB.Exec(`UPDATE student_login_tokens SET used_at=CURRENT_TIMESTAMP WHERE token=`+ph(ctx, 1), token); err != nil {
		return "", "", err
	}
	return email, nextPath, nil
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
