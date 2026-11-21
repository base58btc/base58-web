package handlers

import (
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"github.com/gorilla/mux"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/emails"
	"github.com/kodylow/base58-website/internal/helpers"
)

// Tombstones prevent old links from reviving checkout or mailing-list signup.
// They do not read or mutate the historical records in Notion.
func registerRetiredRoutes(r *mux.Router, ctx *config.AppContext) {
	for _, path := range []string{"/register", "/reserve", "/summary", "/success", "/waitlist"} {
		r.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			retiredPage(w, http.StatusGone, "Class checkout is unavailable", "Please contact hello@base58.school about existing registrations.")
		})
	}
	for _, path := range []string{"/{newsletter}/subscribe", "/confirm/{token}", "/{newsletter}/schedule", "/{missive}/unschedule", "/check-email/{email}"} {
		r.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			retiredPage(w, http.StatusGone, "Newsletter signup and scheduling are unavailable", "Please contact hello@base58.school with any questions.")
		})
	}
	for _, path := range []string{"/stripe-hook", "/opennode-hook"} {
		r.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			// Never acknowledge an unprocessed payment as successful. Outstanding
			// payments must be reconciled before deploying this retirement.
			w.Header().Set("Retry-After", "3600")
			http.Error(w, "Legacy payment processing is unavailable", http.StatusServiceUnavailable)
		})
	}
	r.HandleFunc("/newsletter/unsubscribe/{token}", func(w http.ResponseWriter, r *http.Request) {
		legacyUnsubscribe(w, r, ctx)
	}).Methods(http.MethodGet)
}

func retiredPage(w http.ResponseWriter, status int, title, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	// All arguments are fixed server strings, never request data.
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><meta name="viewport" content="width=device-width, initial-scale=1"><title>` + title + `</title><main><h1>` + title + `</h1><p>` + message + `</p><a href="/courses">Browse courses</a></main></html>`))
}

// Keep validating the original token format so links in already-delivered mail
// continue to work. This function does not issue new subscription tokens.
func legacySubscriptionToken(secret []byte, token string) (string, string, error) {
	invalid := errors.New("invalid unsubscribe link")
	if len(secret) == 0 || len(token) > 4096 {
		return "", "", invalid
	}
	parts := strings.Split(token, "-")
	if len(parts) != 4 || len(parts[0]) != 16 {
		return "", "", invalid
	}
	email, err := hex.DecodeString(parts[1])
	if err != nil || len(email) == 0 || len(email) > 320 {
		return "", "", invalid
	}
	list, err := hex.DecodeString(parts[2])
	if err != nil || len(list) == 0 || len(list) > 256 {
		return "", "", invalid
	}
	stamp, err := hex.DecodeString(parts[3])
	if err != nil || len(stamp) != 8 {
		return "", "", invalid
	}
	expected, _ := helpers.GetSubscribeToken(secret, string(email), string(list), binary.LittleEndian.Uint64(stamp))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(parts[0])) != 1 {
		return "", "", invalid
	}
	if _, err := mail.ParseAddress(string(email)); err != nil {
		return "", "", invalid
	}
	return string(email), string(list), nil
}

func legacyUnsubscribe(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if ctx == nil || ctx.Env == nil || ctx.DB == nil || len(ctx.Env.SecretBytes()) == 0 {
		http.Error(w, "Unsubscribe is temporarily unavailable. Please contact hello@base58.school.", http.StatusServiceUnavailable)
		return
	}
	email, list, err := legacySubscriptionToken(ctx.Env.SecretBytes(), mux.Vars(r)["token"])
	if err != nil {
		http.Error(w, "Invalid unsubscribe link", http.StatusBadRequest)
		return
	}
	address, _ := mail.ParseAddress(email)
	normalized := normalizeEmail(address.Address)
	// Preserve opt-outs independently of Notion. Any future subscriber import
	// must apply these suppressions before sending mail.
	_, err = ctx.DB.ExecContext(r.Context(), `INSERT INTO email_opt_outs (email, newsletter, original_email)
VALUES ($1, $2, $3) ON CONFLICT (email, newsletter) DO UPDATE
SET original_email=EXCLUDED.original_email, mailer_cancelled_at=NULL`, normalized, list, email)
	if err != nil {
		ctx.Err.Printf("record legacy email opt-out: %v", err)
		http.Error(w, "Unable to save your unsubscribe request. Please try again.", http.StatusServiceUnavailable)
		return
	}
	// Use the original spelling: historical mailer subscription keys include it.
	// Retry on every visit, even if an earlier cancellation failed after saving.
	if ctx.Env.MailEndpoint == "" || ctx.Env.MailerSecret == "" {
		http.Error(w, "Your opt-out is saved, but queued email cancellation is unavailable. Please contact hello@base58.school.", http.StatusServiceUnavailable)
		return
	}
	if err := emails.SendSubDeleteRequest(ctx, email, list); err != nil {
		ctx.Err.Printf("cancel legacy subscription mail: %v", err)
		http.Error(w, "Your opt-out is saved, but queued email cancellation failed. Please try this link again.", http.StatusServiceUnavailable)
		return
	}
	if _, err := ctx.DB.ExecContext(r.Context(), `UPDATE email_opt_outs SET mailer_cancelled_at=CURRENT_TIMESTAMP WHERE email=$1 AND newsletter=$2`, normalized, list); err != nil {
		ctx.Err.Printf("mark legacy mail cancellation: %v", err)
	}
	retiredPage(w, http.StatusOK, "You’re unsubscribed", "Your email preference has been saved.")
}
