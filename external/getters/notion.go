package getters

import (
	"context"
	"fmt"
	"github.com/kodylow/base58-website/internal/types"
	"github.com/sorcererxw/go-notion"
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

func parseFileURL(key string, props map[string]notion.PropertyValue) string {
	prop, ok := props[key]
	if !ok || len(prop.Files) == 0 {
		return ""
	}
	return fileGetURL(prop.Files[0])
}

func parseFormat(options *[]*notion.SelectOption) []types.CourseFormat {
	var formats []types.CourseFormat

	if options == nil {
		return formats
	}

	for _, opt := range *options {
		formats = append(formats, types.CourseFormat(opt.Name))
	}

	return formats
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

func parseCurrency(opt *notion.SelectOption) types.Currency {
	if opt == nil {
		/* FIXME: log err */
		return types.USD
	}
	curr, ok := types.ParseCurrency(opt.Name)
	if !ok {
		/* FIXME: log err */
		return types.USD
	}
	return curr
}

func parseLevel(opt *notion.SelectOption) types.CourseLevel {
	if opt == nil {
		/* FIXME: log err */
		return types.Devs
	}
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

func parseChapterInfo(key string, props map[string]notion.PropertyValue) []types.ChapterInfo {
	input := parseRichText(key, props)
	items := strings.Split(input, "|")

	cis := make([]types.ChapterInfo, len(items))
	for i, item := range items {
		for j := 0; j < len(item); j++ {
			if string(item[j]) == ":" {
				cis[i] = types.ChapterInfo{
					Title: item[:j],
					Desc:  item[j+1:],
				}
				break
			}
		}
	}

	return cis
}

func parseStrSlice(key string, props map[string]notion.PropertyValue) []string {
	input := parseRichText(key, props)
	return strings.Split(input, ",")
}

func parseCourse(pageID string, props map[string]notion.PropertyValue) *types.Course {

	course := &types.Course{
		ID:           pageID,
		Tag:          parseRichText("Tag", props),
		Title:        parseRichText("Title", props),
		Difficulty:   props["Difficulty"].Select.Name,
		Format:       parseFormat(props["Format"].MultiSelect),
		Topic:        props["Topic"].Select.Name,
		Availability: props["Availability"].Select.Name,
		Popularity:   uint(props["Popularity"].Number),
		PriceUSD:     uint(props["PriceUSD"].Number),
		Flavor:       parseRichText("Flavor", props),
		ShortDesc:    parseRichText("ShortDesc", props),
		LongDesc:     parseRichText("LongDesc", props),
		PreReqs:      parseRichText("PreReqs", props),
		Visible:      *props["Visible"].Checkbox,
		Feature:      *props["Feature"].Checkbox,
		Chapters:     parseChapterInfo("Chapters", props),
		Includes:     parseStrSlice("Includes", props),
		CourseURL:    props["CourseURL"].URL,
		HeaderImg:    parseFileURL("HeaderImg", props),
		Rating:       float32(props["Rating"].Number),
		//WelcomeEmail:  props["WelcomeEmail"].URL,
		//WaitlistEmail: props["WaitlistEmail"].URL,
	}

	if props["CourseHost"].Select != nil {
		course.CourseHost = props["CourseHost"].Select.Name
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
		ID:                pageID,
		ClassRef:          parseRichText("ClassRef", props),
		Cost:              uint64(props["Cost"].Number),
		Currency:          parseCurrency(props["Currency"].Select),
		TShirt:            *props["T-Shirt"].Checkbox,
		Online:            *props["Online"].Checkbox,
		TotalSeats:        uint(props["TotalSeats"].Number),
		SeatsAvail:        uint(props["SeatsAvail"].Number),
		TimeDesc:          parseRichText("Time", props),
		Location:          parseRichText("Location", props),
		Instructor:        parseRichText("Instructor", props),
		Date:              trimstrings(dates),
		AddlDetails:       parseRichText("AddlDetails", props),
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

func ListCourses(n *types.Notion) ([]*types.Course, error) {
	var courses []*types.Course

	hasMore := true
	nextCursor := ""
	for hasMore {
		var pages []*notion.Page

		pages, nextCursor, hasMore, _ = n.Client.QueryDatabase(context.Background(),
			n.Config.CoursesDb, notion.QueryDatabaseParam{
				StartCursor: nextCursor,
			})

		/* Convert each page into a Course struct */
		for _, page := range pages {
			course := parseCourse(page.ID, page.Properties)
			courses = append(courses, course)
		}
	}

	return courses, nil
}

func GetCourse(n *types.Notion, classSlug string) (*types.Course, error) {
	pages, _, _, err := n.Client.QueryDatabase(context.Background(),
		n.Config.CoursesDb, notion.QueryDatabaseParam{
			Filter: &notion.Filter{
				Property: "Tag",
				Text: &notion.TextFilterCondition{
					Equals: classSlug,
				},
			},
		})
	if err != nil {
		return nil, err
	}

	if len(pages) != 1 {
		return nil, fmt.Errorf("Unable to find %s", classSlug)
	}

	course := parseCourse(pages[0].ID, pages[0].Properties)
	return course, nil
}

func GetCourseSessions(n *types.Notion, course *types.Course) ([]*types.CourseSession, error) {
	var sessions []*types.CourseSession

	hasMore := true
	nextCursor := ""
	for hasMore {
		var err error
		var pages []*notion.Page
		pages, nextCursor, hasMore, err = n.Client.QueryDatabase(context.Background(),
			n.Config.SessionsDb,
			notion.QueryDatabaseParam{
				StartCursor: nextCursor,
				Filter: &notion.Filter{
					Property: "course",
					Relation: &notion.RelationFilterCondition{
						Contains: course.ID,
					},
				},
			})

		if err != nil {
			return nil, err
		}
		for _, page := range pages {
			session := parseSession(page.ID, page.Properties)
			session.CourseName = course.Title
			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}
