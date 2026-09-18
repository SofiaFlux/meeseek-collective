package localconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
)

func TestConfigRoundTripKeepsControlSecretPrivate(t *testing.T) {
	home := t.TempDir()
	token, err := NewControlToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 32 {
		t.Fatalf("control token length=%d", len(token))
	}
	cfg := Config{
		Version:                       CurrentVersion,
		CollectiveID:                  domain.ID("collective_test"),
		OwnerPrincipalID:              domain.ID("owner_test"),
		CubePrincipalID:               domain.ID("cube_test"),
		ConstitutionalRootPrincipalID: domain.ID("root_test"),
		ConstitutionHash:              "constitution-hash",
		ActivePolicySetID:             domain.ID("policy_test"),
		DatabasePath:                  filepath.Join(home, "state", "meeseek.db"),
		EvidencePath:                  filepath.Join(home, "evidence"),
		ControlToken:                  token,
	}
	if err := Write(home, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if got.ControlToken != token || got.CollectiveID != cfg.CollectiveID || got.ActivePolicySetID != cfg.ActivePolicySetID {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(home, "config.json"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("config mode=%#o, want 0600", info.Mode().Perm())
		}
	}
}

func TestResolveHomeUsesEnvironmentOverride(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom-home")
	t.Setenv("MEESEEK_HOME", want)
	got, err := ResolveHome("")
	if err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(want)
	if err != nil {
		t.Fatal(err)
	}
	if got != abs {
		t.Fatalf("resolved home=%q, want %q", got, abs)
	}
}
