package identity

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocalSignerRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owner.key")
	signer, err := NewLocalEd25519(path, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if signer.CustodyProfile() != LocalDevFileCustody {
		t.Fatalf("custody profile = %q", signer.CustodyProfile())
	}

	message := []byte("hello")
	signature, err := signer.Sign(message)
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(signer.PublicKey(), message, signature) {
		t.Fatal("bad signature")
	}

	reloaded, err := NewLocalEd25519(path, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.PrincipalID() != signer.PrincipalID() {
		t.Fatalf("principal changed across reload: %q != %q", reloaded.PrincipalID(), signer.PrincipalID())
	}
	if string(reloaded.PublicKey()) != string(signer.PublicKey()) {
		t.Fatal("public key changed across reload")
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("key mode = %o, want 600", got)
		}
	}
}
