package rules

import (
	"os"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/labels"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/spf13/cobra"
)

const (
	runModeEnv       = "RUN_MODE"
	runModeContainer = "container"
)

func newRuleLabelsLister(log logr.Logger) (*labels.Lister, error) {
	kantraDir, err := util.GetKantraDir()
	if err != nil {
		return nil, err
	}
	cfg := labels.Config{
		Log:       log,
		KantraDir: kantraDir,
	}

	if os.Getenv(runModeEnv) == runModeContainer {
		cfg.Hybrid = &labels.HybridConfig{}
	}
	return labels.New(cfg), nil
}

func newListSourcesCommand(log logr.Logger) *cobra.Command {
	return &cobra.Command{
		Use:   "list-sources",
		Short: "List available source technologies from rule labels",
		RunE: func(cmd *cobra.Command, args []string) error {
			lister, err := newRuleLabelsLister(log)
			if err != nil {
				return err
			}
			return lister.ListSources(cmd.Context(), os.Stdout)
		},
	}
}

func newListTargetsCommand(log logr.Logger) *cobra.Command {
	return &cobra.Command{
		Use:   "list-targets",
		Short: "List available target technologies from rule labels",
		RunE: func(cmd *cobra.Command, args []string) error {
			lister, err := newRuleLabelsLister(log)
			if err != nil {
				return err
			}
			return lister.ListTargets(cmd.Context(), os.Stdout)
		},
	}
}
