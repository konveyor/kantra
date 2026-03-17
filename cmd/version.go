package cmd

import (
	"errors"
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
			fmt.Fprintf(cmd.OutOrStdout(), "version: %s\n", settings.Version)
			fmt.Fprintf(cmd.OutOrStdout(), "SHA: %s\n", settings.BuildCommit)
			fmt.Fprintf(cmd.OutOrStdout(), "image: %s\n", settings.RunnerImage)
			sha, err := readRulesetsSHA()
			switch {
			case err == nil:
				fmt.Fprintf(cmd.OutOrStdout(), "rulesets SHA: %s\n", sha)
			case errors.Is(err, os.ErrNotExist):
				// .sha file not present, omit from output
			default:
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: unable to read rulesets SHA: %v\n", err)
			}
		},
	}
	return versionCmd
}

// readRulesetsSHA reads the .sha file from the rulesets directory.
// The file is written at image build time with the git SHA of the
// tackle2-seed repository used to produce the bundled rulesets.
// It checks the kantra directory first, then falls back to the
// container default path at /opt/rulesets.
func readRulesetsSHA() (string, error) {
	shaFile := filepath.Join(settings.RulesetsLocation, ".sha")
	kantraDir, err := util.GetKantraDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(kantraDir, shaFile))
	if err == nil {
		if sha := strings.TrimSpace(string(data)); sha != "" {
			return sha, nil
		}
		return "", os.ErrNotExist
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if os.Getenv(util.KantraDirEnv) != "" {
		return "", err
	}
	// Fallback for container environment where rulesets are at /opt/rulesets
	data, err = os.ReadFile(filepath.Join("/opt", shaFile))
	if err != nil {
		return "", err
	}
	if sha := strings.TrimSpace(string(data)); sha != "" {
		return sha, nil
	}
	return "", os.ErrNotExist
}
