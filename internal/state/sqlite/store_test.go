package sqlite_test

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/SofiaFlux/meeseek-collective/internal/testutil"
)

func TestOpenAppliesSafetyPragmas(t *testing.T) {
	store := testutil.OpenStore(t)
	assertPragma(t, store.DB(), "journal_mode", "wal")
	assertPragma(t, store.DB(), "synchronous", "2")
	assertPragma(t, store.DB(), "foreign_keys", "1")
	assertPragma(t, store.DB(), "busy_timeout", "5000")
}

func TestOpenAppliesMigrations(t *testing.T) {
	store := testutil.OpenStore(t)
	var count int
	if err := store.DB().QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("schema_migrations table count = %d, want 1", count)
	}
}

func assertPragma(t *testing.T, db *sql.DB, name, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow(fmt.Sprintf("PRAGMA %s", name)).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("PRAGMA %s = %q, want %q", name, got, want)
	}
}
