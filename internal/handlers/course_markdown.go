package handlers

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

var (
	courseCodeFence           = []byte("+++")
	courseMultipleChoiceFence = []byte("~~~")
	courseCodeChallengeFence  = []byte("???")
)

type courseCodeBlock struct {
	ast.Leaf
	Content string
}

type courseMultipleChoiceBlock struct {
	ast.Leaf
	Prompt  string
	Options []courseChoiceOption
	Err     string
}

type courseChoiceOption struct {
	Text        string
	Explanation string
	Correct     bool
}

type courseCodeChallengeBlock struct {
	ast.Leaf
	Prompt         string
	StarterCode    string
	CheckCode      string
	FailureMessage string
	Err            string
}

type courseMarkdownRenderer struct {
	multipleChoiceCount int
	codeChallengeCount  int
}

func markdownHasCourseCode(md []byte) bool {
	return bytes.Contains(md, courseCodeFence) || bytes.Contains(md, courseCodeChallengeFence)
}

func courseMarkdownToHTML(md []byte) []byte {
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	p.Opts.ParserHook = courseParserHook

	courseRenderer := &courseMarkdownRenderer{}
	renderer := mdhtml.NewRenderer(mdhtml.RendererOptions{
		Flags:          mdhtml.CommonFlags | mdhtml.HrefTargetBlank,
		RenderNodeHook: courseRenderer.renderHook,
	})

	return markdown.Render(p.Parse(md), renderer)
}

func courseParserHook(data []byte) (ast.Node, []byte, int) {
	if node, remaining, consumed := parseCourseCodeBlock(data); node != nil {
		return node, remaining, consumed
	}
	if node, remaining, consumed := parseCourseMultipleChoiceBlock(data); node != nil {
		return node, remaining, consumed
	}
	if node, remaining, consumed := parseCourseCodeChallengeBlock(data); node != nil {
		return node, remaining, consumed
	}

	return nil, nil, 0
}

func parseCourseCodeBlock(data []byte) (ast.Node, []byte, int) {
	content, consumed, ok := parseCourseFencedContent(data, courseCodeFence)
	if !ok {
		return nil, nil, 0
	}

	return &courseCodeBlock{Content: content}, nil, consumed
}

func parseCourseMultipleChoiceBlock(data []byte) (ast.Node, []byte, int) {
	content, consumed, ok := parseCourseFencedContent(data, courseMultipleChoiceFence)
	if !ok {
		return nil, nil, 0
	}

	return parseCourseMultipleChoiceContent(content), nil, consumed
}

func parseCourseCodeChallengeBlock(data []byte) (ast.Node, []byte, int) {
	content, consumed, ok := parseCourseFencedContent(data, courseCodeChallengeFence)
	if !ok {
		return nil, nil, 0
	}

	return parseCourseCodeChallengeContent(content), nil, consumed
}

func parseCourseFencedContent(data []byte, fence []byte) (string, int, bool) {
	if !bytes.HasPrefix(data, fence) {
		return "", 0, false
	}

	openLineEnd := bytes.IndexByte(data, '\n')
	if openLineEnd < 0 || !isCourseFenceLine(data[:openLineEnd], fence) {
		return "", 0, false
	}

	contentStart := openLineEnd + 1
	pos := contentStart
	for pos < len(data) {
		lineStart := pos
		lineEndOffset := bytes.IndexByte(data[pos:], '\n')
		lineEnd := len(data)
		next := len(data)
		if lineEndOffset >= 0 {
			lineEnd = pos + lineEndOffset
			next = lineEnd + 1
		}

		if isCourseFenceLine(data[lineStart:lineEnd], fence) {
			return strings.Trim(string(data[contentStart:lineStart]), "\r\n"), next, true
		}
		pos = next
	}

	return "", 0, false
}

func isCourseFenceLine(line []byte, fence []byte) bool {
	return bytes.Equal(bytes.Trim(line, " \t\r"), fence)
}

func parseCourseMultipleChoiceContent(content string) *courseMultipleChoiceBlock {
	lines := splitCourseLines(content)
	var promptLines []string
	var options []courseChoiceOption
	optionsStarted := false
	correctCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "= ") {
			optionsStarted = true
			correct := strings.HasPrefix(trimmed, "= ")
			text, explanation := splitCourseChoiceExplanation(strings.TrimSpace(trimmed[1:]))
			if text == "" {
				return &courseMultipleChoiceBlock{Err: "Multiple choice options must include answer text."}
			}
			if correct {
				correctCount++
			}
			options = append(options, courseChoiceOption{
				Text:        text,
				Explanation: explanation,
				Correct:     correct,
			})
			continue
		}

		if optionsStarted {
			if trimmed == "" {
				continue
			}
			return &courseMultipleChoiceBlock{Err: "Multiple choice option lines must start with '- ' or '= '."}
		}
		promptLines = append(promptLines, line)
	}

	prompt := strings.TrimSpace(strings.Join(promptLines, "\n"))
	if prompt == "" {
		return &courseMultipleChoiceBlock{Err: "Multiple choice blocks need a prompt before the options."}
	}
	if len(options) == 0 {
		return &courseMultipleChoiceBlock{Err: "Multiple choice blocks need at least one option."}
	}
	if correctCount == 0 {
		return &courseMultipleChoiceBlock{Err: "Multiple choice blocks need one correct option marked with '= '."}
	}

	return &courseMultipleChoiceBlock{
		Prompt:  prompt,
		Options: options,
	}
}

func splitCourseChoiceExplanation(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if strings.HasSuffix(raw, "]") {
		open := strings.LastIndex(raw, " [")
		if open >= 0 {
			return strings.TrimSpace(raw[:open]), strings.TrimSpace(strings.TrimSuffix(raw[open+2:], "]"))
		}
	}
	return raw, ""
}

func parseCourseCodeChallengeContent(content string) *courseCodeChallengeBlock {
	sections := splitCourseSections(content)
	if len(sections) < 3 || len(sections) > 4 {
		return &courseCodeChallengeBlock{Err: "Code challenge blocks need prompt, starter code, hidden check code, and optional failure message sections separated by '---'."}
	}

	prompt := strings.TrimSpace(sections[0])
	checkCode := strings.TrimSpace(sections[2])
	if prompt == "" {
		return &courseCodeChallengeBlock{Err: "Code challenge blocks need a prompt."}
	}
	if checkCode == "" {
		return &courseCodeChallengeBlock{Err: "Code challenge blocks need hidden check code."}
	}

	failureMessage := "Not quite. Try again."
	if len(sections) == 4 && strings.TrimSpace(sections[3]) != "" {
		failureMessage = strings.TrimSpace(sections[3])
	}

	return &courseCodeChallengeBlock{
		Prompt:         prompt,
		StarterCode:    strings.Trim(sections[1], "\r\n"),
		CheckCode:      checkCode,
		FailureMessage: failureMessage,
	}
}

func splitCourseLines(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return strings.Split(content, "\n")
}

func splitCourseSections(content string) []string {
	lines := splitCourseLines(content)
	var sections []string
	var current []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			sections = append(sections, strings.Trim(strings.Join(current, "\n"), "\n"))
			current = nil
			continue
		}
		current = append(current, line)
	}
	sections = append(sections, strings.Trim(strings.Join(current, "\n"), "\n"))
	return sections
}

func (r *courseMarkdownRenderer) renderHook(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
	if cb, ok := node.(*courseCodeBlock); ok {
		if entering {
			renderCourseCodeBlock(w, cb)
		}
		return ast.GoToNext, true
	}
	if mc, ok := node.(*courseMultipleChoiceBlock); ok {
		if entering {
			r.multipleChoiceCount++
			renderCourseMultipleChoiceBlock(w, mc, fmt.Sprintf("mc-%d", r.multipleChoiceCount))
		}
		return ast.GoToNext, true
	}
	if challenge, ok := node.(*courseCodeChallengeBlock); ok {
		if entering {
			r.codeChallengeCount++
			renderCourseCodeChallengeBlock(w, challenge, fmt.Sprintf("codecheck-%d", r.codeChallengeCount))
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

func renderCourseMultipleChoiceBlock(w io.Writer, block *courseMultipleChoiceBlock, blockID string) {
	if block.Err != "" {
		renderCourseAuthoringError(w, block.Err)
		return
	}

	escapedID := template.HTMLEscapeString(blockID)
	inputType := "radio"
	if courseCorrectChoiceCount(block.Options) > 1 {
		inputType = "checkbox"
	}
	fmt.Fprintf(w, `<div class="course-challenge course-multiple-choice" data-course-block-id="%s" data-course-block-type="multiple-choice" data-course-block-correct="false">
  <div class="course-challenge-prompt">%s</div>
  <fieldset class="course-choice-list">`, escapedID, renderCourseMarkdownFragment(block.Prompt))
	for i, option := range block.Options {
		correct := "false"
		if option.Correct {
			correct = "true"
		}

		fmt.Fprintf(w, `
    <label class="course-choice" data-choice-index="%d" data-correct="%s">
      <input class="course-choice-input" type="%s" name="%s" value="%d">
      <span class="course-choice-letter">%s</span>
      <span class="course-choice-body">
        <span class="course-choice-text">%s</span>`,
			i, correct, inputType, escapedID, i, choiceLetter(i), renderCourseInlineMarkdown(option.Text))
		if option.Explanation != "" {
			fmt.Fprintf(w, `
        <span class="course-choice-explanation" hidden>%s</span>`, renderCourseInlineMarkdown(option.Explanation))
		}
		fmt.Fprint(w, `
      </span>
    </label>`)
	}
	fmt.Fprint(w, `
  </fieldset>
  <div class="course-challenge-actions">
    <button class="course-challenge-submit" type="button" onclick="submitMultipleChoice(this)">Submit</button>
  </div>
  <div class="course-challenge-feedback" aria-live="polite" hidden></div>
</div>`)
}

func courseCorrectChoiceCount(options []courseChoiceOption) int {
	count := 0
	for _, option := range options {
		if option.Correct {
			count++
		}
	}
	return count
}

func renderCourseCodeChallengeBlock(w io.Writer, block *courseCodeChallengeBlock, blockID string) {
	if block.Err != "" {
		renderCourseAuthoringError(w, block.Err)
		return
	}

	fmt.Fprintf(w, `<div class="course-challenge course-code-challenge" data-course-block-id="%s" data-course-block-type="code-check" data-course-block-correct="false" data-check-code="%s" data-failure-message="%s">
  <div class="course-challenge-prompt">%s</div>
  <div class="cell codeset code-challenge-cell">
    <div class="code-challenge-spacer" aria-hidden="true"></div>
    <textarea class="code-area" spellcheck="false">%s</textarea>
    <div class="code-challenge-spacer" aria-hidden="true"></div>
    <div class="course-challenge-actions">
      <button class="course-challenge-submit" type="button" onclick="submitCodeChallenge(this)">Submit</button>
    </div>
    <div class="code-challenge-spacer" aria-hidden="true"></div>
    <div class="output challenge-output" aria-live="polite"></div>
  </div>
</div>`,
		template.HTMLEscapeString(blockID),
		template.HTMLEscapeString(base64.StdEncoding.EncodeToString([]byte(block.CheckCode))),
		template.HTMLEscapeString(base64.StdEncoding.EncodeToString([]byte(block.FailureMessage))),
		renderCourseMarkdownFragment(block.Prompt),
		template.HTMLEscapeString(block.StarterCode),
	)
}

func renderCourseAuthoringError(w io.Writer, message string) {
	fmt.Fprintf(w, `<div class="course-challenge course-authoring-error" role="alert">
  <strong>Course block error</strong>
  <p>%s</p>
</div>`, template.HTMLEscapeString(message))
}

func renderCourseMarkdownFragment(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}

	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	renderer := mdhtml.NewRenderer(mdhtml.RendererOptions{
		Flags: mdhtml.CommonFlags | mdhtml.HrefTargetBlank,
	})
	return string(markdown.Render(p.Parse([]byte(content)), renderer))
}

func renderCourseInlineMarkdown(content string) string {
	rendered := strings.TrimSpace(renderCourseMarkdownFragment(content))
	if strings.HasPrefix(rendered, "<p>") && strings.HasSuffix(rendered, "</p>") {
		rendered = strings.TrimPrefix(rendered, "<p>")
		rendered = strings.TrimSuffix(rendered, "</p>")
		rendered = strings.TrimSpace(rendered)
	}
	return rendered
}

func choiceLetter(index int) string {
	if index < 0 {
		return ""
	}

	var chars []byte
	for {
		chars = append([]byte{byte('A' + index%26)}, chars...)
		if index < 26 {
			break
		}
		index = index/26 - 1
	}
	return string(chars)
}
