package main

import (
	"os"

	"github.com/SofiaFlux/meeseek-collective/internal/control"
	"github.com/SofiaFlux/meeseek-collective/internal/localconfig"
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	endpoint := os.Getenv("MEESEEK_CONTROL_ENDPOINT")
	token := os.Getenv("MEESEEK_CONTROL_TOKEN")
	if token == "" {
		if cfg, err := localconfig.Load(""); err == nil {
			token = cfg.ControlToken
		}
	}
	return newRootCommandWithClient(control.NewLocalClient(endpoint, token))
}

func newRootCommandWithClient(api control.API) *cobra.Command {
	command := &cobra.Command{
		Use:   "meeseek",
		Short: "Control a Meeseek Collective",
	}
	var jsonOutput bool
	command.PersistentFlags().BoolVar(&jsonOutput, "json", false, "emit stable JSON DTO output")
	command.AddCommand(
		NewInitCommand(),
		newStatusCommand(api, &jsonOutput),
		newTaskCommand(api, &jsonOutput),
		newApproveCommand(api, &jsonOutput),
		newRejectCommand(api, &jsonOutput),
		newInspectCommand(api, &jsonOutput),
	)
	return command
}

func Execute() error {
	return NewRootCommand().Execute()
}
