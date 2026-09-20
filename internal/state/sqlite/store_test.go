package sqlite_test

import (
	"context"
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


func TestFieldFeedbackMigrationCreatesDurableSchema(t *testing.T) {
	store := testutil.OpenStore(t)
	ctx := context.Background()

	for _, table := range []string{
		"field_observations", "field_observation_evidence",
		"feedback_candidates", "feedback_candidate_observations",
		"sanitization_results", "sanitized_feedback", "feedback_emissions",
		"approval_requests", "approval_decisions", "adaptation_grant_requests", "adaptation_grants",
		"experience_proposals", "experience_rules", "experience_rule_evidence",
		"experience_outcomes",
	} {
		var name string
		if err := store.DB().QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name); err != nil {
			t.Fatalf("table %s missing: %v", table, err)
		}
	}

	for _, trigger := range []string{
		"sanitization_results_no_update", "sanitization_results_no_delete",
		"sanitized_feedback_no_update", "sanitized_feedback_no_delete",
		"adaptation_grants_no_update", "adaptation_grants_no_delete",
	} {
		var name string
		if err := store.DB().QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type='trigger' AND name=?`, trigger,
		).Scan(&name); err != nil {
			t.Fatalf("trigger %s missing: %v", trigger, err)
		}
	}

	var taskClassColumn, approvalColumn int
	if err := store.DB().QueryRowContext(ctx,
		`SELECT count(*) FROM pragma_table_info('tasks') WHERE name='task_class'`,
	).Scan(&taskClassColumn); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx,
		`SELECT count(*) FROM pragma_table_info('external_operations') WHERE name='approval_id'`,
	).Scan(&approvalColumn); err != nil {
		t.Fatal(err)
	}
	if taskClassColumn != 1 || approvalColumn != 1 {
		t.Fatalf("migration columns task_class=%d approval_id=%d, want 1/1", taskClassColumn, approvalColumn)
	}
}


func TestAttemptRunManifestMigrationCreatesImmutableSchema(t *testing.T) {
	store := testutil.OpenStore(t)
	ctx := context.Background()
	var table string
	if err := store.DB().QueryRowContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name='attempt_run_manifests'",
	).Scan(&table); err != nil {
		t.Fatal(err)
	}
	for _, trigger := range []string{"attempt_run_manifests_no_update","attempt_run_manifests_no_delete"} {
		var name string
		if err := store.DB().QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='trigger' AND name=?", trigger,
		).Scan(&name); err != nil {
			t.Fatalf("trigger %s missing: %v", trigger, err)
		}
	}
}
