package test

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/cmd/internal/settings"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/spf13/cobra"
)

type testCommand struct {
	testFilterString string
	prune            bool
	runLocal         bool
	mode             string
}

func NewTestCommand(log logr.Logger) *cobra.Command {
	testCmd := &testCommand{}

	testCobraCommand := &cobra.Command{
		Use:   "test",
		Short: "Test YAML rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			noCleanup := false
			if root := cmd.Root(); root != nil {
				if b, err := root.PersistentFlags().GetBool("no-cleanup"); err == nil {
					noCleanup = b
				}
			}

			var testFilter TestsFilter
			if testCmd.testFilterString != "" {
				testFilter = NewInlineNameBasedFilter(testCmd.testFilterString)
			}
			tests, err := Parse(args, testFilter)
			if err != nil {
				log.Error(err, "failed parsing rulesets")
				return err
			}
			if len(tests) == 0 {
				log.Info("no tests found")
				return nil
			}
			if testCmd.runLocal {
				if err := ValidateContainerlessProviders(tests); err != nil {
					log.Error(err, "invalid providers for containerless mode")
					return err
				}
			}
			mode, err := resolveTestCommandMode(testCmd.mode)
			if err != nil {
				log.Error(err, "invalid analysis mode")
				return err
			}
			results, err := NewRunner().Run(tests, TestOptions{
				Context:         cmd.Context(),
				RunLocal:        testCmd.runLocal,
				Mode:            mode,
				ContainerBinary: settings.Settings.ContainerBinary,
				ProviderImages: map[string]string{
					"java":   settings.Settings.JavaProviderImage,
					"go":     settings.Settings.GoProviderImage,
					"python": settings.Settings.PythonProviderImage,
					"nodejs": settings.Settings.NodeJSProviderImage,
					"csharp": settings.Settings.CsharpProviderImage,
				},
				RunnerImage:     settings.Settings.RunnerImage,
				Version:         settings.Version,
				ProgressPrinter: PrintProgress,
				Log:             log.V(3),
				Prune:           testCmd.prune,
				NoCleanup:       noCleanup,
			})
			PrintSummary(os.Stdout, results)
			if err != nil {
				log.Error(err, "failed running tests")
				return err
			}
			return nil
		},
	}
	testCobraCommand.Flags().StringVarP(&testCmd.testFilterString, "test-filter", "t", "", "filter tests / testcases by their names")
	testCobraCommand.Flags().BoolVarP(&testCmd.prune, "prune", "p", false, "whether to prune after the execution; defaults to false")
	testCobraCommand.Flags().BoolVar(&testCmd.runLocal, "run-local", false,
		"run Java and builtin providers on the host (containerless); default is hybrid mode (providers in containers), required for Go, Python, Node.js, and C# tests")
	testCobraCommand.Flags().StringVarP(&testCmd.mode, "mode", "m", "",
		"analysis mode. Must be one of 'full' (source + dependencies) or 'source-only' (default).")
	return testCobraCommand
}

func resolveTestCommandMode(mode string) (string, error) {
	if mode == "" {
		return string(provider.SourceOnlyAnalysisMode), nil
	}
	if mode != string(provider.FullAnalysisMode) &&
		mode != string(provider.SourceOnlyAnalysisMode) {
		return "", fmt.Errorf("mode must be one of 'full' or 'source-only'")
	}
	return mode, nil
}
