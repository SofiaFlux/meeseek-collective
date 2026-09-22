package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/SofiaFlux/summa42/internal/clock"
	"github.com/SofiaFlux/summa42/internal/domain"
	state "github.com/SofiaFlux/summa42/internal/state/sqlite"
)

type Metadata struct {
	MediaType string
	Kind      string
}

type EvidenceObject struct {
	ID          domain.ID
	ContentHash string
	MediaType   string
	Kind        string
	SizeBytes   int64
	CreatedAt   time.Time
}

type Store struct {
	state *state.Store
	root  string
	clock clock.Clock
}

func New(store *state.Store, root string, clk clock.Clock) (*Store, error) {
	if store == nil || clk == nil {
		return nil, errors.New("evidence store requires state store and clock")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("evidence root must not be empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(abs, "blobs", "sha256"), 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(abs, "tmp"), 0o700); err != nil {
		return nil, err
	}
	return &Store{state: store, root: abs, clock: clk}, nil
}

func (s *Store) Put(ctx context.Context, r io.Reader, metadata Metadata) (EvidenceObject, error) {
	if s == nil || s.state == nil || s.clock == nil {
		return EvidenceObject{}, errors.New("evidence store is not configured")
	}
	if r == nil {
		return EvidenceObject{}, errors.New("evidence reader is required")
	}
	metadata.MediaType = strings.TrimSpace(metadata.MediaType)
	metadata.Kind = strings.TrimSpace(metadata.Kind)
	if metadata.MediaType == "" || metadata.Kind == "" {
		return EvidenceObject{}, errors.New("evidence media type and kind are required")
	}

	tmp, err := os.CreateTemp(filepath.Join(s.root, "tmp"), "evidence-*")
	if err != nil {
		return EvidenceObject{}, err
	}
	tmpPath := tmp.Name()
	keepTemp := true
	defer func() {
		_ = tmp.Close()
		if keepTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, h), r)
	if err != nil {
		return EvidenceObject{}, err
	}
	if err := tmp.Sync(); err != nil {
		return EvidenceObject{}, err
	}
	if err := tmp.Close(); err != nil {
		return EvidenceObject{}, err
	}

	contentHash := hex.EncodeToString(h.Sum(nil))
	if err := verifyHash(tmpPath, contentHash); err != nil {
		return EvidenceObject{}, fmt.Errorf("verify staged evidence: %w", err)
	}
	blobDir := filepath.Join(s.root, "blobs", "sha256", contentHash[:2])
	if err := os.MkdirAll(blobDir, 0o700); err != nil {
		return EvidenceObject{}, err
	}
	blobPath := filepath.Join(blobDir, contentHash)

	if err := persistBlob(tmpPath, blobPath, contentHash, size); err != nil {
		return EvidenceObject{}, err
	}
	keepTemp = false
	if err := syncDirectory(blobDir); err != nil {
		return EvidenceObject{}, err
	}

	now := s.clock.Now().UTC()
	object := EvidenceObject{
		ID: domain.NewID("evidence"), ContentHash: contentHash, MediaType: metadata.MediaType,
		Kind: metadata.Kind, SizeBytes: size, CreatedAt: now,
	}
	if _, err := s.state.DB().ExecContext(ctx,
		`INSERT INTO evidence_objects(evidence_id, content_hash, media_type, kind, size_bytes, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		object.ID, object.ContentHash, object.MediaType, object.Kind, object.SizeBytes, now.Format(time.RFC3339Nano),
	); err != nil {
		return EvidenceObject{}, err
	}
	return object, nil
}

func persistBlob(tempPath, blobPath, expectedHash string, expectedSize int64) error {
	if info, err := os.Stat(blobPath); err == nil {
		if info.Size() != expectedSize {
			return fmt.Errorf("existing evidence blob size mismatch for %s", expectedHash)
		}
		if err := verifyHash(blobPath, expectedHash); err != nil {
			return err
		}
		return os.Remove(tempPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.Rename(tempPath, blobPath); err != nil {
		if info, statErr := os.Stat(blobPath); statErr == nil && info.Size() == expectedSize {
			if verifyErr := verifyHash(blobPath, expectedHash); verifyErr == nil {
				_ = os.Remove(tempPath)
				return nil
			}
		}
		return err
	}
	return verifyHash(blobPath, expectedHash)
}

func verifyHash(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return err
	}
	if got := hashString(h); got != expected {
		return fmt.Errorf("evidence blob hash mismatch: got %s want %s", got, expected)
	}
	return nil
}

func hashString(h hash.Hash) string {
	return hex.EncodeToString(h.Sum(nil))
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil && !errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.ENOTSUP) {
		return err
	}
	return nil
}
