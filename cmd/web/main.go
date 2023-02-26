package main

import (
	"fmt"
	"log"
	"net/http"
    "time"
	"github.com/kodylow/base58-website/static"
    "github.com/kodylow/base58-website/internal/config"
)

const portNumber = ":8080"

var app config.AppConfig
var session *scs.SessionManager
var infoLog *log.Logger
var errorLog *log.Logger

func main() {
	r := mux.NewRouter()

	// Route to handle the root path
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Define a slice of courses to render in the template
		var courses := []static.Course{
			static.IntroToTransactions,
			static.IntroToScript,
			static.EnterSegWit,
			static.PublicPrivateKeys,
			static.SigningTransactions,
			static.Multisig,
		}

		fmt.Println(fmt.Sprintf("Starting application on port %s", portNumber))
        
        srv := &http.Server{
            Addr:    portNumber,
            Handler: routes(&app),
        }
    
        var err = srv.ListenAndServe()
        log.Fatal(err)
	})

	// Start the web server
	http.ListenAndServe(":8080", r)
}

func run() error {
    session = scs.New()
	session.Lifetime = 24 * time.Hour
	session.Cookie.Persist = true
	session.Cookie.SameSite = http.SameSiteLaxMode
	session.Cookie.Secure = app.InProduction

	app.Session = session
}