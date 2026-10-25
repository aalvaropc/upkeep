package store

import (
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func open(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func tables(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

func TestFreshDatabaseReachesLatestSchema(t *testing.T) {
	s := open(t, filepath.Join(t.TempDir(), "upkeep.db"))
	v, err := s.SchemaVersion(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if v != len(migrations) {
		t.Fatalf("schema version = %d, want %d", v, len(migrations))
	}
	want := []string{"actions", "events", "model_calls", "prs", "runs", "sessions", "steps"}
	if got := tables(t, s.db); !slices.Equal(got, want) {
		t.Fatalf("tables = %v, want %v", got, want)
	}
	var mode string
	if err := s.db.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal mode = %q, want wal", mode)
	}
}

func TestReopenKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upkeep.db")
	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(t.Context(), "INSERT INTO sessions (id, mode, created_at, last_seen_at, expires_at) VALUES ('s1', 'replay', 1, 1, 2)"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = open(t, path)
	var n int
	if err := s.db.QueryRowContext(t.Context(), "SELECT count(*) FROM sessions").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("sessions after reopen = %d, want 1", n)
	}
	if v, _ := s.SchemaVersion(t.Context()); v != len(migrations) {
		t.Fatalf("schema version after reopen = %d", v)
	}
}

func TestForeignKeysAreEnforced(t *testing.T) {
	s := open(t, filepath.Join(t.TempDir(), "upkeep.db"))
	_, err := s.db.ExecContext(t.Context(), "INSERT INTO runs (id, session_id, mode, status, started_at) VALUES ('r1', 'missing', 'replay', 'queued', 1)")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "foreign key") {
		t.Fatalf("insert with a missing session: error = %v, want a foreign key error", err)
	}
}

func TestStrictTypes(t *testing.T) {
	s := open(t, filepath.Join(t.TempDir(), "upkeep.db"))
	_, err := s.db.ExecContext(t.Context(), "INSERT INTO sessions (id, mode, created_at, last_seen_at, expires_at) VALUES ('s1', 'replay', 'soon', 1, 2)")
	if err == nil {
		t.Fatal("a text value in an INTEGER column of a STRICT table was accepted")
	}
}

func TestFailedMigrationRollsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upkeep.db")
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	steps := [][]string{
		{"CREATE TABLE a (x INTEGER)"},
		{"CREATE TABLE b (x INTEGER)", "THIS IS NOT SQL"},
	}
	if err := migrate(t.Context(), db, steps); err == nil || !strings.Contains(err.Error(), "migration 2") {
		t.Fatalf("migrate error = %v, want a migration 2 error", err)
	}
	if v, _ := userVersion(t.Context(), db); v != 1 {
		t.Fatalf("schema version = %d, want 1", v)
	}
	if got := tables(t, db); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("tables = %v, want only a", got)
	}
}

func TestNewerSchemaIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upkeep.db")
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Open(t.Context(), path); err == nil || !strings.Contains(err.Error(), "newer than this binary") {
		t.Fatalf("Open error = %v, want a newer schema error", err)
	}
}
