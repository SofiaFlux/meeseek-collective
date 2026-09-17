package main

import (
	"encoding/json"
	"fmt"

	"github.com/SofiaFlux/meeseek-collective/internal/control"
	"github.com/spf13/cobra"
)

func newStatusCommand(api control.API, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show Collective status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			status, err := api.Status(cmd.Context())
			if err != nil {
				return err
			}
			if *jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(status)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Collective %s: %s\n", status.CollectiveID, status.State)
			return err
		},
	}
}
