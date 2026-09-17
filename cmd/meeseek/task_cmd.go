package main

import (
	"encoding/json"
	"fmt"

	"github.com/SofiaFlux/meeseek-collective/internal/control"
	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/spf13/cobra"
)

func newTaskCommand(api control.API, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "task", Short: "Create and inspect Tasks"}
	command.AddCommand(newTaskCreateCommand(api, jsonOutput), newTaskShowCommand(api, jsonOutput))
	return command
}

func newTaskCreateCommand(api control.API, jsonOutput *bool) *cobra.Command {
	var purposeKind string
	var purposeID string
	var acceptance []string
	var capabilities []string
	var authority []string
	var enforcement string
	var resourceEnvelope string
	var priority int

	command := &cobra.Command{
		Use:   "create",
		Short: "Create a root Task through the control plane",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			request := control.CreateTaskRequest{
				Purpose:              domain.PurposeRef{Kind: domain.PurposeKind(purposeKind), ID: domain.ID(purposeID)},
				AcceptanceCriteria:   append([]string(nil), acceptance...),
				RequiredCapabilities: append([]string(nil), capabilities...),
				RequiredEnforcement:  domain.EnforcementLevel(enforcement),
				AuthorityCeiling:     append([]string(nil), authority...),
				ResourceEnvelopeID:   domain.ID(resourceEnvelope),
				Priority:             priority,
			}
			task, err := api.CreateTask(cmd.Context(), request)
			if err != nil {
				return err
			}
			if *jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(task)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created Task %s (%s)\n", task.ID, task.State)
			return err
		},
	}
	command.Flags().StringVar(&purposeKind, "purpose-kind", "", "purpose kind")
	command.Flags().StringVar(&purposeID, "purpose-id", "", "purpose id")
	command.Flags().StringSliceVar(&acceptance, "acceptance", nil, "acceptance criterion (repeat or comma-separate)")
	command.Flags().StringSliceVar(&capabilities, "capability", nil, "required capability")
	command.Flags().StringSliceVar(&authority, "authority", nil, "authority ceiling capability")
	command.Flags().StringVar(&enforcement, "enforcement", string(domain.EnforcementPartial), "required enforcement level")
	command.Flags().StringVar(&resourceEnvelope, "resource-envelope", "", "resource envelope id")
	command.Flags().IntVar(&priority, "priority", 0, "task priority")
	_ = command.MarkFlagRequired("purpose-kind")
	_ = command.MarkFlagRequired("purpose-id")
	_ = command.MarkFlagRequired("acceptance")
	_ = command.MarkFlagRequired("resource-envelope")
	return command
}

func newTaskShowCommand(api control.API, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "show <task-id>",
		Short: "Show a Task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			task, err := api.Task(cmd.Context(), domain.ID(args[0]))
			if err != nil {
				return err
			}
			if *jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(task)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Task %s: %s\n", task.ID, task.State)
			return err
		},
	}
}
