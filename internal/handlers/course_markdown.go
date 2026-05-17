package handlers

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

var courseCodeFence = []byte("+++")

type courseCodeBlock struct {
	ast.Leaf
	Content string
}

func markdownHasCourseCode(md []byte) bool {
	return bytes.Contains(md, courseCodeFence)
}

func courseMarkdownToHTML(md []byte) []byte {
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	p.Opts.ParserHook = courseParserHook

	renderer := mdhtml.NewRenderer(mdhtml.RendererOptions{
		Flags:          mdhtml.CommonFlags | mdhtml.HrefTargetBlank,
		RenderNodeHook: courseRenderHook,
	})

	return markdown.Render(p.Parse(md), renderer)
}

func courseParserHook(data []byte) (ast.Node, []byte, int) {
	if node, remaining, consumed := parseCourseCodeBlock(data); node != nil {
		return node, remaining, consumed
	}

	return nil, nil, 0
}

func parseCourseCodeBlock(data []byte) (ast.Node, []byte, int) {
	if !bytes.HasPrefix(data, courseCodeFence) {
		return nil, nil, 0
	}

	contentStart := len(courseCodeFence)
	if contentStart < len(data) && data[contentStart] == '\r' {
		contentStart++
	}
	if contentStart < len(data) && data[contentStart] == '\n' {
		contentStart++
	}

	closingOffset := bytes.Index(data[contentStart:], append([]byte("\n"), courseCodeFence...))
	if closingOffset < 0 {
		return nil, data, 0
	}

	closingStart := contentStart + closingOffset
	closingEnd := closingStart + 1 + len(courseCodeFence)
	if closingEnd < len(data) && data[closingEnd] == '\r' {
		closingEnd++
	}
	if closingEnd < len(data) && data[closingEnd] == '\n' {
		closingEnd++
	}

	return &courseCodeBlock{
		Content: strings.Trim(string(data[contentStart:closingStart]), "\r\n"),
	}, nil, closingEnd
}

func courseRenderHook(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
	if cb, ok := node.(*courseCodeBlock); ok {
		if entering {
			renderCourseCodeBlock(w, cb)
		}
		return ast.GoToNext, true
	}

	return ast.GoToNext, false
}

func renderCourseCodeBlock(w io.Writer, cb *courseCodeBlock) {
	escaped := template.HTMLEscapeString(cb.Content)
	fmt.Fprintf(w, `<div class="cell codeset">
  <button class="run-btn" type="button" onclick="runCellCode(this)" aria-label="Run code">
    <svg fill="currentColor" height="16" width="16" viewBox="0 0 330 330" aria-hidden="true">
      <path d="M37.728,328.12c2.266,1.256,4.77,1.88,7.272,1.88c2.763,0,5.522-0.763,7.95-2.28l240-149.999c4.386-2.741,7.05-7.548,7.05-12.72c0-5.172-2.664-9.979-7.05-12.72L52.95,2.28c-4.625-2.891-10.453-3.043-15.222-0.4C32.959,4.524,30,9.547,30,15v300C30,320.453,32.959,325.476,37.728,328.12z"></path>
    </svg>
  </button>
  <textarea class="code-area" spellcheck="false">%s</textarea>
  <div class="output-prompt" aria-hidden="true"></div>
  <div class="output" aria-live="polite"></div>
</div>`, escaped)
}
