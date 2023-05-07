package handlers

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
	"github.com/joncalhoun/form"
	"github.com/kodylow/base58-website/external/getters"
	"github.com/kodylow/base58-website/internal/types"
	"github.com/kodylow/base58-website/internal/config"
	"io/ioutil"

	stripe "github.com/stripe/stripe-go/v74"
	"github.com/stripe/stripe-go/v74/paymentintent"
)

/* if not in prod, we rebild this every request (expensive, but fast) */
func BuildTemplateCache(ctx *config.AppContext) error {

	index, err := template.ParseFiles("templates/index.tmpl", "templates/course_desc.tmpl", "templates/sections/head.tmpl", "templates/sections/nav.tmpl", "templates/sections/footer.tmpl")
	if err != nil {
		return err
	}
	ctx.TemplateCache["index.tmpl"] = index


	inputs, err := template.New("checkout_form").Funcs(template.FuncMap{
		"fn_options": func(id string) []types.OptionItem {
			if id == "shirt" {
				return ShirtOptions()
			}
			if id == "checkout" {
				// FIXME: how to do this dynamically?
				return MakeCheckoutOpts(100)
			}
			return []types.OptionItem{}
		},
	}).ParseFiles("templates/forms/inputs.tmpl")
	if err != nil {
		return err
	}
	fb := form.Builder{
		InputTemplate: inputs,
	}

	funcMap := fb.FuncMap()
	funcMap["LastIdx"] = LastIdx
	funcMap["FiatPrice"] = FiatPrice
	funcMap["BtcPrice"] = BtcPrice

	register, err := template.New("register.tmpl").Funcs(funcMap).ParseFiles("templates/register.tmpl", "templates/sections/head.tmpl", "templates/sections/footer.tmpl", "templates/sections/nav.tmpl")
	if err != nil {
		return err
	}
	ctx.TemplateCache["register.tmpl"] = register

	inputs, err = template.New("waitlist_form").Funcs(template.FuncMap{
		"fn_options": func(id string) []types.OptionItem {
			return []types.OptionItem{}
		},
	}).ParseFiles("templates/forms/inputs.tmpl")
	if err != nil {
		return err
	}
	fb = form.Builder{
		InputTemplate: inputs,
	}

	funcMap = fb.FuncMap()
	funcMap["LastIdx"] = LastIdx
	funcMap["FiatPrice"] = FiatPrice
	funcMap["BtcPrice"] = BtcPrice
	waitlist, err := template.New("waitlist.tmpl").Funcs(funcMap).ParseFiles("templates/waitlist.tmpl", "templates/sections/head.tmpl", "templates/sections/footer.tmpl", "templates/sections/nav.tmpl")
	if err != nil {
		return err
	}
	ctx.TemplateCache["waitlist.tmpl"] = waitlist

	checkout, err := template.ParseFiles("templates/checkout.tmpl", "templates/sections/head.tmpl", "templates/sections/nav.tmpl", "templates/sections/footer.tmpl")
	if err != nil {
		return err
	}
	ctx.TemplateCache["checkout.tmpl"] = checkout

	success, err := template.New("success").Funcs(template.FuncMap{
		"LastIdx": LastIdx,
	}).ParseFiles("templates/success.tmpl", "templates/sections/head.tmpl", "templates/sections/nav.tmpl", "templates/sections/footer.tmpl")
	if err != nil {
		return err
	}
	ctx.TemplateCache["success.tmpl"] = success

	success, err = template.New("success").Funcs(template.FuncMap{
		"LastIdx": LastIdx,
	}).ParseFiles("templates/success.tmpl", "templates/sections/head.tmpl", "templates/sections/nav.tmpl", "templates/sections/footer.tmpl")
	if err != nil {
		return err
	}
	ctx.TemplateCache["success.tmpl"] = success
	return nil
}

func maybeRebuildCache(ctx *config.AppContext) error {
	if !ctx.ReloadCache() {
		return nil
	}

	return BuildTemplateCache(ctx)
}

func Routes(ctx *config.AppContext) (http.Handler, error) {
	ctx.TemplateCache = make(map[string]*template.Template)

	r := mux.NewRouter()

	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		maybeRebuildCache(ctx)
		Home(w, r, ctx)
	}).Methods("GET")
	r.HandleFunc("/classes/{class}", func(w http.ResponseWriter, r *http.Request) {
		maybeRebuildCache(ctx)
		Courses(w, r, ctx)
	})
	r.HandleFunc("/waitlist", func(w http.ResponseWriter, r *http.Request) {
		maybeRebuildCache(ctx)
		Waitlist(w, r, ctx)
	})
	r.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		maybeRebuildCache(ctx)
		Register(w, r, ctx)
	})
	r.HandleFunc("/success", func(w http.ResponseWriter, r *http.Request) {
		maybeRebuildCache(ctx)
		Success(w, r, ctx)
	})
	r.HandleFunc("/stripe-hook", func(w http.ResponseWriter, r *http.Request) {
		StripeHook(w, r, ctx)
	}).Methods("POST")

	/* serve files from the "static" directory */
	fs := http.FileServer(http.Dir("static"))
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", fs))

	/* special favicon handling */
	err := AddFaviconRoutes(r)

	if err != nil {
		return r, err
	}

	return r, nil
}

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
	Page    Page
}

type WaitlistData struct {
	Course  *types.Course
	Session *types.CourseSession
	Form    types.WaitList
	Page    Page
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

func Register(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
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
			http.Error(w, "Unable to load page, please try again later", http.StatusInternalServerError)
			ctx.Err.Printf("/register failed to fetch sessions %s\n", err.Error())
			return
		}

		// FIXME: how to update/inject func map for an existing template? 
		pageTpl := ctx.TemplateCache["register.tmpl"]

		/* token! */
		now := time.Now().UTC().UnixNano()
		idemToken := getSessionToken(ctx.Env.SecretBytes(), session.ID, now, session.Cost)

		w.Header().Set("Content-Type", "text/html")
		err = pageTpl.Execute(w, RegistrationData{
			Course:  course,
			Session: session,
			Page:        getPage("Course Registration"),
			Form: types.ClassRegistration{
				Idempotency: idemToken,
				Timestamp:   strconv.FormatInt(now, 10),
				SessionUUID: session.ID,
				Cost:        session.Cost,
			}})
		if err != nil {
			http.Error(w, "Unable to load page, please try again later", http.StatusInternalServerError)
			ctx.Err.Printf("/register templ exec failed %s\n", err.Error())
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
		http.Error(w, "Unable to load page, please try again later", http.StatusInternalServerError)
		ctx.Err.Printf("/register unable to decode class registrattion %s\n", err.Error())
		return
	}

	/* Check that the Idempotency token is valid */
	if !checkToken(form.Idempotency, ctx.Env.SecretBytes(),
		form.SessionUUID, form.Timestamp, form.Cost) {
		http.Error(w, "Invalid session token", http.StatusBadRequest)
		ctx.Err.Printf("/register not a good session token \n")
		return
	}

	// FIXME: keep track of token usage//timeout?

	/* Save to signups! Note: won't be considered final
	 * until there's a payment ref attached */
	id, err := getters.SaveRegistration(ctx.Notion, &form)
	// TODO: what happens if there's a duplicate/idempotent token?
	if err != nil {
		http.Error(w, "Oops, we weren't able to save", http.StatusInternalServerError)
		ctx.Err.Printf("/register Unable to save registration %s\n", err.Error())
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
		http.Error(w, "Page not found", http.StatusNotFound)
		ctx.Err.Printf("/register unable to find checkout method %s\n", form.CheckoutVia)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
	return
}

func Waitlist(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	sessionID, ok := getSessionKey("s", r)
	if !ok {
		/* If there's no session-key, redirect to the front page */
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	course, session, err := getters.GetSessionInfo(ctx.Notion, sessionID)
	if err != nil {
		http.Error(w, "Unable to fetch session info", http.StatusInternalServerError)
		ctx.Err.Printf("/register get session info failed  %s\n", err.Error())
		return
	}

	switch r.Method {
	case http.MethodGet:

		/* token! */
		now := time.Now().UTC().UnixNano()
		//idemToken := getSessionToken(ctx.Env.SecretBytes(), session.ID, now, uint64(0))
		pageTpl := ctx.TemplateCache["waitlist.tmpl"]
		w.Header().Set("Content-Type", "text/html")
		err = pageTpl.Execute(w, WaitlistData{
			Course:  course,
			Session: session,
			Page: getPage("Course Waitlist"),
			Form: types.WaitList{
				Idempotency: "waitlist",
				SessionUUID: session.ID,
				Timestamp:   strconv.FormatInt(now, 10),
			}})
		if err != nil {
			http.Error(w, "Unable to load page", http.StatusInternalServerError)
			ctx.Err.Printf("/waitlist tmpl exec failed %s\n", err.Error())
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
	err = dec.Decode(&form, r.PostForm)
	if err != nil {
		http.Error(w, "Unable to decode inputs", http.StatusInternalServerError)
		ctx.Err.Printf("/waitlist form decode failed %s\n", err.Error())
		return
	}

	/* Check that the Idempotency token is valid */
	/*
		if !checkToken(form.Idempotency, ctx.Env.SecretBytes(), form.SessionUUID, form.Timestamp, uint64(0)) {
			http.Error(w, "Invalid session token", http.StatusBadRequest)
			ctx.Err.Printf("/waitlist invalid session token %s\n", form.Idempotency)
			return
		}
	*/

	/* FIXME: Check that not already saved to waitlist */

	/* Save to waitlist! */
	err = getters.SaveWaitlist(ctx.Notion, &form)
	if err != nil {
		http.Error(w, "Unable to save waitlist", http.StatusInternalServerError)
		ctx.Err.Printf("/waitlist save failed %s\n", err.Error())
		return
	}

	/* Send a confirmation email! */
	err = SendWaitlistConfirmed(ctx, form.Email, session)
	if err != nil {
		http.Error(w, "Unable to send waitlist email confirmation", http.StatusInternalServerError)
		ctx.Err.Printf("/waitlist send confirmation failed %s\n", err.Error())
		return
	}

	// TODO: show waitlist success?
	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(form)
	w.Write(b)
}

type StripeCheckout struct {
	ClientSecret string
	PubKey       string
	Email        string
	SessionID    string
	Page         Page
}

func FiatCheckoutStart(w http.ResponseWriter, r *http.Request, ctx *config.AppContext, checkout *types.Checkout) {
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

	pageTpl := ctx.TemplateCache["checkout.tmpl"]

	w.Header().Set("Content-Type", "text/html")
	err := pageTpl.Execute(w, &StripeCheckout{
		ClientSecret: pi.ClientSecret,
		PubKey:       ctx.Env.Stripe.Pubkey,
		Email:        checkout.Email,
		SessionID:    checkout.SessionID,
		Page:         getPage("Checkout"),
		// TODO: other checkout info??
	})
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/checkout tmpl execute failed %s\n", err.Error())
	}
}

type SuccessData struct {
	Course  *types.Course
	Session *types.CourseSession
	Page    Page
}

func Success(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	/* Show a success page! */
	sessionID, ok := getSessionKey("s", r)
	if !ok {
		/* If there's no session-key, redirect to the front page */
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	course, session, err := getters.GetSessionInfo(ctx.Notion, sessionID)
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/success get session info failed %s\n", err.Error())
		return
	}

	tmpl := ctx.TemplateCache["success.tmpl"]
	err = tmpl.Execute(w, &SuccessData{
		Course:  course,
		Session: session,
		Page:    getPage(""),
	})
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/success execute template failed %s\n", err.Error())
	}
}

func StripeHook(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	const MaxBodyBytes = int64(65536)
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Unable to load page, please try again later", http.StatusServiceUnavailable)
		ctx.Err.Printf("/stripe-hook failed body read %s\n", err.Error())
		return
	}
	event := stripe.Event{}

	if err := json.Unmarshal(payload, &event); err != nil {
		http.Error(w, "Unable to process, please try again later", http.StatusBadRequest)
		ctx.Err.Printf("/stripe-hook body json unmarshal %s\n", err.Error())
		return
	}

	switch event.Type {
	case "payment_intent.succeeded":
		var payment stripe.PaymentIntent
		err := json.Unmarshal(event.Data.Raw, &payment)
		if err != nil {
			http.Error(w, "Unable to process, please try again later", http.StatusBadRequest)
			ctx.Err.Printf("/stripe-hook payment body json unmarshal %s\n", err.Error())
			return
		}
		/* Get out payment data */
		pageID := payment.Metadata["registration_id"]
		refID := payment.ID

		/* Add RefId to class signups table.
		 * This marks this signup as confirmed */
		sessionUUID, err := getters.UpdateRegistration(ctx.Notion, pageID, refID)
		if err != nil {
			http.Error(w, "Unable to process, please try again later", http.StatusBadRequest)
			ctx.Err.Printf("/stripe-hook unable to update signup %s %s\n", pageID, err.Error())
			return
		}

		/* Decrement available class count */
		err = getters.CountClassRegistration(ctx.Notion, sessionUUID)
		if err != nil {
			http.Error(w, "Unable to process, please try again later", http.StatusInternalServerError)
			ctx.Err.Printf("/stripe-hook decrement signup count failed %s %s\n", pageID, err.Error())
			return
		}

		// TODO: send email with receipt!!

		ctx.Infos.Println("great success!")
	default:
		ctx.Err.Printf("/stripe-hook unhandled event type %s\n", event.Type)
	}

	w.WriteHeader(http.StatusOK)
}

func Home(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	data, err := getHomeData(ctx.Notion)
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/index home data fetch failed %s\n", err.Error())
		return
	}

	// Render the template with the data
	tmpl := ctx.TemplateCache["index.tmpl"]
	err = tmpl.ExecuteTemplate(w, "index.tmpl", data)
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/index home exec template failed %s\n", err.Error())
		return
	}
}

type CourseData struct {
	Course   *types.Course
	Sessions []*types.CourseSession
	Page     Page
}

func Courses(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	/* If there's no class-key, redirect to the front page */
	params := mux.Vars(r)
	clss := params["class"]

	if clss == "" {
		/* redirect to "/". A lot of hard coded links exist pointing
		 * at "/classes", so we redirect! */
		http.Redirect(w, r, "/#courses", http.StatusSeeOther)
		return
	}

	courses, err := getters.ListCourses(ctx.Notion)
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/courses list courses attempt failed %s\n", err.Error())
		return
	}

	for _, course := range courses {
		if clss == course.TmplName {
			/* FIXME: put course page data into notion? */
			f, err := ioutil.ReadFile("templates/course.tmpl")
			if err != nil {
				http.Error(w, "Unable to load page", http.StatusInternalServerError)
				ctx.Err.Printf("/courses tmpl read failed %s\n", err.Error())
				return
			}
			t, err := template.New("course").Funcs(template.FuncMap{
				"LastIdx": LastIdx,
			}).Parse(string(f))
			if err != nil {
				http.Error(w, "Unable to load page", http.StatusInternalServerError)
				ctx.Err.Printf("/courses tmpl 'course' read failed %s\n", err.Error())
				return
			}

			/* FIXME: generalize? */
			var bundled []*types.Course
			if clss  == "tx-deep-dive" {
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
				http.Error(w, "Unable to load page", http.StatusInternalServerError)
				ctx.Err.Printf("/courses course sessions fetch failed %s\n", err.Error())
				return
			}

			err = t.Execute(w, CourseData{
				Course:   course,
				Sessions: sessions,
				Page:     getPage(course.PublicName),
			})
			if err != nil {
				http.Error(w, "Unable to load page", http.StatusInternalServerError)
				ctx.Err.Printf("/courses exec templ failed %s\n", err.Error())
				return
			}

			return
		}
	}

	/* We didn't find it */
	http.Error(w, "Unable to find course", http.StatusNotFound)
	ctx.Err.Printf("/course course not found %s\n", clss)
}

func Styles(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/css")
	http.ServeFile(w, r, "static/css/styles.css")
}

// PageData is a struct that holds the data for a page
type Page struct {
	Title string
	Copyright   int
}

type pageData struct {
	Page        Page
	Courses     []*types.Course
	Current     []*types.Course
	Coming      []*types.Course
}

func getPage(title string) Page {
	if title == "" {
		title = "Base58"
	}
	return Page {
		Title: title,
		Copyright:   time.Now().Year(),
	}
}

func getHomeData(n *types.Notion) (pageData, error) {
	courses, err := getters.ListCourses(n)

	if err != nil {
		return pageData{}, err
	}

	var current, coming []*types.Course
	for _, course := range courses {
		if course.ComingSoon {
			coming = append(coming, course)
		} else {
			current = append(current, course)
		}
	}
	return pageData{
		Page:        getPage(""),
		Courses:     courses,
		Current:     current,
		Coming:      coming,
	}, nil
}

/* these two functions make all the assets in /favicons as accessible */
func getFaviconHandler(name string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, fmt.Sprintf("static/favicons/%s", name))
	}
}

func AddFaviconRoutes(r *mux.Router) error {
	files, err := ioutil.ReadDir("static/favicons/")
	if err != nil {
		return err
	}

	/* If asked for a favicon, we'll serve it up */
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		r.HandleFunc(fmt.Sprintf("/%s", file.Name()), getFaviconHandler(file.Name())).Methods("GET")
	}

	return nil
}
