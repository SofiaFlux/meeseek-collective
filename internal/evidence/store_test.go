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

	state "github.com/SofiaFlux/meeseek-collective/internal/state/sqlite"
	"github.com/SofiaFlux/meeseek-collective/internal/testutil"
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
