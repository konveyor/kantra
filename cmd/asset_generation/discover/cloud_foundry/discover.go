package cloud_foundry

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	discover "github.com/konveyor/asset-generation/pkg/discover/cloud_foundry"
	korifiDiscover "github.com/konveyor/asset-generation/pkg/discover/cloud_foundry/korifi/provider"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var (
	useLive        bool
	input          string
	output         string
	cfURL          string
	username       string
	kubeconfigPath string
	spaces         []string
	outputPrefix   string
	outputFolder   string

	logger logr.Logger
)

func NewDiscoverCloudFoundryCommand(log logr.Logger) (string, *cobra.Command) {
	logger = log
	cmd := &cobra.Command{
		Aliases: []string{"cf"},
		Use:     "cloud-foundry",
		Short:   "Discover Cloud Foundry applications",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.ParseFlags(args); err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return discoverManifest(cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&input, "input", "", "specify the location of the manifest.yaml to analyze.")
	cmd.Flags().StringVar(&output, "output", "", "output file (default: standard output).")
	// live discovery flags
	cmd.Flags().BoolVar(&useLive, "use-live-connection", false, "uses live platform connections for real-time discovery")
	cmd.Flags().StringVar(&cfURL, "cf-url", "", "cf API URL (e.g., http://172.18.0.2).")
	cmd.Flags().StringVar(&username, "username", "", "Korifi username")
	cmd.Flags().StringVar(&kubeconfigPath, "kubeconfig-path", "~/.kube/config", "kubeconfig-path default: ~/.kube/config")
	cmd.Flags().StringSliceVar(&spaces, "spaces", []string{}, "list of spaces to check (e.g. --spaces=\"s1,s2\"). If not provided, all spaces will be checked.")
	cmd.Flags().StringVar(&outputPrefix, "output-prefix", "manifest_", "output prefix filename (default: \"manifest_\").")
	cmd.Flags().StringVar(&outputFolder, "output-folder", "./manifests", "output folder path (default: \"./manifests-\"). If doesn't exist, it will be created.")

	cmd.MarkFlagFilename("input", "yaml", "yml")
	cmd.MarkFlagFilename("output")
	cmd.MarkFlagsOneRequired("input", "use-live-connection")
	cmd.MarkFlagsMutuallyExclusive("input", "use-live-connection")
	cmd.MarkFlagsMutuallyExclusive("input", "output-prefix")
	cmd.MarkFlagsMutuallyExclusive("output", "output-prefix")

	cmd.MarkFlagsRequiredTogether("use-live-connection", "username", "cf-url")

	return "Cloud Foundry V3 (local manifest)", cmd
}

func discoverManifest(writer io.Writer) error {
	var err error
	var b []byte
	if !useLive {
		b, err = os.ReadFile(input)
		if err != nil {
			return err
		}
		err = writeManifest(writer, b)
	} else {
		korifiConfig := korifiDiscover.KorifiConfig{
			KubeconfigPath: kubeconfigPath,
			Username:       username,
			BaseURL:        cfURL,
		}

		korifiProvider := korifiDiscover.NewKorifiProvider(korifiConfig)
		ld, err := discover.NewLiveDiscoverer(logger, korifiProvider, &spaces)
		if err != nil {
			return err
		}
		cfManifests, err := ld.Discover()
		if err != nil {
			return err
		}
		for i, cfManifest := range *cfManifests {
			b, err := json.Marshal(cfManifest)
			if err != nil {
				return err
			}

			err = os.MkdirAll(outputFolder, os.ModePerm)
			if err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}
			filename := fmt.Sprintf("%s%s.yaml", outputPrefix, cfManifest.Space)
			fullPath := filepath.Join(outputFolder, filename)

			if err := os.WriteFile(fullPath, b, 0644); err != nil {
				return err
			}
			fmt.Fprintf(writer, "Writing %d of %d manifests: %s\n", i, len(*cfManifests), fullPath)
		}
	}
	return err
}

func writeManifest(writer io.Writer, b []byte) error {
	ma := discover.CloudFoundryManifest{}
	err := yaml.Unmarshal(b, &ma)
	if err != nil {
		return err
	}
	a, err := discover.Discover(ma)
	if err != nil {
		return err

	}
	fmt.Printf("discovered manifest: %v\n", a)
	b, err = yaml.Marshal(a)
	if err != nil {
		return err

	}
	if output == "" {
		fmt.Fprintf(writer, "%s", b)
		return nil
	}
	return os.WriteFile(output, b, 0644)
}
