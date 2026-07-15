package rules

import (
	"github.com/go-logr/logr"
	yamlruletest "github.com/konveyor-ecosystem/kantra/cmd/rules/test"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
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

// NewLegacyTestCommand returns the pre-0.10 top-level `test` command for backwards compatibility.
func NewLegacyTestCommand(log logr.Logger) *cobra.Command {
	cmd := yamlruletest.NewTestCommand(log)
	util.AnnotateCommandDeprecation(cmd, util.MovedDeprecationMessage("'kantra test'", "'kantra rules test'"))
	runE := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		util.WarnMovedDeprecation(c.ErrOrStderr(), log, "'kantra test'", "'kantra rules test'")
		return runE(c, args)
	}
	return cmd
}
