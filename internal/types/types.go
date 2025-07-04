package types

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"
)

type (

	/* Configs for the app! */
	EnvConfig struct {
		Port         string
		Domain       string
		External     string
		Secret       string
		MailerSecret string
		MailDomain   string
		MailEndpoint string
		Notion       NotionConfig
		Stripe       StripeConfig
		OpenNode     OpenNodeConfig
	}

	CourseAvail  string
	CourseFormat string
	CourseLevel  string
	ShirtSize    string
	CheckoutOpt  string
	Currency     string

	ChapterInfo struct {
		Title         string
		Desc          string
	}

	Course struct {
		ID            string
		Tag           string
		Title         string
		Difficulty    string
		Format        []CourseFormat
		Topic         string
		Availability  string
		Popularity    uint
		PriceUSD      uint
		Flavor        string
		ShortDesc     string
		LongDesc      string
		PreReqs       string
		Visible       bool
		Feature       bool
		CourseURL     string
		CourseHost    string
		Rating        float32
		WelcomeEmail  string
		WaitlistEmail string
		Includes      []string
		Chapters      []ChapterInfo
	}

	CourseSession struct {
		ID                string
		ClassRef          string
		CourseName        string
		Cost              uint64
		Currency          Currency
		TShirt            bool
		Online            bool
		TotalSeats        uint
		SeatsAvail        uint
		SignupCode        string
		Date              []string
		TimeDesc          string
		Location          string
		Instructor        string
		PromoURL          string
		AddlDetails       string
		LocationSpecifics string
		ScheduleSpecifics string
	}

	ClassRegistration struct {
		Email       string
		MailingAddr *string
		Shirt       *ShirtSize
		ReplitUser  string
		CheckoutVia CheckoutOpt
		Session     string
		Count       uint
		SignupCode  string
	}

	WaitList struct {
		Email       string
		Idempotency string
		Timestamp   string
		SessionUUID string
		PromoURL    string
		CourseName  string
	}

	OptionItem struct {
		Key   string
		Value string
	}

	Checkout struct {
		RegisterID   string
		Email        string
		SessionID    string
		Price        uint64
		Type         CheckoutOpt
		Idempotency  string
		PromoURL     string
		CourseName   string
		Count        uint64
		Session      *CourseSession
		DiscountCode string
	}

	Confirmed struct {
		Idempotency string
		Email       string
		ClassRef    string
		Count       uint
	}

	Subscriber struct {
		Email         string
		Subs          []*Subscription
		Pages         []string
	}

	Subscription struct {
		Name          string
		ID            string
	}

	FurlCard struct {
		URL           string
		Domain        string
		Title         string
		Description   string
		ImageURL      string
		ImageAlt      string
		ExtraOneLabel string
		ExtraOneData  string
		ExtraTwoLabel string
		ExtraTwoData  string
	}
)

const (
	Replit   CourseAvail = "replit"
	InPerson CourseAvail = "in-person"
	Online   CourseAvail = "online"
	Udemy    CourseAvail = "udemy"
)

func (c CourseAvail) SelfPacedOnline() bool {
	return c == Replit || c == Udemy
}

const (
	USD  Currency = "usd"
	SATS Currency = "sats"
)

const (
	Devs     CourseLevel = "devs"
	Everyone CourseLevel = "everyone"
	Entry    CourseLevel = "entry-dev"
	Expert   CourseLevel = "exp-devs"
)

const (
	Small ShirtSize = "small"
	Med   ShirtSize = "med"
	Large ShirtSize = "large"
	XL    ShirtSize = "xl"
	XXL   ShirtSize = "xxl"
)

const (
	Bitcoin CheckoutOpt = "btc"
	Fiat    CheckoutOpt = "fiat"
)

func (c CourseSession) IsUnscheduled() bool {
	return len(c.Date) > 0 && c.Date[0] == "TBD"
}

func (c Course) FurlImg() string {
	return fmt.Sprintf("courses/%s_card.png", c.Title)
}

func (c Course) PromoURL(domain string) string {
	return fmt.Sprintf("https://%s/static/img/courses/%s-800.png", domain, c.Tag)
}

func (c Course) Featured(difficulty string) bool {
	return difficulty == c.Difficulty && c.Visible && c.Feature
}

func (c Course) Stars() []string {
	stars := []string {"empty", "empty", "empty", "empty", "empty"}
	rating := c.Rating * 100

	half := math.Mod(float64(rating), 100) > 0
	fulls := int(rating / 100)
	for i := 0; i < 5; i++ {
		if i < fulls {
			stars[i] = "full"
		}
		if i == fulls && half {
			stars[i] = "half"
		}
	}

	return stars;
}

func (c CourseSession) Dates() []time.Time {
	dateStr := "1/2/2006"
	ret := make([]time.Time, 0)
	for _, entry := range c.Date {
		d, _ := time.Parse(dateStr, entry)
		ret = append(ret, d)
	}

	return ret
}

func (c CourseSession) FormatDateRange() string {
	dates := c.FmtDates()
	if len(dates) == 1 {
		return dates[0]
	}

	return dates[0] + " - " + dates[len(dates)-1]
}

func (c CourseSession) GetOptionDesc() string {
	var desc string
	desc += c.Location
	desc += ", " + c.FormatDateRange()

	if !c.Online {
		desc += " (in-person) "
	}

	return desc
}

func (c CourseSession) Details() string {
	return fmt.Sprintf("Led by %s @ %s", c.Instructor, c.TimeDesc)
}

/* List of dates in a nice, readable format */
func (c CourseSession) FmtDates() []string {
	dateFmt := "Mon Jan 2, 2006"
	ds := c.Dates()
	out := make([]string, len(ds))

	for i, d := range ds {
		out[i] = d.Format(dateFmt)
	}
	return out
}

func (s CheckoutOpt) String() string {
	return string(s)
}

var mapEnumCheckoutOpt = func() map[string]CheckoutOpt {
	m := make(map[string]CheckoutOpt)
	m[string(Bitcoin)] = Bitcoin
	m[string(Fiat)] = Fiat

	return m
}()

func ParseCheckoutOpt(str string) (CheckoutOpt, bool) {
	ss, ok := mapEnumCheckoutOpt[strings.ToLower(str)]
	return ss, ok
}

func (s ShirtSize) String() string {
	return string(s)
}

var mapEnumShirtSize = func() map[string]ShirtSize {
	m := make(map[string]ShirtSize)
	m[string(Small)] = Small
	m[string(Med)] = Med
	m[string(Large)] = Large
	m[string(XL)] = XL
	m[string(XXL)] = XXL

	return m
}()

func ParseShirtSize(str string) (ShirtSize, bool) {
	ss, ok := mapEnumShirtSize[strings.ToLower(str)]
	return ss, ok
}

func (s CourseAvail) String() string {
	return string(s)
}

var mapEnumCourseAvail = func() map[string]CourseAvail {
	m := make(map[string]CourseAvail)
	m[string(Replit)] = Replit
	m[string(InPerson)] = InPerson
	m[string(Online)] = Online
	m[string(Udemy)] = Udemy

	return m
}()

func ParseCourseAvail(str string) (CourseAvail, bool) {
	ss, ok := mapEnumCourseAvail[strings.ToLower(str)]
	return ss, ok
}

var mapEnumCurrency = func() map[string]Currency {
	m := make(map[string]Currency)
	m[string(USD)] = USD
	m[string(SATS)] = SATS

	return m
}()

func ParseCurrency(option string) (Currency, bool) {
	curr, ok := mapEnumCurrency[strings.ToLower(option)]
	return curr, ok
}

func (s CourseLevel) String() string {
	return string(s)
}

var mapEnumCourseLevel = func() map[string]CourseLevel {
	m := make(map[string]CourseLevel)
	m[string(Devs)] = Devs
	m[string(Everyone)] = Everyone
	m[string(Entry)] = Entry
	m[string(Expert)] = Expert

	return m
}()

func ParseCourseLevel(str string) (CourseLevel, bool) {
	ss, ok := mapEnumCourseLevel[strings.ToLower(str)]
	return ss, ok
}

func (e *EnvConfig) SecretBytes() []byte {
	data, _ := hex.DecodeString(e.Secret)
	return data
}

func FiatPrice(val uint64) uint64 {
	return val
}

func BtcPrice(val uint64) uint64 {
	return uint64(float64(val) * .85)
}

func (c *Checkout) ComputePrice(opt CheckoutOpt) uint64 {
	switch opt {
	case Bitcoin:
		return BtcPrice(c.Price)
	case Fiat:
		return FiatPrice(c.Price)
	}

	panic(fmt.Sprintf("checkout option \"%s\" does not compute", opt))
}

func (c *Checkout) ComputeCount() uint64 {
	/* FIXME: more intelligent parsing lang for count discounts */
	if c.DiscountCode == "6s-1" && c.Count == 6 {
		return c.Count - 1
	}

	return c.Count
}

/* Returns the dollar value (USD) of a checkout */
func (c *Checkout) ComputeTotal(opt CheckoutOpt) uint64 {
	price := c.ComputePrice(opt)
	count := c.ComputeCount()

	return price * count
}

/* Produces a human readable description */
func (c *Checkout) MakeDesc() string {
	var seatStr string
	if c.Count == 1 {
		seatStr = "seat"
	} else {
		seatStr = "seats"
	}

	return fmt.Sprintf("%d %s in Base58's %s class. %s", c.Count, seatStr, c.CourseName, c.Session.GetOptionDesc())
}

/* Returns true if subscribed is new state */
func (s *Subscriber) AddSubscription(name string) bool {
	if s.Subs == nil {
		s.Subs = make([]*Subscription, 0)
	}

	for _,sub := range s.Subs {
		if sub.Name == name {
			return false 
		}
	}

	s.Subs = append(s.Subs, &Subscription{
		Name: name,
	})
	return true
}

/* Returns true if unsubscribed is new state */
func (s *Subscriber) RmSubscription(name string) bool {
	if s.Subs == nil {
		return false
	}

	newSubs := make([]*Subscription, 0)
	unsubscribed := false

	for _,sub := range s.Subs {
		if sub.Name == name {
			unsubscribed = true
			continue
		}
		newSubs = append(newSubs, sub)
	}

	s.Subs = newSubs
	return unsubscribed
}
