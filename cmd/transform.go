package cmd

import (
	"github.com/spf13/cobra"
)

func NewTransformCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transform",
		Short: "Transform application source code or windup XML rules",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}
	cmd.AddCommand(NewOpenRewriteCommand())
	cmd.AddCommand(NewWindupShimCommand())
	return cmd
}
