package cmd

import (
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
)

func NewTransformCommand(log logr.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transform",
		Short: "Transform application source code",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}
	cmd.AddCommand(NewOpenRewriteCommand(log))
	return cmd
}
