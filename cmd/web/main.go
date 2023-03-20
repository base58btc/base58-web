package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/alexedwards/scs/v2"
	"github.com/gorilla/mux"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/handlers"
	"github.com/kodylow/base58-website/internal/types"
)

const configFile = "config.toml"

var app config.AppConfig
var session *scs.SessionManager

func loadConfig() *types.EnvConfig {
	var config types.EnvConfig

	_, err := toml.DecodeFile(configFile, &config)
	if err != nil {
		log.Fatal(err)
	}
	return &config
}

func main() {
	// Initialize the application configuration
	app.InProduction = false // change to true in prod
	app.InfoLog = log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	app.ErrorLog = log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	// Load configs from config.toml
	app.Env = loadConfig()

	// Start the server
	srv := &http.Server{
		Addr:    app.Env.Port,
		Handler: Routes(),
	}

	fmt.Printf("Starting application on port %s\n", app.Env.Port)
	err := run()
	if err != nil {
		log.Fatal(err)
	}
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
	session.Lifetime = 72 * time.Hour
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

	// Set up the routes, we'll have one page per course
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handlers.Home(w, r, app.Env)
	}).Methods("GET")
	r.HandleFunc("/classes", func(w http.ResponseWriter, r *http.Request) {
		handlers.Courses(w, r, app.Env)
	})
	r.HandleFunc("/waitlist", func(w http.ResponseWriter, r *http.Request) {
		handlers.Waitlist(w, r, app.Env)
	})
	r.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		handlers.Register(w, r, app.Env)
	})
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", fs))

	return r
}
