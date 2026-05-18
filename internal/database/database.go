package database

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/kodylow/base58-website/internal/types"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

func Open(env *types.EnvConfig, isProd bool) (*sql.DB, error) {
	if env == nil || env.DBDriver == "" || env.DatabaseURL == "" {
		return nil, nil
	}

	driver, err := normalizeDriver(env.DBDriver)
	if err != nil {
		return nil, err
	}
	if isProd && driver != "pgx" {
		return nil, fmt.Errorf("prod database must use postgres")
	}
	if !isProd && driver == "pgx" && strings.Contains(env.DatabaseURL, "ondigitalocean.com") {
		return nil, fmt.Errorf("refusing to use DigitalOcean managed Postgres outside prod")
	}

	db, err := sql.Open(driver, env.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func normalizeDriver(driver string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "postgres", "postgresql", "pgx":
		return "pgx", nil
	case "sqlite", "sqlite3":
		return "sqlite3", nil
	default:
		return "", fmt.Errorf("unsupported database driver %q", driver)
	}
}
