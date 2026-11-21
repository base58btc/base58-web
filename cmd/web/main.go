package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/alexedwards/scs/postgresstore"
	"github.com/alexedwards/scs/v2"
	"github.com/joho/godotenv"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/database"
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
	config.DBDriver = os.Getenv("DB_DRIVER")
	config.DatabaseURL = os.Getenv("DATABASE_URL")
	config.AdminEmails = os.Getenv("ADMIN_BOOTSTRAP_EMAILS")
	config.EditorSyncToken = os.Getenv("EDITOR_SYNC_TOKEN")
	config.CourseAssetBucket = os.Getenv("COURSE_ASSET_BUCKET")
	config.CourseAssetRegion = os.Getenv("COURSE_ASSET_REGION")
	config.CourseAssetPrefix = os.Getenv("COURSE_ASSET_PREFIX")

	config.Notion = types.NotionConfig{
		Token:      os.Getenv("NOTION_TOKEN"),
		CoursesDb:  os.Getenv("NOTION_COURSES"),
		SessionsDb: os.Getenv("NOTION_SESSIONS"),
	}

	isProd := os.Getenv("IS_PROD") == "1"

	return &config, isProd
}

func main() {
	// Initialize the application configuration
	app.Infos = log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	app.Err = log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	// Load configuration from .env/SECRETS_FILE and process environment.
	env, isProd := loadConfig()
	app.IsProd = isProd

	// Decode carts in pre-retirement sessions without restoring checkout behavior.
	handlers.RegisterLegacyCartTypes()

	// Start the server
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

	db, err := database.Open(env, app.IsProd)
	if err != nil {
		return err
	}
	app.DB = db
	if db != nil {
		app.Session.Store = postgresstore.New(db)

		go func() {
			result, err := database.SyncCoursesFromNotion(db, env.DBDriver, app.Notion)
			if err != nil {
				app.Err.Printf("course sync from Notion failed: %s", err.Error())
			} else {
				app.Infos.Printf("course sync from Notion complete: seen=%d upserts=%d skipped=%d", result.Seen, result.Upserts, result.Skipped)
			}
			if err := handlers.SyncLocalCourseVersions(&app); err != nil {
				app.Err.Printf("local course version sync failed: %s", err.Error())
			}
		}()
	}

	return nil
}
