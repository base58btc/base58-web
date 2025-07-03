package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
	"github.com/kodylow/base58-website/checkout"
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

var webpages []string = []string{"about", "courses", "404", "401", "workshop", "contact", "index", "workshop/book", "workshop/become"}

/* Thank you StackOverflow https://stackoverflow.com/a/50581032 */
func findAndParseTemplates(rootDir string, funcMap template.FuncMap) (*template.Template, error) {
    cleanRoot := filepath.Clean(rootDir)
    pfx := len(cleanRoot)+1
    root := template.New("")

    err := filepath.Walk(cleanRoot, func(path string, info os.FileInfo, e1 error) error {
        if !info.IsDir() && strings.HasSuffix(path, ".tmpl") {
            if e1 != nil {
                return e1
            }

            b, e2 := ioutil.ReadFile(path)
            if e2 != nil {
                return e2
            }

            name := path[pfx:]
            t := root.New(name).Funcs(funcMap)
            _, e2 = t.Parse(string(b))
            if e2 != nil {
                return e2
            }
        }

        return nil
    })

    return root, err
}

func BuildTemplateCache(ctx *config.AppContext) error {

	var err error
	funcMap := template.FuncMap{
		"LastIdx":     LastIdx,
		"FiatPrice":   types.FiatPrice,
		"BtcPrice":    types.BtcPrice,
		"AvailOnline": AvailOnline,
		"ShirtOpts": ShirtOptions,
		"TixCount":  TixCount,
		"toHTML": func(s string) template.HTML {
			b := helpers.ConvertMdToHTML(ctx, "prereq", s)
			return template.HTML(string(b))
		},
		"toText": func(s string) template.HTML {
			b := helpers.ConvertMdToHTML(ctx, "text", s)
			return template.HTML(string(b))
		},
		"ishtml": func(s string) template.HTML {
			return template.HTML(s)
		},
	}
	ctx.TemplateCache, err = findAndParseTemplates("templates", funcMap)
	return err
}

func RegisterCheckoutTypes() {
	checkout.RegisterCartSerialization()
	gob.Register(CourseItem{})
}

func registerLNURL(ctx *config.AppContext, r *mux.Router) {
	/* LNURL hack oof */
	/* This goes to chain.fail (on nixbox) which then fwds to nodebox */
	r.HandleFunc("/.well-known/lnurlp/{user:hello|nifty|niftynei|pay|zap}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://chain.fail/based-lnurl/lnurl", http.StatusSeeOther)
	})
	r.HandleFunc("/lnurl_api/lnurl", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://chain.fail/based-lnurl/lnurl", http.StatusSeeOther)
	})
	r.HandleFunc("/lnurl_api/invoice", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://chain.fail/based-lnurl/invoice?"+r.URL.Query().Encode(), http.StatusSeeOther)
	})

}

func Routes(ctx *config.AppContext) (http.Handler, error) {
	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(func (w http.ResponseWriter, r *http.Request) {
		handle404(w, r, ctx)
	})

	err := BuildTemplateCache(ctx)
	if err != nil {
		return r, err
	}

	registerLNURL(ctx, r)

	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		RenderPage(w, r, ctx, "index")
	}).Methods("GET")

	/* List of 'normie' pages */
	for _, page := range webpages {
		/* Normie Pages */
		renderPage := page
		r.HandleFunc("/"+renderPage, func(w http.ResponseWriter, r *http.Request) {
			RenderPage(w, r, ctx, renderPage)
		}).Methods("GET")
	}

	r.HandleFunc("/courses/larp", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/workshop", http.StatusSeeOther)
	})
	r.HandleFunc("/courses/{course}", func(w http.ResponseWriter, r *http.Request) {
		Courses(w, r, ctx)
	})

	for _, page := range types.TextPages {
		renderPage := page
		r.HandleFunc("/"+renderPage.Tag, func(w http.ResponseWriter, r *http.Request) {
			RenderTextPage(w, r, ctx, renderPage)
		}).Methods("GET")
	}
	
	r.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		Register(w, r, ctx)
	})
	r.HandleFunc("/reserve", func(w http.ResponseWriter, r *http.Request) {
		Reserve(w, r, ctx)
	})
	r.HandleFunc("/summary", func(w http.ResponseWriter, r *http.Request) {
		Summary(w, r, ctx, nil)
	})
	r.HandleFunc("/success", func(w http.ResponseWriter, r *http.Request) {
		Success(w, r, ctx)
	})
	r.HandleFunc("/check-email", func(w http.ResponseWriter, r *http.Request) {
		CheckEmail(w, r, ctx)
	})
	r.HandleFunc("/workshop/contact/{formtype}", func(w http.ResponseWriter, r *http.Request) {
		WorkshopContact(w, r, ctx)
	}).Methods("POST")
	r.HandleFunc("/contact/ok", func(w http.ResponseWriter, r *http.Request) {
		FormContact(w, r, ctx)
	}).Methods("POST")
	r.HandleFunc("/{newsletter}/subscribe", func(w http.ResponseWriter, r *http.Request) {
		SubscribeEmail(w, r, ctx)
	}).Methods("POST")

	r.HandleFunc("/confirm/{token}", func(w http.ResponseWriter, r *http.Request) {
		ConfirmEmail(w, r, ctx)
	}).Methods("GET")

	r.HandleFunc("/newsletter/unsubscribe/{token}", func(w http.ResponseWriter, r *http.Request) {
		UnsubscribeEmail(w, r, ctx)
	}).Methods("GET")

	r.HandleFunc("/stripe-hook", func(w http.ResponseWriter, r *http.Request) {
		StripeHook(w, r, ctx)
	}).Methods("POST")

	r.HandleFunc("/opennode-hook", func(w http.ResponseWriter, r *http.Request) {
		OpenNodeHook(w, r, ctx)
	}).Methods("POST")

	r.HandleFunc("/tools/wif", func(w http.ResponseWriter, r *http.Request) {
		WIF(w, r, ctx)
	})

	r.HandleFunc("/tools/keyaddr", func(w http.ResponseWriter, r *http.Request) {
		KeyAddr(w, r, ctx)
	})

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

func handle404(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	w.WriteHeader(http.StatusNotFound)
	RenderPage(w, r, ctx, "404")
	ctx.Err.Printf("404 err: %s", r.URL.Path)
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

type WorkshopData struct {
	Page   Page
	Course *types.Course
}

type RegistrationData struct {
	Course        *types.Course
	Sessions      []*SessionOption
	DefaultSelect string
	HasCode       bool
	KeyCode       string
	Count         uint
	Page          Page
}

type SummaryData struct {
	Page     Page
	Cart     checkout.Cart
	SubTotal checkout.Price
	Discount checkout.Price
	Taxes    checkout.Price
	Shipping *checkout.Price
	Total    checkout.Price
}

type ReservationData struct {
	Course        *types.Course
	Sessions      []*SessionOption
	DefaultSelect string
	DefaultQty    string
	SeatOpts      []string
	Page          Page
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

type SessionOption struct {
	UUID       string
	OptionDesc string
	Cost       uint64
	Date       []string
	TimeDesc   string
	Location   string
	Instructor string
	IdemKey    string
	AvailSeats uint
}

func needsSessionCode(sessions []*types.CourseSession) bool {
	for _, sesh := range sessions {
		if sesh.SignupCode != "" {
			return true
		}
	}
	return false
}

func makeSessionOptions(ctx *config.AppContext, sessions []*types.CourseSession) []*SessionOption {
	var opts []*SessionOption
	for _, sesh := range sessions {
		/* token! */
		now := time.Now().UTC().UnixNano()
		idemToken := getSessionToken(ctx.Env.SecretBytes(), sesh.ID, now, sesh.Cost)

		/* add the timestamp + cost onto the token end */
		idemToken += ":" + strconv.FormatInt(now, 10)
		idemToken += ":" + strconv.FormatUint(sesh.Cost, 10)
		idemToken += ":" + sesh.ID

		opt := &SessionOption{
			UUID:       idemToken,
			OptionDesc: sesh.FormatDateRange(),
			Cost:       sesh.Cost,
			Date:       sesh.Date,
			TimeDesc:   sesh.TimeDesc,
			Location:   sesh.Location,
			Instructor: sesh.Instructor,
			AvailSeats: sesh.SeatsAvail,
		}
		opts = append(opts, opt)
	}

	return opts
}

func Register(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	courseID, ok := getSessionKey("c", r)

	if !ok {
		/* If there's no session-key, redirect to the front page */
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	var sessions sessionList
	course, sessions, err := getters.GetCourseInfo(ctx.Notion, courseID)
	if err != nil || len(sessions) == 0 {
		http.Error(w, "Unable to load page, please try again later", http.StatusInternalServerError)
		ctx.Err.Printf("/register failed to fetch course sessions %s\n", err.Error())
		return
	}

	registerType, _ := getSessionKey("t", r)

	if r.Method == http.MethodGet {
		title := "Course Registration"
		furlCard := defaultCard(ctx, r, title)

		/* Filter out anything in the past or happening in the next 1hr */
		sessions = helpers.FilterSessions(sessions, time.Now())

		keycode, _ := getSessionKey("key", r)

		/* Sort sessions by date, soonest first */
		sort.Sort(sessions)

		/* Leave out sessions where available seats < 6 */
		var seatCount uint
		if registerType == "table" {
			sessions = helpers.FilterSessionsByAvail(sessions, 6)
			seatCount = 6
		} else {
			/* Leave out sessoins w/o any available seats */
			sessions = helpers.FilterWaitlistSessions(sessions)
			seatCount = 1
		}

		w.Header().Set("Content-Type", "text/html")
		sessionOpts := makeSessionOptions(ctx, sessions)

		defaultSession := ""
		if len(sessionOpts) > 0 {
			defaultSession = sessionOpts[0].UUID
		}
		err = ctx.TemplateCache.ExecuteTemplate(w, "course.tmpl", RegistrationData{
			Course:        course,
			DefaultSelect: defaultSession,
			Sessions:      sessionOpts,
			HasCode:       needsSessionCode(sessions),
			KeyCode:       keycode,
			Count:         seatCount,
			Page:          getPage(ctx, title, furlCard),
		})

		if err != nil {
			http.Error(w, "Unable to load page, please try again later", http.StatusInternalServerError)
			ctx.Err.Printf("/register templ exec failed %s\n", err.Error())
		}
		return
	}

	if r.Method != http.MethodPost {
		handle404(w, r, ctx)
		return
	}

	r.ParseForm()
	dec := schema.NewDecoder()
	dec.IgnoreUnknownKeys(true)
	var form types.ClassRegistration
	err = dec.Decode(&form, r.PostForm)
	if err != nil {
		http.Error(w, "Unable to load page, please try again later", http.StatusInternalServerError)
		ctx.Err.Printf("/register unable to decode class registrattion %s\n", err.Error())
		return
	}

	/* Check that the Idempotency token is valid */
	infos := strings.Split(form.Session, ":")
	if len(infos) != 4 {
		http.Error(w, "Invalid session token", http.StatusBadRequest)
		ctx.Err.Printf("/register not a good session token \n")
		return
	}

	idem := infos[0]
	time := infos[1]
	cost, err := strconv.ParseUint(infos[2], 10, 64)
	if err != nil {
		http.Error(w, "Invalid session token", http.StatusBadRequest)
		ctx.Err.Printf("/register session token cost did not parse %s\n", form.Session)
	}
	sessionUUID := infos[3]

	// FIXME: keep track of token usage//timeout?
	if !checkToken(idem, ctx.Env.SecretBytes(), sessionUUID, time, cost) {
		http.Error(w, "Invalid session token", http.StatusBadRequest)
		ctx.Err.Printf("/register not a good session token %s\n", form.Session)
		return
	}

	var session *types.CourseSession
	for _, sesh := range sessions {
		if sessionUUID == sesh.ID {
			session = sesh
			break
		}
	}
	if session == nil {
		http.Error(w, "Invalid session token", http.StatusBadRequest)
		ctx.Err.Printf("/cant find session token %s\n", sessionUUID)
		return
	}

	cc := &types.Checkout{
		Price:       cost,
		Type:        form.CheckoutVia,
		Idempotency: idem,
		SessionID:   sessionUUID,
		Email:       form.Email,
		PromoURL:    course.PromoURL(ctx.Env.Domain),
		CourseName:  course.Title,
		Count:       uint64(form.Count),
		Session:     session,
		/* FIXME: auto matically gives a discount of 1 to 6 seats */
		DiscountCode: "6c-1",
	}

	/* Save to signups! Note: won't be considered final
	 * until there's a payment ref attached */
	cc.RegisterID, err = getters.SaveRegistration(ctx.Notion, idem, sessionUUID, &form, cc)

	// TODO: what happens if there's a duplicate/idempotent token?
	if err != nil {
		http.Error(w, "Oops, we weren't able to save", http.StatusInternalServerError)
		ctx.Err.Printf("/register Unable to save registration %s\n", err.Error())
		return
	}

	/* Verify that the code is OK */
	if session.SignupCode != "" && strings.ToLower(form.SignupCode) != strings.ToLower(session.SignupCode) {
		courseURL := course.PromoURL(ctx.Env.Domain)
		errMsg := fmt.Sprintf("You haven't applied yet! Apply here: <a href=\"%s\">%s</a>", courseURL, courseURL)
		http.Error(w, errMsg, http.StatusNotFound)
		ctx.Err.Printf("/register %s\n", form.CheckoutVia)
		return
	}

	/* Ok, now we go to checkout! */
	switch form.CheckoutVia {
	case types.Bitcoin:
		openCheckout := getters.OpenCheckoutInfo{
			Amount:      float64(cc.ComputePrice(types.Bitcoin)),
			Description: cc.MakeDesc(),
			Email:       cc.Email,
			OrderID:     cc.RegisterID,
			SuccessID:   cc.SessionID,
		}
		BtcCheckoutStart(w, r, ctx, openCheckout)
		return

	case types.Fiat:
		item := ConvertToItem(session, ctx, uint(form.Count))
		cart := &checkout.Cart{
			Token: idem,
			Infos: &checkout.CheckoutData{
				Email: form.Email,
			},
			Items:  []checkout.CartItem{item},
			BtcOk:  true,
			FiatOk: true,
		}
		FiatCheckoutStart(w, r, ctx, cart)
		return

	default:
		handle404(w, r, ctx)
		ctx.Err.Printf("/register unable to find checkout method %s\n", form.CheckoutVia)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
	return
}

/* Separate checkout for Workshop */
func Reserve(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	courseID, ok := getSessionKey("c", r)

	if !ok {
		/* If there's no session-key, redirect to the front page */
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	var sessions sessionList
	course, sessions, err := getters.GetCourseInfo(ctx.Notion, courseID)
	if err != nil || len(sessions) == 0 {
		http.Error(w, "Unable to load page, please try again later", http.StatusInternalServerError)
		ctx.Err.Printf("/reserve failed to fetch course sessions %s\n", err.Error())
		return
	}

	reservationOption, _ := getSessionKey("t", r)
	defaultQty := uint(1)
	if reservationOption == "table" {
		defaultQty = uint(6)
	}

	if r.Method == http.MethodGet {
		furlCard := courseCard(ctx, r, course)

		/* Filter out anything in the past or happening in the next 1hr */
		sessions = helpers.FilterSessions(sessions, time.Now())

		/* Sort sessions by date, soonest first */
		sort.Sort(sessions)

		/* Leave out sessoins w/o any available seats */
		sessions = helpers.FilterWaitlistSessions(sessions)

		w.Header().Set("Content-Type", "text/html")
		sessionOpts := makeSessionOptions(ctx, sessions)

		err = ctx.TemplateCache.ExecuteTemplate(w, "reserve.tmpl", ReservationData{
			Course:        course,
			DefaultSelect: sessionOpts[0].UUID,
			DefaultQty:    strconv.Itoa(int(defaultQty)),
			SeatOpts:      []string{"1", "2", "3", "4", "5", "6"},
			Sessions:      sessionOpts,
			Page:          getPage(ctx, course.Title, furlCard),
		})

		if err != nil {
			ctx.Err.Printf("/reserve templ exec failed %s\n", err.Error())
		}
		return
	}

	if r.Method != http.MethodPost {
		handle404(w, r, ctx)
		return
	}

	r.ParseForm()
	dec := schema.NewDecoder()
	dec.IgnoreUnknownKeys(true)
	var form types.ClassRegistration
	err = dec.Decode(&form, r.PostForm)
	if err != nil {
		http.Error(w, "Unable to load page, please try again later", http.StatusInternalServerError)
		ctx.Err.Printf("/reserve unable to decode class registrattion %s\n", err.Error())
		return
	}

	/* Check that the Idempotency token is valid */
	infos := strings.Split(form.Session, ":")
	if len(infos) != 4 {
		http.Error(w, "Invalid session token", http.StatusBadRequest)
		ctx.Err.Printf("/reserve not a good session token \n")
		return
	}

	idem := infos[0]
	time := infos[1]
	cost, err := strconv.ParseUint(infos[2], 10, 64)
	if err != nil {
		http.Error(w, "Invalid session token", http.StatusBadRequest)
		ctx.Err.Printf("/reserve session token cost did not parse %s\n", form.Session)
	}
	sessionUUID := infos[3]

	// FIXME: keep track of token usage//timeout?
	if !checkToken(idem, ctx.Env.SecretBytes(), sessionUUID, time, cost) {
		http.Error(w, "Invalid session token", http.StatusBadRequest)
		ctx.Err.Printf("/reserve not a good session token %s\n", form.Session)
		return
	}

	var session *types.CourseSession
	for _, sesh := range sessions {
		if sessionUUID == sesh.ID {
			session = sesh
			break
		}
	}

	if session == nil {
		http.Error(w, "Invalid session identifier", http.StatusBadRequest)
		ctx.Err.Printf("/cant find session token %s\n", sessionUUID)
		return
	}

	item := ConvertToItem(session, ctx, uint(form.Count))
	cart := &checkout.Cart{
		Token: idem,
		Infos: &checkout.CheckoutData{
			Email: form.Email,
		},
		Items:     []checkout.CartItem{item},
		Discounts: []string{"6c-1"},
		BtcOk:     true,
		FiatOk:    true,
	}

	/* Save to signups! Won't be considered final until there's a payment ref attached */
	cart.LookupID, err = getters.SaveCart(ctx.Notion, cart)

	// TODO: what happens if there's a duplicate/idempotent token?
	if err != nil {
		http.Error(w, "Oops, we weren't able to save cart", http.StatusInternalServerError)
		ctx.Err.Printf("/reserve Unable to save cart for reservation %s\n", err.Error())
		return
	}

	ctx.Session.Put(r.Context(), "cart", cart)
	Summary(w, r, ctx, cart)
}

func Summary(w http.ResponseWriter, r *http.Request, ctx *config.AppContext, cart *checkout.Cart) {

	if cart == nil {
		_, ok := ctx.Session.Get(r.Context(), "cart").(checkout.Cart)

		if !ok {
			http.Error(w, "No checkout in progrss!", http.StatusInternalServerError)
			ctx.Err.Printf("/summary Unable to load cart for checkout\n")
			return
		}
	}

	if r.Method == http.MethodGet {

		title := "Checkout Summary"
		furlCard := defaultCard(ctx, r, title)

		err := ctx.TemplateCache.ExecuteTemplate(w, "summary.tmpl", &SummaryData{
			Page:     getPage(ctx, title, furlCard),
			Cart:     *cart,
			SubTotal: cart.SubTotal(checkout.USD),
			Discount: cart.Discount(checkout.USD),
			Taxes:    cart.Taxes(checkout.USD),
			Total:    cart.Total(checkout.USD),
		})

		if err != nil {
			http.Error(w, "Unable to load page", http.StatusInternalServerError)
			ctx.Err.Printf("/summary tmpl execute failed %s\n", err.Error())
		}
		return
	}

	if r.Method != http.MethodPost {
		handle404(w, r, ctx)
		return
	}

	r.ParseForm()
	dec := schema.NewDecoder()
	dec.IgnoreUnknownKeys(true)
	var form checkout.SummaryPage
	err := dec.Decode(&form, r.PostForm)
	if err != nil {
		http.Error(w, "Unable to load page, please try again later", http.StatusInternalServerError)
		ctx.Err.Printf("/reserve unable to decode class registrattion %s\n", err.Error())
		return
	}

	switch form.CheckoutVia {
	case checkout.Bitcoin:
		amtVal := cart.Total(checkout.USD)
		/* OPennode expects $00.00) */
		amount := float64(amtVal.Value / 100)
		openCheckout := getters.OpenCheckoutInfo{
			Amount:      amount,
			Description: cart.MakeDesc(),
			Email:       cart.Infos.Email,
			OrderID:     cart.LookupID,
			/* FIXME: better way to show display endpage? */
			SuccessID: cart.Items[0].GetID(),
		}
		BtcCheckoutStart(w, r, ctx, openCheckout)
		return
	case checkout.Fiat:
		FiatCheckoutStart(w, r, ctx, cart)
		return
	default:
		handle404(w, r, ctx)
		ctx.Err.Printf("/register unable to find checkout method %s\n", form.CheckoutVia)
		return
	}
	if true {
		w.Write([]byte(cart.LookupID))
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
	return

}

func getSubscribeToken(sec []byte, email, newsletter string, timestamp uint64) (string, string) {
	/* Make a lil hash using the email + timestamp + newsletter */
	h := sha256.New()
	h.Write(sec)
	h.Write([]byte(email))
	h.Write([]byte(newsletter))
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, timestamp)
	h.Write(b)

	/* Token is 8-bytes hash prefix, hex of email,
	 * hex of newsletter, hex of timestamp 
	 */

	hashB := h.Sum(nil)
	hash := hex.EncodeToString(hashB[:8])
	emailHex := hex.EncodeToString([]byte(email))
	subHex := hex.EncodeToString([]byte(newsletter))
	timeHex := hex.EncodeToString(b)
	return hash, fmt.Sprintf("%s-%s-%s-%s", hash, emailHex, subHex, timeHex)
}

type SubToken struct {
	Time       time.Time
	Email      string
	Newsletter string
}

func parseSubscribeToken(sec []byte, token string) (*SubToken, error) {
	parts := strings.Split(token, "-")
	if len(parts) != 4 {
		return nil, fmt.Errorf("Invalid token format %s", token)
	}
	
	emailB, err := hex.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	subB, err := hex.DecodeString(parts[2])
	if err != nil {
		return nil, err
	}
	timeB, err := hex.DecodeString(parts[3])
	if err != nil {
		return nil, err
	}
	timestamp := binary.LittleEndian.Uint64(timeB)
	hash, _ := getSubscribeToken(sec, string(emailB), string(subB), timestamp)
	if hash != parts[0] {
		return nil, fmt.Errorf("Invalid token %s", token)
	}

	return &SubToken{ 
		Time: time.Unix(0, int64(timestamp)),
		Email: string(emailB),
		Newsletter: string(subB),
	}, nil
}

func FormContact(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	r.ParseForm()
	email := r.Form.Get("contact-email")
	message := r.Form.Get("contact-message")

	/* Validate email */
	if _, err := mail.ParseAddress(email); err != nil {
		w.Write([]byte(fmt.Sprintf(`
		<div class="error-message w-form-fail" style="display: block;">
                  <div class="error-text">"%s" is not a valid email.</div>
                </div>
		`, email)))
		return
	}

	/* Send hello@base58.school + the email the message */
	_, err := emails.SendContactEmail(ctx, "hello@base58.school", message, email, "contact")	
	if err != nil {
		ctx.Err.Printf("Failed sending self message: %s", err)
		if strings.Contains(err.Error(), "scheduled.idem_key") {
			w.Write([]byte(`<div class="error-message w-form-fail" style="display: block;">
			  <div class="error-text">That message has already been sent!</div>
			</div>`))
		} else {
			w.Write([]byte(`<div class="error-message w-form-fail" style="display: block;">
			  <div class="error-text">Server error. Please try again later.</div>
			</div>`))
		}
		return
	}
	
	_, err = emails.SendContactEmail(ctx, email, message, email, "contact")
	if err != nil {
		ctx.Err.Printf("Failed sending %s message: %s", email, err)
		w.Write([]byte(`<div class="error-message w-form-fail" style="display: block;">
                  <div class="error-text">Unable to send mail. Please try again later.</div>
                </div>`))
		return
	}

	ctx.Infos.Printf("Message to get in touch sent!")

	/* show a banner about it sending successfully */
	w.Write([]byte(`<div class="success-message w-form-done" style="display: block;">
	  <div class="success-text">Thank you! Your message has been received!</div>
	</div>`))
}
func WorkshopContact(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	params := mux.Vars(r)
	formtype := params["formtype"]

	r.ParseForm()
	email := r.Form.Get("workshop-email")
	message := r.Form.Get("workshop-message")
	/* Validate email */
	if _, err := mail.ParseAddress(email); err != nil {
		w.Write([]byte(fmt.Sprintf(`
		<div class="error-message w-form-fail" style="display: block;">
                  <div class="error-text">"%s" is not a valid email.</div>
                </div>
		`, email)))
		return
	}

	/* Send hello@base58.school + the email the message */
	_, err := emails.SendContactEmail(ctx, "hello@base58.school", message, email, formtype)	
	if err != nil {
		ctx.Err.Printf("Failed sending self message: %s", err)
		if strings.Contains(err.Error(), "scheduled.idem_key") {
			w.Write([]byte(`<div class="error-message w-form-fail" style="display: block;">
			  <div class="error-text">That message has already been sent!</div>
			</div>`))
		} else {
			w.Write([]byte(`<div class="error-message w-form-fail" style="display: block;">
			  <div class="error-text">Server error. Please try again later.</div>
			</div>`))
		}
		return
	}
	
	_, err = emails.SendContactEmail(ctx, email, message, email, formtype)
	if err != nil {
		ctx.Err.Printf("Failed sending %s message: %s", email, err)
		w.Write([]byte(`<div class="error-message w-form-fail" style="display: block;">
                  <div class="error-text">Unable to send mail. Please try again later.</div>
                </div>`))
		return
	}

	ctx.Infos.Printf("Message to %s Work(shop) sent!", formtype)

	/* show a banner about it sending successfully */
	w.Write([]byte(`<div class="success-message w-form-done" style="display: block;">
	  <div class="success-text">Thank you! Your message has been received!</div>
	</div>`))
}

func SubscribeEmail(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	params := mux.Vars(r)
	newsletter := params["newsletter"]

	r.ParseForm()
	email := r.Form.Get("newsletter-email")
	/* Validate email */
	if _, err := mail.ParseAddress(email); err != nil {
		w.Write([]byte(fmt.Sprintf(`
        	<div class="form_message-error-wrapper w-form-fail" style="display:block;">
                <div class="form_message-error-2">
                <div>"%s" not a valid email. Please try again.</div>
                </div>
                </div>
		`, email)))
		return
	}

	timestamp := uint64(time.Now().UTC().UnixNano())
	_, token := getSubscribeToken(ctx.Env.SecretBytes(), email, newsletter, timestamp)

	ctx.Infos.Printf("%s subscribe token is %s. sending confirmation email", email, token)
	_, err := emails.SendNewsletterSubEmail(ctx, email, token, newsletter)
	if err != nil {
		w.Write([]byte(fmt.Sprintf(`
        	<div class="form_message-error-wrapper w-form-fail" style="display:block;">
                <div class="form_message-error-2">
                <div>Unable to subscribe %s. Please try again.</div>
                </div>
                </div>
		`, email)))
		ctx.Infos.Printf("Unable to send mail to %s: %s", email, err)
		return
	}
	w.Write([]byte(fmt.Sprintf(`
        	<div class="form_message-error-wrapper w-form-done" style="display:block;">
                <div class="form_message-success-4">
                <div>Confirmation email sent to %s. Check your inbox.</div>
                </div>
                </div>
	`, email)))
}

func ConfirmEmail(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	/* If there's no token-key, redirect to the front page */
	params := mux.Vars(r)
	token := params["token"]

	if token == "" {
		ctx.Infos.Printf("No token found for newsletter confirmation request")
		/* Return the homepage page */
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	subToken, err := parseSubscribeToken(ctx.Env.SecretBytes(), token)
	if err != nil {
		ctx.Infos.Printf("Email subscribe token validation failed. %s", err)
		/* Return the homepage page */
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	/* Expiry time is one week */
	expiryTime := subToken.Time.AddDate(0, 0, 7)
	if expiryTime.Before(time.Now()) {
		ctx.Infos.Printf("Email subscribe token too old.")
		/* Return the homepage page */
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return

	}

	/* Add to email list */
	subscriber, err := getters.FindSubscriber(ctx.Notion, subToken.Email)
	if err != nil {
		ctx.Infos.Printf("Subscribe failed for newsletter confirmation request %s: %s", subToken.Email, err)
		/* FIXME: show an error banner or something */
		/* Return the homepage page */
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if subscriber == nil {
		subscriber, err = getters.SubscribeEmail(ctx.Notion, subToken.Email, subToken.Newsletter)
		if err != nil {
			ctx.Infos.Printf("Subscribe failed for newsletter confirmation request %s: %s", subToken.Email, err)
			/* FIXME: show an error banner or something */
			/* Return the homepage page */
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}

	changed := subscriber.AddSubscription(subToken.Newsletter)
	if changed {
		err = getters.UpdateSubs(ctx.Notion, subscriber)
	}

	if err != nil {
		ctx.Infos.Printf("Subscribe failed for newsletter confirmation request %s: %s", subToken.Email, err)
		/* FIXME: show an error banner or something */
		/* Return the homepage page */
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	
	var title, actionText string
	if subToken.Newsletter == "newsletter" {
		title = "Subscribed Success"
		actionText = "subscribed to"
	} else {
		title = "You're on the Waitlist"
		actionText = "added to"
	}
	// Render the template with the data
	furlCard := defaultCard(ctx, r, title)
	err = ctx.TemplateCache.ExecuteTemplate(w, "emails/subscribe.tmpl", &SubscribePage{
		Page: getPage(ctx, title, furlCard),
		Text: title,
		ActionText: actionText,
		Email: subToken.Email,
		Newsletter: subToken.Newsletter,
	})

	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/emails/unsubscribe exec template failed %s\n", err.Error())
		return
	}
}

type SubscribePage struct {
	Page        Page
	Email       string
	Text        string
	ActionText  string
	Newsletter  string
}

func UnsubscribeEmail(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	/* If there's no token-key, redirect to the front page */
	params := mux.Vars(r)
	token := params["token"]

	subToken, err := parseSubscribeToken(ctx.Env.SecretBytes(), token)
	if err != nil {
		ctx.Infos.Printf("Invalid token %s for unsubscribe: %s", token, err)
		/* Return the homepage page */
		RenderPage(w, r, ctx, "index")
		return
	}

	/* Find record for that token */
	subscriber, err := getters.FindSubscriber(ctx.Notion, subToken.Email)
	if err != nil || subscriber == nil {
		ctx.Infos.Printf("No subscriber found for token %s: %s", token, err)
		/* Return the homepage page */
		RenderPage(w, r, ctx, "index")
		return
	}

	changed := subscriber.RmSubscription(subToken.Newsletter)
	if changed {
		err := getters.UpdateSubs(ctx.Notion, subscriber)
		if err != nil {
			ctx.Infos.Printf("Error unsubscribing %s from %s: %s", subscriber.Email, subToken.Newsletter, err)
		} else {
			ctx.Infos.Printf("Unsubscribed %s from %s", subscriber.Email, subToken.Newsletter)
		}
	} else {
		ctx.Infos.Printf("Subscriber %s already unsubscribed from %s", subscriber.Email, subToken.Newsletter)
	}


	// Render the template with the data
	title := "Unsubscribe"
	furlCard := defaultCard(ctx, r, title)
	err = ctx.TemplateCache.ExecuteTemplate(w, "emails/subscribe.tmpl", &SubscribePage{
		Page: getPage(ctx, title, furlCard),
		Email: subscriber.Email,
		Text: "Sorry to see you go",
		ActionText: "unsubscribed from",
		Newsletter: subToken.Newsletter,
	})

	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/emails/subscribe exec template failed %s\n", err.Error())
		return
	}
}

type StripeCheckout struct {
	ClientSecret string
	PubKey       string
	Email        string
	SessionID    string
	PromoURL     string
	CourseName   string
	Desc         string
	Count        uint64
	Total        uint64
	Page         Page
}

func FiatCheckoutStart(w http.ResponseWriter, r *http.Request, ctx *config.AppContext, cart *checkout.Cart) {
	stripe.Key = ctx.Env.Stripe.Key

	/* First we register a payment intent */
	total := cart.Total(checkout.USD)
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(int64(total.Value)),
		Currency: stripe.String(string(stripe.CurrencyUSD)),
		AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
			Enabled: stripe.Bool(true),
		},
		/* Sends customer a receipt from Stripe */
		ReceiptEmail: &cart.Infos.Email,
	}

	params.AddMetadata("b58_registration_id", cart.LookupID)
	if ctx.Env.Stripe.IsTest() {
		params.AddMetadata("integration_check", "accept_a_payment")
	}

	pi, _ := paymentintent.New(params)

	title := fmt.Sprintf("Checkout for %s", cart.Items[0].GetDisplayName())
	furlCard := buildCard(ctx.Env.Domain, title, r.URL.String(), "", cart.Items[0].GetImgURL(), nil)

	err := ctx.TemplateCache.ExecuteTemplate(w, "checkout.tmpl", &StripeCheckout{
		ClientSecret: pi.ClientSecret,
		PubKey:       ctx.Env.Stripe.Pubkey,
		Email:        cart.Infos.Email,
		SessionID:    cart.LookupID,
		Page:         getPage(ctx, "Checkout", furlCard),
		PromoURL:     cart.Items[0].GetImgURL(),
		CourseName:   cart.Items[0].GetDisplayName(),
		Desc:         cart.Items[0].GetDetails(),
		Total:        total.Value / 100,
		Count:        uint64(cart.Items[0].GetQty()),
	})
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/checkout tmpl execute failed %s\n", err.Error())
	}
}

func BtcCheckoutStart(w http.ResponseWriter, r *http.Request, ctx *config.AppContext, checkout getters.OpenCheckoutInfo) {

	ctx.Infos.Printf("callbackpath? %s", ctx.CallbackPath())
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

	email := "nifty@btcpp.dev"
	newsletter := "newsletter"
	timestamp := uint64(time.Now().UTC().UnixNano())

	_, token := getSubscribeToken(ctx.Env.SecretBytes(), email, newsletter, timestamp)
	mail, err := emails.SendNewsletterSubEmail(ctx, email, token, newsletter)
	if err != nil {
		ctx.Err.Printf("/check-email unable to send mail %s", err)
		return
	}
	w.Write(mail)
}

func Success(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	/* Show a success page! */
	sessionUUID, ok := getSessionKey("sid", r)
	if !ok {
		/* If there's no session-key, redirect to the front page */
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	course, session, err := getters.GetSessionInfoUUID(ctx.Notion, sessionUUID)
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/success get session info failed %s\n", err.Error())
		return
	}

	title := "You're Going!"
	extraData := make([]ExtraData, 2)
	extraData[0] = ExtraData{
		Label: "Instructor",
		Data:  session.Instructor,
	}
	extraData[1] = ExtraData{
		Label: "Location",
		Data:  session.Location,
	}

	furlCard := buildCard(ctx.Env.Domain, title, r.URL.String(), course.ShortDesc, session.PromoURL, extraData)

	err = ctx.TemplateCache.ExecuteTemplate(w, "success.tmpl", &SuccessData{
		Course:  course,
		Session: session,
		Page:    getPage(ctx, title, furlCard),
	})
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/success execute template failed %s\n", err.Error())
	}
}

type CourseItem struct {
	checkout.Item
}

func (ci CourseItem) OnCallback(appCtx interface{}, cart *checkout.Cart, paymentID string) error {

	ctx := appCtx.(*config.AppContext)

	course, session, err := getters.GetSessionInfoUUID(ctx.Notion, ci.GetID())
	if err != nil {
		return err
	}

	/* Add seats for registration */
	err = getters.AddClassRegistration(ctx.Notion, paymentID, cart.LookupID, ci.GetID(), cart.Token, cart.Infos.Email, cart.Infos.MailingAddr, cart.Infos.ShirtSize)

	if err != nil {
		return err
	}

	/* Decrement available class count */
	err = getters.CountClassRegistration(ctx.Notion, ci.GetID(), ci.GetQty())
	if err != nil {
		return err
	}

	/* Send email confirming class registration! */
	confirmed := &types.Confirmed{
		Idempotency: cart.Token,
		Email:       cart.Infos.Email,
	}
	_, err = emails.SendRegistrationEmail(ctx, course, session, confirmed)
	return err
}

func completeCheckout(ctx *config.AppContext, cartID, paymentID string) error {
	/* Check if we've already marked this cart as paid */
	alreadyPaid, err := getters.CheckCartNotPaid(ctx.Notion, paymentID)

	if err != nil {
		return err
	}

	if alreadyPaid {
		return fmt.Errorf("Cart marked as already paid (PaymentRef: %s)", paymentID)
	}

	cart, err := getters.MarkCartPaid(ctx.Notion, cartID, paymentID)
	if err != nil {
		return err
	}

	/* For each item, execute the item callback! */
	for _, item := range cart.Items {
		err = item.OnCallback(ctx, cart, paymentID)
		if err != nil {
			ctx.Err.Printf("call back on item %s failed: %s", item.GetDisplayName(), err)
		}
	}

	// TODO: send a 'unified' receipt!!
	// TODO: Notify ~me~ that a cart was paid!
	return nil
}

func ConvertToItem(sesh *types.CourseSession, ctx *config.AppContext, qty uint) *CourseItem {
	return &CourseItem{
		checkout.Item{
			ID:          sesh.ID,
			LookupID:    sesh.ClassRef,
			DisplayName: sesh.CourseName,
			Details:     sesh.Details(),
			Options:     sesh.GetOptionDesc(),
			Qty:         qty,
			Price: checkout.Price{
				/* FIXME: Convert to cents */
				Value: uint64(sesh.Cost * 100),
				/* FIXME: use sats as well? */
				Unit: checkout.USD,
			},
			ImgURL: sesh.PromoURL,
		},
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
		cartID := payment.Metadata["b58_registration_id"]
		refID := payment.ID

		if cartID == "" {
			/* no registration id means not a base58 payment...*/
			break
		}

		err = completeCheckout(ctx, cartID, refID)
		if err != nil {
			http.Error(w, "Unable to process, please try again later", http.StatusBadRequest)
			ctx.Err.Printf("/stripe-hook unable to finalize signup %s %s\n", cartID, err.Error())
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
		err = completeCheckout(ctx, ev.OrderID, ev.ID)
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

func RenderPage(w http.ResponseWriter, r *http.Request, ctx *config.AppContext, page string) {
	data, err := getHomeData(ctx, ctx.Notion)
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/%s data fetch failed %s\n", page, err.Error())
		return
	}

	// Render the template with the data
	template := fmt.Sprintf("%s.tmpl", page)
	err = ctx.TemplateCache.ExecuteTemplate(w, template, data)
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/%s exec template failed %s\n", template, err.Error())
		return
	}
}

type ContentPage struct {
	Page    Page
	Text    string
}

func RenderTextPage(w http.ResponseWriter, r *http.Request, ctx *config.AppContext, tPage types.TextPage) {

	/* Read page from disk (if not in cache) */
	pageBytes, err := ioutil.ReadFile(fmt.Sprintf("templates/text/%s.md", tPage.Tag))
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to load text template templates/text/%s.md", tPage.Tag), http.StatusInternalServerError)
		ctx.Err.Printf("Unable to load text template %s.md\n", tPage.Tag, err.Error())
		return
	}

	// Render the template with the data
	furlCard := defaultCard(ctx, r, tPage.Title)
	err = ctx.TemplateCache.ExecuteTemplate(w, "text/text.tmpl", &ContentPage{
		Text:    string(pageBytes),
		Page:    getPage(ctx, tPage.Title, furlCard),
	})
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("text.tmpl exec failed %s\n", err.Error())
		return
	}
}

type CourseData struct {
	Course     *types.Course
	Sessions   []*types.CourseSession
	SeatsAvail uint
	Page       Page
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

func countSeats(s sessionList) uint {
	var acc uint
	for _, sesh := range s {
		acc += sesh.SeatsAvail
	}

	return acc
}

func Courses(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	/* If there's no class-key, redirect to the front page */
	params := mux.Vars(r)
	clss := params["course"]

	if clss == "" {
		/* Return the courses page */
		RenderPage(w, r, ctx, "courses")
		return
	}

	course, err := getters.GetCourse(ctx.Notion, clss)
	if err != nil {
		http.Error(w, "Unable to load page, course not found", http.StatusInternalServerError)
		ctx.Err.Printf("/courses unable to find course %s\n", err.Error())
		return
	}

	var sessions sessionList
	sessions, err = getters.GetCourseSessions(ctx.Notion, course)
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/courses course sessions fetch failed %s\n", err.Error())
		return
	}

	/* Filter out anything in the past or happening in the next 1hr */
	sessions = helpers.FilterSessions(sessions, time.Now())

	/* Sort sessions by date, soonest first */
	sort.Sort(sessions)

	extraData := make([]ExtraData, 0)
	if len(sessions) > 0 {
		extraData = append(extraData, ExtraData{
			Label: "Next Session Starts",
			Data:  sessions[0].FmtDates()[0],
		})
		extraData = append(extraData, ExtraData{
			Label: "Seats Left",
			Data:  string(sessions[0].SeatsAvail),
		})
	}

	furlCard := courseCardWithExtras(ctx, r, course, extraData)

	err = ctx.TemplateCache.ExecuteTemplate(w, "courses/course.tmpl", CourseData{
		Course:     course,
		SeatsAvail: countSeats(sessions),
		Sessions:   sessions,
		Page:       getPage(ctx, course.Title, furlCard),
	})

	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/courses exec templ failed %s\n", err.Error())
		return
	}
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
	Card      types.FurlCard
	Year      uint
}

type pageData struct {
	Page    Page
	Courses []*types.Course
	Current []*types.Course
	Coming  []*types.Course
}

type ExtraData struct {
	Label string
	Data  string
}

func defaultCard(ctx *config.AppContext, r *http.Request, title string) types.FurlCard {
	return buildCard(ctx.Env.Domain, title, r.URL.String(), "default_card.png", "", nil)
}

func courseCard(ctx *config.AppContext, r *http.Request, course *types.Course) types.FurlCard {
	return courseCardWithExtras(ctx, r, course, nil)
}

func courseCardWithExtras(ctx *config.AppContext, r *http.Request, course *types.Course, extras []ExtraData) types.FurlCard {

	domain := ctx.Env.Domain
	imgName := course.FurlImg()
	imgURL := fmt.Sprintf("https://%s/static/img/%s", domain, imgName)

	return buildCard(domain, course.Title, r.URL.String(), imgURL, course.Flavor, extras)
}

func buildCard(domain, title, URL, imgName, desc string, extraData []ExtraData) types.FurlCard {
	card := types.FurlCard{
		URL:         URL, /* Of the page we're on */
		Domain:      domain,
		Title:       title,
		Description: desc,
		ImageURL:    fmt.Sprintf("https://%s/static/img/%s", domain, imgName),
	}

	for i, extra := range extraData {
		if i == 0 {
			card.ExtraOneLabel = extra.Label
			card.ExtraOneData = extra.Data
		}
		if i == 1 {
			card.ExtraTwoLabel = extra.Label
			card.ExtraTwoData = extra.Data
		}
	}

	return card
}

func getPage(ctx *config.AppContext, title string, card types.FurlCard) Page {
	return Page{
		Title:     title,
		Copyright: time.Now().Year(),
		Domain:    ctx.SitePath(),
		Callbacks: ctx.CallbackPath(),
		Card:      card,
	}
}

func getHomeData(ctx *config.AppContext, n *types.Notion) (pageData, error) {
	courses, err := getters.ListCourses(n)

	/* Sort courses by Popularity */
	sort.Slice(courses, func(i, j int) bool {
		return courses[i].Popularity < courses[j].Popularity
	})

	if err != nil {
		return pageData{}, err
	}
	return pageData{
		Page:    getPage(ctx, "", types.FurlCard{}),
		Courses: courses,
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

type KeyAddrForm struct {
	Value string
}

type KeyAddrData struct {
	Value  string
	Addr   string
	ErrMsg string
	Page   Page
}

func KeyAddr(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	/* Show a address from scriptPubkey page! */
	title := "Address Calculator"
	furlCard := defaultCard(ctx, r, title)

	var data KeyAddrData
	var err error
	if r.Method == http.MethodPost {
		r.ParseForm()
		dec := schema.NewDecoder()
		var form KeyAddrForm
		err = dec.Decode(&form, r.PostForm)
		if err != nil {
			http.Error(w, "Unable to load page, please try again later", http.StatusInternalServerError)
			ctx.Err.Printf("/tools/keyaddr unable to decode wif value %s\n", err.Error())
			return
		}

		/* Note: Currently only support mainnet */
		/* Option values: bc, tb (test+sig), bcrt (reg) */
		data.Addr, err = MakeAddr(form.Value, "bc")
		if err != nil {
			data.ErrMsg = err.Error()
		}
		data.Value = form.Value
	}

	data.Page = getPage(ctx, title, furlCard)
	err = ctx.TemplateCache.ExecuteTemplate(w, "tools/keyaddr.tmpl", &data)
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/tools/keyaddr execute template failed %s\n", err.Error())
	}
}

type WIFForm struct {
	Value string
}

type WIFData struct {
	Value  string
	WIF    string
	ErrMsg string
	Page   Page
}

func WIF(w http.ResponseWriter, r *http.Request, ctx *config.AppContext) {
	/* Show a WIF calc page! */

	title := "WIF Calculator"
	furlCard := defaultCard(ctx, r, title)

	var data WIFData
	var err error
	if r.Method == http.MethodPost {
		r.ParseForm()
		dec := schema.NewDecoder()
		var form WIFForm
		err = dec.Decode(&form, r.PostForm)
		if err != nil {
			http.Error(w, "Unable to load page, please try again later", http.StatusInternalServerError)
			ctx.Err.Printf("/tools/wif unable to decode wif value %s\n", err.Error())
			return
		}

		/* Note: Currently only support testnet/regtest */
		data.WIF, err = MakeWIF(form.Value, DEFAULT_NETWORK_BYTE)
		if err != nil {
			data.ErrMsg = err.Error()
		}
		data.Value = form.Value
	}

	data.Page = getPage(ctx, title, furlCard)
	err = ctx.TemplateCache.ExecuteTemplate(w, "tools/wif.tmpl", &data)
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		ctx.Err.Printf("/tools/wif execute template failed %s\n", err.Error())
	}
}
