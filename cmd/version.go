package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/konveyor-ecosystem/kantra/cmd/internal/settings"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
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
			if sha, err := readRulesetsSHA(); err == nil {
				fmt.Printf("rulesets SHA: %s\n", sha)
			}
		},
	}
	return versionCmd
}

// readRulesetsSHA reads the .sha file from the rulesets directory.
// The file is written at image build time with the git SHA of the
// tackle2-seed repository used to produce the bundled rulesets.
func readRulesetsSHA() (string, error) {
	kantraDir, err := util.GetKantraDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(kantraDir, settings.RulesetsLocation, ".sha"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
