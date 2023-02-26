package main

import (
	"html/template"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/kodylow/base58-website/static"
)

func main() {
	r := mux.NewRouter()

	// Route to handle the root path
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Define a slice of courses to render in the template
		courses := []static.Course{
			static.IntroToTransactions,
			static.IntroToScript,
			static.EnterSegWit,
			static.PublicPrivateKeys,
			static.SigningTransactions,
			static.Multisig,
		}

		// Parse the template file and render it with the courses data
		tmpl, err := template.ParseFiles("templates/index.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		err = tmpl.Execute(w, courses)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	// Start the web server
	http.ListenAndServe(":8080", r)
}