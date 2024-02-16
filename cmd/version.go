package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	BuildCommit = ""
	Version     = "v99.0.0"
)

// Use build flags to set correct Version and BuildCommit
// e.g.:
// --ldflags="-X 'github.com/konveyor-ecosystem/kantra/cmd.Version=1.2.3' -X 'github.com/konveyor-ecosystem/kantra/cmd.BuildCommit=$(git rev-parse HEAD)'"
func NewVersionCommand() *cobra.Command {
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the tool version",
		Long:  "Print this tool version number",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("version: %s\n", Version)
			fmt.Printf("SHA: %s\n", BuildCommit)
		},
	}
	return versionCmd
}
