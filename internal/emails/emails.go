package emails

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kodylow/base58-website/internal/config"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"

	mailer "github.com/base58btc/mailer/mail"
)

type EmailContent struct {
	Content     string
	URI         string
	Unsubscribe string
}

func emailRenderHook(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
	graphStyles := `
	font-optical-sizing: auto;
	font-style: normal;
	line-height: 26px;
	font-size: 16px;
	color: rgb(54, 55, 55);
	`
	if paragraph, ok := node.(*ast.Paragraph); ok && entering {
		paragraph.Attribute = &ast.Attribute{
			Attrs: make(map[string][]byte),
		}
		paragraph.Attribute.Attrs["style"] = []byte(graphStyles)
	}
	if list, ok := node.(*ast.List); ok && entering {
		styleAttr := `padding-inline-start: 1.5rem;`
		list.Attribute = &ast.Attribute{
			Attrs: make(map[string][]byte),
		}
		list.Attribute.Attrs["style"] = []byte(styleAttr)
	}
	if listItem, ok := node.(*ast.ListItem); ok {
		var toWrite string
		if entering {
			toWrite = fmt.Sprintf(`<li>
			<p styles="%s">`, graphStyles)
		} else {
			toWrite = `</p></li>`
		}
		listItem.Tight = true
		io.WriteString(w, toWrite)
		return ast.GoToNext, true
	}
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
			text-align: center;
			align-items: center;
			margin-bottom: auto;
			padding-bottom: 1rem;
			padding-top: 1rem;
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
			styleAttr = `style="text-decoration-line:underline; text-underline-offset:4px; font-weight:400;"`
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

func BuildHTMLEmail(ctx *config.AppContext, markdown []byte) ([]byte, error) {
	return BuildHTMLEmailUnsub(ctx, markdown, "")
}

func BuildHTMLEmailUnsub(ctx *config.AppContext, markdown []byte, unsubscribe string) ([]byte, error) {

	/* Convert markdown to HTML */
	htmlOut := mdToHTML(markdown)

	/* Embed into our email wrapper template */
	var email bytes.Buffer
	err := ctx.TemplateCache.ExecuteTemplate(&email, "emails/tmp.tmpl", &EmailContent{
		Content:     string(htmlOut),
		URI:         ctx.CallbackPath(),
		Unsubscribe: unsubscribe,
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

func addAuthStamp(ctx *config.AppContext, req *http.Request) {
	timestamp := strconv.Itoa(int(time.Now().UTC().Unix()))
	secret := ctx.Env.MailerSecret
	authStamp := makeAuthStamp(secret, timestamp, req)

	req.Header.Set("Authorization", authStamp)
	req.Header.Set("X-Base58-Timestamp", timestamp)
}

type Mail struct {
	JobKey   string
	Sub      string
	Missive  string
	Email    string
	ReplyTo  string
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

func SendContactEmail(ctx *config.AppContext, email, message, from, formtype string) ([]byte, error) {
	var err error

	/* Jobkey is the message + email */
	h := sha256.New()
	h.Write([]byte(email))
	h.Write([]byte(message))
	jobkey := fmt.Sprintf("%s-%s", formtype, hex.EncodeToString(h.Sum(nil)[:8]))

	var title string
	if from != email {
		if formtype == "contact" {
			title = fmt.Sprintf("New Message from %s: Get In Touch", from)
		} else {
			title = fmt.Sprintf("New Message from %s: %s a Work(shop)", formtype, from)
		}
		message = fmt.Sprintf("%s\n\nSent from: <%s>", message, from)
	} else {
		if formtype == "contact" {
			title = "Your Request to Get In Touch"
			message = fmt.Sprintf("Hey there! We got your inquiry.  Here's a copy of the message you sent. We'll be in touch shortly.<br/><br/> %s", message)
		} else {
			title = fmt.Sprintf("Your Request to %s a Work(shop)", formtype)
			message = fmt.Sprintf(`Hey there! We got your inquiry to %s a Work(shop). Here's a copy of the message you sent. We'll be in touch shortly.<br/></br/> %s`, formtype, message)
		}
	}

	mail := &Mail{
		JobKey:   jobkey,
		Email:    email,
		Title:    title,
		SendAt:   time.Now(),
		TextBody: []byte(message),
	}

	mail.HTMLBody, err = BuildHTMLEmail(ctx, []byte(message))
	if err != nil {
		return nil, err
	}

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
		JobKey:       "base58:" + mail.JobKey,
		Subscription: mail.Sub,
		Missive:      mail.Missive,
		ToAddr:       mail.Email,
		FromAddr:     "hello@base58.school",
		FromName:     "Base58⛓️ 🔓",
		ReplyTo:      mail.ReplyTo,
		Title:        mail.Title,
		HTMLBody:     string(mail.HTMLBody),
		TextBody:     string(mail.TextBody),
		Attachments:  attaches,
		SendAt:       float64(mail.SendAt.UTC().Unix()),
		Domain:       ctx.Env.MailDomain,
	}

	return SendMailRequest(ctx, mailReq)
}

func makeSubKey(email, newsletter string) string {
	/* Hash email+newsletter, take first 8 bytes */
	mac := hmac.New(sha256.New, []byte(email))
	mac.Write([]byte(newsletter))
	hashfix := hex.EncodeToString(mac.Sum(nil)[:8])
	return fmt.Sprintf("%s-%s", hashfix, newsletter)
}

func sendMailerReq(ctx *config.AppContext, endpoint string, method string, payload []byte) error {
	client := &http.Client{Timeout: 15 * time.Second}

	url := ctx.Env.MailEndpoint + endpoint
	req, err := http.NewRequest(method, url, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}

	addAuthStamp(ctx, req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mailer request %s failed with HTTP %d", endpoint, resp.StatusCode)
	}

	var ret mailer.ReturnVal
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, &ret); err != nil {
		return err
	}

	if !ret.Success {
		return fmt.Errorf("Mailer request %s failed (%d): %s", endpoint, ret.Code, ret.Message)
	}

	return nil
}

func SendSubDeleteRequest(ctx *config.AppContext, email, sub string) error {
	/* Send as a DELETE request w/ JSON body */
	subkey := makeSubKey(email, sub)
	subdelete := &mailer.SubDelete{
		SubKey: subkey,
	}
	payload, err := json.Marshal(subdelete)
	if err != nil {
		return err
	}

	err = sendMailerReq(ctx, "/sub", http.MethodDelete, payload)
	if err != nil {
		return fmt.Errorf("Sub delete request failed. %s, %s : %s", sub, email, err)
	}
	ctx.Infos.Printf("Rm'd subscription %s", subkey)
	return nil
}

func SendMailRequest(ctx *config.AppContext, mail *mailer.MailRequest) error {
	/* Send as a PUT request w/ JSON body */
	payload, err := json.Marshal(mail)
	if err != nil {
		return err
	}

	err = sendMailerReq(ctx, "/job", http.MethodPut, payload)
	if err != nil {
		return fmt.Errorf("Unable to schedule mail to %s: %s", mail.ToAddr, err)
	}

	ctx.Infos.Printf("Sent mail to %s at domain %s", mail.ToAddr, mail.Domain)
	return nil
}
