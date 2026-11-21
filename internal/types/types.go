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
		Port              string
		Domain            string
		External          string
		Secret            string
		MailerSecret      string
		MailDomain        string
		MailEndpoint      string
		DBDriver          string
		DatabaseURL       string
		AdminEmails       string
		EditorSyncToken   string
		CourseAssetBucket string
		CourseAssetRegion string
		CourseAssetPrefix string
	}

	CourseAvail  string
	CourseFormat string
	CourseLevel  string
	Currency     string

	ChapterInfo struct {
		Title string
		Desc  string
	}

	Course struct {
		ID           string
		Tag          string
		Title        string
		Difficulty   string
		Format       []CourseFormat
		Topic        string
		Availability string
		Popularity   uint
		PriceUSD     uint
		Flavor       string
		ShortDesc    string
		LongDesc     string
		PreReqs      string
		Visible      bool
		Feature      bool
		CourseURL    string
		CourseHost   string
		HeaderImg    string
		Rating       float32
		Includes     []string
		Chapters     []ChapterInfo
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

func (c CourseSession) IsUnscheduled() bool {
	return len(c.Date) > 0 && c.Date[0] == "TBD"
}

func (c Course) FurlImg() string {
	return fmt.Sprintf("courses/%s_card.png", c.Tag)
}

func (c Course) PromoURL(domain string) string {
	return fmt.Sprintf("https://%s/static/img/courses/%s-800.png", domain, c.Tag)
}

func (c Course) Featured(difficulty string) bool {
	return difficulty == c.Difficulty && c.Visible && c.Feature
}

func (c Course) Stars() []string {
	stars := []string{"empty", "empty", "empty", "empty", "empty"}
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

	return stars
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
