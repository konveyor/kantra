package rules

import (
	"github.com/go-logr/logr"
	yamlruletest "github.com/konveyor-ecosystem/kantra/cmd/rules/test"
	"github.com/spf13/cobra"
)

// NewRulesCommand returns the `rules` command group (kantra rules …).
func NewRulesCommand(log logr.Logger) *cobra.Command {
	rulesCmd := &cobra.Command{
		Use:   "rules",
		Short: "Inspect available sources and targets, and test rule files",
	}
	rulesCmd.AddCommand(newListSourcesCommand(log))
	rulesCmd.AddCommand(newListTargetsCommand(log))
	rulesCmd.AddCommand(newRulesTestCommand(log))
	return rulesCmd
}

func newRulesTestCommand(log logr.Logger) *cobra.Command {
	cmd := yamlruletest.NewTestCommand(log)
	cmd.Use = "test [paths...]"
	cmd.Short = "Run YAML rule tests against rule files"
	return cmd
}
