package types

import (
	"encoding/hex"
	"fmt"
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
		SendGrid     SendGridConfig
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
		AppRequired  bool
		Level        CourseLevel
		Visible      bool
		ReplitURL    string
		UdemyURL     string
		WelcomeEmail string
		WaitlistEmail string
	}

	CourseSession struct {
		ID                string
		ClassRef          string
		CourseName        string
		Cost              uint64
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
		Idempotency string
		Timestamp   string
		SessionUUID string
		Cost        uint64
		Count       uint
		PromoURL    string
		CourseName  string
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
		RegisterID  string
		Email       string
		SessionID   string
		Price       uint64
		Type        CheckoutOpt
		Idempotency string
		PromoURL    string
		CourseName  string
		Count       uint64
	}

	Confirmed struct {
		Idempotency string
		Email       string
		ClassRef    string
		Count       uint
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
	Bitcoin CheckoutOpt = "btc"
	Fiat    CheckoutOpt = "fiat"
)

func (c CourseSession) Dates() []time.Time {
	dateStr := "1/2/2006"
	ret := make([]time.Time, 0)
	for _, entry := range c.Date {
		d, _ := time.Parse(dateStr, entry)
		ret = append(ret, d)
	}

	return ret
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

/* Returns the dollar value (USD) of a checkout */
func (c *Checkout) ComputeTotal(opt CheckoutOpt) uint64 {
	if opt == Bitcoin {
		return BtcPrice(c.Price) * c.Count
	}

	return FiatPrice(c.Price) * c.Count
}

/* Produces a human readable description */
func (c *Checkout) MakeDesc() string {
	var seatStr string
	if c.Count == 1 {
		seatStr = "seat"
	} else {
		seatStr = "seats"
	}

	return fmt.Sprintf("%d %s in Base58's %s class", c.Count, seatStr, c.CourseName)
}
