package main

// import (
// 	"net/http"

// 	"github.com/justinas/nosurf"
// 	"github.com/kodylow/base58-website/internal/helpers"
// )

// // NoSurf adds CSRF protection to all POST requests
// func NoSurf(next http.Handler) http.Handler {
// 	csrfHandler := nosurf.New(next)

// 	csrfHandler.SetBaseCookie(http.Cookie{
// 		HttpOnly: true,
// 		Path:     "/",
// 		Secure:   app.InProduction,
// 		SameSite: http.SameSiteLaxMode,
// 	})
// 	return csrfHandler
// }

// // SessionLoad loads and saves the session on every request
// func SessionLoad(next http.Handler) http.Handler {
// 	return session.LoadAndSave(next)
// }

// // Auth checks whether user is logged in
// func Auth(next http.Handler) http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		if !helpers.IsAuthenticated(r) {
// 			session.Put(r.Context(), "error", "Log in first")
// 			http.Redirect(w, r, "/user-login", http.StatusSeeOther)
// 			return
// 		}
// 		next.ServeHTTP(w, r)
// 	})
// }
