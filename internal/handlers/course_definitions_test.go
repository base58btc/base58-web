package handlers

import (
	"bytes"
	"encoding/json"
	"html/template"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestCourseDefinitionCellsUseRenderedIDsAndExcludeChecks(t *testing.T) {
	md := []byte("# Definitions\n\n+++\nold = 1\n+++\n\n+++ helper python-vm\nfrom helpers import decode\n+++\n\n???\nid: challenge\nruntime: python-vm\nWrite a function.\n---\ndef answer(): pass\n---\nhidden_fixture = 42\nassert answer() == hidden_fixture\n???\n")
	cells := courseDefinitionCells(md, "01-definitions", "/courses/demo/01-definitions")
	if len(cells) != 3 {
		t.Fatalf("expected 3 editable cells, got %#v", cells)
	}
	html := string(courseMarkdownToHTML(md))
	for i, id := range []string{"code-1", "helper", "challenge"} {
		if cells[i].BlockID != id || !strings.Contains(html, `id="`+id+`"`) {
			t.Fatalf("definition destination does not match rendered cell: %#v", cells[i])
		}
		if strings.Contains(cells[i].Source, "hidden_fixture") {
			t.Fatal("challenge checks leaked into definition candidates")
		}
	}
	if cells[0].Runtime != "" || cells[1].Runtime != "python-vm" || cells[2].Runtime != "python-vm" {
		t.Fatalf("lost interpreter boundaries: %#v", cells)
	}
}

func TestCourseDefinitionsFollowLessonOrderAndPinnedVersion(t *testing.T) {
	files := map[string]string{
		"01-intro.md":           "# Intro\n\n+++ pinned python-vm\nversion_name = 'pinned'\n+++\n",
		"02-part/01-example.md": "# Example\n\n+++ nested python-vm\nfrom helpers import decode\n+++\n",
		"03-current.md":         "# Current\n\n+++ now python-vm\ncurrent = 1\n+++\n",
		"04-future.md":          "# Future\n\n+++ later python-vm\nfuture = 1\n+++\n",
	}
	root := t.TempDir()
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, "demo", path)), 0o755); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(root, "demo", path), content)
	}
	local, err := loadLocalCourse(root, "demo", "03-current")
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := buildCourseOutlineFromFiles("demo", "03-current", files)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(local.prerequisiteCells(), pinned.prerequisiteCells()) {
		t.Fatal("local and published course indices differ")
	}
	// Editing local course files must not alter a student's pinned definition source.
	writeTestFile(t, filepath.Join(root, "demo", "01-intro.md"), "# Intro\n\n+++ changed python-vm\nnew_version = 2\n+++\n")
	cells := pinned.prerequisiteCells()
	if len(cells) != 2 || cells[0].BlockID != "pinned" || cells[1].LessonPath != "02-part/01-example" || cells[1].URL != "/courses/demo/02-part/01-example" {
		t.Fatalf("unexpected prerequisite cells: %#v", cells)
	}
}

func TestCourseDefinitionJSONCannotCloseItsScriptElement(t *testing.T) {
	tmpl, err := template.ParseFiles(filepath.Join("..", "..", "templates", "courses", "lesson.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	want := []courseDefinitionCell{{LessonPath: "01", URL: "/courses/demo/01", BlockID: "code-1", Source: `value = '</script><img src=x onerror=alert(1)>'`}}
	var out bytes.Buffer
	err = tmpl.ExecuteTemplate(&out, "lesson.tmpl", CourseLessonData{CourseSlug: "demo", Current: &CourseNavItem{Path: "02"}, DefinitionCells: want})
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`<script id="course-definition-cells" type="application/json">(.*?)</script>`).FindSubmatch(out.Bytes())
	if len(match) != 2 || bytes.Contains(match[1], []byte("<img")) {
		t.Fatal("unsafe or missing definition JSON")
	}
	var got []courseDefinitionCell
	if err := json.Unmarshal(match[1], &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("definition source changed: %#v", got)
	}
}
