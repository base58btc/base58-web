package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
	"github.com/kodylow/base58-website/external/getters"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/emails"
	"github.com/kodylow/base58-website/internal/helpers"
	"github.com/kodylow/base58-website/internal/types"
	"io/ioutil"

	stripe "github.com/stripe/stripe-go/v74"
	"github.com/stripe/stripe-go/v74/paymentintent"
	"github.com/stripe/stripe-go/v74/webhook"
)

/* if not in prod, we rebild this every request (expensive, but fast) */
func BuildTemplateCache(ctx *config.AppContext) error {

	index, err := template.ParseFiles("templates/index.tmpl", "templates/course_desc.tmpl", "templates/sections/head.tmpl", "templates/sections/nav.tmpl", "templates/sections/footer.tmpl")
	if err != nil {
		return err
	}
	ctx.TemplateCache["index.tmpl"] = index

	courses, err := template.New("course").Funcs(template.FuncMap{
		"LastIdx":     LastIdx,
		"FiatPrice":   types.FiatPrice,
		"BtcPrice":    types.BtcPrice,
		"AvailOnline": AvailOnline,
	}).ParseFiles("templates/course.tmpl", "templates/sections/head.tmpl", "templates/sections/footer.tmpl", "templates/sections/nav.tmpl")
	if err != nil {
		return err
	}
	ctx.TemplateCache["course.tmpl"] = courses

	register, err := template.New("register.tmpl").Funcs(template.FuncMap{
		"ShirtOpts": ShirtOptions,
		"TixCount":  TixCount,
		"LastIdx":   LastIdx,
		"FiatPrice": types.FiatPrice,
		"BtcPrice":  types.BtcPrice,
	}).ParseFiles("templates/register.tmpl", "templates/sections/head.tmpl", "templates/sections/footer.tmpl", "templates/sections/nav.tmpl")
	if err != nil {
		return err
	}
	ctx.TemplateCache["register.tmpl"] = register

	checkout, err := template.ParseFiles("templates/checkout.tmpl", "templates/sections/head.tmpl", "templates/sections/nav.tmpl", "templates/sections/footer.tmpl")
	if err != nil {
		return err
	}
	ctx.TemplateCache["checkout.tmpl"] = checkout

	waitlist, err := template.New("waitlist").Funcs(template.FuncMap{
		"FiatPrice": types.FiatPrice,
		"BtcPrice":  types.BtcPrice,
		"LastIdx":   LastIdx,
	}).ParseFiles("templates/waitlist.tmpl", "templates/sections/head.tmpl", "templates/sections/footer.tmpl", "templates/sections/nav.tmpl")
	if err != nil {
		return err
	}
	ctx.TemplateCache["waitlist.tmpl"] = waitlist

	success, err := template.New("success").Funcs(template.FuncMap{
		"LastIdx": LastIdx,
	}).ParseFiles("templates/success.tmpl", "templates/sections/head.tmpl", "templates/sections/nav.tmpl", "templates/sections/footer.tmpl")
	if err != nil {
		return err
	}
	ctx.TemplateCache["success.tmpl"] = success

	waitlistS, err := template.New("waitlist_success").Funcs(template.FuncMap{
		"FiatPrice": types.FiatPrice,
		"BtcPrice":  types.BtcPrice,
	}).ParseFiles("templates/waitlist_success.tmpl", "templates/sections/head.tmpl", "templates/sections/nav.tmpl", "templates/sections/footer.tmpl")
	if err != nil {
		panic(err)
	}
	ctx.TemplateCache["waitlist_success.tmpl"] = waitlistS

	return nil
}

func maybeRebuildCache(ctx *config.AppContext) error {
	if !ctx.ReloadCache() {
		return nil
	}

	return BuildTemplateCache(ctx)
}

func Routes(ctx *config.AppContext) (http.Handler, error) {
	r := mux.NewRouter()

	err := BuildTemplateCache(ctx)
	if err != nil {
		return r, err
	}

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
	r.HandleFunc("/check-email", func(w http.ResponseWriter, r *http.Request) {
		CheckEmail(w, r, ctx)
	})
	r.HandleFunc("/stripe-hook", func(w http.ResponseWriter, r *http.Request) {
		StripeHook(w, r, ctx)
	}).Methods("POST")

	r.HandleFunc("/opennode-hook", func(w http.ResponseWriter, r *http.Request) {
		OpenNodeHook(w, r, ctx)
	}).Methods("POST")

	/* serve files from the "static" directory */
	fs := http.FileServer(http.Dir("static"))
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", fs))

	/* special favicon handling */
	err = AddFaviconRoutes(r)

	if err != nil {
		return r, err
	}

	return r, nil
}

func TixCount(availSeats uint) []types.OptionItem {
	var items []types.OptionItem
	for i := uint(1); i <= availSeats; i++ {
		opt := types.OptionItem{
			Key:   strconv.FormatUint(uint64(i), 10),
			Value: strconv.FormatUint(uint64(i), 10),
		}
		items = append(items, opt)
	}
	return items
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
		{Key: string(types.Bitcoin), Value: fmt.Sprintf("USD $%d", types.BtcPrice(amount))},
		{Key: string(types.Fiat), Value: fmt.Sprintf("USD $%d", types.FiatPrice(amount))},
	}
}

func AvailOnline(avail []types.CourseAvail) bool {
	for _, a := range avail {
		if a.SelfPacedOnline() {
			return true
		}
	}
	return false
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

		pageTpl := ctx.TemplateCache["register.tmpl"]

		/* token! */
		now := time.Now().UTC().UnixNano()
		idemToken := getSessionToken(ctx.Env.SecretBytes(), session.ID, now, session.Cost)

		w.Header().Set("Content-Type", "text/html")
		err = pageTpl.Execute(w, RegistrationData{
			Course:  course,
			Session: session,
			Page:    getPage(ctx, "Course Registration"),
			Form: types.ClassRegistration{
				Idempotency: idemToken,
				Timestamp:   strconv.FormatInt(now, 10),
				SessionUUID: session.ID,
				Cost:        session.Cost,
				PromoURL:    course.PromoURL,
				CourseName:  course.PublicName,
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

	checkout := &types.Checkout{
		Price:       form.Cost,
		Type:        form.CheckoutVia,
		Idempotency: form.Idempotency,
		SessionID:   sessionID,
		Email:       form.Email,
		PromoURL:    form.PromoURL,
		CourseName:  form.CourseName,
		Count:       uint64(form.Count),
	}

	// FIXME: keep track of token usage//timeout?

	/* Save to signups! Note: won't be considered final
	 * until there's a payment ref attached */
	checkout.RegisterID, err = getters.SaveRegistration(ctx.Notion, &form, checkout)

	// TODO: what happens if there's a duplicate/idempotent token?
	if err != nil {
		http.Error(w, "Oops, we weren't able to save", http.StatusInternalServerError)
		ctx.Err.Printf("/register Unable to save registration %s\n", err.Error())
		return
	}

	/* Ok, now we go to checkout! */
	switch form.CheckoutVia {
	case types.Bitcoin:
		BtcCheckoutStart(w, r, ctx, checkout)
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
		idemToken := getSessionToken(ctx.Env.SecretBytes(), session.ID, now, uint64(0))
		//pageTpl := ctx.TemplateCache["waitlist.tmpl"]
		waitlist, err := template.New("waitlist").Funcs(template.FuncMap{
			"FiatPrice": types.FiatPrice,
			"LastIdx":   LastIdx,
			"BtcPrice":  types.BtcPrice,
		}).ParseFiles("templates/waitlist.tmpl", "templates/sections/head.tmpl", "templates/sections/footer.tmpl", "templates/sections/nav.tmpl")
		if err != nil {
			panic(err)
		}
		err = waitlist.ExecuteTemplate(w, "waitlist.tmpl", WaitlistData{
			Course:  course,
			Session: session,
			Page:    getPage(ctx, "Course Waitlist"),
			Form: types.WaitList{
				Idempotency: idemToken,
				SessionUUID: session.ID,
				Timestamp:   strconv.FormatInt(now, 10),
				PromoURL:    course.PromoURL,
				CourseName:  course.PublicName,
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
	if !checkToken(form.Idempotency, ctx.Env.SecretBytes(), form.SessionUUID, form.Timestamp, uint64(0)) {
		http.Error(w, "Invalid session token", http.StatusBadRequest)
		ctx.Err.Printf("/waitlist invalid session token %s\n", form.Idempotency)
		return
	}

	/* Check that not already saved to waitlist */
	/* FIXME: also check contact info + session UUID? */
	onlist, err := getters.CheckIdemWaitlist(ctx.Notion, form.Idempotency)
	if err != nil {
		http.Error(w, "Unable to check waitlist status", http.StatusInternalServerError)
		ctx.Err.Printf("/waitlist idem chck failed %s\n", err.Error())
		return
	}

	if !onlist {
		/* Save to waitlist! */
		err = getters.SaveWaitlist(ctx.Notion, &form)
		if err != nil {
			http.Error(w, "Unable to save waitlist", http.StatusInternalServerError)
			ctx.Err.Printf("/waitlist save failed %s\n", err.Error())
			return
		}

		/* Send a confirmation email! */
		err = emails.SendWaitlistEmail(ctx, form.Idempotency, form.Email, course, session)
		if err != nil {
			http.Error(w, "Unable to send waitlist email confirmation", http.StatusInternalServerError)
			ctx.Err.Printf("/waitlist send confirmation failed %s\n", err.Error())
			return
		}
	}

	/* Show waitlist success */
	waitTmpl, err := template.ParseFiles("templates/waitlist_success.tmpl", "templates/sections/head.tmpl", "templates/sections/nav.tmpl", "templates/sections/footer.tmpl")
	if err != nil {
		panic(err)
	}
	err = waitTmpl.ExecuteTemplate(w, "waitlist_success.tmpl", &SuccessData{
		Course:  course,
		Session: session,
		Page:    getPage(ctx, ""),
	})
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/success execute template failed %s\n", err.Error())
	}
}

type StripeCheckout struct {
	ClientSecret string
	PubKey       string
	Email        string
	SessionID    string
	PromoURL     string
	CourseName   string
	Count        uint64
	Total        uint64
	Page         Page
}

func FiatCheckoutStart(w http.ResponseWriter, r *http.Request, ctx *config.AppContext, checkout *types.Checkout) {
	stripe.Key = ctx.Env.Stripe.Key

	/* convert to cents for stripe! */
	price := checkout.ComputeTotal(types.Fiat)
	priceAsCents := int64(price * 100)

	/* First we register a payment intent */
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(priceAsCents),
		Currency: stripe.String(string(stripe.CurrencyUSD)),
		AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
			Enabled: stripe.Bool(true),
		},
		/* Sends customer a receipt from Stripe */
		ReceiptEmail: &checkout.Email,
	}

	params.AddMetadata("b58_registration_id", checkout.RegisterID)
	if ctx.Env.Stripe.IsTest() {
		params.AddMetadata("integration_check", "accept_a_payment")
	}

	pi, _ := paymentintent.New(params)

	tmpl, err := template.ParseFiles("templates/checkout.tmpl", "templates/sections/head.tmpl", "templates/sections/nav.tmpl", "templates/sections/footer.tmpl")
	if err != nil {
		panic("oh no")
	}
	err = tmpl.ExecuteTemplate(w, "checkout.tmpl", &StripeCheckout{
		ClientSecret: pi.ClientSecret,
		PubKey:       ctx.Env.Stripe.Pubkey,
		Email:        checkout.Email,
		SessionID:    checkout.SessionID,
		Page:         getPage(ctx, "Checkout"),
		PromoURL:     checkout.PromoURL,
		CourseName:   checkout.CourseName,
		Total:        price,
		Count:        checkout.Count,
	})
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/checkout tmpl execute failed %s\n", err.Error())
	}
}

func BtcCheckoutStart(w http.ResponseWriter, r *http.Request, ctx *config.AppContext, checkout *types.Checkout) {
	payment, err := getters.InitOpenNodeCheckout(ctx, &ctx.Env.OpenNode, checkout)

	if err != nil {
		http.Error(w, "unable to init btc payment", http.StatusInternalServerError)
		ctx.Err.Printf("opennode payment init failed", err.Error())
		return
	}

	/* FIXME: v2: implement on-site btc checkout */
	/* for now we go ahead and just redirect to opennode, see you latrrr */
	ctx.Infos.Println(payment)
	http.Redirect(w, r, payment.HostedCheckoutURL, http.StatusFound)
}

type SuccessData struct {
	Course  *types.Course
	Session *types.CourseSession
	Page    Page
}

func CheckEmail(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	sessionUUID := "apr23-tx-inperson"
	course, session, err := getters.GetSessionInfo(ctx.Notion, sessionUUID)
	if err != nil {
		ctx.Err.Printf("/check-email unable to get sessioninfo from notion %s", sessionUUID)
		return
	}

	welcomeEmail, welcomeText, err := emails.Build(ctx, course.WelcomeEmail, course, session)
	if err != nil {
		ctx.Err.Printf("/check-email unable to build email %v", err)
		return
	}

	/* FIXME: make a receipt file */
	mail := &emails.Mail{
		JobKey:   "testkey" + strconv.Itoa(int(time.Now().UTC().Unix())),
		Email:    "niftynei@gmail.com",
		Title:    fmt.Sprintf("Your Registration for Base58's %s", course.PublicName),
		SendAt:   time.Now(),
		HTMLBody: welcomeEmail,
		TextBody: welcomeText,
	}

	err = emails.ComposeAndSendMail(ctx, mail)
	if err != nil {
		ctx.Err.Printf("/check-email unable to send mail %s", err)
		return
	}
	w.Write(welcomeEmail)
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

	success, err := template.New("success").Funcs(template.FuncMap{
		"LastIdx": LastIdx,
	}).ParseFiles("templates/success.tmpl", "templates/sections/head.tmpl", "templates/sections/nav.tmpl", "templates/sections/footer.tmpl")
	if err != nil {
		panic(err)
	}
	err = success.ExecuteTemplate(w, "success.tmpl", &SuccessData{
		Course:  course,
		Session: session,
		Page:    getPage(ctx, ""),
	})
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/success execute template failed %s\n", err.Error())
	}
}

func finalizeRegistration(ctx *config.AppContext, pageID, refID string) error {

	/* Update cart with payment details */
	sessionUUID, confirmed, err := getters.FinalizeRegistration(ctx.Notion, pageID, refID)
	if err != nil {
		return err
	}

	/* Decrement available class count */
	err = getters.CountClassRegistration(ctx.Notion, sessionUUID, confirmed.Count)
	if err != nil {
		return err
	}

	/* Send email confirming class registration! */
	course, session, err := getters.GetSessionInfoUUID(ctx.Notion, sessionUUID)
	if err != nil {
		return err
	}

	return emails.SendRegistrationEmail(ctx, course, session, confirmed)
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

	event, err := webhook.ConstructEvent(payload, r.Header.Get("Stripe-Signature"), ctx.Env.Stripe.EndpointSec)

	if err != nil {
		ctx.Err.Println("Error verifying webhook sig", err)
		w.WriteHeader(http.StatusBadRequest)
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
		pageID := payment.Metadata["b58_registration_id"]
		refID := payment.ID

		if pageID == "" {
			/* no registration id means not a base58 payment...*/
			break
		}

		err = finalizeRegistration(ctx, pageID, refID)
		if err != nil {
			http.Error(w, "Unable to process, please try again later", http.StatusBadRequest)
			ctx.Err.Printf("/stripe-hook unable to finalize signup %s %s\n", pageID, err.Error())
			return
		}

		ctx.Infos.Println("great success!")
	default:
		ctx.Err.Printf("/stripe-hook unhandled event type %s\n", event.Type)
	}

	w.WriteHeader(http.StatusOK)
}

func computeHash(key, id string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(id))
	return hex.EncodeToString(mac.Sum(nil))
}

func validHash(key, id, msgMAC string) bool {
	actual := computeHash(key, id)
	return msgMAC == actual
}

type ChargeEvent struct {
	ID          string `schema:"id"`
	Status      string `schema:"status"`
	OrderID     string `schema:"order_id"`
	HashedOrder string `schema:"hashed_order"`
}

var decoder = schema.NewDecoder()

func OpenNodeHook(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	err := r.ParseForm()
	if err != nil {
		ctx.Err.Printf("Error reading request body: %v\n", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var ev ChargeEvent
	decoder.IgnoreUnknownKeys(true)
	err = decoder.Decode(&ev, r.PostForm)
	if err != nil {
		ctx.Err.Printf("Unable to unmarshal: %v\n", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	/* Check the hashed order is ok */
	if !validHash(ctx.Env.OpenNode.Key, ev.ID, ev.HashedOrder) {
		ctx.Err.Printf("Invalid request from opennode %s %s", ev.ID, ev.HashedOrder)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	/* If it's paid, update and add seats */
	if ev.Status == "paid" {
		err = finalizeRegistration(ctx, ev.OrderID, ev.ID)
		if err != nil {
			ctx.Err.Printf("/opennode-hook unable to update signup %s %s\n", ev.OrderID, err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		ctx.Infos.Println("opennode great success!")
	} else {
		/* Everything else we log + ignore */
		ctx.Infos.Printf("received opennode update %s %s %s", ev.Status, ev.ID, ev.OrderID)
	}

	w.WriteHeader(http.StatusOK)
}

func Home(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	data, err := getHomeData(ctx, ctx.Notion)
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

type sessionList []*types.CourseSession

func (s sessionList) Len() int {
	return len(s)
}

func (s sessionList) Less(i, j int) bool {

	if len(s[i].Date) == 0 {
		return true
	}
	if len(s[j].Date) == 0 {
		return false
	}

	/* Sort by time first */
	return s[i].Dates()[0].Before(s[j].Dates()[0])
}

func (s sessionList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
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
			/* FIXME: generalize? */
			var bundled []*types.Course
			if clss == "tx-deep-dive" {
				bundled = append(bundled, course)
				for _, c := range courses {
					if strings.HasPrefix(c.TmplName, course.TmplName) {
						bundled = append(bundled, c)
					}
				}
			} else {
				bundled = []*types.Course{course}
			}
			var sessions sessionList
			sessions, err = getters.GetCourseSessions(ctx.Notion, bundled)

			if err != nil {
				http.Error(w, "Unable to load page", http.StatusInternalServerError)
				ctx.Err.Printf("/courses course sessions fetch failed %s\n", err.Error())
				return
			}

			/* Filter out anything in the past or happening in the next 1hr */
			sessions = helpers.FilterSessions(sessions, time.Now())

			/* Sort sessions by date, soonest first */
			sort.Sort(sessions)

			t := ctx.TemplateCache["course.tmpl"]
			err = t.ExecuteTemplate(w, "course.tmpl", CourseData{
				Course:   course,
				Sessions: sessions,
				Page:     getPage(ctx, course.PublicName),
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
	Title     string
	Copyright int
	Domain    string
	Callbacks string
}

type pageData struct {
	Page    Page
	Courses []*types.Course
	Current []*types.Course
	Coming  []*types.Course
}

func getPage(ctx *config.AppContext, title string) Page {
	if title == "" {
		title = "Base58"
	}
	return Page{
		Title:     title,
		Copyright: time.Now().Year(),
		Domain:    ctx.SitePath(),
		Callbacks: ctx.CallbackPath(),
	}
}

func getHomeData(ctx *config.AppContext, n *types.Notion) (pageData, error) {
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
		Page:    getPage(ctx, ""),
		Courses: courses,
		Current: current,
		Coming:  coming,
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
