package getters

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/kodylow/base58-website/internal/types"
	"github.com/sorcererxw/go-notion"
	"os"
	"strings"
)

func fileGetURL(file *notion.File) string {
	if file.Internal != nil {
		return file.Internal.URL
	}
	if file.External != nil {
		return file.External.URL
	}
	return ""
}

func parseAvail(avail []*notion.SelectOption) []types.CourseAvail {
	var avails []types.CourseAvail

	for _, opt := range avail {
		avail, ok := types.ParseCourseAvail(opt.Name)
		if ok {
			avails = append(avails, avail)
		}
		/* FIXME: log err */
	}

	return avails
}

func parseLevel(opt *notion.SelectOption) types.CourseLevel {
	level, ok := types.ParseCourseLevel(opt.Name)
	if !ok {
		/* FIXME: log err */
		return types.Devs
	}
	return level
}

func parseRichText(key string, props map[string]notion.PropertyValue) string {
	val, ok := props[key]
	if !ok {
		/* FIXME: log err? */
		return ""
	}
	var contentList []*notion.RichText
	if len(val.Title) > 0 {
		contentList = val.Title
	} else {
		contentList = val.RichText
	}

	var builder strings.Builder
	/* We're gonna lose formatting, yolo */
	for _, text := range contentList {
		builder.WriteString(text.Text.Content)
		if text.Text.Link != nil {
			if text.Text.Content != text.Text.Link.URL {
				builder.WriteString(" (" + text.Text.Link.URL + ")")
			}
		}
	}
	return builder.String()
}

func parseCourse(pageID string, props map[string]notion.PropertyValue) *types.Course {
	course := &types.Course{
		ID:           pageID,
		TmplName:     parseRichText("Name", props),
		PublicName:   parseRichText("PublicName", props),
		Availability: parseAvail(props["Availability"].MultiSelect),
		ShortDesc:    parseRichText("ShortDesc", props),
		LongDesc:     parseRichText("LongDesc", props),
		PreReqs:      parseRichText("PreReqs", props),
		ComingSoon:   props["Coming Soon"].Checkbox,
		AppRequired:  props["Application Required"].Checkbox,
		Level:        parseLevel(props["Difficulty"].Select),
		Visible:      props["Visible"].Checkbox,
		ReplitURL:    props["ReplitURL"].URL,
		UdemyURL:     props["UdemyURL"].URL,
		WelcomeEmail:     props["WelcomeEmail"].URL,
	}

	if len(props["HeaderImg"].Files) > 0 {
		file := props["HeaderImg"].Files[0]
		course.PromoURL = fileGetURL(file)
	} else {
		/* default image */
		course.PromoURL = "/static/img/at_computer.jpg"
	}

	return course
}

func trimstrings(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.TrimSpace(s)
	}
	return out
}

func parseSession(pageID string, props map[string]notion.PropertyValue) *types.CourseSession {
	dates := strings.Split(parseRichText("Dates", props), ",")
	session := &types.CourseSession{
		ID:         pageID,
		ClassRef:   parseRichText("ClassRef", props),
		Cost:       uint64(props["Cost"].Number),
		TShirt:     props["T-Shirt"].Checkbox,
		Online:     props["Online"].Checkbox,
		TotalSeats: uint(props["TotalSeats"].Number),
		SeatsAvail: uint(props["SeatsAvail"].Number),
		TimeDesc:   parseRichText("Time", props),
		Location:   parseRichText("Location", props),
		Instructor: parseRichText("Instructor", props),
		Date:       trimstrings(dates),
		AddlDetails: parseRichText("AddlDetails", props),
		ScheduleSpecifics: parseRichText("ScheduleSpecifics", props),
		LocationSpecifics: parseRichText("LocationSpecifics", props),
	}
	if props["Signup Code"].Select != nil {
		session.SignupCode = props["Signup Code"].Select.Name
	}

	if len(props["AdImg"].Files) > 0 {
		file := props["AdImg"].Files[0]
		session.PromoURL = fileGetURL(file)
	} else {
		/* default image */
		session.PromoURL = "/static/img/base58_vertical.png"
	}

	return session
}

/* Fake list of courses */
func fakeCourselist() []*types.Course {
	return []*types.Course {
		&types.Course{
			ID: "12345",
			TmplName: "fake1",
			PublicName: "TempCourse",
			Availability: []types.CourseAvail { types.Replit, types.InPerson },
			ShortDesc: "This is a temporary class bullet",
			ComingSoon: false,
			Level: types.Devs,
			AppRequired: false,
			Visible: true,
		},
	}
}

func ListCourses(n *types.Notion) ([]*types.Course, error) {
	/* For now, just return the fake courses */
	//return fakeCourselist(), nil

	/* FIXME: pagination */
	pages, _, _, _ := n.Client.QueryDatabase(context.Background(),
		n.Config.CoursesDb, notion.QueryDatabaseParam{})

	var courses []*types.Course
	/* Convert each page into a Course struct */
	for _, page := range pages {
		course := parseCourse(page.ID, page.Properties)
		courses = append(courses, course)
	}

	return courses, nil
}

func GetSessionInfo(n *types.Notion, sessionID string) (*types.Course, *types.CourseSession, error) {
	pages, _, _, err := n.Client.QueryDatabase(context.Background(),
		n.Config.SessionsDb, notion.QueryDatabaseParam{
			Filter: &notion.Filter{
				Property: "ClassRef",
				Text: &notion.TextFilterCondition{
					Equals: sessionID,
				},
			},
		})
	if err != nil {
		return nil, nil, err
	}

	if len(pages) != 1 {
		return nil, nil, fmt.Errorf("Unable to find %s", sessionID)
	}

	sessionPage := pages[0]
	courseID := sessionPage.Properties["course"].Relation[0].ID
	session := parseSession(sessionPage.ID, sessionPage.Properties)

	page, err := n.Client.RetrievePage(context.Background(), courseID)
	if err != nil {
		return nil, nil, err
	}
	course := parseCourse(page.ID, page.Properties)
	session.CourseName = course.PublicName
	return course, session, nil
}

func GetCourseSessions(n *types.Notion, courses []*types.Course) ([]*types.CourseSession, error) {
	var sessions []*types.CourseSession

	/* Build a map of course IDs we're looking for? */
	var orFilter []*notion.Filter
	idDict := make(map[string]*types.Course)
	for _, course := range courses {
		idDict[course.ID] = course
		filter := &notion.Filter{
			Property: "course",
			Relation: &notion.RelationFilterCondition{
				Contains: course.ID,
			},
		}
		orFilter = append(orFilter, filter)
	}

	/* FIXME: pagination */
	pages, _, _, err := n.Client.QueryDatabase(context.Background(),
		n.Config.SessionsDb,
		notion.QueryDatabaseParam{
			Filter: &notion.Filter{Or: orFilter},
		})

	if err != nil {
		return nil, err
	}
	for _, page := range pages {
		course := idDict[page.Properties["course"].Relation[0].ID]
		session := parseSession(page.ID, page.Properties)
		session.CourseName = course.PublicName
		sessions = append(sessions, session)
	}

	return sessions, nil
}

func UniqueID(contact string, ref string, counter int32) string {
	/* sha256 of ref || contact|| count (4, le) */
	h := sha256.New()
	h.Write([]byte(ref))
	h.Write([]byte(contact))

	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(counter))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

func SaveRegistration(n *types.Notion, r *types.ClassRegistration, c *types.Checkout) (string, error) {
	parent := notion.NewDatabaseParent(n.Config.CartsDB)
	props := map[string]*notion.PropertyValue{
		"Idempotent": notion.NewTitlePropertyValue(
			[]*notion.RichText{
				{Type: notion.RichTextText,
					Text: &notion.Text{Content: r.Idempotency}},
			}...),
		"Contact": notion.NewRichTextPropertyValue(
			[]*notion.RichText{
				{Type: notion.RichTextText,
					Text: &notion.Text{Content: r.Email}},
			}...),
		"session": notion.NewRelationPropertyValue(
			[]*notion.ObjectReference{{ID: r.SessionUUID}}...,
		),
		"Amount": &notion.PropertyValue{
			Type:   notion.PropertyNumber,
			Number: float64(c.ComputeTotal(r.CheckoutVia)),
		},
		"Currency": &notion.PropertyValue{
			Type: notion.PropertySelect,
			Select: &notion.SelectOption{
				Name: "USD",
			},
		},
		"Seats": &notion.PropertyValue{
			Type:   notion.PropertyNumber,
			Number: float64(r.Count),
		},

	}

	if r.Shirt != nil {
		props["T-Shirt Size"] = &notion.PropertyValue{
			Type: notion.PropertySelect,
			Select: &notion.SelectOption{
				Name: r.Shirt.String(),
			},
		}
	}

	if r.MailingAddr != nil {
		props["Mailing Address"] = notion.NewRichTextPropertyValue(
			[]*notion.RichText{
				{Type: notion.RichTextText,
					Text: &notion.Text{Content: *r.MailingAddr}},
			}...)
	}
	page, err := n.Client.CreatePage(context.Background(), parent, props)
	if err != nil {
		return "", err
	}
	return page.ID, err
}

func CheckIdemWaitlist(n *types.Notion, idemTok string) (bool, error) {
	/* Check that not already added */
	pages, _, _, err := n.Client.QueryDatabase(context.Background(),
		n.Config.WaitlistDb, notion.QueryDatabaseParam{
			Filter: &notion.Filter{
				Property: "Idempotent",
				Text: &notion.TextFilterCondition{
					Equals: idemTok,
				},
			},
		})
	if err != nil {
		return false, err
	}

	return len(pages) > 0, nil
}

func FinalizeRegistration(n *types.Notion, pageID string, refID string) (string, uint, error) {
	/* Check that not already added */
	pages, _, _, err := n.Client.QueryDatabase(context.Background(),
		n.Config.CartsDB, notion.QueryDatabaseParam{
			Filter: &notion.Filter{
				Property: "PaymentRef",
				Text: &notion.TextFilterCondition{
					Equals: refID,
				},
			},
		})
	if err != nil {
		return "", 0, err
	}

	if len(pages) > 0 {
		return "", 0, fmt.Errorf("Already finalized %s", refID)
	}

	/* Update the page to have the payment ref */
	page, err := n.Client.UpdatePageProperties(context.Background(), pageID,
		map[string]*notion.PropertyValue{
			"PaymentRef": notion.NewRichTextPropertyValue(
				[]*notion.RichText{
					{Type: notion.RichTextText,
						Text: &notion.Text{Content: refID}},
				}...),
		})
	if err != nil {
		return "", 0, err
	}

	/* Add seats for each of registrations */
	sessionUUID := page.Properties["session"].Relation[0].ID
	cartUUID := page.ID
	email := parseRichText("Contact", page.Properties)
	mailingAddr := parseRichText("Mailing Address", page.Properties)
	parent := notion.NewDatabaseParent(n.Config.SignupsDb)
	seatCount := uint(page.Properties["Seats"].Number)
	var tShirt string
	if page.Properties["T-Shirt Size"].Select != nil {
		tShirt = page.Properties["T-Shirt Size"].Select.Name
	} else {
		tShirt = ""
	}
	for i := 0; i < int(seatCount); i++ {
		refID := UniqueID(email, cartUUID, int32(i))
		props := map[string]*notion.PropertyValue{
			"Contact": notion.NewTitlePropertyValue(
				[]*notion.RichText{
					{Type: notion.RichTextText,
						Text: &notion.Text{Content: email}},
				}...),
			"RefID": notion.NewRichTextPropertyValue(
				[]*notion.RichText{
					{Type: notion.RichTextText,
						Text: &notion.Text{Content: refID}},
				}...),
			"session": notion.NewRelationPropertyValue(
				[]*notion.ObjectReference{{ID: sessionUUID}}...,
			),
			"CartRef": notion.NewRelationPropertyValue(
				[]*notion.ObjectReference{{ID: cartUUID}}...,
			),
		}

		if mailingAddr != "" {
			props["Mailing Address"] = notion.NewRichTextPropertyValue(
				[]*notion.RichText{
					{Type: notion.RichTextText,
						Text: &notion.Text{Content: mailingAddr}},
				}...)
		}

		if tShirt != "" {
			props["T-Shirt Size"] = &notion.PropertyValue{
				Type: notion.PropertySelect,
				Select: &notion.SelectOption{
					Name: tShirt,
				},
			}
		}

		_, err := n.Client.CreatePage(context.Background(), parent, props)
		if err != nil {
			return "", uint(i + 1), err
		}
	}

	return sessionUUID, seatCount, nil
}

func CountClassRegistration(n *types.Notion, sessionUUID string, seats uint) error {
	page, err := n.Client.RetrievePage(context.Background(), sessionUUID)
	if err != nil {
		return err
	}

	session := parseSession(page.ID, page.Properties)

	/* Nothing to do, we oversold. Oops */
	if session.SeatsAvail < seats {
		fmt.Fprintf(os.Stderr, "Oops we oversold this class %s\n", session.ClassRef)
		return nil
	}

	seatsNowAvail := session.SeatsAvail - seats
	_, err = n.Client.UpdatePageProperties(context.Background(), sessionUUID,
		map[string]*notion.PropertyValue{
			"SeatsAvail": notion.NewNumberPropertyValue(float64(seatsNowAvail)),
		})
	return err
}

func SaveWaitlist(n *types.Notion, w *types.WaitList) error {
	parent := notion.NewDatabaseParent(n.Config.WaitlistDb)
	_, err := n.Client.CreatePage(context.Background(), parent,
		map[string]*notion.PropertyValue{
			"Contact": notion.NewTitlePropertyValue(
				[]*notion.RichText{
					{Type: notion.RichTextText,
						Text: &notion.Text{Content: w.Email}},
				}...),
			"Session": notion.NewRelationPropertyValue(
				[]*notion.ObjectReference{{ID: w.SessionUUID}}...,
				),
			"Idempotent": notion.NewRichTextPropertyValue(
				[]*notion.RichText{
					{Type: notion.RichTextText,
						Text: &notion.Text{Content: w.Idempotency}},
				}...),
			},
		)
	return err
}
