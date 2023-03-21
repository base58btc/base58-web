package getters

import (
	"context"
	"fmt"
	"github.com/kodylow/base58-website/internal/types"
	"github.com/sorcererxw/go-notion"
	"os"
	"strings"
)

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
	if len(val.RichText) == 0 {
		if len(val.Title) != 0 {
			return val.Title[0].Text.Content
		}
		/* FIXME: log err? */
		return ""
	}

	return val.RichText[0].Text.Content
}

func parseCourse(pageID string, props map[string]notion.PropertyValue) *types.Course {
	course := &types.Course{
		ID:           pageID,
		TmplName:     parseRichText("Name", props),
		PublicName:   parseRichText("PublicName", props),
		Availability: parseAvail(props["Availability"].MultiSelect),
		ShortDesc:    parseRichText("ShortDesc", props),
		ComingSoon:   props["Coming Soon"].Checkbox,
		AppRequired:  props["Application Required"].Checkbox,
		Level:        parseLevel(props["Difficulty"].Select),
		Visible:      props["Visible"].Checkbox,
	}
	return course
}

func parseSession(pageID string, props map[string]notion.PropertyValue) *types.CourseSession {
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
		Date:       strings.Split(parseRichText("Dates", props), ","),
	}
	if props["Signup Code"].Select != nil {
		session.SignupCode = props["Signup Code"].Select.Name
	}
	return session
}

func ListCourses(n *types.Notion) ([]*types.Course, error) {
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

func SaveRegistration(n *types.Notion, r *types.ClassRegistration) (string, error) {
	parent := notion.NewDatabaseParent(n.Config.SignupsDb)
	props := map[string]*notion.PropertyValue{
		"Email": notion.NewTitlePropertyValue(
			[]*notion.RichText{
				{Type: notion.RichTextText,
					Text: &notion.Text{Content: r.Email}},
			}...),
		"session": notion.NewRelationPropertyValue(
			[]*notion.ObjectReference{{ID: r.SessionUUID}}...,
		),
		"Idempotent": notion.NewRichTextPropertyValue(
			[]*notion.RichText{
				{Type: notion.RichTextText,
					Text: &notion.Text{Content: r.Idempotency}},
			}...),
		"Replit": notion.NewRichTextPropertyValue(
			[]*notion.RichText{
				{Type: notion.RichTextText,
					Text: &notion.Text{Content: r.ReplitUser}},
			}...),
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

func UpdateRegistration(n *types.Notion, pageID string, refID string) (string, error) {
	page, err := n.Client.UpdatePageProperties(context.Background(), pageID,
		map[string]*notion.PropertyValue{
			"PaymentRef": notion.NewRichTextPropertyValue(
				[]*notion.RichText{
					{Type: notion.RichTextText,
						Text: &notion.Text{Content: refID}},
				}...),
		})
	if err != nil {
		return "", err
	}

	sessionUUID := page.Properties["session"].Relation[0].ID
	return sessionUUID, nil
}

func CountClassRegistration(n *types.Notion, sessionUUID string) error {
	page, err := n.Client.RetrievePage(context.Background(), sessionUUID)
	if err != nil {
		return err
	}

	session := parseSession(page.ID, page.Properties)

	/* Nothing to do, we oversold. Oops */
	if session.SeatsAvail < 1 {
		fmt.Fprintf(os.Stderr, "Oops we oversold this class %s\n", session.ClassRef)
		return nil
	}

	seatsNowAvail := session.SeatsAvail - 1
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
			"Email": notion.NewTitlePropertyValue(
				[]*notion.RichText{
					{Type: notion.RichTextText,
						Text: &notion.Text{Content: w.Email}},
				}...),
			"Session": notion.NewRelationPropertyValue(
				[]*notion.ObjectReference{{ID: w.SessionUUID}}...,
			),
			"Idempotent": &notion.PropertyValue{
				Type: notion.PropertySelect,
				Select: &notion.SelectOption{
					Name: w.Idempotency,
				},
			},
		})
	return err
}
