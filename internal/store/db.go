package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func Open(driver, dsn string) (*sql.DB, error) {
	switch driver {
	case "sqlite":
		db, err := sql.Open("sqlite", dsn+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
		if err != nil {
			return nil, err
		}
		if err := db.Ping(); err != nil {
			return nil, err
		}
		return db, nil
	case "postgres":
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			return nil, err
		}
		if err := db.Ping(); err != nil {
			return nil, err
		}
		return db, nil
	default:
		return nil, fmt.Errorf("unknown driver %q", driver)
	}
}

func Migrate(db *sql.DB, driver string) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations(version TEXT PRIMARY KEY, applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := loadApplied(db)
	if err != nil {
		return err
	}

	files, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return err
	}
	var upFiles []string
	for _, f := range files {
		name := f.Name()
		if strings.HasSuffix(name, ".up.sql") {
			upFiles = append(upFiles, name)
		}
	}
	sort.Strings(upFiles)

	for _, name := range upFiles {
		version := strings.TrimSuffix(name, ".up.sql")
		if applied[version] {
			continue
		}
		content, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version) VALUES(?)`, version); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func loadApplied(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		m[v] = true
	}
	return m, rows.Err()
}
