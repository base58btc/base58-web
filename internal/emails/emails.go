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
	"strings"
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

var globalctx *config.AppContext

func emailRenderHook(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
	if blockquote, ok := node.(*ast.BlockQuote); ok && entering {
		var attr *ast.Attribute
		if c := blockquote.AsContainer(); c != nil {
			if c.Attribute != nil {
				attr = c.Attribute
			} else {
				attr = &ast.Attribute{
					Attrs: make(map[string][]byte, 0),
				}
				c.Attribute = attr
			}
		}
		if attr == nil {
			return ast.GoToNext, false
		}

		styleValue := `
			border: none;
			margin: 0;
			align-items: center;
			margin-bottom: auto;
			padding-bottom: 1rem;
			padding-top: 1rem;
			display: flex;
			justify-content: center;
		`
		attr.Attrs["style"] = []byte(styleValue)
	}
	if anchor, ok := node.(*ast.Link); ok && entering {
		var styleAttr string
		dest := string(anchor.Destination)
		if strings.HasPrefix(dest, "button#") {
			trimmed := strings.TrimPrefix(dest, "button#")
			anchor.Destination = []byte(trimmed)

			styleAttr = `style="
				color: #2d2d2d;
				background-color: #f7931a;
				border: 1.5px solid #000;
				border-radius: 6px;
				cursor: pointer;
				display: inline-block;
				line-height: inherit;
				padding: .75rem 1.5rem;
				font-family: tenon, sans-serif;
				font-size: 1.1rem;
				font-weight: 500;
				text-align: center;
				text-decoration: none;
			"`
		} else {
			styleAttr = `style="text-underline-offset:4px; text-decoration-line:underline; text-underline-offset:4px; font-weight:400;"`
		}
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

func findEmailMarkdown(ctx *config.AppContext, tmplURL string) (*template.Template, error) {
	t, ok := ctx.EmailCache[tmplURL]
	if !ok {
		ctx.Infos.Printf("cache miss for %s", tmplURL)
		req, _ := http.NewRequest("GET", tmplURL, nil)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		tmpl, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("error returned from %s: %s", tmplURL, err.Error())
		}

		t = template.Must(template.New("").Parse(string(tmpl)))
		ctx.EmailCache[tmplURL] = t
	}

	return t, nil
}

func BuildHTMLEmail(ctx *config.AppContext, markdown []byte) ([]byte, error) {

	globalctx = ctx
	/* Convert markdown to HTML */
	htmlOut := mdToHTML(markdown)

	/* Embed into our email wrapper template */
	var email bytes.Buffer
	err := ctx.TemplateCache.ExecuteTemplate(&email, "emails/tmp.tmpl", &EmailContent{
		Content: string(htmlOut),
		URI:     ctx.CallbackPath(),
	})

	if err != nil {
		return nil, err
	}

	return email.Bytes(), nil
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

func buildConfirmURL(ctx *config.AppContext, token string) string {
	return fmt.Sprintf("%s/confirm/%s", ctx.SitePath(), token)
}

type SubConfirmEmail struct {
	Email      string
	ConfirmURL string
	Newsletter string
}


func SendNewsletterSubEmail(ctx *config.AppContext, email, token, newsletter string) ([]byte, error) {

	var title, template string
	if newsletter == "newsletter" {
		title = "Newsletter Subscription"
		template = "emails/confirm-sub.tmpl"
	} else {
		title = fmt.Sprintf("%s Course Waitlist", newsletter)
		template = "emails/confirm-waitlist.tmpl"
	}
	jobkey := "subscribe-" + token
	mail := &Mail{
		JobKey: jobkey,
		Email:  email,
		Title:  fmt.Sprintf("[Action Required] Confirm Base58 %s", title),
		SendAt: time.Now(),
	}

	/* Swap in the tokens */
	var buf bytes.Buffer
	err := ctx.TemplateCache.ExecuteTemplate(&buf, template, &SubConfirmEmail{
		Email:      email,
		ConfirmURL: buildConfirmURL(ctx, token),
		Newsletter: newsletter,

	})

	if err != nil {
		return nil, err
	}

	mail.TextBody = buf.Bytes()

	mail.HTMLBody, err = BuildHTMLEmail(ctx, buf.Bytes())
	if err != nil {
		return nil, err
	}

	return mail.HTMLBody, ComposeAndSendMail(ctx, mail)
}

func SendRegistrationEmail(ctx *config.AppContext, course *types.Course, session *types.CourseSession, confirm *types.Confirmed) ([]byte, error) {
	var err error
	mail := &Mail{
		JobKey: confirm.Idempotency,
		Email:  confirm.Email,
		Title:  fmt.Sprintf("Your Registration for Base58's %s", course.Title),
		SendAt: time.Now(),
	}

	emailTmpl, err := findEmailMarkdown(ctx, course.WelcomeEmail)

	/* Swap in the tokens */
	var buf bytes.Buffer
	err = emailTmpl.Execute(&buf, &EmailInfos{
		Course:  course,
		Session: session,
	})
	if err != nil {
		return nil, err
	}

	mail.TextBody = buf.Bytes()

	mail.HTMLBody, err = BuildHTMLEmail(ctx, buf.Bytes())
	if err != nil {
		return nil, err
	}

	/* FIXME: if ticketed event, send a ticket PDF too! x confirm.Count */
	//refID := UniqueID(email, idem, int32(i))

	/* FIXME: add a receipt PDF */

	return mail.HTMLBody, ComposeAndSendMail(ctx, mail)
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
		JobKey:      "base58:" + mail.JobKey,
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
