package emails

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/types"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"

	mailer "github.com/base58btc/mailer/mail"
)

type EmailInfos struct {
	Course  *types.Course
	Session *types.CourseSession
}

type EmailContent struct {
	Content string
	URI     string
}

func emailRenderHook(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
	if anchor, ok := node.(*ast.Link); ok && entering {
		styleAttr := `style="text-underline-offset:4px; text-decoration-line:underline; text-underline-offset:4px; font-weight:400;"`
		anchor.AdditionalAttributes = append(anchor.AdditionalAttributes, styleAttr)
	}
	if head, ok := node.(*ast.Heading); ok && entering {
		styleAttr := ""
		switch head.Level {
		case 1:
			styleAttr = `color: rgb(17 24 39); letter-spacing: -.025em; font-weight: 700; font-size: 2.25rem; line-height: 2.5rem;`
		case 2:
			styleAttr = `color:rgb(55 65 81);letter-spacing:-.025em;font-weight:700;font-size:1.5rem;line-height:2rem;margin-top:2rem;`
		}

		if styleAttr != "" {
			head.Attribute = &ast.Attribute{
				Attrs: make(map[string][]byte),
			}
			head.Attribute.Attrs["style"] = []byte(styleAttr)
		}
	}

	return ast.GoToNext, false
}

func newEmailRenderer() *html.Renderer {
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{
		RenderNodeHook: emailRenderHook,
		Flags:          htmlFlags,
	}
	return html.NewRenderer(opts)
}

func mdToHTML(md []byte) []byte {
	/* create markdown parser with extensions */
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	/* Create HTML renderer with extensions */
	renderer := newEmailRenderer()

	return markdown.Render(doc, renderer)
}

func Build(ctx *config.AppContext, tmplURL string, course *types.Course, session *types.CourseSession) ([]byte, []byte, error) {
	/* Fetch the email template */
	t, ok := ctx.TemplateCache[tmplURL]

	if !ok {
		ctx.Infos.Printf("cache miss for %s", tmplURL)
		req, _ := http.NewRequest("GET", tmplURL, nil)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, nil, err
		}
		defer resp.Body.Close()
		tmpl, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, nil, err
		}

		if resp.StatusCode != 200 {
			return nil, nil, fmt.Errorf("error returned from %s: %s", tmplURL, err.Error())
		}

		t = template.Must(template.New("").Parse(string(tmpl)))
		ctx.TemplateCache[tmplURL] = t
	}

	/* Swap in the tokens */
	var buf bytes.Buffer
	err := t.Execute(&buf, &EmailInfos{
		Course:  course,
		Session: session,
	})
	if err != nil {
		return nil, nil, err
	}

	/* Convert markdown to HTML */
	htmlOut := mdToHTML(buf.Bytes())

	/* Embed into our email wrapper template */
	et, ok := ctx.TemplateCache["email.tmpl"]
	if !ok {
		ctx.Infos.Println("cache miss for email.tmpl")
		et = template.Must(template.New("tmp.tmpl").Funcs(template.FuncMap{
			"ishtml": func(s string) template.HTML {
				return template.HTML(s)
			},
		}).ParseFiles("templates/emails/tmp.tmpl"))
		ctx.TemplateCache["email.tmpl"] = et
	}

	var email bytes.Buffer
	err = et.ExecuteTemplate(&email, "tmp.tmpl", &EmailContent{
		Content: string(htmlOut),
		URI:     ctx.CallbackPath(),
	})

	if err != nil {
		return nil, nil, err
	}

	return email.Bytes(), buf.Bytes(), nil
}

func makeAuthStamp(secret string, timestamp string, r *http.Request) string {
	h := sha256.New()
	h.Write([]byte(secret))
	h.Write([]byte(timestamp))
	h.Write([]byte(r.URL.Path))
	h.Write([]byte(r.Method))
	return hex.EncodeToString(h.Sum(nil))
}

type Mail struct {
	JobKey   string
	Email    string
	Title    string
	SendAt   time.Time
	HTMLBody []byte
	TextBody []byte
	Files    []*EmailFile
}

type EmailFile struct {
	PDF  []byte
	Name string
}

func SendWaitlistEmail(ctx *config.AppContext, idem string, email string, course *types.Course, session *types.CourseSession) error {

	var err error

	mail := &Mail{
		JobKey: idem,
		Email:  email,
		Title:  fmt.Sprintf("You're on the list for Base58's %s", course.Title),
		SendAt: time.Now(),
	}

	mail.HTMLBody, mail.TextBody, err = Build(ctx, course.WaitlistEmail, course, session)
	if err != nil {
		return err
	}

	return ComposeAndSendMail(ctx, mail)
}

func SendRegistrationEmail(ctx *config.AppContext, course *types.Course, session *types.CourseSession, confirm *types.Confirmed) error {
	var err error
	mail := &Mail{
		JobKey: confirm.Idempotency,
		Email:  confirm.Email,
		Title:  fmt.Sprintf("Your Registration for Base58's %s", course.Title),
		SendAt: time.Now(),
	}
	mail.HTMLBody, mail.TextBody, err = Build(ctx, course.WelcomeEmail, course, session)
	if err != nil {
		return err
	}

	/* FIXME: if ticked event, send a ticket PDF too! x confirm.Count */
	//refID := UniqueID(email, idem, int32(i))

	/* FIXME: add a receipt PDF */

	return ComposeAndSendMail(ctx, mail)
}

func ComposeAndSendMail(ctx *config.AppContext, mail *Mail) error {
	var attaches mailer.AttachSet

	attaches = make([]*mailer.Attachment, len(mail.Files))
	for i, file := range mail.Files {
		attaches[i] = &mailer.Attachment{
			Content: file.PDF,
			Type:    "application/pdf",
			Name:    file.Name,
		}
	}

	/* Build a mail to send */
	mailReq := &mailer.MailRequest{
		JobKey:      mail.JobKey,
		ToAddr:      mail.Email,
		FromAddr:    "hello@base58.school",
		FromName:    "Base58⛓️ 🔓",
		Title:       mail.Title,
		HTMLBody:    string(mail.HTMLBody),
		TextBody:    string(mail.TextBody),
		Attachments: attaches,
		SendAt:      float64(mail.SendAt.UTC().Unix()),
		Domain:      ctx.Env.MailDomain,
	}

	return SendMailRequest(ctx, mailReq)
}

func SendMailRequest(ctx *config.AppContext, mail *mailer.MailRequest) error {
	/* Send as a PUT request w/ JSON body */
	payload, err := json.Marshal(mail)
	if err != nil {
		return err
	}

	client := &http.Client{}

	url := ctx.Env.MailEndpoint + "/job"
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}

	timestamp := strconv.Itoa(int(time.Now().UTC().Unix()))
	secret := ctx.Env.MailerSecret
	authStamp := makeAuthStamp(secret, timestamp, req)

	req.Header.Set("Authorization", authStamp)
	req.Header.Set("X-Base58-Timestamp", timestamp)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	var ret mailer.ReturnVal
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, &ret); err != nil {
		return err
	}

	if !ret.Success {
		return fmt.Errorf("Unable to schedule mail: %s", ret.Message)
	}

	ctx.Infos.Printf("Sent mail to %s at domain %s", mail.ToAddr, mail.Domain)
	return nil
}
