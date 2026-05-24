package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadLocalCourseBuildsNumberedSidebar(t *testing.T) {
	root := t.TempDir()
	courseDir := filepath.Join(root, "intro-proto")
	if err := os.MkdirAll(filepath.Join(courseDir, "04_reading"), 0o755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(courseDir, "02_counting.md"), "# Counting Fruits\n")
	writeTestFile(t, filepath.Join(courseDir, "01_intro.md"), "# Welcome to Protocol Thinking\n\n~~~\nPick one.\n\n= A\n- B\n~~~\n")
	writeTestFile(t, filepath.Join(courseDir, "03_writing.md"), "# Writing Fruits to Send\n")
	writeTestFile(t, filepath.Join(courseDir, "04_reading", "01_stepone.md"), "## Welcome to step one in reading\n")

	outline, err := loadLocalCourse(root, "intro-proto", "04_reading/01_stepone")
	if err != nil {
		t.Fatal(err)
	}

	if len(outline.Items) != 4 {
		t.Fatalf("expected 4 top-level sidebar items, got %d", len(outline.Items))
	}
	if outline.Items[0].Number != "01" || outline.Items[0].Title != "Welcome to Protocol Thinking" {
		t.Fatalf("first item = %#v", outline.Items[0])
	}
	if outline.Items[0].ChallengeCount != 1 {
		t.Fatalf("expected first item challenge count 1, got %d", outline.Items[0].ChallengeCount)
	}
	if outline.Items[3].Number != "04" || outline.Items[3].Title != "Reading" {
		t.Fatalf("directory parent = %#v", outline.Items[3])
	}
	if len(outline.Items[3].Children) != 1 {
		t.Fatalf("expected 1 child under directory parent, got %d", len(outline.Items[3].Children))
	}
	if !outline.Items[3].Active {
		t.Fatal("expected directory parent to be active when child is current")
	}
	if !outline.Items[3].Children[0].Current {
		t.Fatal("expected nested page to be current")
	}
	if outline.Current.Path != "04_reading/01_stepone" {
		t.Fatalf("current path = %s", outline.Current.Path)
	}
	if outline.Current.DisplayNumber != "04.01" {
		t.Fatalf("current display number = %s", outline.Current.DisplayNumber)
	}
	if outline.Previous == nil || outline.Previous.Path != "03_writing" {
		t.Fatalf("previous = %#v", outline.Previous)
	}
	if outline.Next != nil {
		t.Fatalf("expected no next page, got %#v", outline.Next)
	}
}

func TestLoadLocalCourseDirectoryPathSelectsFirstSubpage(t *testing.T) {
	root := t.TempDir()
	courseDir := filepath.Join(root, "intro-proto")
	if err := os.MkdirAll(filepath.Join(courseDir, "04_reading"), 0o755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(courseDir, "01_intro.md"), "# Intro\n")
	writeTestFile(t, filepath.Join(courseDir, "04_reading", "00_overview.md"), "# Reading Overview\n")
	writeTestFile(t, filepath.Join(courseDir, "04_reading", "01_stepone.md"), "# Step One\n")

	outline, err := loadLocalCourse(root, "intro-proto", "04_reading")
	if err != nil {
		t.Fatal(err)
	}

	if outline.Current.Path != "04_reading/00_overview" {
		t.Fatalf("current path = %s", outline.Current.Path)
	}
}

func TestLoadLocalCourseDirectoryPathUsesSiblingMarkdownWhenPresent(t *testing.T) {
	root := t.TempDir()
	courseDir := filepath.Join(root, "intro-proto")
	if err := os.MkdirAll(filepath.Join(courseDir, "04_reading"), 0o755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(courseDir, "01_intro.md"), "# Intro\n")
	writeTestFile(t, filepath.Join(courseDir, "04_reading.md"), "# Reading Overview\n")
	writeTestFile(t, filepath.Join(courseDir, "04_reading", "01_stepone.md"), "# Step One\n")

	outline, err := loadLocalCourse(root, "intro-proto", "04_reading")
	if err != nil {
		t.Fatal(err)
	}

	if outline.Current.Path != "04_reading" {
		t.Fatalf("current path = %s", outline.Current.Path)
	}
	if outline.Current.Title != "Reading Overview" {
		t.Fatalf("current title = %s", outline.Current.Title)
	}
	if !outline.Items[1].Current || !outline.Items[1].Active {
		t.Fatalf("expected top-level directory page to be current and active: %#v", outline.Items[1])
	}
	if len(outline.Items[1].Children) != 1 {
		t.Fatalf("expected child page under sibling markdown parent, got %d", len(outline.Items[1].Children))
	}
	if outline.Next == nil || outline.Next.Path != "04_reading/01_stepone" {
		t.Fatalf("next = %#v", outline.Next)
	}
}

func TestCourseMarkdownRendersCodeBlocks(t *testing.T) {
	html := string(courseMarkdownToHTML([]byte("Before\n\n+++\nprint(\"hello\")\n+++\n\nAfter\n")))

	if !strings.Contains(html, `class="cell codeset course-code-cell"`) {
		t.Fatalf("expected rendered code cell, got %s", html)
	}
	if !strings.Contains(html, `data-course-block-id="code-1"`) {
		t.Fatalf("expected stable code cell id, got %s", html)
	}
	if !strings.Contains(html, `print(&#34;hello&#34;)`) {
		t.Fatalf("expected escaped code content, got %s", html)
	}
	if strings.Contains(html, "+++") {
		t.Fatalf("expected code fence to be consumed, got %s", html)
	}
}

func TestCourseMarkdownRendersVideoBlocks(t *testing.T) {
	html := string(courseMarkdownToHTML([]byte("!!! video\nprovider: youtube\nid: dQw4w9WgXcQ\ntitle: Lesson intro\n!!!\n\n!!! video\nprovider: cloudflare\nid: abc_123-title\n!!!\n")))

	if !strings.Contains(html, `class="course-video course-video-youtube"`) {
		t.Fatalf("expected youtube video block, got %s", html)
	}
	if !strings.Contains(html, `https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ`) {
		t.Fatalf("expected youtube nocookie embed, got %s", html)
	}
	if !strings.Contains(html, `Lesson intro`) {
		t.Fatalf("expected video title, got %s", html)
	}
	if !strings.Contains(html, `class="course-video course-video-cloudflare"`) {
		t.Fatalf("expected cloudflare video block, got %s", html)
	}
	if !strings.Contains(html, `https://iframe.videodelivery.net/abc_123-title`) {
		t.Fatalf("expected cloudflare iframe embed, got %s", html)
	}
	if strings.Contains(html, "!!!") {
		t.Fatalf("expected video fence to be consumed, got %s", html)
	}
}

func TestCourseMarkdownRendersBlossomVideoBlocks(t *testing.T) {
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	html := string(courseMarkdownToHTML([]byte("!!! video\nprovider: blossom\nsha256: " + hash + "\nservers:\n  - https://cdn1.example.com\n  - https://cdn2.example.com/base58\nmime: video/mp4\nposter: https://cdn1.example.com/poster.jpg\n!!!\n")))

	if !strings.Contains(html, `class="course-video course-video-blossom"`) {
		t.Fatalf("expected blossom video block, got %s", html)
	}
	if !strings.Contains(html, `poster="https://cdn1.example.com/poster.jpg"`) {
		t.Fatalf("expected poster attribute, got %s", html)
	}
	if !strings.Contains(html, `src="https://cdn1.example.com/`+hash+`"`) {
		t.Fatalf("expected first blossom source, got %s", html)
	}
	if !strings.Contains(html, `src="https://cdn2.example.com/base58/`+hash+`"`) {
		t.Fatalf("expected second blossom source, got %s", html)
	}
}

func TestCourseMarkdownRendersVideoAuthoringErrors(t *testing.T) {
	html := string(courseMarkdownToHTML([]byte("!!! video\nprovider: blossom\nsha256: nope\nservers: http://cdn.example.com\n!!!\n")))

	if !strings.Contains(html, `class="course-challenge course-authoring-error"`) {
		t.Fatalf("expected authoring error block, got %s", html)
	}
	if !strings.Contains(html, `64-character sha256`) {
		t.Fatalf("expected blossom sha256 authoring error, got %s", html)
	}

	html = string(courseMarkdownToHTML([]byte("!!! video\nprovider: vimeo\nid: abc\n!!!\n")))
	if !strings.Contains(html, `Unsupported video provider`) {
		t.Fatalf("expected unsupported provider authoring error, got %s", html)
	}
}

func TestCourseMarkdownRendersMultipleChoiceBlocks(t *testing.T) {
	html := string(courseMarkdownToHTML([]byte("Before\n\n~~~\nWhat byte size can hold `20_000`?\n\n- 1 byte [255 is too small.]\n= 2 bytes [Correct: 65,535 can hold it.]\n- 4 bytes [This works, but is larger than necessary.]\n~~~\n\nAfter\n")))

	if !strings.Contains(html, `class="course-challenge course-multiple-choice"`) {
		t.Fatalf("expected multiple choice block, got %s", html)
	}
	if !strings.Contains(html, `data-course-block-id="mc-1"`) {
		t.Fatalf("expected deterministic multiple choice id, got %s", html)
	}
	if !strings.Contains(html, `<code>20_000</code>`) {
		t.Fatalf("expected markdown prompt rendering, got %s", html)
	}
	if !strings.Contains(html, `class="course-choice-letter">A</span>`) {
		t.Fatalf("expected lettered answers, got %s", html)
	}
	if !strings.Contains(html, `type="radio"`) {
		t.Fatalf("expected single-answer multiple choice to render radio inputs, got %s", html)
	}
	if !strings.Contains(html, `data-correct="true"`) {
		t.Fatalf("expected correct answer marker, got %s", html)
	}
	if !strings.Contains(html, `Correct: 65,535 can hold it.`) {
		t.Fatalf("expected inline explanation rendering, got %s", html)
	}
	if strings.Contains(html, "~~~") {
		t.Fatalf("expected multiple choice fence to be consumed, got %s", html)
	}
}

func TestCourseMarkdownRendersStableMultipleChoiceIDs(t *testing.T) {
	html := string(courseMarkdownToHTML([]byte("~~~\nid: pineapple-byte-size\n\nWhat byte size can hold `20_000`?\n\n- 1 byte\n= 2 bytes\n~~~\n")))

	if !strings.Contains(html, `data-course-block-id="pineapple-byte-size"`) {
		t.Fatalf("expected stable multiple choice id, got %s", html)
	}
	if strings.Contains(html, `id: pineapple-byte-size`) {
		t.Fatalf("expected id metadata to be consumed, got %s", html)
	}
}

func TestCourseMarkdownRendersMultipleAnswerBlocks(t *testing.T) {
	html := string(courseMarkdownToHTML([]byte("~~~\nPick all byte-sized aliases.\n\n= byte\n= uint8\n- uint16\n~~~\n")))

	if !strings.Contains(html, `class="course-challenge course-multiple-choice"`) {
		t.Fatalf("expected multiple choice block, got %s", html)
	}
	if !strings.Contains(html, `type="checkbox"`) {
		t.Fatalf("expected multiple-answer block to render checkbox inputs, got %s", html)
	}
	if count := strings.Count(html, `data-correct="true"`); count != 2 {
		t.Fatalf("expected two correct answer markers, got %d in %s", count, html)
	}
	if strings.Contains(html, `can only have one correct option`) {
		t.Fatalf("expected multiple correct options to be accepted, got %s", html)
	}
}

func TestCourseMarkdownRendersMultipleChoiceAuthoringErrors(t *testing.T) {
	html := string(courseMarkdownToHTML([]byte("~~~\nPick one.\n\n- A\n- B\n~~~\n")))

	if !strings.Contains(html, `class="course-challenge course-authoring-error"`) {
		t.Fatalf("expected authoring error block, got %s", html)
	}
	if !strings.Contains(html, `need one correct option`) {
		t.Fatalf("expected missing correct option error, got %s", html)
	}
}

func TestCourseMarkdownRendersCodeChallengeBlocks(t *testing.T) {
	html := string(courseMarkdownToHTML([]byte("???\nWrite `answer`.\n---\ndef answer():\n    pass\n---\nassert answer() == 4\n---\nTry returning 4.\n???\n")))

	if !strings.Contains(html, `class="course-challenge course-code-challenge"`) {
		t.Fatalf("expected code challenge block, got %s", html)
	}
	if !strings.Contains(html, `data-course-block-id="codecheck-1"`) {
		t.Fatalf("expected deterministic code challenge id, got %s", html)
	}
	if !strings.Contains(html, `<code>answer</code>`) {
		t.Fatalf("expected markdown prompt rendering, got %s", html)
	}
	if !strings.Contains(html, `def answer():`) {
		t.Fatalf("expected starter code, got %s", html)
	}
	if strings.Contains(html, `assert answer() == 4`) {
		t.Fatalf("expected hidden check code to be encoded, got %s", html)
	}
	if strings.Contains(html, `Try returning 4.`) {
		t.Fatalf("expected failure message to be encoded, got %s", html)
	}
	if !markdownHasCourseCode([]byte("???\nPrompt\n---\ncode\n---\nassert True\n???\n")) {
		t.Fatal("expected code challenge to require pyodide")
	}
}

func TestCourseMarkdownRendersStableCodeChallengeIDs(t *testing.T) {
	html := string(courseMarkdownToHTML([]byte("???\nid: write-answer\n\nWrite `answer`.\n---\ndef answer():\n    pass\n---\nassert answer() == 4\n???\n")))

	if !strings.Contains(html, `data-course-block-id="write-answer"`) {
		t.Fatalf("expected stable code challenge id, got %s", html)
	}
	if strings.Contains(html, `id: write-answer`) {
		t.Fatalf("expected id metadata to be consumed, got %s", html)
	}
}

func TestCourseMarkdownRendersCodeChallengeAuthoringErrors(t *testing.T) {
	html := string(courseMarkdownToHTML([]byte("???\nPrompt only.\n???\n")))

	if !strings.Contains(html, `class="course-challenge course-authoring-error"`) {
		t.Fatalf("expected authoring error block, got %s", html)
	}
	if !strings.Contains(html, `need prompt, starter code, hidden check code`) {
		t.Fatalf("expected missing sections error, got %s", html)
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
