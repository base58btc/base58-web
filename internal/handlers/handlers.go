package handlers

import (
	"fmt"
	"html/template"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
	"github.com/kodylow/base58-website/external/getters"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/emails"
	"github.com/kodylow/base58-website/internal/helpers"
	"github.com/kodylow/base58-website/internal/types"
	"io/ioutil"
)

var webpages []string = []string{"about", "courses", "404", "401", "contact", "index"}

/* Thank you StackOverflow https://stackoverflow.com/a/50581032 */
func findAndParseTemplates(rootDir string, funcMap template.FuncMap) (*template.Template, error) {
	cleanRoot := filepath.Clean(rootDir)
	pfx := len(cleanRoot) + 1
	root := template.New("")

	err := filepath.Walk(cleanRoot, func(path string, info os.FileInfo, e1 error) error {
		if !info.IsDir() && strings.HasSuffix(path, ".tmpl") {
			if e1 != nil {
				return e1
			}

			b, e2 := ioutil.ReadFile(path)
			if e2 != nil {
				return e2
			}

			name := path[pfx:]
			t := root.New(name).Funcs(funcMap)
			_, e2 = t.Parse(string(b))
			if e2 != nil {
				return e2
			}
		}

		return nil
	})

	return root, err
}

func BuildTemplateCache(ctx *config.AppContext) error {

	var err error
	funcMap := template.FuncMap{
		"LastIdx":     LastIdx,
		"AvailOnline": AvailOnline,
		"toHTML": func(s string) template.HTML {
			b := helpers.ConvertMdToHTML(ctx, "prereq", s)
			return template.HTML(string(b))
		},
		"toText": func(s string) template.HTML {
			b := helpers.ConvertMdToHTML(ctx, "text", s)
			return template.HTML(string(b))
		},
		"dashboardProgressLabel": dashboardProgressLabel,
		"dashboardInitials":      dashboardInitials,
		"ishtml": func(s string) template.HTML {
			return template.HTML(s)
		},
	}
	ctx.TemplateCache, err = findAndParseTemplates("templates", funcMap)
	return err
}

func Routes(ctx *config.AppContext) (http.Handler, error) {
	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handle404(w, r, ctx)
	})

	err := BuildTemplateCache(ctx)
	if err != nil {
		return r, err
	}

	RegisterAdminRoutes(r, ctx)
	registerRetiredRoutes(r, ctx)

	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		RenderPage(w, r, ctx, "index")
	}).Methods("GET")
	r.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		Dashboard(w, r, ctx)
	}).Methods("GET")
	r.HandleFunc("/dashboard/courses/{course}/reset", func(w http.ResponseWriter, r *http.Request) {
		DashboardResetCourse(w, r, ctx)
	}).Methods("POST")
	r.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		StudentLogin(w, r, ctx)
	}).Methods("GET", "POST")
	r.HandleFunc("/login/{token}", func(w http.ResponseWriter, r *http.Request) {
		StudentAuthToken(w, r, ctx)
	}).Methods("GET")
	r.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		StudentLogout(w, r, ctx)
	}).Methods("GET", "POST")

	/* List of 'normie' pages */
	for _, page := range webpages {
		/* Normie Pages */
		renderPage := page
		r.HandleFunc("/"+renderPage, func(w http.ResponseWriter, r *http.Request) {
			RenderPage(w, r, ctx, renderPage)
		}).Methods("GET")
	}

	r.HandleFunc("/courses/larp", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://workshop.base58.school", http.StatusMovedPermanently)
	})
	r.HandleFunc("/workshop", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://workshop.base58.school", http.StatusMovedPermanently)
	}).Methods("GET")
	r.HandleFunc("/workshop/book", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://workshop.base58.school", http.StatusMovedPermanently)
	}).Methods("GET")
	r.HandleFunc("/workshop/become", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://workshop.base58.school/training", http.StatusFound)
	}).Methods("GET")
	r.HandleFunc("/courses/{course}/{page:.*}", func(w http.ResponseWriter, r *http.Request) {
		CourseLesson(w, r, ctx)
	}).Methods("GET")
	r.HandleFunc("/courses/{course}/versions/latest/start", func(w http.ResponseWriter, r *http.Request) {
		StartLatestCourseVersion(w, r, ctx)
	}).Methods("POST")
	r.HandleFunc("/courses/{course}/publish-draft", func(w http.ResponseWriter, r *http.Request) {
		PublishCourseDraft(w, r, ctx)
	}).Methods("POST")
	r.HandleFunc("/api/courses/{course}/progress", func(w http.ResponseWriter, r *http.Request) {
		CourseProgress(w, r, ctx)
	}).Methods("GET", "POST")
	r.HandleFunc("/api/courses/{course}/environment", func(w http.ResponseWriter, r *http.Request) {
		CourseBrowserEnvironment(w, r, ctx)
	}).Methods("GET")
	r.HandleFunc("/api/courses/{course}/commands", func(w http.ResponseWriter, r *http.Request) {
		CourseCommands(w, r, ctx)
	}).Methods("GET", "POST")
	r.HandleFunc("/api/courses/{course}/commands/{resource:lease|chunks|checkpoints}/{key:[A-Za-z0-9_-]+}", func(w http.ResponseWriter, r *http.Request) {
		CourseCommands(w, r, ctx)
	}).Methods("GET", "PUT", "POST", "DELETE")
	r.HandleFunc("/api/editor/courses/{course}/sync", func(w http.ResponseWriter, r *http.Request) {
		CourseEditorSync(w, r, ctx)
	}).Methods("POST")
	r.HandleFunc("/api/editor/courses/{course}/publish", func(w http.ResponseWriter, r *http.Request) {
		CourseEditorPublish(w, r, ctx)
	}).Methods("POST")

	r.HandleFunc("/courses/{course}/signup", func(w http.ResponseWriter, r *http.Request) {
		LocalCourseSignup(w, r, ctx)
	}).Methods("POST")
	r.HandleFunc("/courses/{course}", func(w http.ResponseWriter, r *http.Request) {
		Courses(w, r, ctx)
	}).Methods("GET")

	for _, page := range types.TextPages {
		renderPage := page
		r.HandleFunc("/"+renderPage.Tag, func(w http.ResponseWriter, r *http.Request) {
			RenderTextPage(w, r, ctx, renderPage)
		}).Methods("GET")
	}

	r.HandleFunc("/workshop/contact/{formtype}", func(w http.ResponseWriter, r *http.Request) {
		WorkshopContact(w, r, ctx)
	}).Methods("POST")
	r.HandleFunc("/contact/ok", func(w http.ResponseWriter, r *http.Request) {
		FormContact(w, r, ctx)
	}).Methods("POST")

	r.HandleFunc("/tools/wif", func(w http.ResponseWriter, r *http.Request) {
		WIF(w, r, ctx)
	})

	r.HandleFunc("/tools/keyaddr", func(w http.ResponseWriter, r *http.Request) {
		KeyAddr(w, r, ctx)
	})

	/* serve files from the "static" directory */
	fs := http.FileServer(http.Dir("static"))
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", fs))

	/* special favicon handling */
	err = AddFaviconRoutes(r)

	if err != nil {
		return r, err
	}

	return r, nil
}

func handle404(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	w.WriteHeader(http.StatusNotFound)
	RenderPage(w, r, ctx, "404")
	ctx.Err.Printf("404 err: %s", r.URL.Path)
}

func AvailOnline(avail []types.CourseAvail) bool {
	for _, a := range avail {
		if a.SelfPacedOnline() {
			return true
		}
	}
	return false
}

func LastIdx(size int) int {
	return size - 1
}

func FormContact(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	r.ParseForm()
	email := r.Form.Get("contact-email")
	message := r.Form.Get("contact-message")

	/* Validate email */
	if _, err := mail.ParseAddress(email); err != nil {
		w.Write([]byte(fmt.Sprintf(`
		<div class="error-message w-form-fail" style="display: block;">
                  <div class="error-text">"%s" is not a valid email.</div>
                </div>
		`, email)))
		return
	}

	/* Send hello@base58.school + the email the message */
	_, err := emails.SendContactEmail(ctx, "hello@base58.school", message, email, "contact")
	if err != nil {
		ctx.Err.Printf("Failed sending self message: %s", err)
		if strings.Contains(err.Error(), "scheduled.idem_key") {
			w.Write([]byte(`<div class="error-message w-form-fail" style="display: block;">
			  <div class="error-text">That message has already been sent!</div>
			</div>`))
		} else {
			w.Write([]byte(`<div class="error-message w-form-fail" style="display: block;">
			  <div class="error-text">Server error. Please try again later.</div>
			</div>`))
		}
		return
	}

	_, err = emails.SendContactEmail(ctx, email, message, email, "contact")
	if err != nil {
		ctx.Err.Printf("Failed sending %s message: %s", email, err)
		w.Write([]byte(`<div class="error-message w-form-fail" style="display: block;">
                  <div class="error-text">Unable to send mail. Please try again later.</div>
                </div>`))
		return
	}

	ctx.Infos.Printf("Message to get in touch sent!")

	/* show a banner about it sending successfully */
	w.Write([]byte(`<div class="success-message w-form-done" style="display: block;">
	  <div class="success-text">Thank you! Your message has been received!</div>
	</div>`))
}
func WorkshopContact(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	params := mux.Vars(r)
	formtype := params["formtype"]

	r.ParseForm()
	email := r.Form.Get("workshop-email")
	message := r.Form.Get("workshop-message")
	/* Validate email */
	if _, err := mail.ParseAddress(email); err != nil {
		w.Write([]byte(fmt.Sprintf(`
		<div class="error-message w-form-fail" style="display: block;">
                  <div class="error-text">"%s" is not a valid email.</div>
                </div>
		`, email)))
		return
	}

	/* Send hello@base58.school + the email the message */
	_, err := emails.SendContactEmail(ctx, "hello@base58.school", message, email, formtype)
	if err != nil {
		ctx.Err.Printf("Failed sending self message: %s", err)
		if strings.Contains(err.Error(), "scheduled.idem_key") {
			w.Write([]byte(`<div class="error-message w-form-fail" style="display: block;">
			  <div class="error-text">That message has already been sent!</div>
			</div>`))
		} else {
			w.Write([]byte(`<div class="error-message w-form-fail" style="display: block;">
			  <div class="error-text">Server error. Please try again later.</div>
			</div>`))
		}
		return
	}

	_, err = emails.SendContactEmail(ctx, email, message, email, formtype)
	if err != nil {
		ctx.Err.Printf("Failed sending %s message: %s", email, err)
		w.Write([]byte(`<div class="error-message w-form-fail" style="display: block;">
                  <div class="error-text">Unable to send mail. Please try again later.</div>
                </div>`))
		return
	}

	ctx.Infos.Printf("Message to %s Work(shop) sent!", formtype)

	/* show a banner about it sending successfully */
	w.Write([]byte(`<div class="success-message w-form-done" style="display: block;">
	  <div class="success-text">Thank you! Your message has been received!</div>
	</div>`))
}

type LoginPage struct {
	Page        Page
	Destination string
}

func Render401(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	err := ctx.TemplateCache.ExecuteTemplate(w, "401.tmpl", &LoginPage{
		Page:        getPage(ctx, "Login", types.FurlCard{}),
		Destination: r.URL.Path,
	})
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/401.tmpl exec template failed %s\n", err.Error())
		return
	}
}

/* Login redirects to the admin email login flow. */
func Login(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func RenderPage(w http.ResponseWriter, r *http.Request, ctx *config.AppContext, page string) {
	data, err := getHomeData(ctx, ctx.Notion)
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/%s data fetch failed %s\n", page, err.Error())
		return
	}

	// Render the template with the data
	template := fmt.Sprintf("%s.tmpl", page)
	err = ctx.TemplateCache.ExecuteTemplate(w, template, data)
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/%s exec template failed %s\n", template, err.Error())
		return
	}
}

type ContentPage struct {
	Page Page
	Text string
}

func RenderTextPage(w http.ResponseWriter, r *http.Request, ctx *config.AppContext, tPage types.TextPage) {

	/* Read page from disk (if not in cache) */
	pageBytes, err := ioutil.ReadFile(fmt.Sprintf("templates/text/%s.md", tPage.Tag))
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to load text template templates/text/%s.md", tPage.Tag), http.StatusInternalServerError)
		ctx.Err.Printf("Unable to load text template %s.md %s\n", tPage.Tag, err.Error())
		return
	}

	// Render the template with the data
	furlCard := defaultCard(ctx, r, tPage.Title)
	err = ctx.TemplateCache.ExecuteTemplate(w, "text/text.tmpl", &ContentPage{
		Text: string(pageBytes),
		Page: getPage(ctx, tPage.Title, furlCard),
	})
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("text.tmpl exec failed %s\n", err.Error())
		return
	}
}

type CourseData struct {
	Course       *types.Course
	Sessions     []*types.CourseSession
	SeatsAvail   uint
	Page         Page
	LocalCourse  bool
	LoggedIn     bool
	Enrolled     bool
	AccessError  string
	Notice       string
	StartURL     string
	ResumeURL    string
	LocalOutline []*CourseNavItem
	CSRFField    template.HTML
}

type sessionList []*types.CourseSession

func (s sessionList) Len() int {
	return len(s)
}

func (s sessionList) Less(i, j int) bool {

	if len(s[i].Date) == 0 {
		return true
	}
	if len(s[j].Date) == 0 {
		return false
	}

	/* Sort by time first */
	return s[i].Dates()[0].Before(s[j].Dates()[0])
}

func (s sessionList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func countSeats(s sessionList) uint {
	var acc uint
	for _, sesh := range s {
		acc += sesh.SeatsAvail
	}

	return acc
}

func Courses(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	/* If there's no class-key, redirect to the front page */
	params := mux.Vars(r)
	clss := params["course"]

	if clss == "" {
		/* Return the courses page */
		RenderPage(w, r, ctx, "courses")
		return
	}

	if canonical, ok := canonicalLocalCourseSlug(clss); ok {
		if canonical != clss {
			http.Redirect(w, r, "/courses/"+canonical, http.StatusMovedPermanently)
			return
		}
		LocalCourseLanding(w, r, ctx, canonical)
		return
	}

	course, err := getters.GetCourse(ctx.Notion, clss)
	if err != nil {
		http.Error(w, "Unable to load page, course not found", http.StatusInternalServerError)
		ctx.Err.Printf("/courses unable to find course %s\n", err.Error())
		return
	}

	var sessions sessionList
	sessions, err = getters.GetCourseSessions(ctx.Notion, course)
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/courses course sessions fetch failed %s\n", err.Error())
		return
	}

	/* Filter out anything in the past or happening in the next 1hr */
	sessions = helpers.FilterSessions(sessions, time.Now())

	/* Sort sessions by date, soonest first */
	sort.Sort(sessions)

	extraData := make([]ExtraData, 0)
	if len(sessions) > 0 {
		extraData = append(extraData, ExtraData{
			Label: "Next Session Starts",
			Data:  sessions[0].FmtDates()[0],
		})
		extraData = append(extraData, ExtraData{
			Label: "Seats Left",
			Data:  strconv.FormatUint(uint64(sessions[0].SeatsAvail), 10),
		})
	}

	furlCard := courseCardWithExtras(ctx, r, course, extraData)

	err = ctx.TemplateCache.ExecuteTemplate(w, "courses/course.tmpl", CourseData{
		Course:     course,
		SeatsAvail: countSeats(sessions),
		Sessions:   sessions,
		Page:       getPage(ctx, course.Title, furlCard),
	})

	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/courses exec templ failed %s\n", err.Error())
		return
	}
}

func Styles(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/css")
	http.ServeFile(w, r, "static/css/styles.css")
}

// PageData is a struct that holds the data for a page
type Page struct {
	Title     string
	Copyright int
	Domain    string
	Callbacks string
	Card      types.FurlCard
	Year      uint
}

type pageData struct {
	Page    Page
	Courses []*types.Course
	Current []*types.Course
	Coming  []*types.Course
}

type ExtraData struct {
	Label string
	Data  string
}

func defaultCard(ctx *config.AppContext, r *http.Request, title string) types.FurlCard {
	return buildCard(ctx.Env.Domain, title, r.URL.String(), "default_card.png", "", nil)
}

func courseCard(ctx *config.AppContext, r *http.Request, course *types.Course) types.FurlCard {
	return courseCardWithExtras(ctx, r, course, nil)
}

func courseCardWithExtras(ctx *config.AppContext, r *http.Request, course *types.Course, extras []ExtraData) types.FurlCard {

	domain := ctx.Env.Domain
	imgName := course.FurlImg()
	imgURL := fmt.Sprintf("https://%s/static/img/%s", domain, imgName)

	return buildCard(domain, course.Title, r.URL.String(), imgURL, course.Flavor, extras)
}

func buildCard(domain, title, URL, imgName, desc string, extraData []ExtraData) types.FurlCard {
	card := types.FurlCard{
		URL:         URL, /* Of the page we're on */
		Domain:      domain,
		Title:       title,
		Description: desc,
		ImageURL:    fmt.Sprintf("https://%s/static/img/%s", domain, imgName),
	}

	for i, extra := range extraData {
		if i == 0 {
			card.ExtraOneLabel = extra.Label
			card.ExtraOneData = extra.Data
		}
		if i == 1 {
			card.ExtraTwoLabel = extra.Label
			card.ExtraTwoData = extra.Data
		}
	}

	return card
}

func getPage(ctx *config.AppContext, title string, card types.FurlCard) Page {
	return Page{
		Title:     title,
		Copyright: time.Now().Year(),
		Domain:    ctx.SitePath(),
		Callbacks: ctx.CallbackPath(),
		Card:      card,
	}
}

func getHomeData(ctx *config.AppContext, n *types.Notion) (pageData, error) {
	courses, err := getters.ListCourses(n)

	/* Sort courses by Popularity */
	sort.Slice(courses, func(i, j int) bool {
		return courses[i].Popularity < courses[j].Popularity
	})

	if err != nil {
		return pageData{}, err
	}
	return pageData{
		Page:    getPage(ctx, "", types.FurlCard{}),
		Courses: courses,
	}, nil
}

/* these two functions make all the assets in /favicons as accessible */
func getFaviconHandler(name string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, fmt.Sprintf("static/favicons/%s", name))
	}
}

func AddFaviconRoutes(r *mux.Router) error {
	files, err := ioutil.ReadDir("static/favicons/")
	if err != nil {
		return err
	}

	/* If asked for a favicon, we'll serve it up */
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		r.HandleFunc(fmt.Sprintf("/%s", file.Name()), getFaviconHandler(file.Name())).Methods("GET")
	}

	return nil
}

type KeyAddrForm struct {
	Value string
}

type KeyAddrData struct {
	Value  string
	Addr   string
	ErrMsg string
	Page   Page
}

func KeyAddr(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	/* Show a address from scriptPubkey page! */
	title := "Address Calculator"
	furlCard := defaultCard(ctx, r, title)

	var data KeyAddrData
	var err error
	if r.Method == http.MethodPost {
		r.ParseForm()
		dec := schema.NewDecoder()
		var form KeyAddrForm
		err = dec.Decode(&form, r.PostForm)
		if err != nil {
			http.Error(w, "Unable to load page, please try again later", http.StatusInternalServerError)
			ctx.Err.Printf("/tools/keyaddr unable to decode wif value %s\n", err.Error())
			return
		}

		/* Note: Currently only support mainnet */
		/* Option values: bc, tb (test+sig), bcrt (reg) */
		data.Addr, err = MakeAddr(form.Value, "bc")
		if err != nil {
			data.ErrMsg = err.Error()
		}
		data.Value = form.Value
	}

	data.Page = getPage(ctx, title, furlCard)
	err = ctx.TemplateCache.ExecuteTemplate(w, "tools/keyaddr.tmpl", &data)
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/tools/keyaddr execute template failed %s\n", err.Error())
	}
}

type WIFForm struct {
	Value string
}

type WIFData struct {
	Value  string
	WIF    string
	ErrMsg string
	Page   Page
}

func WIF(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	/* Show a WIF calc page! */

	title := "WIF Calculator"
	furlCard := defaultCard(ctx, r, title)

	var data WIFData
	var err error
	if r.Method == http.MethodPost {
		r.ParseForm()
		dec := schema.NewDecoder()
		var form WIFForm
		err = dec.Decode(&form, r.PostForm)
		if err != nil {
			http.Error(w, "Unable to load page, please try again later", http.StatusInternalServerError)
			ctx.Err.Printf("/tools/wif unable to decode wif value %s\n", err.Error())
			return
		}

		/* Note: Currently only support testnet/regtest */
		data.WIF, err = MakeWIF(form.Value, DEFAULT_NETWORK_BYTE)
		if err != nil {
			data.ErrMsg = err.Error()
		}
		data.Value = form.Value
	}

	data.Page = getPage(ctx, title, furlCard)
	err = ctx.TemplateCache.ExecuteTemplate(w, "tools/wif.tmpl", &data)
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/tools/wif execute template failed %s\n", err.Error())
	}
}
