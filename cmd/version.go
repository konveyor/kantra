package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewVersionCommand() *cobra.Command {
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the tool version",
		Long:  "Print this tool version number",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Version: %s\n", Settings.Version)
			fmt.Printf("SHA: %s\n", Settings.BuildCommit)
		},
	}
	return versionCmd
}
