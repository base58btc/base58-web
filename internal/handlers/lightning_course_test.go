package handlers

import (
	"bytes"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Validate the actual notebook port against the production renderer. An optional
// output directory provides complete lesson pages for local visual inspection.
func TestLightningCourseMarkdown(t *testing.T) {
	testNotebookCourseMarkdown(t, "lightning", 6)
}

func TestTaprootCourseMarkdown(t *testing.T) {
	testNotebookCourseMarkdown(t, "taproot", 9)
}

func testNotebookCourseMarkdown(t *testing.T, slug string, pageCount int) {
	t.Helper()
	root := filepath.Join("..", "..", "courses")
	files, err := filepath.Glob(filepath.Join(root, slug, "*.md"))
	if err != nil || len(files) != pageCount {
		t.Fatalf("expected %d %s lessons: %v (%d)", pageCount, slug, err, len(files))
	}
	tmpl, err := template.ParseFiles(
		filepath.Join("..", "..", "templates", "courses", "lesson.tmpl"),
		filepath.Join("..", "..", "templates", "sections", "head.tmpl"),
	)
	if err != nil {
		t.Fatal(err)
	}
	openCell := regexp.MustCompile(`(?m)^\+\+\+ ([A-Za-z0-9_-]+) python-vm$`)
	openChallenge := regexp.MustCompile(`(?m)^\?\?\?\nid: ([A-Za-z0-9_-]+)\nruntime: python-vm$`)
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			lesson := strings.TrimSuffix(filepath.Base(file), ".md")
			outline, err := loadLocalCourse(root, slug, lesson)
			if err != nil {
				t.Fatal(err)
			}
			intro := lesson == "00-welcome"
			if len(outline.Pages) != pageCount || (!intro && !markdownHasCourseCode(outline.Markdown)) {
				t.Fatal("lesson is missing navigation or executable cells")
			}
			html := isolatedCourseImages(courseMarkdownToHTML(outline.Markdown))
			if bytes.Contains(html, []byte("course-authoring-error")) || bytes.Contains(html, []byte("+++")) || bytes.Contains(html, []byte("???")) || bytes.Contains(html, []byte("~~~")) || bytes.Contains(html, []byte("::: note")) || bytes.Contains(html, []byte("::: references")) {
				t.Fatal("unrendered fence or authoring error in lesson")
			}
			cells := openCell.FindAllSubmatch(outline.Markdown, -1)
			challenges := openChallenge.FindAllSubmatch(outline.Markdown, -1)
			if (!intro && len(cells)+len(challenges) == 0) || bytes.Count(html, []byte(`data-course-runtime="python-vm"`)) != len(cells)+2*len(challenges) {
				t.Fatal("native Python cell count does not match the Markdown")
			}
			seen := map[string]bool{}
			for _, cell := range append(cells, challenges...) {
				id := string(cell[1])
				if seen[id] || !bytes.Contains(html, []byte(`data-course-block-id="`+id+`"`)) {
					t.Fatalf("missing or duplicate stable cell ID %s", id)
				}
				seen[id] = true
			}
			if !intro && !bytes.Contains(html, []byte(`class="course-choice-explanation" hidden`)) {
				t.Fatal("question feedback was not rendered")
			}
			var page bytes.Buffer
			err = tmpl.ExecuteTemplate(&page, "lesson.tmpl", CourseLessonData{
				CourseSlug: slug, CourseTitle: outline.CourseTitle,
				HeroURL:         "/static/img/courses/" + slug + ".png",
				DefinitionCells: outline.prerequisiteCells(),
				Current:         outline.Current, Previous: outline.Previous, Next: outline.Next,
				Sidebar: outline.Items, ContentHTML: template.HTML(html), HasCode: markdownHasCourseCode(outline.Markdown), IsLocalPreview: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if export := os.Getenv("BASE58_RENDER_COURSE_DIR"); export != "" {
				dest := filepath.Join(export, "courses", slug, lesson)
				if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(dest, page.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
