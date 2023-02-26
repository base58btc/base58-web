package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/handlers"
)

const portNumber = ":8080"

var app config.AppConfig
var session *scs.SessionManager

func main() {
	// Initialize the application configuration
	app.InProduction = false // change to true in prod
	app.InfoLog = log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	app.ErrorLog = log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	// Load environment variables from .env file
	err := godotenv.Load("./.env")

	// Start the server
	srv := &http.Server{
		Addr:    portNumber,
		Handler: Routes(),
	}

	fmt.Println("test")

	fmt.Printf("Starting application on port %s\n", portNumber)
	err = run()
	err = srv.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Initialize the application configuration
	app.InProduction = false // change to true in production
	app.InfoLog = log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	app.ErrorLog = log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	// Initialize the session manager
	session = scs.New()
	session.Lifetime = 24 * time.Hour
	session.Cookie.Persist = true
	session.Cookie.SameSite = http.SameSiteLaxMode
	session.Cookie.Secure = app.InProduction

	app.Session = session
	return nil
}

// Routes sets up the routes for the application
func Routes() http.Handler {
	// Create a file server to serve static files from the "static" directory
	fs := http.FileServer(http.Dir("static"))

	r := mux.NewRouter()

	// Set up the routes
	r.HandleFunc("/", handlers.Home).Methods("GET")
	r.HandleFunc("/notion", handlers.Notion).Methods("GET")
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", fs))

	return r
}
