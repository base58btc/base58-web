package types

import (
	"strings"
)

type (

	/* Configs for the app! */
	EnvConfig struct {
		Port   string
		Notion NotionConfig
	}

	CourseAvail string
	CourseLevel string
	ShirtSize   string

	Course struct {
		ID	     string
		TmplName     string
		PublicName   string
		Availability []CourseAvail
		ShortDesc    string
		ComingSoon   bool
		// FIXME: link to application?
		AppRequired bool
		Level       CourseLevel
		Visible     bool
	}

	CourseSession struct {
		ClassRef   string
		CourseName string
		Cost       uint64
		TShirt     bool
		Online     bool
		TotalSeats uint
		SeatsAvail uint
		SignupCode string
		Date	   []string
		TimeDesc   string
		Location   string
		Instructor string
	}

	SessionSignup struct {
		Email          string
		ClassRef       string
		PaymentRef     string
		ReplitName     string
		MailingAddress string
		Shirt          *ShirtSize
	}
)

const (
	Replit   CourseAvail = "replit"
	InPerson CourseAvail = "in-person"
	Online   CourseAvail = "online"
	Udemy    CourseAvail = "udemy"
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
