package cmd

import (
	"os"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/cmd/internal/settings"
	"github.com/konveyor-ecosystem/kantra/pkg/testing"
	"github.com/spf13/cobra"
)

type testCommand struct {
	testFilterString string
	prune            bool
}

func NewTestCommand(log logr.Logger) *cobra.Command {
	testCmd := &testCommand{}

	testCobraCommand := &cobra.Command{
		Use:   "test",
		Short: "Test YAML rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			var testFilter testing.TestsFilter
			if testCmd.testFilterString != "" {
				testFilter = testing.NewInlineNameBasedFilter(testCmd.testFilterString)
			}
			tests, err := testing.Parse(args, testFilter)
			if err != nil {
				log.Error(err, "failed parsing rulesets")
				return err
			}
			if len(tests) == 0 {
				log.Info("no tests found")
				return nil
			}
			results, err := testing.NewRunner().Run(tests, testing.TestOptions{
				ContainerBinary: settings.Settings.ContainerBinary,
				ProviderImages: map[string]string{
					"java":   settings.Settings.JavaProviderImage,
					"go":     settings.Settings.GenericProviderImage,
					"python": settings.Settings.GenericProviderImage,
					"nodejs": settings.Settings.GenericProviderImage,
					"csharp": settings.Settings.CsharpProviderImage,
				},
				RunnerImage:     settings.Settings.RunnerImage,
				Version:         settings.Version,
				ProgressPrinter: testing.PrintProgress,
				Log:             log.V(3),
				Prune:           testCmd.prune,
				NoCleanup:       noCleanup,
			})
			testing.PrintSummary(os.Stdout, results)
			if err != nil {
				log.Error(err, "failed running tests")
				return err
			}
			return nil
		},
	}
	testCobraCommand.Flags().StringVarP(&testCmd.testFilterString, "test-filter", "t", "", "filter tests / testcases by their names")
	testCobraCommand.Flags().BoolVarP(&testCmd.prune, "prune", "p", false, "whether to prune after the execution; defaults to false")
	return testCobraCommand
}
