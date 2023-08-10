package cmd

import (
	"github.com/spf13/cobra"
)

func NewTransformCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "transform",

		Short: "Transform application source code or windup XML rules",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}
	cmd.AddCommand(NewOpenRewriteCommand())
	return cmd
}
