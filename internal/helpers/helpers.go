package helpers

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"time"

	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/types"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

var renderers map[string]*html.Renderer = make(map[string]*html.Renderer)


/* Blogpost on how to write renderers https://blog.kowalczyk.info/article/cxn3/advanced-markdown-processing-in-go.html */
func preReqRenderHook(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
	if anchor, ok := node.(*ast.Link); ok && entering {
		targetAttr := `target="_blank"`
		anchor.AdditionalAttributes = append(anchor.AdditionalAttributes, targetAttr)
	}
	if head, ok := node.(*ast.Heading); ok && entering {
		styleAttr := ""
		switch head.Level {
		case 1:
			styleAttr = `letter-spacing: -.025em; font-weight: 500; font-size: 1.125rem; line-height: 1.75rem;margin-top:1.5rem;`
		case 2:
			styleAttr = `letter-spacing:-.025em;font-weight:500;margin-top:1.5rem;`
		}

		if styleAttr != "" {
			head.Attribute = &ast.Attribute{
				Attrs: make(map[string][]byte),
			}
			head.Attribute.Attrs["style"] = []byte(styleAttr)
		}
	}
	if list, ok := node.(*ast.List); ok && entering {
		classList := `list`
		list.Attribute = &ast.Attribute{
			Attrs: make(map[string][]byte),
		}
		list.Attribute.Attrs["role"] = []byte(classList)
	}
	if listItem, ok := node.(*ast.ListItem); ok {
		var toWrite string
		if entering {
			toWrite = `<li>
			<p class="pre-requisites">`
		} else {
			toWrite = `</p></li>`
		}
		listItem.Tight = true
		io.WriteString(w, toWrite)
		return ast.GoToNext, true
	}

	return ast.GoToNext, false
}

func textPageRenderHook(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
	if head, ok := node.(*ast.Heading); ok && entering {
		attr := head.Attribute
		if attr == nil {
			attr = &ast.Attribute{}
		}
		switch head.Level {
		case 1:
			attr.Classes = append(attr.Classes, []byte("heading-2 margin-vertical margin-small"))
		case 2:
			attr.Classes = append(attr.Classes, []byte("course-details-title margin-vertical margin-small"))
		case 3:
			attr.Classes = append(attr.Classes, []byte("course-details-body semi-bold margin-vertical margin-xsmall"))
		}

		head.Attribute = attr
	}
	if graph, ok := node.(*ast.Paragraph); ok && entering {
		attr := graph.Attribute
		if attr == nil {
			attr = &ast.Attribute{}
		}
		attr.Classes = append(attr.Classes, []byte("margin-bottom margin-xsmall"))
		graph.Attribute = attr

	}
	if list, ok := node.(*ast.List); ok && entering {
		classList := `list`
		list.Attribute = &ast.Attribute{
			Attrs: make(map[string][]byte),
		}
		list.Attribute.Attrs["role"] = []byte(classList)
		list.Attribute.Attrs["style"] = []byte("list-style-type: disc;")
	}
	if listItem, ok := node.(*ast.ListItem); ok {
		var toWrite string
		if entering {
			toWrite = `<li>
			<p>`
		} else {
			toWrite = `</p></li>`
		}
		listItem.Tight = true
		io.WriteString(w, toWrite)
		return ast.GoToNext, true
	}

	return ast.GoToNext, false
}

func newBlockRenderer(hook html.RenderNodeFunc) *html.Renderer {
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{
		Flags:          htmlFlags,
		RenderNodeHook: hook,
	}

	return html.NewRenderer(opts)
}

func getRenderer(renderFmt string) *html.Renderer {
	renderer, ok := renderers[renderFmt]
	if !ok {
		switch renderFmt {
		case "prereq": 
			renderer = newBlockRenderer(preReqRenderHook)
		case "text":
			renderer = newBlockRenderer(textPageRenderHook)
		default:
			renderer = newBlockRenderer(textPageRenderHook)
		}
		renderers[renderFmt] = renderer
	}

	return renderer
}

/* Convert a block of MD text to HTML */
func mdToHTML(format string, md []byte) []byte {
	/* create markdown parser with extensions */
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	/* Create HTML renderer with extensions */
	renderer := getRenderer(format)
	return markdown.Render(doc, renderer)
}

func hashContent(format, mdContent string) string {
	/* sha256 of ref || contact|| count (4, le) */
	h := sha256.New()
	h.Write([]byte(format))
	h.Write([]byte(mdContent))
	return hex.EncodeToString(h.Sum(nil))
}

func ConvertMdToHTML(ctx *config.AppContext, format, mdContent string) []byte {
	if ctx.DocCache == nil {
		ctx.DocCache = make(map[string][]byte)
	}

	/* Fetch the md template */
	tag := hashContent(format, mdContent)
	htmlOut, ok := ctx.DocCache[tag]

	if ok {
		return htmlOut
	}

	/* Convert markdown to HTML */
	htmlOut = mdToHTML(format, []byte(mdContent))
	ctx.DocCache[tag] = htmlOut
	return htmlOut
}

func FilterWaitlistSessions(sessions []*types.CourseSession) []*types.CourseSession {
	return FilterSessionsByAvail(sessions, 0)
}

func FilterSessionsByAvail(sessions []*types.CourseSession, minAvail uint) []*types.CourseSession {
	filtered := make([]*types.CourseSession, 0)

	for _, sesh := range sessions {
		if sesh.SeatsAvail > minAvail {
			filtered = append(filtered, sesh)
		}
	}
	return filtered
}

func FilterSessions(sessions []*types.CourseSession, from time.Time) []*types.CourseSession {
	filtered := make([]*types.CourseSession, 0)

	for _, sesh := range sessions {
		if sesh.IsUnscheduled() {
			filtered = append(filtered, sesh)
			continue
		}
		seshStart := sesh.Dates()[0]
		if !seshStart.Before(from.Add(time.Minute * 60)) {
			filtered = append(filtered, sesh)
		}
	}

	return filtered
}
