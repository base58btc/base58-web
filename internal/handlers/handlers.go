package handlers

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/kodylow/base58-website/external/getters"
	"github.com/kodylow/base58-website/internal/types"
	"github.com/kodylow/base58-website/static"
	"io/ioutil"
)

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
