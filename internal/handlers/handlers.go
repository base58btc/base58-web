package handlers

import (
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/kodylow/base58-website/static"
)

func Home(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Home handler called")

	// Parse the template file
	tmpl, err := template.ParseFiles("templates/index.tmpl")
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Define the data to be rendered in the template
	data := getData()

	// Render the template with the data
	err = tmpl.Execute(w, data)
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// Styles serves the styles.css file
func Styles(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Styles handler called")

	// Serve the styles.css file from the "static" directory
	http.ServeFile(w, r, "static/css/styles.css")
}

// PageData is a struct that holds the data for a page
type pageData struct {
	Courses     []static.Course
	IntroTitle  string
	Base58Pitch string
}

// GetData returns the data for a page
func getData() pageData {
	return pageData{
		Courses:     static.Courses,
		IntroTitle:  static.IntroTitle,
		Base58Pitch: static.Base58Pitch,
	}
}
