package emails

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"

	"github.com/kodylow/base58-website/internal/types"
	"github.com/kodylow/base58-website/internal/config"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

type EmailInfos struct {
	Course *types.Course
	Session *types.CourseSession
}

type EmailContent struct {
	Content string
	URI string
}

func mdToHTML(md []byte) []byte {
	/* create markdown parser with extensions */
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	// create HTML renderer with extensions
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	return markdown.Render(doc, renderer)
}

func Build(ctx *config.AppContext, tmplURL string, course *types.Course, session *types.CourseSession) ([]byte, []byte, error) {
	/* Fetch the email template */
	t, ok := ctx.TemplateCache[tmplURL]

	if !ok {
		ctx.Infos.Println("cache miss for %s", tmplURL)
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
		Course: course,
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

	ctx.Infos.Println(string(htmlOut))
	var email bytes.Buffer
	err = et.ExecuteTemplate(&email, "tmp.tmpl", &EmailContent{
		Content: string(htmlOut),
		URI: ctx.CallbackPath(),
	})

	if err != nil {
		return nil, nil, err
	}

	return email.Bytes(), buf.Bytes(), nil
}