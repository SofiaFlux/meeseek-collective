package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SofiaFlux/summa42/internal/control"
	"github.com/SofiaFlux/summa42/internal/identity"
	"github.com/SofiaFlux/summa42/internal/localconfig"
	"github.com/spf13/cobra"
)

func newMissionCommand(api control.API, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "mission", Short: "Manage the Collective Mission"}
	var ownerKey string
	create := &cobra.Command{
		Use:   "create <statement>",
		Short: "Create the active Mission with an Owner signature",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			statement := strings.TrimSpace(args[0])
			if statement == "" {
				return fmt.Errorf("Mission statement is required")
			}
			keyPath := strings.TrimSpace(ownerKey)
			if keyPath == "" {
				home, err := localconfig.ResolveHome("")
				if err != nil {
					return err
				}
				keyPath = filepath.Join(home, "keys", "owner.key")
			}
			if _, err := os.Stat(keyPath); err != nil {
				return fmt.Errorf("Owner key must already exist: %w", err)
			}
			signer, err := identity.NewLocalEd25519(keyPath, "owner")
			if err != nil {
				return fmt.Errorf("load Owner key: %w", err)
			}
			mission, err := api.CreateMission(cmd.Context(), statement, signer)
			if err != nil {
				return err
			}
			if *jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(mission)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created Mission %s\n", mission.ID)
			return err
		},
	}
	create.Flags().StringVar(&ownerKey, "owner-key", "", "path to an existing Owner Ed25519 private key (default: SUMMA42_HOME/keys/owner.key)")
	show := &cobra.Command{
		Use:   "show",
		Short: "Show the active Mission",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			mission, err := api.ActiveMission(cmd.Context())
			if err != nil {
				return err
			}
			if *jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(mission)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Mission %s: %s\n", mission.ID, mission.Statement)
			return err
		},
	}
	command.AddCommand(create, show)
	return command
}
