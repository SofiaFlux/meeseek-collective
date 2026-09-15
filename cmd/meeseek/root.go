package main

import "github.com/spf13/cobra"

func NewRootCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "meeseek",
		Short: "Control a Meeseek Collective",
	}
	command.AddCommand(NewInitCommand())
	return command
}

func Execute() error {
	return NewRootCommand().Execute()
}
