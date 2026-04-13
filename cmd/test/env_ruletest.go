package test

import (
	"fmt"

	kantraprovider "github.com/konveyor-ecosystem/kantra/pkg/provider"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
)

// RuleTestRunParams holds values that vary per test group or run
type RuleTestRunParams struct {
	Input        string
	AnalysisMode string

	Cleanup      bool
	ContextLines int

	// KantraDir is used in containerless mode. If empty, [util.GetKantraDir] is used.
	KantraDir string

	Providers []kantraprovider.ProviderInfo
	// WorkDir is the test temp directory; used as OutputDir for hybrid ruleset cache.
	WorkDir string
}

// environmentConfigForRuleTests builds a [kantraprovider.EnvironmentConfig] for YAML rule tests.
func environmentConfigForRuleTests(opts TestOptions, run RuleTestRunParams) (kantraprovider.EnvironmentConfig, error) {
	if opts.RunLocal {
		kantraDir := run.KantraDir
		if kantraDir == "" {
			var err error
			kantraDir, err = util.GetKantraDir()
			if err != nil {
				return kantraprovider.EnvironmentConfig{}, fmt.Errorf("failed to get kantra dir: %w", err)
			}
		}
		return kantraprovider.EnvironmentConfig{
			Mode:         kantraprovider.ModeLocal,
			Input:        run.Input,
			AnalysisMode: run.AnalysisMode,
			Log:          opts.Log,
			Cleanup:      run.Cleanup,
			ContextLines: run.ContextLines,
			KantraDir:    kantraDir,
		}, nil
	}
	return kantraprovider.EnvironmentConfig{
		Mode:                  kantraprovider.ModeNetwork,
		Input:                 run.Input,
		AnalysisMode:          run.AnalysisMode,
		Log:                   opts.Log,
		Cleanup:               run.Cleanup,
		ContextLines:          run.ContextLines,
		Providers:             run.Providers,
		ContainerBinary:       opts.ContainerBinary,
		RunnerImage:           opts.RunnerImage,
		Version:               opts.Version,
		OutputDir:             run.WorkDir,
		EnableDefaultRulesets: false,
	}, nil
}

// newEnvironmentForRuleTests returns a [kantraprovider.Environment] for kantra test
func newEnvironmentForRuleTests(opts TestOptions, run RuleTestRunParams) (kantraprovider.Environment, error) {
	cfg, err := environmentConfigForRuleTests(opts, run)
	if err != nil {
		return nil, err
	}
	return kantraprovider.NewEnvironment(cfg), nil
}
