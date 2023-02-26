package main

import (
	"github.com/gorilla/mux"
	"github.com/kodylow/golang-bookings/internal/config"
	"net/http"
)

func routes(app *config.AppConfig) http.Handler {
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

		
	})
}
