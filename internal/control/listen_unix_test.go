//go:build !windows

package control

import (
	"path/filepath"
	"testing"
)

func TestListenLocalRefusesToReplaceActiveSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.sock")
	first, err := ListenLocal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	second, err := ListenLocal(path)
	if err == nil {
		_ = second.Close()
		t.Fatal("second listener replaced an active control socket")
	}
}
