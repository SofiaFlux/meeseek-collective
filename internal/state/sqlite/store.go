package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	_ "modernc.org/sqlite"
)

const policyProfileInactiveMarker = "MEESEEK_POLICY_PROFILE_INACTIVE"

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	dsn, err := localFileDSN(path)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func localFileDSN(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("sqlite path must not be empty")
	}
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return "", errors.New("sqlite path must be a local filesystem path")
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	slashPath := filepath.ToSlash(abs)
	if filepath.VolumeName(abs) != "" && !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}

	u := url.URL{Scheme: "file", Path: slashPath}
	q := u.Query()
	q.Set("_busy_timeout", "5000")
	q.Set("_foreign_keys", "on")
	q.Set("_journal_mode", "WAL")
	q.Set("_synchronous", "FULL")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := fn(tx); err != nil {
		return mapCanonicalInvariantError(err)
	}
	if err := tx.Commit(); err != nil {
		return mapCanonicalInvariantError(err)
	}
	committed = true
	return nil
}

func mapCanonicalInvariantError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), policyProfileInactiveMarker) {
		return fmt.Errorf("%w: active policy profile no longer matches decision: %v", domain.ErrPolicyDenied, err)
	}
	return err
}
