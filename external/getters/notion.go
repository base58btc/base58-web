package getters

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/kodylow/base58-website/checkout"
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

func parseFormat(options []*notion.SelectOption) []types.CourseFormat {
	var formats []types.CourseFormat

	for _, opt := range options {
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
		//WelcomeEmail:  props["WelcomeEmail"].URL,
		//WaitlistEmail: props["WaitlistEmail"].URL,
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

func GetSessionInfoUUID(n *types.Notion, sessionUUID string) (*types.Course, *types.CourseSession, error) {
	sessionPage, err := n.Client.RetrievePage(context.Background(), sessionUUID)
	if err != nil {
		return nil, nil, err
	}

	//courseID := sessionPage.Properties["course"].Relation[0].ID
	courseID := sessionPage.Properties["new_courses"].Relation[0].ID
	session := parseSession(sessionPage.ID, sessionPage.Properties)

	page, err := n.Client.RetrievePage(context.Background(), courseID)
	if err != nil {
		return nil, nil, err
	}
	course := parseCourse(page.ID, page.Properties)
	session.CourseName = course.Title
	return course, session, nil
}

func GetCourseInfo(n *types.Notion, courseUUID string) (*types.Course, []*types.CourseSession, error) {
	page, err := n.Client.RetrievePage(context.Background(), courseUUID)
	if err != nil {
		return nil, nil, err
	}

	course := parseCourse(page.ID, page.Properties)
	sessions, err := GetCourseSessions(n, course)
	if err != nil {
		return nil, nil, err
	}

	return course, sessions, nil
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
	session.CourseName = course.Title
	return course, session, nil
}

func GetCourseSessions(n *types.Notion, course *types.Course) ([]*types.CourseSession, error) {
	var sessions []*types.CourseSession

	/* FIXME: pagination */
	pages, _, _, err := n.Client.QueryDatabase(context.Background(),
		n.Config.SessionsDb,
		notion.QueryDatabaseParam{
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

func SaveRegistration(n *types.Notion, idem string, seshUUID string, r *types.ClassRegistration, c *types.Checkout) (string, error) {
	parent := notion.NewDatabaseParent(n.Config.SessionCartsDb)
	props := map[string]*notion.PropertyValue{
		"Idempotent": notion.NewTitlePropertyValue(
			[]*notion.RichText{
				{Type: notion.RichTextText,
					Text: &notion.Text{Content: idem}},
			}...),
		"Contact": notion.NewRichTextPropertyValue(
			[]*notion.RichText{
				{Type: notion.RichTextText,
					Text: &notion.Text{Content: r.Email}},
			}...),
		"session": notion.NewRelationPropertyValue(
			[]*notion.ObjectReference{{ID: seshUUID}}...,
		),
		"Amount": {
			Type:   notion.PropertyNumber,
			Number: float64(c.ComputeTotal(r.CheckoutVia)),
		},
		"Currency": {
			Type: notion.PropertySelect,
			Select: &notion.SelectOption{
				Name: "USD",
			},
		},
		"Seats": {
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

/*
Save the Email + Items in a Cart, plus the Token. Helps us later identify

	who hasn't checked out, and what they were planning to buy (but no quantities)
*/
func SaveCart(n *types.Notion, cart *checkout.Cart) (string, error) {

	itemRefs := make([]*notion.ObjectReference, 0)
	for _, item := range cart.Items {
		itemRefs = append(itemRefs, &notion.ObjectReference{
			ID: item.GetID(),
		})
	}

	/* Concert cart to Base64 */
	cartSerial, err := cart.ToBase64()

	if err != nil {
		return "", err
	}

	parent := notion.NewDatabaseParent(n.Config.ItemCartsDb)
	props := map[string]*notion.PropertyValue{
		"Token": notion.NewTitlePropertyValue(
			[]*notion.RichText{
				{
					Type: notion.RichTextText,
					Text: &notion.Text{Content: cart.Token},
				},
			}...),
		"Contact": notion.NewRichTextPropertyValue(
			[]*notion.RichText{
				{
					Type: notion.RichTextText,
					Text: &notion.Text{Content: cart.Infos.Email},
				},
			}...),
		"Items": notion.NewRelationPropertyValue(itemRefs...),
		"JSON": notion.NewRichTextPropertyValue(
			[]*notion.RichText{
				{
					Type: notion.RichTextText,
					Text: &notion.Text{Content: cartSerial},
				},
			}...),
	}

	if len(cart.Discounts) > 0 {
		var discounts strings.Builder
		for _, discount := range cart.Discounts {
			if discounts.Len() > 0 {
				discounts.WriteString(", ")
			}
			discounts.WriteString(discount)
		}
		props["Discounts"] = notion.NewRichTextPropertyValue(
			[]*notion.RichText{
				{
					Type: notion.RichTextText,
					Text: &notion.Text{Content: discounts.String()},
				},
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

func AddClassRegistration(n *types.Notion, regID, cartUUID, sessionUUID, token, email string, mailingAddr *string, tShirt *checkout.ShirtSize) error {

	props := map[string]*notion.PropertyValue{
		"Contact": notion.NewTitlePropertyValue(
			[]*notion.RichText{
				{Type: notion.RichTextText,
					Text: &notion.Text{Content: email}},
			}...),
		"RefID": notion.NewRichTextPropertyValue(
			[]*notion.RichText{
				{Type: notion.RichTextText,
					Text: &notion.Text{Content: regID}},
			}...),
		"session": notion.NewRelationPropertyValue(
			[]*notion.ObjectReference{{ID: sessionUUID}}...,
		),
		"CartRef": notion.NewRelationPropertyValue(
			[]*notion.ObjectReference{{ID: cartUUID}}...,
		),
	}

	if mailingAddr != nil {
		props["Mailing Address"] = notion.NewRichTextPropertyValue(
			[]*notion.RichText{
				{Type: notion.RichTextText,
					Text: &notion.Text{Content: *mailingAddr}},
			}...)
	}

	if tShirt != nil {
		props["T-Shirt Size"] = &notion.PropertyValue{
			Type: notion.PropertySelect,
			Select: &notion.SelectOption{
				Name: tShirt.String(),
			},
		}
	}

	_, err := n.Client.CreatePage(context.Background(), notion.NewDatabaseParent(n.Config.SignupsDb), props)

	return err
}

func CheckCartNotPaid(n *types.Notion, paymentID string) (bool, error) {
	/* Check that not already added */
	pages, _, _, err := n.Client.QueryDatabase(context.Background(),
		n.Config.SessionCartsDb, notion.QueryDatabaseParam{
			Filter: &notion.Filter{
				Property: "PaymentRef",
				Text: &notion.TextFilterCondition{
					Equals: paymentID,
				},
			},
		})

	return len(pages) > 0, err
}

func MarkCartPaid(n *types.Notion, lookupID string, paymentID string) (*checkout.Cart, error) {
	/* Update the page to have the payment ref */
	page, err := n.Client.UpdatePageProperties(context.Background(), lookupID,
		map[string]*notion.PropertyValue{
			"PaymentRef": notion.NewRichTextPropertyValue(
				[]*notion.RichText{
					{Type: notion.RichTextText,
						Text: &notion.Text{Content: paymentID}},
				}...),
		})

	if err != nil {
		return nil, err
	}

	/* Pull out the cart json data */
	cartSerialized := parseRichText("JSON", page.Properties)
	return checkout.CartFromBase64(cartSerialized)
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
