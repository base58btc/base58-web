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

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
