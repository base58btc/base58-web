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
	writeTestFile(t, filepath.Join(courseDir, "01_intro.md"), "# Welcome to Protocol Thinking\n")
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

	if !strings.Contains(html, `class="cell codeset"`) {
		t.Fatalf("expected rendered code cell, got %s", html)
	}
	if !strings.Contains(html, `print(&#34;hello&#34;)`) {
		t.Fatalf("expected escaped code content, got %s", html)
	}
	if strings.Contains(html, "+++") {
		t.Fatalf("expected code fence to be consumed, got %s", html)
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
