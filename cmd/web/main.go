package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/joho/godotenv"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/handlers"
	"github.com/kodylow/base58-website/internal/types"
	"github.com/rs/cors"
)

var app config.AppContext

func loadConfig() (*types.EnvConfig, bool) {
	var config types.EnvConfig
	var err error

	/* no toml file.. */
	if secrets := os.Getenv("SECRETS_FILE"); secrets != "" {
		fmt.Println("using secrets", secrets)
		err = godotenv.Load(secrets)
	} else if _, err := os.Stat(".env"); err == nil {
		err = godotenv.Load()
	}
	if err != nil {
		log.Fatal(err)
	}

	config.Port = os.Getenv("PORT")
	config.Domain = os.Getenv("LOCAL_DOMAIN")
	config.External = os.Getenv("EXTERN_DOMAIN")
	config.Secret = os.Getenv("TOKEN_SEC")
	config.MailerSecret = os.Getenv("MAIL_SEC")
	config.MailDomain = os.Getenv("MAIL_DOMAIN")
	config.MailEndpoint = os.Getenv("MAIL_API")

	config.Notion = types.NotionConfig{
		Token:          os.Getenv("NOTION_TOKEN"),
		CoursesDb:      os.Getenv("NOTION_COURSES"),
		SessionsDb:     os.Getenv("NOTION_SESSIONS"),
		SessionCartsDb: os.Getenv("NOTION_CARTS"),
		ItemCartsDb:    os.Getenv("NOTION_ITEM_CARTS"),
		SignupsDb:      os.Getenv("NOTION_SIGNUPS"),
		WaitlistDb:     os.Getenv("NOTION_WAITLIST"),
	}

	config.OpenNode = types.OpenNodeConfig{
		Key:      os.Getenv("OPENNODE_KEY"),
		Endpoint: os.Getenv("OPENNODE_API"),
	}

	config.Stripe = types.StripeConfig{
		Key:         os.Getenv("STRIPE_KEY"),
		Pubkey:      os.Getenv("STRIPE_PUBKEY"),
		EndpointSec: os.Getenv("STRIPE_ENDSEC"),
	}

	isProd := os.Getenv("IS_PROD") == "1"

	return &config, isProd
}

func main() {
	// Initialize the application configuration
	app.Infos = log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	app.Err = log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	// Load configs from config.toml
	env, isProd := loadConfig()
	app.IsProd = isProd

	/* Register cart types */
	handlers.RegisterCheckoutTypes()

	// Start the server
	app.EmailCache = make(map[string]*template.Template)
	routes, err := handlers.Routes(&app)
	if err != nil {
		log.Fatal(err)
	}

	cors := cors.New(cors.Options{
		AllowedMethods: []string{http.MethodGet, http.MethodPost},
		AllowedOrigins: []string{"*"},
		AllowedHeaders: []string{"Content-Type", "Accept"},
	})

	corsHandler := cors.Handler(routes)

	fmt.Printf("Starting application on port %s\n", env.Port)
	err = run(env)
	if err != nil {
		log.Fatal(err)
	}

	loader := app.Session.LoadAndSave(corsHandler)
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", env.Port),
		Handler: loader,
	}

	err = srv.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func run(env *types.EnvConfig) error {
	// Initialize the application configuration
	app.IsProd = false // change to true in production
	app.Infos = log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	app.Err = log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)
	app.Env = env

	// Initialize the session manager
	app.Session = scs.New()
	app.Session.Lifetime = 72 * time.Hour
	app.Session.Cookie.Persist = true
	app.Session.Cookie.SameSite = http.SameSiteLaxMode
	app.Session.Cookie.Secure = app.IsProd

	app.Notion = &types.Notion{Config: env.Notion}
	app.Notion.Setup()

	return nil
}
