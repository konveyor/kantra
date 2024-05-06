package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	BuildCommit          = ""
	Version              = "latest"
	RunnerImage          = "quay.io/konveyor/kantra"
	RootCommandName      = "kantra"
	JavaBundlesLocation  = "/jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar"
	JavaProviderImage    = "quay.io/konveyor/java-external-provider"
	GenericProviderImage = "quay.io/konveyor/generic-external-provider"
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
			fmt.Printf("image: %s\n", RunnerImage)
		},
	}
	return versionCmd
}
