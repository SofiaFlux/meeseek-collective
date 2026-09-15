package main

import "github.com/spf13/cobra"

func NewRootCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "meeseek",
		Short: "Control a Meeseek Collective",
	}
}

func Execute() error {
	return NewRootCommand().Execute()
}
