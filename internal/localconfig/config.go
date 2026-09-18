package localconfig

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
)

const CurrentVersion = 1

type Config struct {
	Version                       int       `json:"version"`
	CollectiveID                  domain.ID `json:"collective_id"`
	OwnerPrincipalID              domain.ID `json:"owner_principal_id"`
	CubePrincipalID               domain.ID `json:"cube_principal_id"`
	ConstitutionalRootPrincipalID domain.ID `json:"constitutional_root_principal_id"`
	ConstitutionHash              string    `json:"constitution_hash"`
	ActivePolicySetID             domain.ID `json:"active_policy_set_id"`
	DatabasePath                  string    `json:"database_path"`
	EvidencePath                  string    `json:"evidence_path"`
	ControlToken                  string    `json:"control_token"`
}

func ResolveHome(explicit string) (string, error) {
	home := strings.TrimSpace(explicit)
	if home == "" {
		home = strings.TrimSpace(os.Getenv("MEESEEK_HOME"))
	}
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(userHome, ".meeseek")
	}
	return filepath.Abs(home)
}

func NewControlToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate control token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func Load(home string) (Config, error) {
	resolved, err := ResolveHome(home)
	if err != nil {
		return Config{}, err
	}
	file, err := os.Open(filepath.Join(resolved, "config.json"))
	if err != nil {
		return Config{}, err
	}
	defer file.Close()

	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode local config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Config{}, errors.New("local config contains trailing JSON")
		}
		return Config{}, fmt.Errorf("decode local config trailer: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Write(home string, cfg Config) error {
	resolved, err := ResolveHome(home)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(resolved, 0o700); err != nil {
		return err
	}

	path := filepath.Join(resolved, "config.json")
	file, err := os.CreateTemp(resolved, ".config-*.tmp")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	cleanup := true
	defer func() {
		_ = file.Close()
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cfg); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func (c Config) Validate() error {
	if c.Version != CurrentVersion {
		return fmt.Errorf("unsupported local config version %d", c.Version)
	}
	if c.CollectiveID == "" || c.OwnerPrincipalID == "" || c.CubePrincipalID == "" || c.ConstitutionalRootPrincipalID == "" {
		return errors.New("local config requires collective and principal ids")
	}
	if strings.TrimSpace(c.ConstitutionHash) == "" || c.ActivePolicySetID == "" {
		return errors.New("local config requires constitution hash and active policy set")
	}
	if strings.TrimSpace(c.DatabasePath) == "" || strings.TrimSpace(c.EvidencePath) == "" {
		return errors.New("local config requires state and evidence paths")
	}
	if !filepath.IsAbs(c.DatabasePath) || !filepath.IsAbs(c.EvidencePath) {
		return errors.New("local config state and evidence paths must be absolute")
	}
	if len(strings.TrimSpace(c.ControlToken)) < 32 {
		return errors.New("local config control token is missing or too short")
	}
	return nil
}
