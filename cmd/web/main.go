package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/alexedwards/scs/redisstore"
	"github.com/alexedwards/scs/v2"
	"github.com/gomodule/redigo/redis"
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
	env := loadConfig()

	// Start the server
	srv := &http.Server{
		Addr:    env.Port,
		Handler: Routes(),
	}

	fmt.Printf("Starting application on port %s...", env.Port)
	err := run(env)
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
	app.InProduction = false // change to true in production
	app.InfoLog = log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	app.ErrorLog = log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	// Initialize the session manager
	session = initSession()
	app.Session = session

	// Initialize the Notion client
	notion := &types.Notion{Config: env.Notion}
	notion.Setup()

	// Set up the application context
	app.Context = types.AppContext{
		Env:    env,
		Notion: notion,
	}
	return nil
}

// Routes sets up the routes for the application
func Routes() http.Handler {
	log.Printf("Initializing routes...")
	// Create a file server to serve static files from the "static" directory
	fs := http.FileServer(http.Dir("static"))

	r := mux.NewRouter()

	// Set up the routes, we'll have one page per course
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handlers.Home(w, r, &app.Context)
	}).Methods("GET")
	r.HandleFunc("/classes", func(w http.ResponseWriter, r *http.Request) {
		handlers.Courses(w, r, &app.Context)
	})
	r.HandleFunc("/waitlist", func(w http.ResponseWriter, r *http.Request) {
		handlers.Waitlist(w, r, &app.Context)
	})
	r.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		handlers.Register(w, r, &app.Context)
	})
	r.HandleFunc("/success", func(w http.ResponseWriter, r *http.Request) {
		handlers.Success(w, r, &app.Context)
	})
	r.HandleFunc("/stripe-hook", func(w http.ResponseWriter, r *http.Request) {
		handlers.StripeHook(w, r, &app.Context)
	})

	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", fs))

	return r
}

func initSession() *scs.SessionManager {
	log.Printf("Initializing session...")
	session := scs.New()

	redisPool := newRedisPool()
	session.Store = redisstore.New(redisPool)

	session.Lifetime = 24 * time.Hour
	session.Cookie.Persist = true
	session.Cookie.SameSite = http.SameSiteLaxMode
	session.Cookie.Secure = true

	return session
}

// newRedisPool initializes the Redis connection pool for session management server side
func newRedisPool() *redis.Pool {
	log.Printf("Initializing Redis...")

	redisPool := &redis.Pool{
		DialContext: func(ctx context.Context) (redis.Conn, error) {
			return redis.Dial("tcp", os.Getenv("REDIS"))
		},
	}

	// Ping the Redis server to check the connection
	conn := redisPool.Get()
	defer conn.Close()

	_, err := conn.Do("PING")
	if err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}

	return redisPool
}
