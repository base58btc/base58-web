package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/database"
	"github.com/kodylow/base58-website/internal/types"
)

func catalogFixture() database.CatalogExport {
	return database.CatalogExport{Format: 1, Courses: []*types.Course{
		{ID: "original-taproot-id", Tag: "taproot", Title: "Taproot Intensive", Difficulty: "Experienced", Format: []types.CourseFormat{"Online Cohort"}, Topic: "Scripts & Transactions", Availability: "Coming Soon", Popularity: 5, PriceUSD: 121, Visible: true, Feature: true, HeaderImg: "/static/img/courses/taproot.png"},
		{ID: "original-lightning-id", Tag: "lightning", Title: "Lightning: BOLTs in a flash", Difficulty: "Experienced", Format: []types.CourseFormat{"Online Cohort"}, Topic: "Lightning & Layer 2", Availability: "Coming Soon", Popularity: 7, PriceUSD: 121, Visible: false, Feature: true, HeaderImg: "/static/img/courses/lightning.png", ShortDesc: "Original description", LongDesc: "Original details", PreReqs: "Python and Bitcoin", Flavor: "Protocols", Rating: 4.5, Includes: []string{"11-hours on-demand video", "15 downloadable resources"}, Chapters: []types.ChapterInfo{{Title: "Old chapter", Desc: "Old detail"}}},
	}, Sessions: map[string][]*types.CourseSession{"taproot": {{ID: "schedule-id", ClassRef: "taproot-1", CourseName: "Taproot Intensive", Date: []string{"TBD"}, Online: true, SignupCode: "private-code", TotalSeats: 20, SeatsAvail: 10}}}}
}

func TestCourseCatalogPostgres(t *testing.T) {
	_, _, db := commandTestServer(t, "")
	export := catalogFixture()
	// A scheduled class can be linked from more than one catalog entry.
	export.Sessions["lightning"] = export.Sessions["taproot"]
	ctx := context.Background()
	// The former startup sync may already have created a partial catalog row.
	if _, err := db.Exec(`INSERT INTO courses(slug,title) VALUES('lightning','Partial import'); INSERT INTO course_entitlements(person_id,course_slug) VALUES(1,'lightning')`); err != nil {
		t.Fatal(err)
	}
	n, err := database.ImportCatalog(ctx, db, export)
	if err != nil || n != 2 {
		t.Fatalf("import: %d %v", n, err)
	}
	for _, want := range export.Courses {
		got, err := database.GetCourse(db, want.Tag)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("catalog metadata changed: got %+v want %+v", got, want)
		}
	}
	// An intentionally sparse new catalog record must not break public listings.
	if err := database.SaveCourse(db, &types.Course{Tag: "sparse-course", Title: "Sparse course"}, "published", true); err != nil {
		t.Fatal(err)
	}
	sessions, err := database.GetCourseSessions(db, "taproot")
	if err != nil || !reflect.DeepEqual(sessions, export.Sessions["taproot"]) {
		t.Fatalf("schedule changed: %+v %v", sessions, err)
	}
	linked, err := database.GetCourseSessions(db, "lightning")
	if err != nil || !reflect.DeepEqual(linked, export.Sessions["lightning"]) {
		t.Fatalf("schedule relation lost: %+v %v", linked, err)
	}
	// Invalid related data must roll back the entire import.
	invalid := database.CatalogExport{Format: 1, Courses: []*types.Course{{Tag: "rolled-back", Title: "Should roll back"}}, Sessions: map[string][]*types.CourseSession{"missing": {{ID: "bad"}}}}
	if _, err := database.ImportCatalog(ctx, db, invalid); err == nil {
		t.Fatal("accepted unknown course schedule")
	}
	if _, err := database.GetCourse(db, "rolled-back"); err == nil {
		t.Fatal("partial import was committed")
	}
	if err := database.ActivateLightning(ctx, db); err != nil {
		t.Fatal(err)
	}
	lightning, err := database.GetCourse(db, "lightning")
	if err != nil {
		t.Fatal(err)
	}
	taproot := export.Courses[0]
	if !lightning.Visible || lightning.ID != "original-lightning-id" || lightning.Title != "Lightning: BOLTs in a flash" || lightning.Topic != "Lightning & Layer 2" || lightning.Difficulty != taproot.Difficulty || !reflect.DeepEqual(lightning.Format, taproot.Format) || lightning.Availability != taproot.Availability || lightning.PriceUSD != taproot.PriceUSD || lightning.Feature != taproot.Feature || len(lightning.Chapters) != 5 {
		t.Fatalf("incorrect Lightning activation: %+v", lightning)
	}
	var entitlements int
	if err := db.QueryRow(`SELECT count(*) FROM course_entitlements WHERE person_id=1 AND course_slug='lightning'`).Scan(&entitlements); err != nil || entitlements != 1 {
		t.Fatalf("entitlement lost: %d %v", entitlements, err)
	}
	// Admin edits, source identity, and later schedule edits survive reruns.
	adminCtx := &config.AppContext{DB: db, Env: &types.EnvConfig{DBDriver: "postgres"}}
	admin, err := getAdminCourse(adminCtx, "lightning")
	if err != nil {
		t.Fatal(err)
	}
	admin.Title = "Updated Lightning title"
	admin.Catalog.PriceUSD = 75
	admin.Catalog.Availability = "Open Enrollment"
	admin.Catalog.ID = "must-not-replace-source-id"
	if err := updateCourse(adminCtx, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE course_sessions SET details=jsonb_set(details,'{SeatsAvail}','3') WHERE id='schedule-id'`); err != nil {
		t.Fatal(err)
	}
	if n, err := database.ImportCatalog(ctx, db, export); err != nil || n != 0 {
		t.Fatalf("repeat import: %d %v", n, err)
	}
	if err := database.ActivateLightning(ctx, db); err != nil {
		t.Fatal(err)
	}
	edited, err := database.GetCourse(db, "lightning")
	if err != nil {
		t.Fatal(err)
	}
	if edited.Title != admin.Title || edited.PriceUSD != 75 || edited.Availability != "Open Enrollment" || edited.ID != "original-lightning-id" {
		t.Fatalf("edit overwritten: %+v", edited)
	}
	sessions, err = database.GetCourseSessions(db, "taproot")
	if err != nil || sessions[0].SeatsAvail != 3 {
		t.Fatalf("schedule edit overwritten: %+v %v", sessions, err)
	}
}

func TestTaprootActivationPreservesCatalogAndStudentsPostgres(t *testing.T) {
	_, _, db := commandTestServer(t, "")
	ctx := context.Background()
	// A failed activation must not consume the one-time marker.
	if err := database.ActivateTaproot(ctx, db); err == nil {
		t.Fatal("activated Taproot without its imported catalog record")
	}
	export := catalogFixture()
	if _, err := database.ImportCatalog(ctx, db, export); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO course_entitlements(person_id,course_slug) VALUES(1,'taproot')`); err != nil {
		t.Fatal(err)
	}
	before, err := database.GetCourse(db, "taproot")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.ActivateTaproot(ctx, db); err != nil {
		t.Fatal(err)
	}
	after, err := database.GetCourse(db, "taproot")
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != before.ID || after.Tag != before.Tag || after.Title != before.Title ||
		after.PriceUSD != before.PriceUSD || after.Availability != before.Availability ||
		after.Difficulty != before.Difficulty || !reflect.DeepEqual(after.Format, before.Format) ||
		after.Visible != before.Visible || after.Feature != before.Feature || after.HeaderImg != before.HeaderImg ||
		after.CourseURL != "/courses/taproot/00-welcome" || len(after.Chapters) != 8 {
		t.Fatalf("incorrect Taproot activation: %+v", after)
	}
	var entitlements int
	if err := db.QueryRow(`SELECT count(*) FROM course_entitlements WHERE person_id=1 AND course_slug='taproot'`).Scan(&entitlements); err != nil || entitlements != 1 {
		t.Fatalf("lost enrollment: %d %v", entitlements, err)
	}
	sessions, err := database.GetCourseSessions(db, "taproot")
	if err != nil || !reflect.DeepEqual(sessions, export.Sessions["taproot"]) {
		t.Fatalf("historical sessions changed: %+v %v", sessions, err)
	}
	if _, err := db.Exec(`UPDATE courses SET title='Edited Taproot',description='Edited description',price_usd=75 WHERE slug='taproot'`); err != nil {
		t.Fatal(err)
	}
	if err := database.ActivateTaproot(ctx, db); err != nil {
		t.Fatal(err)
	}
	edited, err := database.GetCourse(db, "taproot")
	if err != nil || edited.Title != "Edited Taproot" || edited.ShortDesc != "Edited description" || edited.PriceUSD != 75 {
		t.Fatalf("activation overwrote admin edits: %+v %v", edited, err)
	}
}

func TestLightningCatalogEnrollmentPostgres(t *testing.T) {
	testNotebookCatalogEnrollment(t, "lightning", "02-invoices", []string{"5 chapters", "2 code exercises", "12 multiple choice questions"})
}

func TestTaprootCatalogEnrollmentPostgres(t *testing.T) {
	testNotebookCatalogEnrollment(t, "taproot", "02-schnorr-signatures", []string{"8 chapters", "3 code exercises", "8 multiple choice questions"})
}

func testNotebookCatalogEnrollment(t *testing.T, slug, lesson string, includes []string) {
	t.Helper()
	coursePath := "/courses/" + slug

	_, _, db := commandTestServer(t, "")
	// commandTestServer seeds fixed IDs for its history fixtures.
	if _, err := db.Exec(`SELECT setval('people_id_seq',2); SELECT setval('course_versions_id_seq',1); SELECT setval('course_attempts_id_seq',2)`); err != nil {
		t.Fatal(err)
	}
	t.Chdir("../..")
	if _, err := database.ImportCatalog(context.Background(), db, catalogFixture()); err != nil {
		t.Fatal(err)
	}
	if err := database.ActivateLightning(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := database.ActivateTaproot(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	session := scs.New()
	ctx := &config.AppContext{DB: db, Session: session, Infos: log.New(io.Discard, "", 0), Err: log.New(io.Discard, "", 0), Env: &types.EnvConfig{DBDriver: "postgres", Domain: "localhost", Port: "8080", CourseAssetBucket: "test", CourseAssetRegion: "nyc3"}}
	if err := SyncLocalCourseVersions(ctx); err != nil {
		t.Fatal(err)
	}
	course, err := database.GetCourse(db, slug)
	if err != nil {
		t.Fatal(err)
	}
	outline, err := loadLocalCourse(localCoursesRoot, slug, "")
	if err != nil {
		t.Fatal(err)
	}
	for i, chapter := range course.Chapters {
		if chapter.Title != outline.Items[i+1].Title {
			t.Errorf("catalog chapter %q differs from lesson %q", chapter.Title, outline.Items[i+1].Title)
		}
	}
	if err := database.SaveCourse(db, &types.Course{Tag: "sparse-course", Title: "Sparse course"}, "published", true); err != nil {
		t.Fatal(err)
	}
	routes, err := Routes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(session.LoadAndSave(routes))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	get := func(path string) string {
		t.Helper()
		response, err := client.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != 200 {
			t.Fatalf("GET %s: %d", path, response.StatusCode)
		}
		return string(body)
	}
	for _, path := range []string{"/", "/courses"} {
		body := get(path)
		if !strings.Contains(body, coursePath) || !strings.Contains(body, course.Title) {
			t.Fatalf("%s missing from %s", slug, path)
		}
	}
	body := get(coursePath)
	for _, text := range append([]string{"Start Course"}, includes...) {
		if !strings.Contains(body, text) {
			t.Errorf("landing missing %q", text)
		}
	}
	for _, text := range []string{"11-hours on-demand video", "15 downloadable resources"} {
		if strings.Contains(body, text) {
			t.Errorf("landing retained placeholder %q", text)
		}
	}
	missing, err := client.Get(server.URL + "/courses/missing-course")
	if err != nil {
		t.Fatal(err)
	}
	missing.Body.Close()
	if missing.StatusCode != 404 {
		t.Fatalf("missing course: %d", missing.StatusCode)
	}
	csrf := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(body)
	if len(csrf) != 2 {
		t.Fatal("missing signup CSRF")
	}
	response, err := client.PostForm(server.URL+coursePath+"/signup", url.Values{"csrf_token": {csrf[1]}, "email": {"catalog-test@example.invalid"}, "display_name": {"Catalog test"}})
	if err != nil {
		t.Fatal(err)
	}
	confirmation, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || !strings.Contains(string(confirmation), "Check your email") {
		t.Fatalf("signup did not request verification: %s %d", response.Request.URL, response.StatusCode)
	}
	// Development exposes the link for local testing; production only emails it.
	link := regexp.MustCompile(`href="[^"]*(/login/[a-f0-9]+)"`).FindStringSubmatch(string(confirmation))
	if len(link) != 2 {
		t.Fatal("missing development verification link")
	}
	get(link[1])
	get(coursePath + "/" + lesson)
	for _, path := range []string{coursePath, "/dashboard"} {
		body := get(path)
		if !strings.Contains(body, coursePath+"/"+lesson) {
			t.Fatalf("resume missing from %s", path)
		}
	}
	var environment struct {
		AssetBaseURL string
		Manifest     struct{ Archives []struct{ Path string } }
	}
	if err := json.Unmarshal([]byte(get("/api/courses/"+slug+"/environment")), &environment); err != nil {
		t.Fatal(err)
	}
	if len(environment.Manifest.Archives) == 0 {
		t.Fatal("course has no prepared runtime")
	}
	// Local bundles are optional on CI; when present the full app serves them.
	if strings.Contains(environment.AssetBaseURL, "/_course-runtime/") {
		response, err := client.Head(environment.AssetBaseURL + environment.Manifest.Archives[0].Path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("runtime archive unavailable: %d", response.StatusCode)
		}
	}
	responseRecorder := httptest.NewRecorder()
	LocalCourseRuntimeAsset(responseRecorder, httptest.NewRequest("GET", "/_course-runtime/"+slug+"/runtime/test", nil), &config.AppContext{IsProd: true})
	if responseRecorder.Code != 404 {
		t.Fatal("local runtime route enabled in production")
	}
	if _, ok := localRuntimeFile(slug, "../../.env"); ok {
		t.Fatal("local runtime route allows traversal")
	}
	// Anonymous visitors cannot fetch account state or create an enrollment without CSRF.
	response, err = http.Get(server.URL + "/api/courses/" + slug + "/environment")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatalf("anonymous environment: %d", response.StatusCode)
	}
	response, err = http.PostForm(server.URL+coursePath+"/signup", url.Values{"email": {"no-csrf@example.invalid"}})
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatalf("CSRF-free signup: %d", response.StatusCode)
	}
}
