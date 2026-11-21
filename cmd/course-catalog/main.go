// course-catalog is an explicit, offline migration tool. The web application
// never reads Notion or refreshes imported metadata automatically.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/kodylow/base58-website/external/getters"
	"github.com/kodylow/base58-website/internal/database"
	"github.com/kodylow/base58-website/internal/types"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	file := flag.String("file", "target/course-catalog.json", "private catalog export path")
	assets := flag.String("assets", "static/img/catalog", "directory for durable imported images; deploy this with static assets")
	flag.Parse()
	if flag.NArg() != 1 {
		return fmt.Errorf("usage: course-catalog [-file path] export|import|activate-lightning")
	}
	secrets := os.Getenv("SECRETS_FILE")
	if secrets == "" {
		secrets = ".env"
	}
	if err := godotenv.Load(secrets); err != nil && !os.IsNotExist(err) {
		return err
	}
	if flag.Arg(0) == "export" {
		return exportCatalog(*file, *assets)
	}
	if flag.Arg(0) != "import" && flag.Arg(0) != "activate-lightning" {
		return fmt.Errorf("unknown command %q", flag.Arg(0))
	}
	db, err := database.Open(&types.EnvConfig{DBDriver: os.Getenv("DB_DRIVER"), DatabaseURL: os.Getenv("DATABASE_URL")}, os.Getenv("IS_PROD") == "1")
	if err != nil {
		return err
	}
	if db == nil {
		return fmt.Errorf("DB_DRIVER and DATABASE_URL are required")
	}
	defer db.Close()
	if flag.Arg(0) == "activate-lightning" {
		if err := database.ActivateLightning(context.Background(), db); err != nil {
			return err
		}
		fmt.Println("Lightning catalog entry connected to the local course.")
		return nil
	}
	input, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	var export database.CatalogExport
	if err := json.Unmarshal(input, &export); err != nil {
		return err
	}
	count, err := database.ImportCatalog(context.Background(), db, export)
	if err != nil {
		return err
	}
	fmt.Printf("Imported %d courses; previously imported catalog edits were preserved.\n", count)
	return nil
}

func exportCatalog(path, assets string) error {
	n := &types.Notion{Config: types.NotionConfig{Token: os.Getenv("NOTION_TOKEN"), CoursesDb: os.Getenv("NOTION_COURSES"), SessionsDb: os.Getenv("NOTION_SESSIONS")}}
	if n.Config.Token == "" || n.Config.CoursesDb == "" || n.Config.SessionsDb == "" {
		return fmt.Errorf("NOTION_TOKEN, NOTION_COURSES and NOTION_SESSIONS are required for export only")
	}
	n.Setup()
	courses, err := getters.ListCourses(n)
	if err != nil {
		return fmt.Errorf("list Notion courses: %w", err)
	}
	export := database.CatalogExport{Format: 1, Courses: courses, Sessions: map[string][]*types.CourseSession{}}
	sessionCount := 0
	for _, c := range courses {
		if strings.TrimSpace(c.Tag) == "" {
			return fmt.Errorf("course %s has no slug; export stopped", c.ID)
		}
		c.HeaderImg, err = preserveImage(c.HeaderImg, assets)
		if err != nil {
			return fmt.Errorf("course %s image: %w", c.Tag, err)
		}
		sessions, err := getters.GetCourseSessions(n, c)
		if err != nil {
			return fmt.Errorf("course %s schedules: %w", c.Tag, err)
		}
		for _, s := range sessions {
			s.PromoURL, err = preserveImage(s.PromoURL, assets)
			if err != nil {
				return fmt.Errorf("course %s schedule image: %w", c.Tag, err)
			}
		}
		export.Sessions[c.Tag] = sessions
		sessionCount += len(sessions)
	}
	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	// Do not overwrite a prior backup or expose signup codes in a world-readable file.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(append(data, '\n'))
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Printf("Exported %d courses and %d schedules to %s. Notion was not modified.\n", len(courses), sessionCount, path)
	return nil
}

func preserveImage(source, dir string) (string, error) {
	if source == "" || strings.HasPrefix(source, "/static/") {
		return source, nil
	}
	// Use a content-addressed static copy so signed Notion URLs can expire.
	u, err := url.Parse(source)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return "", fmt.Errorf("image URL must use HTTP(S)")
	}
	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Get(source)
	if err != nil {
		return "", fmt.Errorf("image download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image download returned HTTP %d", resp.StatusCode)
	}
	const maxImage = 20 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImage+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxImage {
		return "", fmt.Errorf("image exceeds 20 MiB")
	}
	contentType := http.DetectContentType(data)
	ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp"}[contentType]
	if ext == "" {
		return "", fmt.Errorf("unsupported image type %s", contentType)
	}
	cleanDir := filepath.ToSlash(filepath.Clean(dir))
	if !strings.HasPrefix(cleanDir, "static/") {
		return "", fmt.Errorf("assets directory must be inside static/")
	}
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:]) + ext
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
		return "", err
	}
	return "/" + cleanDir + "/" + name, nil
}
