package handlers

import (
	"fmt"
	"html/template"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/kodylow/base58-website/external/getters"
	"github.com/kodylow/base58-website/internal/types"
	"github.com/kodylow/base58-website/static"
	"io/ioutil"
	"github.com/gorilla/schema"
	"github.com/joncalhoun/form"
)

// FIXME: how to have select box // radio // dropdowns ?
type ClassRegistration struct {
	Email string `form:"label=Email;type=email;placeholder=hello@example.com"`
	MailngAddr *string `form:"label=Mailing Address;placeholder=555 Magneto Way, Oxford, UK 282822"`
	Shirt *types.ShirtSize `form:"label=Shirt size, unisex;type=select;id=shirt;placeholder=med"`
	ReplitUser string
	Idempotency string `form:"label=nil;type=hidden"`
}

type WaitList struct {
	Email string `form:"label=Email;type=email;placeholder=hello@example.com"`
	Idempotency string `form:"label=nil;type=hidden"`
	SessionUUID string `form:"label=nil;type=hidden"`
}

type OptionItem struct {
	Key string
	Value string
}

func ShirtOptions() []OptionItem {
	return []OptionItem{
		{Key: string(types.Small), Value: "Small"},
		{Key: string(types.Med), Value: "Medium"},
		{Key: string(types.Large), Value: "Large"},
		{Key: string(types.XL), Value: "XL"},
		{Key: string(types.XXL), Value: "XXL"},}
}

type RegistrationData struct {
	Course *types.Course
	Session *types.CourseSession
	Form	ClassRegistration
}

type WaitlistData struct {
	Course *types.Course
	Session *types.CourseSession
	Form	WaitList
}

func getSessionKey(p string, r *http.Request) (string, bool) {
	ok := r.URL.Query().Has(p)
	key := r.URL.Query().Get(p)
	return key, ok
}

func Register(w http.ResponseWriter, r *http.Request, env *types.EnvConfig) {
	n := &types.Notion{Config: env.Notion}
	n.Setup()

	switch r.Method {
	case http.MethodGet:
		sessionID, ok := getSessionKey("s", r)
		if !ok {
			/* If there's no session-key, redirect to the front page */
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		course, session, err := getters.GetSessionInfo(n, sessionID)
		if err != nil {
			log.Fatal(w, err.Error(), http.StatusInternalServerError)
			return
		}
		f, err := ioutil.ReadFile("templates/forms/inputs.tmpl")
		if err != nil {
			log.Fatal(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tpl := template.Must(template.New("").Funcs(
			template.FuncMap{
				"fn_options": func (id string) []OptionItem {
					if id == "shirt" {
						return ShirtOptions()
					}
					return []OptionItem{}
				},
			}).Parse(string(f)))
		fb := form.Builder{
			InputTemplate: tpl,
		}

		f, err = ioutil.ReadFile("templates/register.tmpl")
		if err != nil {
			log.Fatal(w, err.Error(), http.StatusInternalServerError)
			return
		}
		funcMap := fb.FuncMap()
		funcMap["LastIdx"] = func (size int) int { return size - 1}
		funcMap["FiatPrice"] = func (val uint64) uint64 { return val }
		funcMap["BtcPrice"] = func (val uint64) uint64 { return uint64(float64(val) * .85) }
		pageTpl := template.Must(template.New("").Funcs(funcMap).Parse(string(f)))
		w.Header().Set("Content-Type", "text/html")
		err = pageTpl.Execute(w, RegistrationData{
			Course: course,
			Session: session,
			Form: ClassRegistration{
				// FIXME: pick randomly? use a timestamp?
				Idempotency: "akjbaisisisbneka",
		}})
		if err != nil {
			log.Fatal(w, err.Error(), http.StatusInternalServerError)
		}
		return
	case http.MethodPost:
		/* Goes to the bottom! */
	default:
		http.NotFound(w, r)
		return
	}

	r.ParseForm()
	dec := schema.NewDecoder()
	dec.IgnoreUnknownKeys(true)
	var form ClassRegistration
	err := dec.Decode(&form, r.PostForm)
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusBadRequest)
		return
	}

	/* TODO: Write to notion?
	 *  - update avail class seats (-1)
	 *  - add entry to class signups table
	 */
	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(form)
	fmt.Println(string(b))
	w.Write(b)
}

func Waitlist(w http.ResponseWriter, r *http.Request, env *types.EnvConfig) {
	n := &types.Notion{Config: env.Notion}
	n.Setup()

	sessionID, ok := getSessionKey("s", r)
	if !ok {
		/* If there's no session-key, redirect to the front page */
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	switch r.Method {
	case http.MethodGet:
		course, session, err := getters.GetSessionInfo(n, sessionID)
		if err != nil {
			log.Fatal(w, err.Error(), http.StatusInternalServerError)
			return
		}

		f, err := ioutil.ReadFile("templates/forms/inputs.tmpl")
		if err != nil {
			log.Fatal(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tpl := template.Must(template.New("").Funcs(
			template.FuncMap{
				"fn_options": func (id string) []OptionItem {
					return []OptionItem{}
				},
			}).Parse(string(f)))
		fb := form.Builder{ InputTemplate: tpl, }

		f, err = ioutil.ReadFile("templates/waitlist.tmpl")
		if err != nil {
			log.Fatal(w, err.Error(), http.StatusInternalServerError)
			return
		}
		funcMap := fb.FuncMap()
		funcMap["LastIdx"] = func (size int) int { return size - 1}
		funcMap["FiatPrice"] = func (val uint64) uint64 { return val }
		funcMap["BtcPrice"] = func (val uint64) uint64 { return uint64(float64(val) * .85) }
		pageTpl := template.Must(template.New("").Funcs(funcMap).Parse(string(f)))
		w.Header().Set("Content-Type", "text/html")
		err = pageTpl.Execute(w, WaitlistData{
			Course: course,
			Session: session,
			Form: WaitList{
				// FIXME: hash of timestamp + sessionid
				Idempotency: "akjbaisisisbneka",
				SessionUUID: session.ID,
		}})
		if err != nil {
			log.Fatal(w, err.Error(), http.StatusInternalServerError)
		}
		return
	case http.MethodPost:
		/* Goes to the bottom! */
	default:
		http.NotFound(w, r)
		return
	}

	// FIXME: check idempotency! (our timestamp + UUID hash?)
	r.ParseForm()
	dec := schema.NewDecoder()
	dec.IgnoreUnknownKeys(true)
	var form WaitList
	err := dec.Decode(&form, r.PostForm)
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusBadRequest)
		return
	}

	/* Save to waitlist! */
	err = getters.SaveWaitlist(n, form.SessionUUID, form.Email)
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusBadRequest)
		return
	}
	// TODO: show waitlist success?
	// TODO: send confirmation email!
	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(form)
	w.Write(b)
}

func Home(w http.ResponseWriter, r *http.Request, env *types.EnvConfig) {
	// Parse the template file
	tmpl, err := template.ParseFiles("templates/index.tmpl")
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusInternalServerError)
		return
	}

	n := &types.Notion{Config: env.Notion}
	n.Setup()

	// Define the data to be rendered in the template
	data, err := getHomeData(n)
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Render the template with the data
	err = tmpl.Execute(w, data)
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

type CourseData struct {
	Course   *types.Course
	Sessions []*types.CourseSession
}

func Courses(w http.ResponseWriter, r *http.Request, env *types.EnvConfig) {
	n := &types.Notion{Config: env.Notion}
	n.Setup()

	/* If there's no class-key, redirect to the front page */
	hasK := r.URL.Query().Has("k")
	if !hasK {
		/* redirect to "/". A lot of hard coded links exist pointing
		 * at "/classes", so we redirect! */
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	k := r.URL.Query().Get("k")
	courses, err := getters.ListCourses(n)
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, course := range courses {
		if k == course.TmplName {
			/* FIXME: put course page data into notion? */
			f, err := ioutil.ReadFile("templates/course.tmpl")
			if err != nil {
				log.Fatal(w, err.Error(), http.StatusInternalServerError)
				return
			}
			t, err := template.New("course").Funcs(template.FuncMap{
				"LastIdx" : func (size int) int { return size - 1 },
			}).Parse(string(f))
			if err != nil {
				log.Fatal(w, err.Error(), http.StatusInternalServerError)
				return
			}

			/* FIXME: generalize? */
			var bundled []*types.Course
			if k == "tx-deep-dive" {
				bundled = append(bundled, course)
				for _, c := range courses {
					if strings.HasPrefix(c.TmplName, course.TmplName) {
						bundled = append(bundled, c)
					}
				}
			} else {
				bundled = []*types.Course{ course }
			}
			sessions, err := getters.GetCourseSessions(n, bundled)
			if err != nil {
				log.Fatal(w, err.Error(), http.StatusInternalServerError)
				return
			}

			err = t.Execute(w, CourseData{
				Course: course,
				Sessions: sessions,
			})
			if err != nil {
				log.Fatal(w, err.Error(), http.StatusInternalServerError)
				return
			}

			return
		}
	}

	/* We didn't find it */
	log.Fatal(w, fmt.Errorf("Unable to find course %s", k),
		http.StatusNotFound)
}

// Styles serves the styles.css file
func Styles(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Styles handler called")

	// Serve the styles.css file from the "static" directory
	w.Header().Add("Content-Type", "text/css")
	http.ServeFile(w, r, "static/css/styles.css")
}

// PageData is a struct that holds the data for a page
type pageData struct {
	Courses     []*types.Course
	IntroTitle  string
	Base58Pitch string
}

func getHomeData(n *types.Notion) (pageData, error) {
	courses, err := getters.ListCourses(n)
	return pageData{
		Courses:     courses,
		IntroTitle:  static.IntroTitle,
		Base58Pitch: static.Base58Pitch,
	}, err
}
