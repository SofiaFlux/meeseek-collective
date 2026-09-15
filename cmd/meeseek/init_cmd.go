package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SofiaFlux/meeseek-collective/internal/bootstrap"
	"github.com/SofiaFlux/meeseek-collective/internal/clock"
	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/identity"
	state "github.com/SofiaFlux/meeseek-collective/internal/state/sqlite"
	"github.com/spf13/cobra"
)

type localConfig struct {
	Version                       int       `json:"version"`
	CollectiveID                  domain.ID `json:"collective_id"`
	OwnerPrincipalID              domain.ID `json:"owner_principal_id"`
	CubePrincipalID               domain.ID `json:"cube_principal_id"`
	ConstitutionalRootPrincipalID domain.ID `json:"constitutional_root_principal_id"`
	ConstitutionHash              string    `json:"constitution_hash"`
	DatabasePath                  string    `json:"database_path"`
	EvidencePath                  string    `json:"evidence_path"`
}

func NewInitCommand() *cobra.Command {
	var home string
	command := &cobra.Command{
		Use:   "init",
		Short: "Initialize a local Meeseek Collective",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			resolvedHome := home
			if resolvedHome == "" {
				userHome, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				resolvedHome = filepath.Join(userHome, ".meeseek")
			}
			return initCollective(command, resolvedHome)
		},
	}
	command.Flags().StringVar(&home, "home", "", "Collective home directory (default ~/.meeseek)")
	return command
}

func initCollective(command *cobra.Command, home string) error {
	home, err := filepath.Abs(home)
	if err != nil {
		return err
	}
	configPath := filepath.Join(home, "config.json")
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("%w: %s", bootstrap.ErrAlreadyInitialized, home)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	stateDir := filepath.Join(home, "state")
	evidenceDir := filepath.Join(home, "evidence")
	keysDir := filepath.Join(home, "keys")
	ceremonyDir := filepath.Join(home, "ceremony")
	for _, dir := range []string{home, stateDir, evidenceDir, keysDir, ceremonyDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}

	dbPath := filepath.Join(stateDir, "meeseek.db")
	store, err := state.Open(command.Context(), dbPath)
	if err != nil {
		return err
	}
	defer store.DB().Close()

	owner, err := identity.NewLocalEd25519(filepath.Join(keysDir, "owner.key"), "owner")
	if err != nil {
		return err
	}
	cube, err := identity.NewLocalEd25519(filepath.Join(keysDir, "cube.key"), "cube")
	if err != nil {
		return err
	}
	root, err := identity.NewConstitutionalRootForCeremony(filepath.Join(ceremonyDir, "constitution-root.key"))
	if err != nil {
		return err
	}

	service := bootstrap.New(store, clock.System{})
	result, err := service.Init(command.Context(), bootstrap.InitRequest{
		Constitution:       []byte(bootstrap.DefaultConstitution),
		Owner:              owner,
		Cube:               cube,
		ConstitutionalRoot: root,
	})
	if err != nil {
		return err
	}

	config := localConfig{
		Version:                       1,
		CollectiveID:                  result.CollectiveID,
		OwnerPrincipalID:              result.OwnerPrincipalID,
		CubePrincipalID:               result.CubePrincipalID,
		ConstitutionalRootPrincipalID: result.ConstitutionalRootPrincipalID,
		ConstitutionHash:              result.ConstitutionHash,
		DatabasePath:                  dbPath,
		EvidencePath:                  evidenceDir,
	}
	if err := writeJSONAtomic(configPath, config); err != nil {
		return err
	}

	out := command.OutOrStdout()
	fmt.Fprintf(out, "collective_id=%s\n", result.CollectiveID)
	fmt.Fprintf(out, "owner_principal_id=%s\n", result.OwnerPrincipalID)
	fmt.Fprintf(out, "cube_principal_id=%s\n", result.CubePrincipalID)
	fmt.Fprintf(out, "constitutional_root_principal_id=%s\n", result.ConstitutionalRootPrincipalID)
	fmt.Fprintf(out, "constitution_hash=%s\n", result.ConstitutionHash)
	return nil
}

func writeJSONAtomic(path string, value any) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
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
	if err := encoder.Encode(value); err != nil {
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
	cleanup = false
	return nil
}
