package handlers

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/schema"
	"github.com/joncalhoun/form"
	"github.com/kodylow/base58-website/external/getters"
	"github.com/kodylow/base58-website/internal/types"
	"github.com/kodylow/base58-website/static"
	"io/ioutil"

	stripe "github.com/stripe/stripe-go/v74"
	"github.com/stripe/stripe-go/v74/paymentintent"
)

func ShirtOptions() []types.OptionItem {
	return []types.OptionItem{
		{Key: string(types.Small), Value: "Small"},
		{Key: string(types.Med), Value: "Medium"},
		{Key: string(types.Large), Value: "Large"},
		{Key: string(types.XL), Value: "XL"},
		{Key: string(types.XXL), Value: "XXL"}}
}

/* Amount is in whole dollar USD */
func MakeCheckoutOpts(amount uint64) []types.OptionItem {
	// FIXME: convert USD to btc amount??
	return []types.OptionItem{
		{Key: string(types.Bitcoin), Value: fmt.Sprintf("USD $%d", BtcPrice(amount))},
		{Key: string(types.Fiat), Value: fmt.Sprintf("USD $%d", FiatPrice(amount))},
	}
}

func BtcPrice(val uint64) uint64 {
	return uint64(float64(val) * .85)
}

func FiatPrice(val uint64) uint64 {
	return val
}

func LastIdx(size int) int {
	return size - 1
}

type RegistrationData struct {
	Course  *types.Course
	Session *types.CourseSession
	Form    types.ClassRegistration
}

type WaitlistData struct {
	Course  *types.Course
	Session *types.CourseSession
	Form    types.WaitList
}

func getSessionKey(p string, r *http.Request) (string, bool) {
	ok := r.URL.Query().Has(p)
	key := r.URL.Query().Get(p)
	return key, ok
}

func getSessionToken(sec []byte, sessionUUID string, now int64, cost uint64) string {
	/* Make a lil hash using the sessionUUID + timestamp */
	h := sha256.New()
	h.Write(sec)
	h.Write([]byte(sessionUUID))
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(now))
	h.Write(b)
	binary.LittleEndian.PutUint64(b, cost)
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

func checkToken(token string, sec []byte, sessionUUID string, timeStr string, cost uint64) bool {
	ts, err := strconv.ParseInt(timeStr, 10, 64)
	if err != nil {
		return false
	}

	expToken := getSessionToken(sec, sessionUUID, ts, cost)
	return expToken == token
}

func Register(w http.ResponseWriter, r *http.Request, ctx *types.AppContext) {
	sessionID, ok := getSessionKey("s", r)
	if !ok {
		/* If there's no session-key, redirect to the front page */
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	switch r.Method {
	case http.MethodGet:
		course, session, err := getters.GetSessionInfo(ctx.Notion, sessionID)
		if err != nil {
			log.Fatal(w, err.Error(), http.StatusInternalServerError)
			return
		}
		f, err := ioutil.ReadFile("templates/forms/inputs.tmpl")
		if err != nil {
			log.Fatal(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tpl := template.Must(template.New("").Funcs(
			template.FuncMap{
				"fn_options": func(id string) []types.OptionItem {
					if id == "shirt" {
						return ShirtOptions()
					}
					if id == "checkout" {
						return MakeCheckoutOpts(session.Cost)
					}
					return []types.OptionItem{}
				},
			}).Parse(string(f)))
		fb := form.Builder{
			InputTemplate: tpl,
		}

		f, err = ioutil.ReadFile("templates/register.tmpl")
		if err != nil {
			log.Fatal(w, err.Error(), http.StatusInternalServerError)
			return
		}
		funcMap := fb.FuncMap()
		funcMap["LastIdx"] = LastIdx
		funcMap["FiatPrice"] = FiatPrice
		funcMap["BtcPrice"] = BtcPrice
		pageTpl := template.Must(template.New("").Funcs(funcMap).Parse(string(f)))

		/* token! */
		now := time.Now().UTC().UnixNano()
		idemToken := getSessionToken(ctx.Env.SecretBytes(), session.ID, now, session.Cost)

		w.Header().Set("Content-Type", "text/html")
		err = pageTpl.Execute(w, RegistrationData{
			Course:  course,
			Session: session,
			Form: types.ClassRegistration{
				Idempotency: idemToken,
				Timestamp:   strconv.FormatInt(now, 10),
				SessionUUID: session.ID,
				Cost:        session.Cost,
			}})
		if err != nil {
			log.Fatal(w, err.Error(), http.StatusInternalServerError)
		}
		return
	case http.MethodPost:
		/* Goes to the bottom! */
	default:
		http.NotFound(w, r)
		return
	}

	r.ParseForm()
	dec := schema.NewDecoder()
	dec.IgnoreUnknownKeys(true)
	var form types.ClassRegistration
	err := dec.Decode(&form, r.PostForm)
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusBadRequest)
		return
	}

	/* Check that the Idempotency token is valid */
	if !checkToken(form.Idempotency, ctx.Env.SecretBytes(),
		form.SessionUUID, form.Timestamp, form.Cost) {
		log.Fatal(w, fmt.Errorf("Invalid session token"), http.StatusBadRequest)
		return
	}

	// FIXME: keep track of token usage//timeout?

	/* Save to signups! Note: won't be considered final
	 * until there's a payment ref attached */
	id, err := getters.SaveRegistration(ctx.Notion, &form)
	// TODO: what happens if there's a duplicate/idempotent token?
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusBadRequest)
		return
	}

	checkout := &types.Checkout{
		RegisterID:  id,
		Price:       form.Cost,
		Type:        form.CheckoutVia,
		Idempotency: form.Idempotency,
		SessionID:   sessionID,
		Email:       form.Email,
	}

	/* Ok, now we go to checkout! */
	switch form.CheckoutVia {
	case types.Bitcoin:
	case types.Fiat:
		FiatCheckoutStart(w, r, ctx, checkout)
		return
	default:
		log.Fatal(w, err.Error(), http.StatusNotFound)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
	return
}

func Waitlist(w http.ResponseWriter, r *http.Request, ctx *types.AppContext) {
	sessionID, ok := getSessionKey("s", r)
	if !ok {
		/* If there's no session-key, redirect to the front page */
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	switch r.Method {
	case http.MethodGet:
		course, session, err := getters.GetSessionInfo(ctx.Notion, sessionID)
		if err != nil {
			log.Fatal(w, err.Error(), http.StatusInternalServerError)
			return
		}

		f, err := ioutil.ReadFile("templates/forms/inputs.tmpl")
		if err != nil {
			log.Fatal(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tpl := template.Must(template.New("").Funcs(
			template.FuncMap{
				"fn_options": func(id string) []types.OptionItem {
					return []types.OptionItem{}
				},
			}).Parse(string(f)))
		fb := form.Builder{InputTemplate: tpl}

		f, err = ioutil.ReadFile("templates/waitlist.tmpl")
		if err != nil {
			log.Fatal(w, err.Error(), http.StatusInternalServerError)
			return
		}
		funcMap := fb.FuncMap()
		funcMap["LastIdx"] = LastIdx
		funcMap["FiatPrice"] = FiatPrice
		funcMap["BtcPrice"] = BtcPrice

		/* token! */
		now := time.Now().UTC().UnixNano()
		idemToken := getSessionToken(ctx.Env.SecretBytes(), session.ID, now, uint64(0))
		pageTpl := template.Must(template.New("").Funcs(funcMap).Parse(string(f)))
		w.Header().Set("Content-Type", "text/html")
		err = pageTpl.Execute(w, WaitlistData{
			Course:  course,
			Session: session,
			Form: types.WaitList{
				Idempotency: idemToken,
				SessionUUID: session.ID,
			}})
		if err != nil {
			log.Fatal(w, err.Error(), http.StatusInternalServerError)
		}
		return
	case http.MethodPost:
		/* Goes to the bottom! */
	default:
		http.NotFound(w, r)
		return
	}

	r.ParseForm()
	dec := schema.NewDecoder()
	dec.IgnoreUnknownKeys(true)
	var form types.WaitList
	err := dec.Decode(&form, r.PostForm)
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusBadRequest)
		return
	}

	/* Check that the Idempotency token is valid */
	if !checkToken(form.Idempotency, ctx.Env.SecretBytes(), form.SessionUUID, form.Timestamp, uint64(0)) {
		log.Fatal(w, fmt.Errorf("Invalid session token"), http.StatusBadRequest)
		return
	}

	/* FIXME: Check that not already saved to waitlist */

	/* Save to waitlist! */
	err = getters.SaveWaitlist(ctx.Notion, &form)
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusBadRequest)
		return
	}
	// TODO: show waitlist success?
	// TODO: send confirmation email!
	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(form)
	w.Write(b)
}

type StripeCheckout struct {
	ClientSecret string
	PubKey       string
	Email        string
	SessionID    string
}

func FiatCheckoutStart(w http.ResponseWriter, r *http.Request, ctx *types.AppContext, checkout *types.Checkout) {
	stripe.Key = ctx.Env.Stripe.Key

	/* add cents for stripe! */
	price := int64(FiatPrice(checkout.Price) * 100)
	/* First we register a payment intent */
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(price),
		Currency: stripe.String(string(stripe.CurrencyUSD)),
		AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
			Enabled: stripe.Bool(true),
		},
		/* Sends customer a receipt from Stripe */
		/* ReceiptEmail: checkout.Email, */
	}

	params.AddMetadata("registration_id", checkout.RegisterID)
	if ctx.Env.Stripe.IsTest() {
		params.AddMetadata("integration_check", "accept_a_payment")
	}

	pi, _ := paymentintent.New(params)

	/* Now show the stripe checkout page! */
	f, err := ioutil.ReadFile("templates/checkout.tmpl")
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusInternalServerError)
		return
	}
	pageTpl := template.Must(template.New("").Parse(string(f)))

	w.Header().Set("Content-Type", "text/html")
	err = pageTpl.Execute(w, &StripeCheckout{
		ClientSecret: pi.ClientSecret,
		PubKey:       ctx.Env.Stripe.Pubkey,
		Email:        checkout.Email,
		SessionID:    checkout.SessionID,
		// TODO: other checkout info??
	})
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusInternalServerError)
	}
}

type SuccessData struct {
	Course  *types.Course
	Session *types.CourseSession
}

func Success(w http.ResponseWriter, r *http.Request, ctx *types.AppContext) {
	/* Show a success page! */
	sessionID, ok := getSessionKey("s", r)
	if !ok {
		/* If there's no session-key, redirect to the front page */
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	course, session, err := getters.GetSessionInfo(ctx.Notion, sessionID)
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusInternalServerError)
		return
	}

	f, err := ioutil.ReadFile("templates/success.tmpl")
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusInternalServerError)
		return
	}
	t, err := template.New("success").Funcs(template.FuncMap{
		"LastIdx": LastIdx,
	}).Parse(string(f))
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = t.Execute(w, &SuccessData{
		Course:  course,
		Session: session,
	})
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusInternalServerError)
	}
}

func StripeHook(w http.ResponseWriter, r *http.Request, ctx *types.AppContext) {
	const MaxBodyBytes = int64(65536)
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading request body: %v\n", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	event := stripe.Event{}

	if err := json.Unmarshal(payload, &event); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse webhook body json: %v\n", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch event.Type {
	case "payment_intent.succeeded":
		var payment stripe.PaymentIntent
		err := json.Unmarshal(event.Data.Raw, &payment)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing webhook JSON: %v\n", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		/* Get out payment data */
		pageID := payment.Metadata["registration_id"]
		refID := payment.ID

		/* Add RefId to class signups table.
		 * This marks this signup as confirmed */
		sessionUUID, err := getters.UpdateRegistration(ctx.Notion, pageID, refID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to update signup %s: %v\n", pageID, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		/* Decrement available class count */
		err = getters.CountClassRegistration(ctx.Notion, sessionUUID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to decrement signup %s: %v\n", pageID, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// TODO: send email with receipt!!

		fmt.Println("great success!")
	default:
		fmt.Fprintf(os.Stderr, "Unhandled event type: %s\n", event.Type)
	}

	w.WriteHeader(http.StatusOK)
}

func Home(w http.ResponseWriter, r *http.Request, ctx *types.AppContext) {
	// Parse the template file
	tmpl, err := template.ParseFiles("templates/index.tmpl")
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Define the data to be rendered in the template
	data, err := getHomeData(ctx.Notion)
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Render the template with the data
	err = tmpl.Execute(w, data)
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

type CourseData struct {
	Course   *types.Course
	Sessions []*types.CourseSession
}

func Courses(w http.ResponseWriter, r *http.Request, ctx *types.AppContext) {
	/* If there's no class-key, redirect to the front page */
	hasK := r.URL.Query().Has("k")
	if !hasK {
		/* redirect to "/". A lot of hard coded links exist pointing
		 * at "/classes", so we redirect! */
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	k := r.URL.Query().Get("k")
	courses, err := getters.ListCourses(ctx.Notion)
	if err != nil {
		log.Fatal(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, course := range courses {
		if k == course.TmplName {
			/* FIXME: put course page data into notion? */
			f, err := ioutil.ReadFile("templates/course.tmpl")
			if err != nil {
				log.Fatal(w, err.Error(), http.StatusInternalServerError)
				return
			}
			t, err := template.New("course").Funcs(template.FuncMap{
				"LastIdx": LastIdx,
			}).Parse(string(f))
			if err != nil {
				log.Fatal(w, err.Error(), http.StatusInternalServerError)
				return
			}

			/* FIXME: generalize? */
			var bundled []*types.Course
			if k == "tx-deep-dive" {
				bundled = append(bundled, course)
				for _, c := range courses {
					if strings.HasPrefix(c.TmplName, course.TmplName) {
						bundled = append(bundled, c)
					}
				}
			} else {
				bundled = []*types.Course{course}
			}
			sessions, err := getters.GetCourseSessions(ctx.Notion, bundled)
			if err != nil {
				log.Fatal(w, err.Error(), http.StatusInternalServerError)
				return
			}

			err = t.Execute(w, CourseData{
				Course:   course,
				Sessions: sessions,
			})
			if err != nil {
				log.Fatal(w, err.Error(), http.StatusInternalServerError)
				return
			}

			return
		}
	}

	/* We didn't find it */
	log.Fatal(w, fmt.Errorf("Unable to find course %s", k),
		http.StatusNotFound)
}

// Styles serves the styles.css file
func Styles(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Styles handler called")

	// Serve the styles.css file from the "static" directory
	w.Header().Add("Content-Type", "text/css")
	http.ServeFile(w, r, "static/css/styles.css")
}

// PageData is a struct that holds the data for a page
type pageData struct {
	Courses     []*types.Course
	IntroTitle  string
	Base58Pitch string
}

func getHomeData(n *types.Notion) (pageData, error) {
	courses, err := getters.ListCourses(n)
	return pageData{
		Courses:     courses,
		IntroTitle:  static.IntroTitle,
		Base58Pitch: static.Base58Pitch,
	}, err
}
