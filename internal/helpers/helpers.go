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

func blockRenderHook(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
	if anchor, ok := node.(*ast.Link); ok && entering {
		styleAttr := `style="text-underline-offset:4px; text-decoration-line:underline; text-underline-offset:4px; font-weight:400;"`
		anchor.AdditionalAttributes = append(anchor.AdditionalAttributes, styleAttr)
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
		classList := `mt-4 space-y-3`
		list.Attribute = &ast.Attribute{
			Attrs: make(map[string][]byte),
		}
		list.Attribute.Attrs["class"] = []byte(classList)
	}
	if paragraph, ok := node.(*ast.Paragraph); ok && entering {
		classList := `mt-4`
		paragraph.Attribute = &ast.Attribute{
			Attrs: make(map[string][]byte),
		}
		paragraph.Attribute.Attrs["class"] = []byte(classList)
	}
	if listItem, ok := node.(*ast.ListItem); ok {
		var toWrite string
		/* Add cute svgs for bulleted lists */
		if entering {
			toWrite = `<li style="display:flex; column-gap: 0.75rem; margin-top:0.5rem;">
			<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="rgb(255 126 1)" style="flex:none;height:1.25rem;width:1.25rem;margin-top:0.25rem;color:rgb(255 126 1)" >
  <path stroke-linecap="round" stroke-linejoin="round" d="M4.26 10.147a60.436 60.436 0 00-.491 6.347A48.627 48.627 0 0112 20.904a48.627 48.627 0 018.232-4.41 60.46 60.46 0 00-.491-6.347m-15.482 0a50.57 50.57 0 00-2.658-.813A59.905 59.905 0 0112 3.493a59.902 59.902 0 0110.399 5.84c-.896.248-1.783.52-2.658.814m-15.482 0A50.697 50.697 0 0112 13.489a50.702 50.702 0 017.74-3.342M6.75 15a.75.75 0 100-1.5.75.75 0 000 1.5zm0 0v-3.675A55.378 55.378 0 0112 8.443m-7.007 11.55A5.981 5.981 0 006.75 15.75v-1.5" />
</svg>
			<span>`
		} else {
			toWrite = `</span></li>`
		}
		listItem.Tight = true
		io.WriteString(w, toWrite)
		return ast.GoToNext, true
	}

	return ast.GoToNext, false
}

func newBlockRenderer() *html.Renderer {
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{
		RenderNodeHook: blockRenderHook,
		Flags:          htmlFlags,
	}
	return html.NewRenderer(opts)
}

/* Convert a block of MD text to HTML */
func mdToHTML(md []byte) []byte {
	/* create markdown parser with extensions */
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	/* Create HTML renderer with extensions */
	renderer := newBlockRenderer()
	return markdown.Render(doc, renderer)
}

func hashContent(mdContent string) string {
	/* sha256 of ref || contact|| count (4, le) */
	h := sha256.New()
	h.Write([]byte(mdContent))
	return hex.EncodeToString(h.Sum(nil))
}

func ConvertMdToHTML(ctx *config.AppContext, mdContent string) []byte {
	if ctx.DocCache == nil {
		ctx.DocCache = make(map[string][]byte)
	}

	/* Fetch the email template */
	tag := hashContent(mdContent)
	htmlOut, ok := ctx.DocCache[tag]

	if ok {
		return htmlOut
	}

	/* Convert markdown to HTML */
	htmlOut = mdToHTML([]byte(mdContent))
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
