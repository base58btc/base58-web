package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/alexedwards/scs/v2"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/handlers"
	"github.com/kodylow/base58-website/internal/types"
)

const configFile = "config.toml"

var app config.AppContext
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
	app.IsProd = false // change to true in prod
	app.Infos = log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	app.Err = log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	// Load configs from config.toml
	env := loadConfig()

	// Start the server
	app.TemplateCache = make(map[string]*template.Template)
	routes, err := handlers.Routes(&app)
	if err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", env.Port),
		Handler: routes,
	}

	fmt.Printf("Starting application on port %s\n", env.Port)
	err = run(env)
	if err != nil {
		log.Fatal(err)
	}
	err = srv.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func run(env *types.EnvConfig) error {
	// Initialize the application configuration
	app.IsProd = false // change to true in production
	app.Redraw = true  // reload the draw cache by default
	app.Infos = log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	app.Err = log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)
	app.Env = env

	// Initialize the session manager
	session = scs.New()
	session.Lifetime = 72 * time.Hour
	session.Cookie.Persist = true
	session.Cookie.SameSite = http.SameSiteLaxMode
	session.Cookie.Secure = app.IsProd

	app.Session = session

	app.Notion = &types.Notion{Config: env.Notion}
	app.Notion.Setup()

	return nil
}
