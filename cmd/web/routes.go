package main

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/kodylow/base58-website/internal/handlers"
)

// Routes sets up the routes for the application
func Routes() http.Handler {
	// Create a file server to serve static files from the "static" directory
	fs := http.FileServer(http.Dir("static"))

	r := mux.NewRouter()

	// Set up the routes
	r.HandleFunc("/", handlers.Home).Methods("GET")
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", fs))

	return r
}
