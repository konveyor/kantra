package helm

import (
	"fmt"
	"io"
	"maps"
	"os"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/strvals"

	"github.com/konveyor-ecosystem/kantra/cmd/asset_generation/internal/printers"
	helmProvider "github.com/konveyor/asset-generation/pkg/providers/generators/helm"
)

var (
	input      string
	outputDir  string
	chartDir   string
	nonK8SOnly bool
	setValues  []string
)

func NewGenerateHelmCommand(log logr.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "helm",
		Short: "generate the helm template manifests",
		RunE: func(cmd *cobra.Command, args []string) error {
			return helm(cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&chartDir, "chart-dir", "", "Directory to the Helm chart to use for chart generation.")
	cmd.Flags().StringVar(&input, "input", "", "Specifies the discover manifest file")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Directory to save the generated Helm chart. Defaults to stdout")
	cmd.Flags().BoolVar(&nonK8SOnly, "non-k8s-only", false, "Render only the non-Kubernetes templates located in the files/konveyor directory of the chart")
	cmd.Flags().StringArrayVar(&setValues, "set", []string{}, "Set values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)")

	// Validation
	cmd.MarkFlagDirname("chart-dir")
	cmd.MarkFlagDirname("output-dir")
	cmd.MarkFlagFilename("input")
	// Required
	cmd.MarkFlagRequired("chart-dir")
	cmd.MarkFlagRequired("input")

	return cmd
}

func helm(out io.Writer) error {

	values, err := loadDiscoverManifest()
	if err != nil {
		return err
	}
	if len(setValues) > 0 {
		v, err := parseValues()
		if err != nil {
			return err
		}
		maps.Copy(values, v)
	}

	cfg := helmProvider.Config{
		ChartPath:              chartDir,
		Values:                 values,
		SkipRenderK8SManifests: nonK8SOnly,
	}
	generator := helmProvider.New(cfg)
	rendered, err := generator.Generate()
	if err != nil {
		return err
	}

	output := printers.NewOutput(out)

	if outputDir != "" {
		err = os.MkdirAll(outputDir, 0755)
		if err != nil {
			return err
		}
		for filename, contents := range rendered {
			err = printers.ToFile(outputDir, filename, contents)
			if err != nil {
				return err
			}
		}
	} else {
		for filename, contents := range rendered {
			header := fmt.Sprintf("---\n# Source: %s\n", filename)
			err = output.ToStdoutWithHeader(header, contents)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func loadDiscoverManifest() (map[string]interface{}, error) {
	d, err := os.ReadFile(input)
	if err != nil {
		return nil, fmt.Errorf("unable to load discover manifest: %s", err)
	}
	var m map[string]interface{}
	err = yaml.Unmarshal(d, &m)
	return m, err
}

func parseValues() (map[string]interface{}, error) {
	// User specified a value via --set
	base := make(map[string]interface{})
	for _, value := range setValues {
		if err := strvals.ParseInto(value, base); err != nil {
			return nil, fmt.Errorf("failed parsing --set data:%s", err)
		}
	}
	return base, nil
}
