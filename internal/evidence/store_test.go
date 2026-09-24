package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	state "github.com/SofiaFlux/summa42/internal/state/sqlite"
	"github.com/SofiaFlux/summa42/internal/testutil"
)

func TestPutWritesContentAddressedBlobAndPersistsMetadata(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := state.Open(ctx, filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB().Close()
	clk := testutil.NewClock(time.Date(2026, 9, 15, 14, 0, 0, 0, time.UTC))
	evidenceStore, err := New(store, filepath.Join(dir, "evidence"), clk)
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte("verified result")
	got, err := evidenceStore.Put(ctx, strings.NewReader(string(payload)), Metadata{MediaType: "text/plain", Kind: "ATTEMPT_OUTPUT"})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	wantHash := hex.EncodeToString(digest[:])
	if got.ContentHash != wantHash {
		t.Fatalf("content hash = %q, want %q", got.ContentHash, wantHash)
	}
	if got.SizeBytes != int64(len(payload)) || got.MediaType != "text/plain" {
		t.Fatalf("unexpected evidence object: %#v", got)
	}

	blobPath := filepath.Join(dir, "evidence", "blobs", "sha256", wantHash[:2], wantHash)
	stored, err := os.ReadFile(blobPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != string(payload) {
		t.Fatalf("blob contents = %q", stored)
	}

	var count int
	if err := store.DB().QueryRowContext(ctx,
		`SELECT count(*) FROM evidence_objects WHERE evidence_id = ? AND content_hash = ? AND media_type = ? AND size_bytes = ?`,
		got.ID, wantHash, "text/plain", len(payload),
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("metadata rows = %d, want 1", count)
	}
}

func TestPutDeduplicatesPhysicalBlobByContentHash(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := state.Open(ctx, filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB().Close()
	clk := testutil.NewClock(time.Date(2026, 9, 15, 14, 0, 0, 0, time.UTC))
	evidenceStore, err := New(store, filepath.Join(dir, "evidence"), clk)
	if err != nil {
		t.Fatal(err)
	}

	first, err := evidenceStore.Put(ctx, strings.NewReader("same"), Metadata{MediaType: "text/plain", Kind: "ATTEMPT_OUTPUT"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := evidenceStore.Put(ctx, strings.NewReader("same"), Metadata{MediaType: "text/plain", Kind: "VERIFICATION"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ContentHash != second.ContentHash {
		t.Fatalf("same content produced different hashes: %q vs %q", first.ContentHash, second.ContentHash)
	}
	if first.ID == second.ID {
		t.Fatal("separate evidence references must retain distinct identities")
	}

	blobDir := filepath.Join(dir, "evidence", "blobs", "sha256", first.ContentHash[:2])
	entries, err := os.ReadDir(blobDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("physical blob count = %d, want 1", len(entries))
	}
}

func TestGetRoundTripsPutContent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := state.Open(ctx, filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB().Close()
	clk := testutil.NewClock(time.Date(2026, 9, 15, 14, 0, 0, 0, time.UTC))
	evidenceStore, err := New(store, filepath.Join(dir, "evidence"), clk)
	if err != nil {
		t.Fatal(err)
	}
	object, err := evidenceStore.Put(ctx, strings.NewReader(`{"a":1}`), Metadata{MediaType: "application/json", Kind: "ado.review.decision"})
	if err != nil {
		t.Fatal(err)
	}
	loaded, data, err := evidenceStore.Get(ctx, object.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"a":1}` {
		t.Fatalf("data = %q", data)
	}
	if loaded.ID != object.ID || loaded.ContentHash != object.ContentHash || loaded.MediaType != "application/json" || loaded.Kind != "ado.review.decision" || loaded.SizeBytes != object.SizeBytes {
		t.Fatalf("loaded = %+v, want %+v", loaded, object)
	}
	if loaded.CreatedAt.IsZero() {
		t.Fatal("created at is zero")
	}
}

func TestGetUnknownIDErrors(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := state.Open(ctx, filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB().Close()
	clk := testutil.NewClock(time.Date(2026, 9, 15, 14, 0, 0, 0, time.UTC))
	evidenceStore, err := New(store, filepath.Join(dir, "evidence"), clk)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := evidenceStore.Get(ctx, domain.NewID("evidence")); err == nil {
		t.Fatal("expected error for unknown evidence ID")
	}
}

func TestGetCorruptShortHashErrorsWithoutPanic(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := state.Open(ctx, filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB().Close()
	clk := testutil.NewClock(time.Date(2026, 9, 15, 14, 0, 0, 0, time.UTC))
	evidenceStore, err := New(store, filepath.Join(dir, "evidence"), clk)
	if err != nil {
		t.Fatal(err)
	}
	id := domain.NewID("evidence")
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO evidence_objects(evidence_id, content_hash, media_type, kind, size_bytes, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, "ab", "application/json", "ado.review.decision", 1, "2026-09-24T08:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := evidenceStore.Get(ctx, id); err == nil {
		t.Fatal("expected error for corrupt short content hash")
	}
}
