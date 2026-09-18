package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SofiaFlux/meeseek-collective/internal/control"
	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/identity"
	"github.com/SofiaFlux/meeseek-collective/internal/localconfig"
	"github.com/spf13/cobra"
)

func newApproveCommand(api control.API, jsonOutput *bool) *cobra.Command {
	var ownerKey string
	command := &cobra.Command{
		Use:   "approve <approval-id>",
		Short: "Approve a pending action with an Owner signature",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ownerKey = strings.TrimSpace(ownerKey)
			if ownerKey == "" {
				home, err := localconfig.ResolveHome("")
				if err != nil {
					return err
				}
				ownerKey = filepath.Join(home, "keys", "owner.key")
			}
			if _, err := os.Stat(ownerKey); err != nil {
				return fmt.Errorf("Owner key must already exist: %w", err)
			}
			signer, err := identity.NewLocalEd25519(ownerKey, "owner")
			if err != nil {
				return fmt.Errorf("load Owner key: %w", err)
			}
			approval, err := api.Approve(cmd.Context(), domain.ID(args[0]), signer)
			if err != nil {
				return err
			}
			if *jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(approval)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Approval %s: %s by %s\n", approval.ApprovalID, approval.Status, approval.ApprovedBy)
			return err
		},
	}
	command.Flags().StringVar(&ownerKey, "owner-key", "", "path to an existing Owner Ed25519 private key (default: MEESEEK_HOME/keys/owner.key)")
	return command
}
