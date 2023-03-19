package getters

import (
	"context"
	"github.com/kodylow/base58-website/internal/types"
	"github.com/sorcererxw/go-notion"
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

func ListCourses(n *types.Notion) ([]*types.Course, error) {
	/* FIXME: pagination */
	pages, _, _, _ := n.Client.QueryDatabase(context.Background(),
		n.Config.CoursesDb, notion.QueryDatabaseParam{})

	var courses []*types.Course
	/* Convert each page into a Course struct */
	for _, page := range pages {
		course := &types.Course{
			ID:           page.ID,
			TmplName:     parseRichText("Name", page.Properties),
			PublicName:   parseRichText("PublicName", page.Properties),
			Availability: parseAvail(page.Properties["Availability"].MultiSelect),
			ShortDesc:    parseRichText("ShortDesc", page.Properties),
			ComingSoon:   page.Properties["Coming Soon"].Checkbox,
			AppRequired:  page.Properties["Application Required"].Checkbox,
			Level:        parseLevel(page.Properties["Difficulty"].Select),
			Visible:   page.Properties["Visible"].Checkbox,
		}
		courses = append(courses, course)
	}

	return courses, nil
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
		n.Config.SessionsDb, notion.QueryDatabaseParam{
			Filter: &notion.Filter{ Or: orFilter, },
		})

	if err != nil {
		return nil, err
	}
	for _, page := range pages {
		course := idDict[page.Properties["course"].Relation[0].ID]
		session := &types.CourseSession{
			ClassRef: parseRichText("ClassRef", page.Properties),
			CourseName: course.PublicName,
			Cost: uint64(page.Properties["Cost"].Number),
			TShirt: page.Properties["T-Shirt"].Checkbox,
			Online: page.Properties["Online"].Checkbox,
			TotalSeats: uint(page.Properties["TotalSeats"].Number),
			SeatsAvail: uint(page.Properties["SeatsAvail"].Number),
			TimeDesc: parseRichText("Time", page.Properties),
			Location: parseRichText("Location", page.Properties),
			Instructor: parseRichText("Instructor", page.Properties),
			Date: strings.Split(parseRichText("Dates", page.Properties), ","),
		}
		if page.Properties["Signup Code"].Select != nil {
			session.SignupCode = page.Properties["Signup Code"].Select.Name
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
}
