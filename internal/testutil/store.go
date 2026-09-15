package testutil

import (
	"context"
	"path/filepath"
	"testing"

	state "github.com/SofiaFlux/meeseek-collective/internal/state/sqlite"
)

func OpenStore(t *testing.T) *state.Store {
	t.Helper()
	store, err := state.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.DB().Close()
	})
	return store
}
