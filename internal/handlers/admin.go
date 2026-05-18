package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/emails"
	"github.com/kodylow/base58-website/internal/types"
)

type AdminPage struct {
	Page         Page
	Title        string
	Flash        string
	Error        string
	HasDB        bool
	People       []AdminPerson
	Person       AdminPerson
	Courses      []AdminCourse
	Course       AdminCourse
	Entitlements []AdminEntitlement
	AuditEvents  []AdminAuditEvent
	CurrentAdmin AdminUser
	CSRFField    template.HTML
	Email        string
	LoginLink    string
	Query        string
}

type AdminUser struct {
	ID          int64
	PersonID    int64
	Role        string
	Status      string
	Email       string
	DisplayName string
}

type AdminPerson struct {
	ID           int64
	DisplayName  string
	AvatarURL    string
	XURL         string
	InstagramURL string
	LinkedinURL  string
	GithubURL    string
	NostrNpub    string
	Timezone     string
	Status       string
	PrimaryEmail string
	Emails       []AdminEmail
}

type AdminEmail struct {
	ID        int64
	Email     string
	IsPrimary bool
	Verified  string
}

type AdminCourse struct {
	Slug        string
	Title       string
	Description string
	HeaderImg   string
	Status      string
}

type AdminEntitlement struct {
	ID          int64
	PersonID    int64
	CourseSlug  string
	CourseTitle string
	Email       string
	Status      string
	Source      string
	StartsAt    string
	ExpiresAt   string
	GrantedAt   string
	RevokedAt   string
	Notes       string
}

type AdminAuditEvent struct {
	ID         int64
	Actor      string
	Action     string
	TargetType string
	TargetID   string
	Details    string
	CreatedAt  string
}

func RegisterAdminRoutes(r *mux.Router, ctx *config.AppContext) {
	r.HandleFunc("/admin/login", func(w http.ResponseWriter, r *http.Request) {
		AdminLogin(w, r, ctx)
	}).Methods("GET", "POST")
	r.HandleFunc("/admin/auth/{token}", func(w http.ResponseWriter, r *http.Request) {
		AdminAuthToken(w, r, ctx)
	}).Methods("GET")
	r.HandleFunc("/admin/logout", func(w http.ResponseWriter, r *http.Request) {
		AdminLogout(w, r, ctx)
	}).Methods("POST")
	r.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		AdminDashboard(w, r, ctx)
	}).Methods("GET")
	r.HandleFunc("/admin/people", func(w http.ResponseWriter, r *http.Request) {
		AdminPeople(w, r, ctx)
	}).Methods("GET")
	r.HandleFunc("/admin/people/new", func(w http.ResponseWriter, r *http.Request) {
		AdminNewPerson(w, r, ctx)
	}).Methods("GET", "POST")
	r.HandleFunc("/admin/people/{id:[0-9]+}", func(w http.ResponseWriter, r *http.Request) {
		AdminPersonDetail(w, r, ctx)
	}).Methods("GET", "POST")
	r.HandleFunc("/admin/people/{id:[0-9]+}/emails", func(w http.ResponseWriter, r *http.Request) {
		AdminAddPersonEmail(w, r, ctx)
	}).Methods("POST")
	r.HandleFunc("/admin/people/{id:[0-9]+}/entitlements", func(w http.ResponseWriter, r *http.Request) {
		AdminGrantEntitlement(w, r, ctx)
	}).Methods("POST")
	r.HandleFunc("/admin/courses", func(w http.ResponseWriter, r *http.Request) {
		AdminCourses(w, r, ctx)
	}).Methods("GET")
	r.HandleFunc("/admin/courses/new", func(w http.ResponseWriter, r *http.Request) {
		AdminNewCourse(w, r, ctx)
	}).Methods("GET", "POST")
	r.HandleFunc("/admin/courses/{slug}", func(w http.ResponseWriter, r *http.Request) {
		AdminCourseDetail(w, r, ctx)
	}).Methods("GET", "POST")
	r.HandleFunc("/admin/entitlements/{id:[0-9]+}/revoke", func(w http.ResponseWriter, r *http.Request) {
		AdminRevokeEntitlement(w, r, ctx)
	}).Methods("POST")
	r.HandleFunc("/admin/audit", func(w http.ResponseWriter, r *http.Request) {
		AdminAudit(w, r, ctx)
	}).Methods("GET")
}

func AdminLogin(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	data := baseAdminPage(ctx, "Admin Login")
	if ctx.DB == nil {
		data.HasDB = false
		data.Error = "Database is not configured. Add DB_DRIVER and DATABASE_URL, then run migrations."
		renderAdmin(w, r, ctx, "admin/login.tmpl", data)
		return
	}
	if r.Method == http.MethodPost {
		r.ParseForm()
		email := normalizeEmail(r.Form.Get("email"))
		data.Email = email
		token, err := createAdminLoginToken(ctx, email)
		if err != nil {
			data.Error = err.Error()
		} else {
			data.Flash = "Check your email for an admin login link."
			link := ctx.SitePath() + "/admin/auth/" + token
			if !ctx.IsProd {
				data.LoginLink = link
				ctx.Infos.Printf("admin login link for %s: %s", email, link)
			}
			_ = sendAdminLoginEmail(ctx, email, link)
		}
	}
	renderAdmin(w, r, ctx, "admin/login.tmpl", data)
}

func AdminAuthToken(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if ctx.DB == nil {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	token := mux.Vars(r)["token"]
	admin, err := consumeAdminLoginToken(ctx, token)
	if err != nil {
		data := baseAdminPage(ctx, "Admin Login")
		data.Error = err.Error()
		renderAdmin(w, r, ctx, "admin/login.tmpl", data)
		return
	}
	ctx.Session.Put(r.Context(), "admin_user_id", admin.ID)
	ctx.Session.Put(r.Context(), "admin_role", admin.Role)
	ctx.Session.Put(r.Context(), "admin_email", admin.Email)
	writeAudit(ctx, admin.Email, "admin.login", "admin_user", strconv.FormatInt(admin.ID, 10), "")
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func AdminLogout(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	admin := currentAdminFromSession(r, ctx)
	if !validateAdminCSRF(w, r, ctx) {
		return
	}
	ctx.Session.Remove(r.Context(), "admin_user_id")
	ctx.Session.Remove(r.Context(), "admin_role")
	ctx.Session.Remove(r.Context(), "admin_email")
	ctx.Session.Remove(r.Context(), "admin_csrf")
	if admin.ID > 0 {
		writeAudit(ctx, admin.Email, "admin.logout", "admin_user", strconv.FormatInt(admin.ID, 10), "")
	}
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func AdminDashboard(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if !requireAdmin(w, r, ctx) {
		return
	}
	data := baseAdminPage(ctx, "Admin")
	data.HasDB = ctx.DB != nil
	if ctx.DB != nil {
		data.People, _ = listPeople(ctx, "")
		data.Courses, _ = listAdminCourses(ctx)
		data.AuditEvents, _ = listAuditEvents(ctx, 8)
		if len(data.People) > 8 {
			data.People = data.People[:8]
		}
		if len(data.Courses) > 8 {
			data.Courses = data.Courses[:8]
		}
	}
	renderAdmin(w, r, ctx, "admin/index.tmpl", data)
}

func AdminPeople(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if !requireAdmin(w, r, ctx) {
		return
	}
	if !requireAdminDB(w, r, ctx, "People") {
		return
	}
	data := baseAdminPage(ctx, "People")
	data.Query = strings.TrimSpace(r.URL.Query().Get("q"))
	var err error
	data.People, err = listPeople(ctx, data.Query)
	if err != nil {
		data.Error = err.Error()
	}
	renderAdmin(w, r, ctx, "admin/people.tmpl", data)
}

func AdminNewPerson(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if !requireAdmin(w, r, ctx) {
		return
	}
	if !requireAdminDB(w, r, ctx, "New Person") {
		return
	}
	data := baseAdminPage(ctx, "New Person")
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			data.Error = "Unable to read form."
		} else {
			id, err := createPerson(ctx, personFromForm(r), normalizeEmail(r.Form.Get("email")))
			if err != nil {
				data.Error = err.Error()
			} else {
				writeAudit(ctx, adminActor(r, ctx), "person.create", "person", strconv.FormatInt(id, 10), "")
				http.Redirect(w, r, fmt.Sprintf("/admin/people/%d?flash=created", id), http.StatusSeeOther)
				return
			}
		}
	}
	renderAdmin(w, r, ctx, "admin/person_form.tmpl", data)
}

func AdminPersonDetail(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if !requireAdmin(w, r, ctx) {
		return
	}
	if !requireAdminDB(w, r, ctx, "Person") {
		return
	}
	id, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	data := baseAdminPage(ctx, "Person")
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			data.Error = "Unable to read form."
		} else if err := updatePerson(ctx, id, personFromForm(r)); err != nil {
			data.Error = err.Error()
		} else {
			writeAudit(ctx, adminActor(r, ctx), "person.update", "person", strconv.FormatInt(id, 10), "")
			http.Redirect(w, r, fmt.Sprintf("/admin/people/%d?flash=saved", id), http.StatusSeeOther)
			return
		}
	}
	person, err := getPerson(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data.Person = person
	data.Entitlements, _ = listPersonEntitlements(ctx, id)
	data.Courses, _ = listAdminCourses(ctx)
	data.Flash = r.URL.Query().Get("flash")
	renderAdmin(w, r, ctx, "admin/person_detail.tmpl", data)
}

func AdminAddPersonEmail(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if !requireAdmin(w, r, ctx) {
		return
	}
	if !requireAdminDB(w, r, ctx, "Person") {
		return
	}
	id, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	r.ParseForm()
	email := normalizeEmail(r.Form.Get("email"))
	if email != "" {
		if err := addPersonEmail(ctx, id, email, r.Form.Get("primary") == "1"); err == nil {
			writeAudit(ctx, adminActor(r, ctx), "person.email.add", "person", strconv.FormatInt(id, 10), email)
		}
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/people/%d", id), http.StatusSeeOther)
}

func AdminGrantEntitlement(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if !requireAdmin(w, r, ctx) {
		return
	}
	if !requireAdminDB(w, r, ctx, "Person") {
		return
	}
	id, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	r.ParseForm()
	courseSlug := strings.TrimSpace(r.Form.Get("course_slug"))
	notes := strings.TrimSpace(r.Form.Get("notes"))
	if courseSlug != "" {
		entID, err := grantEntitlement(ctx, id, courseSlug, "manual", notes)
		if err == nil {
			writeAudit(ctx, adminActor(r, ctx), "entitlement.grant", "entitlement", strconv.FormatInt(entID, 10), fmt.Sprintf("person=%d course=%s", id, courseSlug))
		}
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/people/%d", id), http.StatusSeeOther)
}

func AdminCourses(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if !requireAdmin(w, r, ctx) {
		return
	}
	if !requireAdminDB(w, r, ctx, "Courses") {
		return
	}
	data := baseAdminPage(ctx, "Courses")
	var err error
	data.Courses, err = listAdminCourses(ctx)
	if err != nil {
		data.Error = err.Error()
	}
	renderAdmin(w, r, ctx, "admin/courses.tmpl", data)
}

func AdminNewCourse(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if !requireAdmin(w, r, ctx) {
		return
	}
	if !requireAdminDB(w, r, ctx, "New Course") {
		return
	}
	data := baseAdminPage(ctx, "New Course")
	if r.Method == http.MethodPost {
		r.ParseForm()
		course := courseFromForm(r)
		if err := createCourse(ctx, course); err != nil {
			data.Error = err.Error()
			data.Course = course
		} else {
			writeAudit(ctx, adminActor(r, ctx), "course.create", "course", course.Slug, "")
			http.Redirect(w, r, "/admin/courses/"+course.Slug+"?flash=created", http.StatusSeeOther)
			return
		}
	}
	renderAdmin(w, r, ctx, "admin/course_form.tmpl", data)
}

func AdminCourseDetail(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if !requireAdmin(w, r, ctx) {
		return
	}
	if !requireAdminDB(w, r, ctx, "Course") {
		return
	}
	slug := mux.Vars(r)["slug"]
	data := baseAdminPage(ctx, "Course")
	if r.Method == http.MethodPost {
		r.ParseForm()
		course := courseFromForm(r)
		course.Slug = slug
		if err := updateCourse(ctx, course); err != nil {
			data.Error = err.Error()
		} else {
			writeAudit(ctx, adminActor(r, ctx), "course.update", "course", slug, "")
			http.Redirect(w, r, "/admin/courses/"+slug+"?flash=saved", http.StatusSeeOther)
			return
		}
	}
	course, err := getAdminCourse(ctx, slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data.Course = course
	data.Entitlements, _ = listCourseEntitlements(ctx, slug)
	data.Flash = r.URL.Query().Get("flash")
	renderAdmin(w, r, ctx, "admin/course_detail.tmpl", data)
}

func AdminRevokeEntitlement(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if !requireAdmin(w, r, ctx) {
		return
	}
	if !requireAdminDB(w, r, ctx, "Admin") {
		return
	}
	id, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	personID, _ := revokeEntitlement(ctx, id)
	writeAudit(ctx, adminActor(r, ctx), "entitlement.revoke", "entitlement", strconv.FormatInt(id, 10), "")
	if personID > 0 {
		http.Redirect(w, r, fmt.Sprintf("/admin/people/%d", personID), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func AdminAudit(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	if !requireAdmin(w, r, ctx) {
		return
	}
	if !requireAdminDB(w, r, ctx, "Audit") {
		return
	}
	data := baseAdminPage(ctx, "Audit")
	data.AuditEvents, _ = listAuditEvents(ctx, 100)
	renderAdmin(w, r, ctx, "admin/audit.tmpl", data)
}

func requireAdmin(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) bool {
	admin := currentAdminFromSession(r, ctx)
	if admin.ID == 0 {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return false
	}
	if r.Method != http.MethodGet && !validateAdminCSRF(w, r, ctx) {
		return false
	}
	return true
}

func requireAdminDB(w http.ResponseWriter, r *http.Request, ctx *config.AppContext, title string) bool {
	if ctx.DB != nil {
		return true
	}
	data := baseAdminPage(ctx, title)
	data.HasDB = false
	data.Error = "Database is not configured. Add DB_DRIVER and DATABASE_URL, then run migrations."
	renderAdmin(w, r, ctx, "admin/index.tmpl", data)
	return false
}

func baseAdminPage(ctx *config.AppContext, title string) AdminPage {
	return AdminPage{
		Page:  getPage(ctx, title, types.FurlCard{}),
		Title: title,
		HasDB: ctx.DB != nil,
	}
}

func renderAdmin(w http.ResponseWriter, r *http.Request, ctx *config.AppContext, tmpl string, data AdminPage) {
	if ctx.DB == nil {
		data.HasDB = false
	}
	data.CurrentAdmin = currentAdminFromSession(r, ctx)
	if data.CSRFField == "" {
		data.CSRFField = template.HTML(fmt.Sprintf(`<input type="hidden" name="csrf_token" value="%s">`, template.HTMLEscapeString(adminCSRFToken(r, ctx))))
	}
	if err := ctx.TemplateCache.ExecuteTemplate(w, tmpl, data); err != nil {
		http.Error(w, "Unable to load admin page", http.StatusInternalServerError)
		ctx.Err.Printf("%s exec failed %s\n", tmpl, err.Error())
	}
}

func adminActor(r *http.Request, ctx *config.AppContext) string {
	admin := currentAdminFromSession(r, ctx)
	if admin.Email != "" {
		return admin.Email
	}
	return ""
}

func currentAdminFromSession(r *http.Request, ctx *config.AppContext) AdminUser {
	if ctx == nil || ctx.Session == nil {
		return AdminUser{}
	}
	id, _ := ctx.Session.Get(r.Context(), "admin_user_id").(int64)
	return AdminUser{
		ID:    id,
		Role:  ctx.Session.GetString(r.Context(), "admin_role"),
		Email: ctx.Session.GetString(r.Context(), "admin_email"),
	}
}

func adminCSRFToken(r *http.Request, ctx *config.AppContext) string {
	token := ctx.Session.GetString(r.Context(), "admin_csrf")
	if token == "" {
		token = randomToken()
		ctx.Session.Put(r.Context(), "admin_csrf", token)
	}
	return token
}

func validateAdminCSRF(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) bool {
	r.ParseForm()
	expected := ctx.Session.GetString(r.Context(), "admin_csrf")
	if expected == "" || r.Form.Get("csrf_token") != expected {
		http.Error(w, "Invalid admin form token", http.StatusForbidden)
		return false
	}
	return true
}

func randomToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

func sendAdminLoginEmail(ctx *config.AppContext, email, link string) error {
	if ctx.Env == nil || ctx.Env.MailEndpoint == "" || ctx.Env.MailDomain == "" {
		return nil
	}
	body := fmt.Sprintf(`<p>Use this link to sign in to Base58 admin:</p><p><a href="%s">%s</a></p><p>This link expires in 30 minutes.</p>`, link, link)
	return emails.ComposeAndSendMail(ctx, &emails.Mail{
		JobKey:   "admin-login-" + randomToken(),
		Email:    email,
		Title:    "Base58 admin login",
		SendAt:   time.Now(),
		HTMLBody: []byte(body),
		TextBody: []byte("Use this link to sign in to Base58 admin: " + link),
	})
}

func personFromForm(r *http.Request) AdminPerson {
	return AdminPerson{
		DisplayName:  strings.TrimSpace(r.Form.Get("display_name")),
		AvatarURL:    strings.TrimSpace(r.Form.Get("avatar_url")),
		XURL:         strings.TrimSpace(r.Form.Get("x_url")),
		InstagramURL: strings.TrimSpace(r.Form.Get("instagram_url")),
		LinkedinURL:  strings.TrimSpace(r.Form.Get("linkedin_url")),
		GithubURL:    strings.TrimSpace(r.Form.Get("github_url")),
		NostrNpub:    strings.TrimSpace(r.Form.Get("nostr_npub")),
		Timezone:     strings.TrimSpace(r.Form.Get("timezone")),
		Status:       defaultString(strings.TrimSpace(r.Form.Get("status")), "active"),
	}
}

func courseFromForm(r *http.Request) AdminCourse {
	return AdminCourse{
		Slug:        strings.TrimSpace(r.Form.Get("slug")),
		Title:       strings.TrimSpace(r.Form.Get("title")),
		Description: strings.TrimSpace(r.Form.Get("description")),
		HeaderImg:   strings.TrimSpace(r.Form.Get("header_img")),
		Status:      defaultString(strings.TrimSpace(r.Form.Get("status")), "draft"),
	}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func adminPostgres(ctx *config.AppContext) bool {
	if ctx == nil || ctx.Env == nil {
		return false
	}
	driver := strings.ToLower(ctx.Env.DBDriver)
	return driver == "postgres" || driver == "postgresql" || driver == "pgx"
}

func ph(ctx *config.AppContext, n int) string {
	if adminPostgres(ctx) {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

func execAdmin(ctx *config.AppContext, query string, args ...any) (sql.Result, error) {
	if ctx.DB == nil {
		return nil, errors.New("database is not configured")
	}
	return ctx.DB.Exec(query, args...)
}

func createAdminLoginToken(ctx *config.AppContext, email string) (string, error) {
	if email == "" {
		return "", errors.New("email is required")
	}
	if !isKnownAdminEmail(ctx, email) && !isBootstrapAdminEmail(ctx, email) {
		return "", errors.New("that email is not configured as an admin")
	}
	token := randomToken()
	expires := time.Now().UTC().Add(30 * time.Minute).Format("2006-01-02 15:04:05")
	_, err := execAdmin(ctx, `INSERT INTO admin_login_tokens (token, email, expires_at) VALUES (`+ph(ctx, 1)+`,`+ph(ctx, 2)+`,`+ph(ctx, 3)+`)`, token, email, expires)
	return token, err
}

func consumeAdminLoginToken(ctx *config.AppContext, token string) (AdminUser, error) {
	var email string
	err := ctx.DB.QueryRow(`SELECT email FROM admin_login_tokens WHERE token=`+ph(ctx, 1)+` AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP`, token).Scan(&email)
	if err != nil {
		return AdminUser{}, errors.New("admin login link is invalid or expired")
	}

	admin, err := adminByEmail(ctx, email)
	if err != nil {
		if !isBootstrapAdminEmail(ctx, email) {
			return AdminUser{}, errors.New("that email is not configured as an admin")
		}
		admin, err = bootstrapAdminUser(ctx, email)
		if err != nil {
			return AdminUser{}, err
		}
	}
	if admin.Status != "active" {
		return AdminUser{}, errors.New("admin account is not active")
	}
	if _, err := execAdmin(ctx, `UPDATE admin_login_tokens SET used_at=CURRENT_TIMESTAMP WHERE token=`+ph(ctx, 1), token); err != nil {
		return AdminUser{}, err
	}
	return admin, nil
}

func isKnownAdminEmail(ctx *config.AppContext, email string) bool {
	_, err := adminByEmail(ctx, email)
	return err == nil
}

func isBootstrapAdminEmail(ctx *config.AppContext, email string) bool {
	if ctx == nil || ctx.Env == nil {
		return false
	}
	for _, candidate := range strings.Split(ctx.Env.AdminEmails, ",") {
		if normalizeEmail(candidate) == email {
			return true
		}
	}
	return false
}

func adminByEmail(ctx *config.AppContext, email string) (AdminUser, error) {
	var admin AdminUser
	err := ctx.DB.QueryRow(`SELECT au.id, au.person_id, au.role, au.status, pe.email, p.display_name
FROM admin_users au
JOIN people p ON p.id = au.person_id
JOIN person_emails pe ON pe.person_id = p.id
WHERE pe.email=`+ph(ctx, 1)+` AND au.status='active'
ORDER BY pe.is_primary DESC LIMIT 1`, email).
		Scan(&admin.ID, &admin.PersonID, &admin.Role, &admin.Status, &admin.Email, &admin.DisplayName)
	return admin, err
}

func bootstrapAdminUser(ctx *config.AppContext, email string) (AdminUser, error) {
	personID, err := ensurePersonForEmail(ctx, email)
	if err != nil {
		return AdminUser{}, err
	}
	var admin AdminUser
	if adminPostgres(ctx) {
		err = ctx.DB.QueryRow(`INSERT INTO admin_users (person_id, role, status) VALUES ($1, 'owner', 'active')
ON CONFLICT (person_id) DO UPDATE SET role='owner', status='active', updated_at=CURRENT_TIMESTAMP
RETURNING id, person_id, role, status`, personID).Scan(&admin.ID, &admin.PersonID, &admin.Role, &admin.Status)
	} else {
		_, err = ctx.DB.Exec(`INSERT INTO admin_users (person_id, role, status) VALUES (?, 'owner', 'active')
ON CONFLICT(person_id) DO UPDATE SET role='owner', status='active', updated_at=CURRENT_TIMESTAMP`, personID)
		if err == nil {
			err = ctx.DB.QueryRow(`SELECT id, person_id, role, status FROM admin_users WHERE person_id=?`, personID).Scan(&admin.ID, &admin.PersonID, &admin.Role, &admin.Status)
		}
	}
	if err != nil {
		return AdminUser{}, err
	}
	admin.Email = email
	return admin, nil
}

func ensurePersonForEmail(ctx *config.AppContext, email string) (int64, error) {
	var id int64
	err := ctx.DB.QueryRow(`SELECT person_id FROM person_emails WHERE email=`+ph(ctx, 1), email).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	name := strings.Split(email, "@")[0]
	return createPerson(ctx, AdminPerson{DisplayName: name, Status: "active"}, email)
}

func createPerson(ctx *config.AppContext, p AdminPerson, email string) (int64, error) {
	if email == "" {
		return 0, errors.New("email is required")
	}
	tx, err := ctx.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var id int64
	if adminPostgres(ctx) {
		err = tx.QueryRow(`INSERT INTO people (display_name, avatar_url, x_url, instagram_url, linkedin_url, github_url, nostr_npub, timezone, status)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
			p.DisplayName, p.AvatarURL, p.XURL, p.InstagramURL, p.LinkedinURL, p.GithubURL, p.NostrNpub, p.Timezone, p.Status).Scan(&id)
	} else {
		res, e := tx.Exec(`INSERT INTO people (display_name, avatar_url, x_url, instagram_url, linkedin_url, github_url, nostr_npub, timezone, status)
VALUES (?,?,?,?,?,?,?,?,?)`,
			p.DisplayName, p.AvatarURL, p.XURL, p.InstagramURL, p.LinkedinURL, p.GithubURL, p.NostrNpub, p.Timezone, p.Status)
		err = e
		if err == nil {
			id, err = res.LastInsertId()
		}
	}
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(`INSERT INTO person_emails (person_id, email, is_primary) VALUES (`+ph(ctx, 1)+`,`+ph(ctx, 2)+`,`+ph(ctx, 3)+`)`, id, email, true); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func updatePerson(ctx *config.AppContext, id int64, p AdminPerson) error {
	_, err := execAdmin(ctx, `UPDATE people SET display_name=`+ph(ctx, 1)+`, avatar_url=`+ph(ctx, 2)+`, x_url=`+ph(ctx, 3)+`, instagram_url=`+ph(ctx, 4)+`, linkedin_url=`+ph(ctx, 5)+`, github_url=`+ph(ctx, 6)+`, nostr_npub=`+ph(ctx, 7)+`, timezone=`+ph(ctx, 8)+`, status=`+ph(ctx, 9)+`, updated_at=CURRENT_TIMESTAMP WHERE id=`+ph(ctx, 10),
		p.DisplayName, p.AvatarURL, p.XURL, p.InstagramURL, p.LinkedinURL, p.GithubURL, p.NostrNpub, p.Timezone, p.Status, id)
	return err
}

func getPerson(ctx *config.AppContext, id int64) (AdminPerson, error) {
	var p AdminPerson
	err := ctx.DB.QueryRow(`SELECT id, display_name, avatar_url, x_url, instagram_url, linkedin_url, github_url, nostr_npub, timezone, status FROM people WHERE id=`+ph(ctx, 1), id).
		Scan(&p.ID, &p.DisplayName, &p.AvatarURL, &p.XURL, &p.InstagramURL, &p.LinkedinURL, &p.GithubURL, &p.NostrNpub, &p.Timezone, &p.Status)
	if err != nil {
		return p, err
	}
	p.Emails, _ = listPersonEmails(ctx, id)
	for _, email := range p.Emails {
		if email.IsPrimary {
			p.PrimaryEmail = email.Email
			break
		}
	}
	return p, nil
}

func listPeople(ctx *config.AppContext, query string) ([]AdminPerson, error) {
	sqlQuery := `SELECT p.id, p.display_name, p.avatar_url, p.x_url, p.instagram_url, p.linkedin_url, p.github_url, p.nostr_npub, p.timezone, p.status, COALESCE(pe.email, '')
FROM people p
LEFT JOIN person_emails pe ON pe.person_id = p.id AND pe.is_primary = `
	if adminPostgres(ctx) {
		sqlQuery += `true`
	} else {
		sqlQuery += `1`
	}
	var args []any
	if query != "" {
		like := "%" + strings.ToLower(query) + "%"
		sqlQuery += ` WHERE lower(p.display_name) LIKE ` + ph(ctx, 1) + ` OR lower(pe.email) LIKE ` + ph(ctx, 2)
		args = append(args, like, like)
	}
	sqlQuery += ` ORDER BY p.id DESC LIMIT 100`
	rows, err := ctx.DB.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var people []AdminPerson
	for rows.Next() {
		var p AdminPerson
		if err := rows.Scan(&p.ID, &p.DisplayName, &p.AvatarURL, &p.XURL, &p.InstagramURL, &p.LinkedinURL, &p.GithubURL, &p.NostrNpub, &p.Timezone, &p.Status, &p.PrimaryEmail); err != nil {
			return nil, err
		}
		people = append(people, p)
	}
	return people, rows.Err()
}

func listPersonEmails(ctx *config.AppContext, id int64) ([]AdminEmail, error) {
	rows, err := ctx.DB.Query(`SELECT id, email, is_primary, COALESCE(CAST(verified_at AS TEXT), '') FROM person_emails WHERE person_id=`+ph(ctx, 1)+` ORDER BY is_primary DESC, email`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var emails []AdminEmail
	for rows.Next() {
		var e AdminEmail
		if err := rows.Scan(&e.ID, &e.Email, &e.IsPrimary, &e.Verified); err != nil {
			return nil, err
		}
		emails = append(emails, e)
	}
	return emails, rows.Err()
}

func addPersonEmail(ctx *config.AppContext, personID int64, email string, primary bool) error {
	if email == "" {
		return errors.New("email is required")
	}
	tx, err := ctx.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if primary {
		if _, err := tx.Exec(`UPDATE person_emails SET is_primary=`+ph(ctx, 1)+` WHERE person_id=`+ph(ctx, 2), false, personID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT INTO person_emails (person_id, email, is_primary) VALUES (`+ph(ctx, 1)+`,`+ph(ctx, 2)+`,`+ph(ctx, 3)+`)`, personID, email, primary); err != nil {
		return err
	}
	return tx.Commit()
}

func createCourse(ctx *config.AppContext, c AdminCourse) error {
	if c.Slug == "" || c.Title == "" {
		return errors.New("slug and title are required")
	}
	_, err := execAdmin(ctx, `INSERT INTO courses (slug, title, description, header_img, status) VALUES (`+ph(ctx, 1)+`,`+ph(ctx, 2)+`,`+ph(ctx, 3)+`,`+ph(ctx, 4)+`,`+ph(ctx, 5)+`)`,
		c.Slug, c.Title, c.Description, c.HeaderImg, c.Status)
	return err
}

func updateCourse(ctx *config.AppContext, c AdminCourse) error {
	_, err := execAdmin(ctx, `UPDATE courses SET title=`+ph(ctx, 1)+`, description=`+ph(ctx, 2)+`, header_img=`+ph(ctx, 3)+`, status=`+ph(ctx, 4)+`, updated_at=CURRENT_TIMESTAMP WHERE slug=`+ph(ctx, 5),
		c.Title, c.Description, c.HeaderImg, c.Status, c.Slug)
	return err
}

func getAdminCourse(ctx *config.AppContext, slug string) (AdminCourse, error) {
	var c AdminCourse
	err := ctx.DB.QueryRow(`SELECT slug, title, description, header_img, status FROM courses WHERE slug=`+ph(ctx, 1), slug).
		Scan(&c.Slug, &c.Title, &c.Description, &c.HeaderImg, &c.Status)
	return c, err
}

func listAdminCourses(ctx *config.AppContext) ([]AdminCourse, error) {
	rows, err := ctx.DB.Query(`SELECT slug, title, description, header_img, status FROM courses ORDER BY title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var courses []AdminCourse
	for rows.Next() {
		var c AdminCourse
		if err := rows.Scan(&c.Slug, &c.Title, &c.Description, &c.HeaderImg, &c.Status); err != nil {
			return nil, err
		}
		courses = append(courses, c)
	}
	return courses, rows.Err()
}

func grantEntitlement(ctx *config.AppContext, personID int64, courseSlug, source, notes string) (int64, error) {
	if adminPostgres(ctx) {
		var id int64
		err := ctx.DB.QueryRow(`INSERT INTO course_entitlements (person_id, course_slug, source, notes, status) VALUES ($1,$2,$3,$4,'active')
ON CONFLICT (person_id, course_slug, source, external_source_id) DO UPDATE SET status='active', revoked_at=NULL, notes=EXCLUDED.notes RETURNING id`,
			personID, courseSlug, source, notes).Scan(&id)
		return id, err
	}
	res, err := ctx.DB.Exec(`INSERT INTO course_entitlements (person_id, course_slug, source, notes, status) VALUES (?,?,?,?, 'active')
ON CONFLICT(person_id, course_slug, source, external_source_id) DO UPDATE SET status='active', revoked_at=NULL, notes=excluded.notes`,
		personID, courseSlug, source, notes)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		ctx.DB.QueryRow(`SELECT id FROM course_entitlements WHERE person_id=? AND course_slug=? AND source=? AND external_source_id=''`, personID, courseSlug, source).Scan(&id)
	}
	return id, nil
}

func revokeEntitlement(ctx *config.AppContext, id int64) (int64, error) {
	var personID int64
	ctx.DB.QueryRow(`SELECT person_id FROM course_entitlements WHERE id=`+ph(ctx, 1), id).Scan(&personID)
	_, err := execAdmin(ctx, `UPDATE course_entitlements SET status='revoked', revoked_at=CURRENT_TIMESTAMP WHERE id=`+ph(ctx, 1), id)
	return personID, err
}

func listPersonEntitlements(ctx *config.AppContext, personID int64) ([]AdminEntitlement, error) {
	return listEntitlements(ctx, `WHERE ce.person_id=`+ph(ctx, 1), personID)
}

func listCourseEntitlements(ctx *config.AppContext, slug string) ([]AdminEntitlement, error) {
	return listEntitlements(ctx, `WHERE ce.course_slug=`+ph(ctx, 1), slug)
}

func listEntitlements(ctx *config.AppContext, where string, args ...any) ([]AdminEntitlement, error) {
	rows, err := ctx.DB.Query(`SELECT ce.id, ce.person_id, ce.course_slug, c.title, COALESCE(pe.email, ''), ce.status, ce.source,
COALESCE(CAST(ce.starts_at AS TEXT), ''), COALESCE(CAST(ce.expires_at AS TEXT), ''), CAST(ce.granted_at AS TEXT), COALESCE(CAST(ce.revoked_at AS TEXT), ''), ce.notes
FROM course_entitlements ce
JOIN courses c ON c.slug = ce.course_slug
JOIN people p ON p.id = ce.person_id
LEFT JOIN person_emails pe ON pe.person_id = p.id AND pe.is_primary = `+primarySQL(ctx)+`
`+where+` ORDER BY ce.granted_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ents []AdminEntitlement
	for rows.Next() {
		var e AdminEntitlement
		if err := rows.Scan(&e.ID, &e.PersonID, &e.CourseSlug, &e.CourseTitle, &e.Email, &e.Status, &e.Source, &e.StartsAt, &e.ExpiresAt, &e.GrantedAt, &e.RevokedAt, &e.Notes); err != nil {
			return nil, err
		}
		ents = append(ents, e)
	}
	return ents, rows.Err()
}

func primarySQL(ctx *config.AppContext) string {
	if adminPostgres(ctx) {
		return "true"
	}
	return "1"
}

func writeAudit(ctx *config.AppContext, actor, action, targetType, targetID, details string) {
	if ctx.DB == nil {
		return
	}
	_, _ = ctx.DB.Exec(`INSERT INTO admin_audit_events (actor, action, target_type, target_id, details) VALUES (`+ph(ctx, 1)+`,`+ph(ctx, 2)+`,`+ph(ctx, 3)+`,`+ph(ctx, 4)+`,`+ph(ctx, 5)+`)`,
		actor, action, targetType, targetID, details)
}

func listAuditEvents(ctx *config.AppContext, limit int) ([]AdminAuditEvent, error) {
	rows, err := ctx.DB.Query(`SELECT id, actor, action, target_type, target_id, details, CAST(created_at AS TEXT) FROM admin_audit_events ORDER BY created_at DESC LIMIT `+ph(ctx, 1), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []AdminAuditEvent
	for rows.Next() {
		var event AdminAuditEvent
		if err := rows.Scan(&event.ID, &event.Actor, &event.Action, &event.TargetType, &event.TargetID, &event.Details, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
