package handlers

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/kodylow/base58-website/internal/config"
)

var (
	courseCodeFence           = []byte("+++")
	courseMultipleChoiceFence = []byte("~~~")
	courseCodeChallengeFence  = []byte("???")
	courseVideoFence          = []byte("!!!")
	courseAdmonitionFence     = []byte(":::")
)

var (
	courseVideoProviderPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)
	courseVideoIDPattern       = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	courseBlossomHashPattern   = regexp.MustCompile(`^[A-Fa-f0-9]{64}$`)
	courseBlockIDPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
)

type courseCodeBlock struct {
	ast.Leaf
	ID      string
	Content string
	Err     string
}

type courseMultipleChoiceBlock struct {
	ast.Leaf
	ID      string
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
	ID             string
	Prompt         string
	StarterCode    string
	CheckCode      string
	FailureMessage string
	Err            string
}

type courseVideoBlock struct {
	ast.Leaf
	Provider string
	ID       string
	Title    string
	MIME     string
	Poster   string
	Servers  []string
	Err      string
}

type courseNoteBlock struct {
	ast.Leaf
	Content string
}

type courseMarkdownRenderer struct {
	multipleChoiceCount int
	codeBlockCount      int
	codeChallengeCount  int
	courseSlug          string
	assetBaseURL        string
}

func markdownHasCourseCode(md []byte) bool {
	return bytes.Contains(md, courseCodeFence) || bytes.Contains(md, courseCodeChallengeFence)
}

func courseMarkdownToHTML(md []byte) []byte {
	return renderCourseMarkdownToHTML(md, "", "")
}

func courseMarkdownToHTMLForCourse(ctx *config.AppContext, courseSlug string, md []byte) []byte {
	return renderCourseMarkdownToHTML(md, courseSlug, courseAssetBaseURL(ctx))
}

func renderCourseMarkdownToHTML(md []byte, courseSlug, assetBaseURL string) []byte {
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	p.Opts.ParserHook = courseParserHook

	courseRenderer := &courseMarkdownRenderer{
		courseSlug:   courseSlug,
		assetBaseURL: assetBaseURL,
	}
	renderer := mdhtml.NewRenderer(mdhtml.RendererOptions{
		Flags:          mdhtml.CommonFlags | mdhtml.HrefTargetBlank,
		RenderNodeHook: courseRenderer.renderHook,
	})

	return markdown.Render(p.Parse(md), renderer)
}

func courseAssetBaseURL(ctx *config.AppContext) string {
	if ctx == nil || ctx.Env == nil || strings.TrimSpace(ctx.Env.CourseAssetBucket) == "" {
		return ""
	}
	region := strings.TrimSpace(ctx.Env.CourseAssetRegion)
	if region == "" {
		region = "nyc3"
	}
	base := "https://" + strings.TrimSpace(ctx.Env.CourseAssetBucket) + "." + region + ".cdn.digitaloceanspaces.com"
	if prefix := strings.Trim(strings.TrimSpace(ctx.Env.CourseAssetPrefix), "/"); prefix != "" {
		base += "/" + prefix
	}
	return base
}

func courseParserHook(data []byte) (ast.Node, []byte, int) {
	if node, remaining, consumed := parseCourseCodeBlock(data); node != nil {
		return node, remaining, consumed
	}
	if node, remaining, consumed := parseCourseVideoBlock(data); node != nil {
		return node, remaining, consumed
	}
	if node, remaining, consumed := parseCourseNoteBlock(data); node != nil {
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
	content, id, consumed, err, ok := parseCourseCodeFencedContent(data)
	if !ok {
		return nil, nil, 0
	}

	return &courseCodeBlock{ID: id, Content: content, Err: err}, nil, consumed
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

func parseCourseVideoBlock(data []byte) (ast.Node, []byte, int) {
	content, consumed, ok := parseCourseNamedFencedContent(data, courseVideoFence, "video")
	if !ok {
		return nil, nil, 0
	}

	return parseCourseVideoContent(content), nil, consumed
}

func parseCourseNoteBlock(data []byte) (ast.Node, []byte, int) {
	content, consumed, ok := parseCourseNamedFencedContent(data, courseAdmonitionFence, "note")
	if !ok {
		return nil, nil, 0
	}

	return &courseNoteBlock{Content: content}, nil, consumed
}

func parseCourseCodeFencedContent(data []byte) (string, string, int, string, bool) {
	if !bytes.HasPrefix(data, courseCodeFence) {
		return "", "", 0, "", false
	}

	openLineEnd := bytes.IndexByte(data, '\n')
	if openLineEnd < 0 {
		return "", "", 0, "", false
	}

	openLine := string(bytes.Trim(data[:openLineEnd], " \t\r"))
	fields := strings.Fields(openLine)
	if len(fields) == 1 && fields[0] == string(courseCodeFence) {
		content, consumed, ok := parseCourseFencedContent(data, courseCodeFence)
		return content, "", consumed, "", ok
	}
	if len(fields) != 2 || fields[0] != string(courseCodeFence) {
		return "", "", 0, "", false
	}

	content, consumed, ok := parseCourseFencedContentAfterOpen(data, courseCodeFence, openLineEnd)
	if !ok {
		return "", "", 0, "", false
	}
	id := fields[1]
	if !courseBlockIDPattern.MatchString(id) {
		return content, "", consumed, "Course code block id can only include letters, numbers, underscores, or hyphens, and must start with a letter or number.", true
	}
	return content, id, consumed, "", true
}

func parseCourseFencedContent(data []byte, fence []byte) (string, int, bool) {
	if !bytes.HasPrefix(data, fence) {
		return "", 0, false
	}

	openLineEnd := bytes.IndexByte(data, '\n')
	if openLineEnd < 0 || !isCourseFenceLine(data[:openLineEnd], fence) {
		return "", 0, false
	}

	return parseCourseFencedContentAfterOpen(data, fence, openLineEnd)
}

func parseCourseFencedContentAfterOpen(data []byte, fence []byte, openLineEnd int) (string, int, bool) {
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

func parseCourseNamedFencedContent(data []byte, fence []byte, name string) (string, int, bool) {
	if !bytes.HasPrefix(data, fence) {
		return "", 0, false
	}

	openLineEnd := bytes.IndexByte(data, '\n')
	if openLineEnd < 0 {
		return "", 0, false
	}

	openLine := string(bytes.Trim(data[:openLineEnd], " \t\r"))
	fields := strings.Fields(openLine)
	if len(fields) != 2 || fields[0] != string(fence) || fields[1] != name {
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
	id, lines, err := parseCourseBlockMetadata(lines)
	if err != "" {
		return &courseMultipleChoiceBlock{Err: err}
	}

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
		ID:      id,
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

func parseCourseVideoContent(content string) *courseVideoBlock {
	fields := make(map[string]string)
	var servers []string
	activeList := ""

	for _, line := range splitCourseLines(content) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			if activeList == "servers" {
				servers = append(servers, strings.TrimSpace(trimmed[2:]))
				continue
			}
			return &courseVideoBlock{Err: "Video list items are only supported under servers."}
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			return &courseVideoBlock{Err: "Video block lines must use key: value fields."}
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		activeList = ""
		if key == "" {
			return &courseVideoBlock{Err: "Video block fields need a key before ':'."}
		}
		if key == "servers" {
			activeList = "servers"
			servers = appendCourseVideoServers(servers, value)
			continue
		}
		fields[key] = value
	}

	provider := strings.ToLower(fields["provider"])
	id := fields["id"]
	if id == "" {
		id = fields["sha256"]
	}
	block := &courseVideoBlock{
		Provider: provider,
		ID:       id,
		Title:    fields["title"],
		MIME:     fields["type"],
		Poster:   fields["poster"],
		Servers:  servers,
	}
	if block.MIME == "" {
		block.MIME = fields["mime"]
	}
	if block.MIME == "" {
		block.MIME = "video/mp4"
	}

	return validateCourseVideoBlock(block)
}

func appendCourseVideoServers(servers []string, raw string) []string {
	for _, server := range strings.Split(raw, ",") {
		server = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(server), "- "))
		if server != "" {
			servers = append(servers, server)
		}
	}
	return servers
}

func validateCourseVideoBlock(block *courseVideoBlock) *courseVideoBlock {
	if block.Provider == "" {
		return &courseVideoBlock{Err: "Video blocks need a provider."}
	}
	if !courseVideoProviderPattern.MatchString(block.Provider) {
		return &courseVideoBlock{Err: "Video provider can only include lowercase letters, numbers, underscores, or hyphens."}
	}
	if block.ID == "" {
		return &courseVideoBlock{Err: "Video blocks need an id."}
	}

	switch block.Provider {
	case "youtube", "cloudflare":
		if !courseVideoIDPattern.MatchString(block.ID) {
			return &courseVideoBlock{Err: "Video id can only include letters, numbers, underscores, or hyphens."}
		}
	case "blossom":
		if !courseBlossomHashPattern.MatchString(block.ID) {
			return &courseVideoBlock{Err: "Blossom video blocks need a 64-character sha256 id."}
		}
		if len(block.Servers) == 0 {
			return &courseVideoBlock{Err: "Blossom video blocks need at least one server."}
		}
		for _, server := range block.Servers {
			if !isSafeCourseVideoServer(server) {
				return &courseVideoBlock{Err: "Blossom servers must be absolute https URLs."}
			}
		}
	default:
		return &courseVideoBlock{Err: fmt.Sprintf("Unsupported video provider %q.", block.Provider)}
	}

	if block.Poster != "" && !isSafeCourseVideoServer(block.Poster) {
		return &courseVideoBlock{Err: "Video poster must be an absolute https URL."}
	}
	if strings.ContainsAny(block.MIME, `"<>`) {
		return &courseVideoBlock{Err: "Video MIME type contains unsupported characters."}
	}

	return block
}

func isSafeCourseVideoServer(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return parsed.Scheme == "https" && parsed.Host != ""
}

func parseCourseCodeChallengeContent(content string) *courseCodeChallengeBlock {
	sections := splitCourseSections(content)
	if len(sections) < 3 || len(sections) > 4 {
		return &courseCodeChallengeBlock{Err: "Code challenge blocks need prompt, starter code, hidden check code, and optional failure message sections separated by '---'."}
	}

	promptLines := splitCourseLines(sections[0])
	id, promptLines, err := parseCourseBlockMetadata(promptLines)
	if err != "" {
		return &courseCodeChallengeBlock{Err: err}
	}

	prompt := strings.TrimSpace(strings.Join(promptLines, "\n"))
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
		ID:             id,
		Prompt:         prompt,
		StarterCode:    strings.Trim(sections[1], "\r\n"),
		CheckCode:      checkCode,
		FailureMessage: failureMessage,
	}
}

func parseCourseBlockMetadata(lines []string) (string, []string, string) {
	if len(lines) == 0 {
		return "", lines, ""
	}

	trimmed := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(strings.ToLower(trimmed), "id:") {
		return "", lines, ""
	}

	id := strings.TrimSpace(trimmed[3:])
	if id == "" {
		return "", nil, "Course block id cannot be empty."
	}
	if !courseBlockIDPattern.MatchString(id) {
		return "", nil, "Course block id can only include letters, numbers, underscores, or hyphens, and must start with a letter or number."
	}

	lines = lines[1:]
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	return id, lines, ""
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
	if image, ok := node.(*ast.Image); ok && entering {
		image.Destination = []byte(r.resolveImageDestination(string(image.Destination)))
		return ast.GoToNext, false
	}
	if cb, ok := node.(*courseCodeBlock); ok {
		if entering {
			r.codeBlockCount++
			blockID := cb.ID
			if blockID == "" {
				blockID = fmt.Sprintf("code-%d", r.codeBlockCount)
			}
			renderCourseCodeBlock(w, cb, blockID)
		}
		return ast.GoToNext, true
	}
	if video, ok := node.(*courseVideoBlock); ok {
		if entering {
			renderCourseVideoBlock(w, video)
		}
		return ast.GoToNext, true
	}
	if note, ok := node.(*courseNoteBlock); ok {
		if entering {
			r.renderCourseNoteBlock(w, note)
		}
		return ast.GoToNext, true
	}
	if mc, ok := node.(*courseMultipleChoiceBlock); ok {
		if entering {
			r.multipleChoiceCount++
			blockID := mc.ID
			if blockID == "" {
				blockID = fmt.Sprintf("mc-%d", r.multipleChoiceCount)
			}
			renderCourseMultipleChoiceBlock(w, mc, blockID)
		}
		return ast.GoToNext, true
	}
	if challenge, ok := node.(*courseCodeChallengeBlock); ok {
		if entering {
			r.codeChallengeCount++
			blockID := challenge.ID
			if blockID == "" {
				blockID = fmt.Sprintf("codecheck-%d", r.codeChallengeCount)
			}
			renderCourseCodeChallengeBlock(w, challenge, blockID)
		}
		return ast.GoToNext, true
	}

	return ast.GoToNext, false
}

func (r *courseMarkdownRenderer) resolveImageDestination(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || r.courseSlug == "" {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err == nil && (parsed.IsAbs() || strings.HasPrefix(raw, "/") || parsed.Fragment != "") {
		return raw
	}
	clean := strings.TrimLeft(raw, "/")
	if strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") || clean == ".." {
		return raw
	}
	base := strings.TrimRight(r.assetBaseURL, "/")
	if base == "" {
		return raw
	}
	return base + "/" + url.PathEscape(r.courseSlug) + "/" + pathEscapeCourseAsset(clean)
}

func pathEscapeCourseAsset(assetPath string) string {
	parts := strings.Split(assetPath, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func renderCourseCodeBlock(w io.Writer, cb *courseCodeBlock, blockID string) {
	if cb.Err != "" {
		renderCourseAuthoringError(w, cb.Err)
		return
	}

	escapedID := template.HTMLEscapeString(blockID)
	idAttr := ""
	if cb.ID != "" {
		idAttr = fmt.Sprintf(` id="%s"`, escapedID)
	}
	escaped := template.HTMLEscapeString(cb.Content)
	fmt.Fprintf(w, `<div%s class="cell codeset course-code-cell" data-course-block-id="%s" data-course-block-type="code-cell">
  <div class="code-runner">
    <button class="run-btn" type="button" onclick="runCellCode(this)" aria-label="Run code">
      <svg fill="currentColor" height="16" width="16" viewBox="0 0 330 330" aria-hidden="true">
        <path d="M37.728,328.12c2.266,1.256,4.77,1.88,7.272,1.88c2.763,0,5.522-0.763,7.95-2.28l240-149.999c4.386-2.741,7.05-7.548,7.05-12.72c0-5.172-2.664-9.979-7.05-12.72L52.95,2.28c-4.625-2.891-10.453-3.043-15.222-0.4C32.959,4.524,30,9.547,30,15v300C30,320.453,32.959,325.476,37.728,328.12z"></path>
      </svg>
    </button>
  </div>
  <div class="code-editor-shell">
    <pre class="code-highlight" aria-hidden="true"><code></code></pre>
    <textarea class="code-area" spellcheck="false">%s</textarea>
  </div>
  <div class="output-prompt" aria-hidden="true"></div>
  <div class="output" aria-live="polite"></div>
</div>`, idAttr, escapedID, escaped)
}

func renderCourseVideoBlock(w io.Writer, block *courseVideoBlock) {
	if block.Err != "" {
		renderCourseAuthoringError(w, block.Err)
		return
	}

	switch block.Provider {
	case "youtube":
		renderCourseIframeVideoBlock(w, block, "https://www.youtube-nocookie.com/embed/"+url.PathEscape(block.ID))
	case "cloudflare":
		renderCourseIframeVideoBlock(w, block, "https://iframe.videodelivery.net/"+url.PathEscape(block.ID))
	case "blossom":
		renderCourseBlossomVideoBlock(w, block)
	}
}

func renderCourseIframeVideoBlock(w io.Writer, block *courseVideoBlock, src string) {
	title := block.Title
	if title == "" {
		title = fmt.Sprintf("%s course video", block.Provider)
	}

	fmt.Fprintf(w, `<figure class="course-video course-video-%s">`, template.HTMLEscapeString(block.Provider))
	if block.Title != "" {
		fmt.Fprintf(w, `
  <figcaption class="course-video-title">%s</figcaption>`, template.HTMLEscapeString(block.Title))
	}
	fmt.Fprintf(w, `
  <div class="course-video-frame">
    <iframe src="%s" title="%s" loading="lazy" allow="accelerometer; autoplay; encrypted-media; gyroscope; picture-in-picture; web-share" allowfullscreen></iframe>
  </div>
</figure>`, template.HTMLEscapeString(src), template.HTMLEscapeString(title))
}

func renderCourseBlossomVideoBlock(w io.Writer, block *courseVideoBlock) {
	fmt.Fprint(w, `<figure class="course-video course-video-blossom">`)
	if block.Title != "" {
		fmt.Fprintf(w, `
  <figcaption class="course-video-title">%s</figcaption>`, template.HTMLEscapeString(block.Title))
	}

	poster := ""
	if block.Poster != "" {
		poster = fmt.Sprintf(` poster="%s"`, template.HTMLEscapeString(block.Poster))
	}
	fmt.Fprintf(w, `
  <video class="course-video-native" controls preload="metadata"%s>`, poster)
	for _, server := range block.Servers {
		src := strings.TrimRight(server, "/") + "/" + block.ID
		fmt.Fprintf(w, `
    <source src="%s" type="%s">`, template.HTMLEscapeString(src), template.HTMLEscapeString(block.MIME))
	}
	fmt.Fprint(w, `
    Your browser does not support embedded videos.
  </video>
</figure>`)
}

func (r *courseMarkdownRenderer) renderCourseNoteBlock(w io.Writer, block *courseNoteBlock) {
	body := renderCourseMarkdownToHTML([]byte(block.Content), r.courseSlug, r.assetBaseURL)
	fmt.Fprintf(w, `<aside class="course-note" role="note">
  <div class="course-note-icon" aria-hidden="true">
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none">
      <path d="M12 21C16.9706 21 21 16.9706 21 12C21 7.02944 16.9706 3 12 3C7.02944 3 3 7.02944 3 12C3 16.9706 7.02944 21 12 21Z" stroke="currentColor" stroke-width="2"></path>
      <path d="M12 11V16" stroke="currentColor" stroke-width="2" stroke-linecap="round"></path>
      <path d="M12 8H12.01" stroke="currentColor" stroke-width="2" stroke-linecap="round"></path>
    </svg>
  </div>
  <div class="course-note-body">%s</div>
</aside>`, body)
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
  <div class="course-challenge-header">
    <div class="course-challenge-prompt">%s</div>
    <span class="course-challenge-result-icon" aria-live="polite"></span>
  </div>
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

	escapedID := template.HTMLEscapeString(blockID)
	fmt.Fprintf(w, `<div class="course-challenge course-code-challenge" data-course-block-id="%s" data-course-block-type="code-check" data-course-block-correct="false" data-check-code="%s" data-failure-message="%s">
  <div class="course-challenge-prompt">%s</div>
  <div class="cell codeset code-challenge-cell" data-course-block-id="%s" data-course-block-type="code-cell">
    <div class="code-runner">
      <button class="run-btn" type="button" onclick="runCellCode(this)" aria-label="Run code">
        <svg fill="currentColor" height="16" width="16" viewBox="0 0 330 330" aria-hidden="true">
          <path d="M37.728,328.12c2.266,1.256,4.77,1.88,7.272,1.88c2.763,0,5.522-0.763,7.95-2.28l240-149.999c4.386-2.741,7.05-7.548,7.05-12.72c0-5.172-2.664-9.979-7.05-12.72L52.95,2.28c-4.625-2.891-10.453-3.043-15.222-0.4C32.959,4.524,30,9.547,30,15v300C30,320.453,32.959,325.476,37.728,328.12z"></path>
        </svg>
      </button>
    </div>
    <div class="code-editor-shell">
      <pre class="code-highlight" aria-hidden="true"><code></code></pre>
      <textarea class="code-area" spellcheck="false">%s</textarea>
    </div>
    <div class="code-challenge-spacer" aria-hidden="true"></div>
    <div class="course-challenge-actions">
      <button class="course-challenge-submit" type="button" onclick="submitCodeChallenge(this)">Run Tests</button>
      <span class="course-challenge-result-icon" aria-live="polite"></span>
    </div>
    <div class="output-prompt" aria-hidden="true"></div>
    <div class="output challenge-output" aria-live="polite"></div>
  </div>
</div>`,
		escapedID,
		template.HTMLEscapeString(base64.StdEncoding.EncodeToString([]byte(block.CheckCode))),
		template.HTMLEscapeString(base64.StdEncoding.EncodeToString([]byte(block.FailureMessage))),
		renderCourseMarkdownFragment(block.Prompt),
		escapedID,
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
