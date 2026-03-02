package cmd

import (
	"fmt"

	"github.com/konveyor-ecosystem/kantra/cmd/internal/settings"
	"github.com/spf13/cobra"
)

func NewVersionCommand() *cobra.Command {
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the tool version",
		Long:  "Print this tool version number",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("version: %s\n", settings.Version)
			fmt.Printf("SHA: %s\n", settings.BuildCommit)
			fmt.Printf("image: %s\n", settings.RunnerImage)
		},
	}
	return versionCmd
}
