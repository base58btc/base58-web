package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/kodylow/base58-website/internal/types"
)

const catalogColumns = `slug,title,description,header_img,status,notion_id,difficulty,formats,topic,availability,popularity,price_usd,flavor,long_description,prerequisites,featured,course_url,course_host,rating,includes,chapters`

type scanner interface{ Scan(...any) error }

func scanCourse(row scanner) (*types.Course, error) {
	c := new(types.Course)
	var status string
	var formats, includes, chapters []byte
	err := row.Scan(&c.Tag, &c.Title, &c.ShortDesc, &c.HeaderImg, &status, &c.ID, &c.Difficulty, &formats, &c.Topic, &c.Availability, &c.Popularity, &c.PriceUSD, &c.Flavor, &c.LongDesc, &c.PreReqs, &c.Feature, &c.CourseURL, &c.CourseHost, &c.Rating, &includes, &chapters)
	if err != nil {
		return nil, err
	}
	for _, field := range []struct {
		data   []byte
		target any
	}{{formats, &c.Format}, {includes, &c.Includes}, {chapters, &c.Chapters}} {
		if err := json.Unmarshal(field.data, field.target); err != nil {
			return nil, fmt.Errorf("course %s: %w", c.Tag, err)
		}
	}
	c.Visible = status == "published"
	return c, nil
}

func ListCourses(db *sql.DB) ([]*types.Course, error) {
	if db == nil {
		return nil, errors.New("course database is not configured")
	}
	rows, err := db.Query(`SELECT ` + catalogColumns + ` FROM courses ORDER BY popularity, title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var courses []*types.Course
	for rows.Next() {
		c, err := scanCourse(rows)
		if err != nil {
			return nil, err
		}
		courses = append(courses, c)
	}
	return courses, rows.Err()
}

func GetCourse(db *sql.DB, slug string) (*types.Course, error) {
	if db == nil {
		return nil, errors.New("course database is not configured")
	}
	return scanCourse(db.QueryRow(`SELECT `+catalogColumns+` FROM courses WHERE slug=$1`, slug))
}

func GetCourseSessions(db *sql.DB, slug string) ([]*types.CourseSession, error) {
	if db == nil {
		return nil, errors.New("course database is not configured")
	}
	rows, err := db.Query(`SELECT details FROM course_sessions WHERE course_slug=$1 ORDER BY id`, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []*types.CourseSession
	for rows.Next() {
		var data []byte
		var s types.CourseSession
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		sessions = append(sessions, &s)
	}
	return sessions, rows.Err()
}

func courseValues(c *types.Course, status string) []any {
	formats, _ := json.Marshal(c.Format)
	includes, _ := json.Marshal(c.Includes)
	chapters, _ := json.Marshal(c.Chapters)
	return []any{c.Tag, c.Title, c.ShortDesc, c.HeaderImg, status, c.ID, c.Difficulty, string(formats), c.Topic, c.Availability, c.Popularity, c.PriceUSD, c.Flavor, c.LongDesc, c.PreReqs, c.Feature, c.CourseURL, c.CourseHost, c.Rating, string(includes), string(chapters)}
}

func courseInsert() string {
	var params []string
	for i := range strings.Split(catalogColumns, ",") {
		params = append(params, fmt.Sprintf("$%d", i+1))
	}
	return `INSERT INTO courses (` + catalogColumns + `) VALUES (` + strings.Join(params, ",") + `)`
}

func courseUpdates(excluded bool) string {
	var assignments []string
	skipped := 0
	for i, col := range strings.Split(catalogColumns, ",") {
		// Stable source identity is only set by the import, never by an admin form.
		if col == "slug" {
			continue
		}
		if !excluded && col == "notion_id" {
			skipped++
			continue
		}
		value := fmt.Sprintf("$%d", i+1-skipped)
		if excluded {
			value = "EXCLUDED." + col
		}
		assignments = append(assignments, col+"="+value)
	}
	return strings.Join(assignments, ",") + `,updated_at=CURRENT_TIMESTAMP`
}

var catalogSlug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func validateCourse(c *types.Course) error {
	if c == nil || !catalogSlug.MatchString(c.Tag) || strings.TrimSpace(c.Title) == "" {
		return errors.New("course requires a title and a lowercase, hyphen-separated slug")
	}
	if math.IsNaN(float64(c.Rating)) || c.Rating < 0 || c.Rating > 5 {
		return errors.New("rating must be between 0 and 5")
	}
	return nil
}

// SaveCourse edits Postgres directly. Imports never run on application startup.
func SaveCourse(db *sql.DB, c *types.Course, status string, create bool) error {
	if err := validateCourse(c); err != nil {
		return err
	}
	if status != "draft" && status != "published" && status != "archived" {
		return errors.New("invalid course status")
	}
	query := courseInsert()
	if !create {
		query = `UPDATE courses SET ` + courseUpdates(false) + ` WHERE slug=$1`
	}
	values := courseValues(c, status)
	if !create {
		values = append(values[:5], values[6:]...)
	}
	result, err := db.Exec(query, values...)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err == nil && changed == 0 {
		return sql.ErrNoRows
	}
	return err
}

type CatalogExport struct {
	Format   int                               `json:"format"`
	Courses  []*types.Course                   `json:"courses"`
	Sessions map[string][]*types.CourseSession `json:"sessions"`
}

// ImportCatalog completes the old partial import without replacing courses or
// touching students, entitlements, progress, emails, or any data in Notion.
// A repeat import preserves subsequent Postgres edits.
func ImportCatalog(ctx context.Context, db *sql.DB, export CatalogExport) (int, error) {
	if export.Format != 1 || len(export.Courses) == 0 {
		return 0, errors.New("expected a nonempty format 1 catalog export")
	}
	seen := map[string]bool{}
	for _, c := range export.Courses {
		if err := validateCourse(c); err != nil {
			return 0, err
		}
		if seen[c.Tag] {
			return 0, fmt.Errorf("duplicate course %s", c.Tag)
		}
		seen[c.Tag] = true
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	count := 0
	for _, c := range export.Courses {
		status := "draft"
		if c.Visible {
			status = "published"
		}
		result, err := tx.ExecContext(ctx, courseInsert()+` ON CONFLICT (slug) DO UPDATE SET `+courseUpdates(true)+` WHERE courses.catalog_imported_at IS NULL`, courseValues(c, status)...)
		if err != nil {
			return 0, err
		}
		changed, _ := result.RowsAffected()
		if changed == 0 {
			continue
		}
		if _, err = tx.ExecContext(ctx, `UPDATE courses SET catalog_imported_at=CURRENT_TIMESTAMP WHERE slug=$1`, c.Tag); err != nil {
			return 0, err
		}
		count++
	}
	for slug, sessions := range export.Sessions {
		if !seen[slug] {
			return 0, fmt.Errorf("schedule has unknown course %s", slug)
		}
		for _, s := range sessions {
			if s == nil || s.ID == "" {
				return 0, errors.New("schedule is missing its source ID")
			}
			data, err := json.Marshal(s)
			if err != nil {
				return 0, err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO course_sessions(id,course_slug,details) VALUES($1,$2,$3) ON CONFLICT(id,course_slug) DO NOTHING`, s.ID, slug, string(data)); err != nil {
				return 0, err
			}
		}
	}
	return count, tx.Commit()
}

// ActivateLightning reuses the imported record and Taproot's catalog settings.
// The marker makes this one-time change safe to rerun after later admin edits.
func ActivateLightning(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO course_catalog_updates(name) VALUES('lightning-course-launch') ON CONFLICT DO NOTHING`)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return tx.Commit()
	}
	chapters, _ := json.Marshal([]types.ChapterInfo{
		{Title: "CSV and CLTV", Desc: "Construct and spend timelocked outputs."},
		{Title: "Lightning invoices", Desc: "Decode an invoice and recover its signing key."},
		{Title: "Revocation generation", Desc: "Implement the per-commitment secret generator."},
		{Title: "Lightning gossip for fun and profit", Desc: "Find a route and calculate its fees and timelocks."},
		{Title: "Build and send an onion", Desc: "Construct the payloads and send a payment."},
	})
	result, err = tx.ExecContext(ctx, `UPDATE courses AS lightning SET
 difficulty=taproot.difficulty,formats=taproot.formats,availability=taproot.availability,
 price_usd=taproot.price_usd,featured=taproot.featured,status=taproot.status,
 header_img=COALESCE(NULLIF(lightning.header_img,''),'/static/img/courses/lightning.png'),
 course_host='Base58',course_url='/courses/lightning/00-welcome',
 description='Explore the Lightning protocol through timelocks, invoices, revocation secrets, gossip, and onion payments.',
 long_description='Work through the Base58 Lightning workshop with interactive Python examples, coding challenges, and questions. Learn how Lightning channels, invoices, transactions, gossip, and onion routing fit together.',
 includes='["5 chapters"]',chapters=$1,updated_at=CURRENT_TIMESTAMP
 FROM courses AS taproot WHERE lightning.slug='lightning' AND taproot.slug='taproot'
 AND lightning.catalog_imported_at IS NOT NULL AND taproot.catalog_imported_at IS NOT NULL`, string(chapters))
	if err != nil {
		return err
	}
	n, _ = result.RowsAffected()
	if n != 1 {
		return errors.New("import both Lightning and Taproot before activating Lightning")
	}
	return tx.Commit()
}

// ActivateTaproot connects the existing catalog identity to its notebook port.
// A rerun preserves subsequent admin edits, enrollment, and historical sessions.
func ActivateTaproot(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO course_catalog_updates(name) VALUES('taproot-course-launch') ON CONFLICT DO NOTHING`)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return tx.Commit()
	}
	chapters, _ := json.Marshal([]types.ChapterInfo{
		{Title: "Finding Taproot in the wild", Desc: "Recognize P2TR outputs and lift x-only public keys."},
		{Title: "Schnorr signatures", Desc: "Parse transactions, compute signature messages, and sign and verify."},
		{Title: "Build a Taproot output", Desc: "Hash a script leaf, choose an internal key, and construct an output."},
		{Title: "Spend a Tapscript", Desc: "Assemble a witness and control block for a script-path spend."},
		{Title: "Balanced and unbalanced script trees", Desc: "Build Merkle trees and compare proofs and transaction sizes."},
		{Title: "Multisig with OP_CHECKSIGADD", Desc: "Construct and spend a 2-of-3 threshold script."},
		{Title: "MuSig2 and Tapscript spends", Desc: "Aggregate keys and partial signatures for a script-path spend."},
		{Title: "FROST threshold signatures", Desc: "Generate shares and create a 2-of-3 key-path signature."},
	})
	result, err = tx.ExecContext(ctx, `UPDATE courses SET
 header_img=COALESCE(NULLIF(header_img,''),'/static/img/courses/taproot.png'),
 course_host='Base58',course_url='/courses/taproot/00-welcome',
 description='Explore Taproot through Schnorr signatures, script trees, and three ways to build multisignature spends.',
 long_description='Work through the Base58 Taproot workshop with editable Python examples, coding challenges, and questions. Build and spend Taproot outputs, compare balanced and unbalanced script trees, and experiment with OP_CHECKSIGADD, MuSig2, and FROST.',
 includes='["8 chapters"]',chapters=$1,updated_at=CURRENT_TIMESTAMP
 WHERE slug='taproot' AND catalog_imported_at IS NOT NULL`, string(chapters))
	if err != nil {
		return err
	}
	n, _ = result.RowsAffected()
	if n != 1 {
		return errors.New("import the Taproot catalog record before activating Taproot")
	}
	return tx.Commit()
}
