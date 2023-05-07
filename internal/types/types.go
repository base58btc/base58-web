package types

import (
	"encoding/hex"
	"strings"
	"time"
)

type (

	/* Configs for the app! */
	EnvConfig struct {
		Port     string
		Secret   string
		Notion   NotionConfig
		Stripe   StripeConfig
		SendGrid SendGridConfig
	}

	CourseAvail string
	CourseLevel string
	ShirtSize   string
	CheckoutOpt string

	Course struct {
		ID           string
		TmplName     string
		PublicName   string
		Availability []CourseAvail
		ShortDesc    string
		LongDesc     string
		PreReqs      string
		PromoURL     string
		ComingSoon   bool
		// FIXME: link to application?
		AppRequired bool
		Level       CourseLevel
		Visible     bool
		ReplitURL   string
		UdemyURL    string
	}

	CourseSession struct {
		ID         string
		ClassRef   string
		CourseName string
		Cost       uint64
		TShirt     bool
		Online     bool
		TotalSeats uint
		SeatsAvail uint
		SignupCode string
		Date       []string
		TimeDesc   string
		Location   string
		Instructor string
		PromoURL   string
		AddlDetails string
	}

	ClassRegistration struct {
		Email       string     `form:"label=Email;type=email;placeholder=hello@example.com"`
		MailingAddr *string    `form:"label=Mailing Address;placeholder=555 Magneto Way, Oxford, UK 282822"`
		Shirt       *ShirtSize `form:"label=Shirt size, unisex;type=select;id=shirt;placeholder=med"`
		ReplitUser  string
		CheckoutVia CheckoutOpt `form:"label=Checkout Via;type=select;id=checkout;"`
		Idempotency string      `form:"label=nil;type=hidden"`
		Timestamp   string      `form:"label=nil;type=hidden"`
		SessionUUID string      `form:"label=nil;type=hidden"`
		Cost        uint64      `form:"label=nil;type=hidden"`
	}

	WaitList struct {
		Email       string `form:"label=Email;type=email;placeholder=hello@example.com"`
		Idempotency string `form:"label=nil;type=hidden"`
		SessionUUID string `form:"label=nil;type=hidden"`
		Timestamp   string `form:"label=nil;type=hidden"`
	}

	OptionItem struct {
		Key   string
		Value string
	}

	Checkout struct {
		RegisterID  string
		Email       string
		SessionID   string
		Price       uint64
		Type        CheckoutOpt
		Idempotency string
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
	Bitcoin CheckoutOpt = "bitcoin"
	Fiat    CheckoutOpt = "usd"
)

func (c CourseSession) Dates() []time.Time {
	dateStr := "1/2/2006"
	ret := make([]time.Time, 0)
	for _, entry := range(c.Date) {
		d, _ := time.Parse(dateStr, entry)
		ret = append(ret, d)
	}

	return ret
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
