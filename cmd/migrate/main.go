package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kodylow/base58-website/internal/database"
	"github.com/kodylow/base58-website/internal/types"
)

type migration struct {
	version string
	path    string
}

func main() {
	driver := flag.String("driver", os.Getenv("DB_DRIVER"), "database driver: postgres")
	databaseURL := flag.String("database", os.Getenv("DATABASE_URL"), "database connection string")
	dir := flag.String("dir", "", "migration directory")
	flag.Parse()

	command := "up"
	if flag.NArg() > 0 {
		command = flag.Arg(0)
	}
	if *dir == "" {
		*dir = migrationDir(*driver)
	}

	db, err := database.Open(&types.EnvConfig{DBDriver: *driver, DatabaseURL: *databaseURL}, false)
	if err != nil {
		log.Fatal(err)
	}
	if db == nil {
		log.Fatal("DB_DRIVER and DATABASE_URL are required")
	}
	defer db.Close()

	if err := ensureSchemaMigrations(db); err != nil {
		log.Fatal(err)
	}

	switch command {
	case "status":
		err = status(db, *dir)
	case "up":
		err = up(db, *dir, bindVar(*driver, 1))
	case "down":
		err = down(db, *dir, bindVar(*driver, 1))
	default:
		err = fmt.Errorf("unknown migration command %q", command)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func migrationDir(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "postgres", "postgresql", "pgx":
		return "migrations/postgres"
	default:
		return "migrations/postgres"
	}
}

func bindVar(driver string, index int) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "postgres", "postgresql", "pgx":
		return fmt.Sprintf("$%d", index)
	default:
		return fmt.Sprintf("$%d", index)
	}
}

func ensureSchemaMigrations(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

func status(db *sql.DB, dir string) error {
	applied, err := appliedVersions(db)
	if err != nil {
		return err
	}
	migrations, err := readMigrations(dir, ".up.sql")
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		state := "pending"
		if applied[migration.version] {
			state = "applied"
		}
		fmt.Printf("%s %s\n", migration.version, state)
	}
	return nil
}

func up(db *sql.DB, dir, bind string) error {
	applied, err := appliedVersions(db)
	if err != nil {
		return err
	}
	migrations, err := readMigrations(dir, ".up.sql")
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		if applied[migration.version] {
			continue
		}
		if err := runMigration(db, migration); err != nil {
			return err
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (`+bind+`)`, migration.version); err != nil {
			return err
		}
		fmt.Printf("applied %s\n", migration.version)
	}
	return nil
}

func down(db *sql.DB, dir, bind string) error {
	applied, err := appliedVersions(db)
	if err != nil {
		return err
	}
	migrations, err := readMigrations(dir, ".down.sql")
	if err != nil {
		return err
	}
	for i := len(migrations) - 1; i >= 0; i-- {
		migration := migrations[i]
		if !applied[migration.version] {
			continue
		}
		if err := runMigration(db, migration); err != nil {
			return err
		}
		if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = `+bind, migration.version); err != nil {
			return err
		}
		fmt.Printf("rolled back %s\n", migration.version)
		return nil
	}
	fmt.Println("no applied migrations")
	return nil
}

func appliedVersions(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

func readMigrations(dir, suffix string) ([]migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var migrations []migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
			continue
		}
		version, _, ok := strings.Cut(entry.Name(), "_")
		if !ok || version == "" {
			return nil, fmt.Errorf("invalid migration filename %s", entry.Name())
		}
		migrations = append(migrations, migration{
			version: version,
			path:    filepath.Join(dir, entry.Name()),
		})
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})
	return migrations, nil
}

func runMigration(db *sql.DB, migration migration) error {
	contents, err := os.ReadFile(migration.path)
	if err != nil {
		return err
	}
	statements := splitSQLStatements(string(contents))
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("%s: %w", migration.path, err)
		}
	}
	return nil
}

func splitSQLStatements(contents string) []string {
	var statements []string
	for _, statement := range strings.Split(contents, ";") {
		lines := strings.Split(statement, "\n")
		var kept []string
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "-- +") {
				continue
			}
			kept = append(kept, line)
		}
		statement = strings.TrimSpace(strings.Join(kept, "\n"))
		if statement != "" {
			statements = append(statements, statement)
		}
	}
	return statements
}
