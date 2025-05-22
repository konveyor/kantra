package cloud_foundry

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	pInterfaces "github.com/konveyor/asset-generation/pkg/providers"
	cfProvider "github.com/konveyor/asset-generation/pkg/providers/cf"
	kProvider "github.com/konveyor/asset-generation/pkg/providers/korifi"
	dTypes "github.com/konveyor/asset-generation/pkg/providers/types/discover"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var (
	useLive           bool
	inputFolder       string
	apiURL            string
	pType             string
	username          string
	kubeconfigPath    string
	spaces            []string
	outputPrefix      string
	outputFolder      string
	cfToken           string
	skipSslValidation bool
	cfConfigPath      string
	logger            logr.Logger
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
			if useLive && len(spaces) == 0 {
				return fmt.Errorf("at least one space is required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return discoverManifest(cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&outputFolder, "output-folder", "./output-manifests", "output folder path (default: \"./output-manifests-\"). If doesn't exist, it will be created.")
	cmd.Flags().StringVar(&inputFolder, "input-folder", "./input-manifests", "input folder path of the manifest to analyze (default: \"./input-manifests-\").")

	// live discovery flags
	cmd.Flags().BoolVar(&useLive, "use-live-connection", false, "uses live platform connections for real-time discovery")
	cmd.Flags().StringVar(&pType, "platformType", "cf", "Platform type (cf or korifi). Default is cf.")
	cmd.Flags().StringVar(&apiURL, "api-url", "", "API URL (e.g., http://172.18.0.2).")
	cmd.Flags().StringVar(&username, "username", "", "Username")

	cmd.Flags().StringVar(&kubeconfigPath, "kubeconfig-path", "~/.kube/config", "kubeconfig path. Default: ~/.kube/config")
	cmd.Flags().StringVar(&cfConfigPath, "cf-config", "~/.cf/config", "Cloud Foundry config path. Default: ~/.cf/config")
	cmd.Flags().StringVar(&cfToken, "cf-token", "", "Cloud Foundry token")

	cmd.Flags().StringSliceVar(&spaces, "spaces", []string{}, "list of spaces to check (e.g. --spaces=\"s1,s2\"). At least one space is required.")
	cmd.Flags().BoolVar(&skipSslValidation, "skip-ssl-validation", false, "Skip SSL validation for the API URL. Deafult: false.")

	// cmd.MarkFlagFilename("input", "yaml", "yml")
	cmd.MarkFlagFilename("output")

	cmd.MarkFlagsMutuallyExclusive("cf-config", "kubeconfig-path")
	cmd.MarkFlagsMutuallyExclusive("cf-config", "cf-token")

	cmd.MarkFlagsRequiredTogether("use-live-connection", "api-url", "spaces")

	return "Cloud Foundry V3 (local manifest)", cmd
}

func discoverManifest(writer io.Writer) error {
	var err error
	if !useLive {
		files, err := os.ReadDir(inputFolder)
		if err != nil {
			log.Fatal(err)
		}

		for _, manifestFile := range files {
			if manifestFile.IsDir() {
				log.Printf("unsupported nested directory. Skipping: %s\n", manifestFile.Name())
				continue
			}

			cfg := cfProvider.Config{
				ManifestPath: filepath.Join(inputFolder, manifestFile.Name()),
				OutputFolder: outputFolder,
			}

			apps, err := pInterfaces.Discover[[]dTypes.Application](&cfg)
			if err != nil {
				return err
			}

			err = OutputAppManifestsYAML(writer, apps)
			if err != nil {
				return err
			}
		}
		return nil
	} else {
		var cfg pInterfaces.Config

		// platformConfig := map[string]any{}
		if pType == "korifi" {
			cfg = &kProvider.Config{
				KubeconfigPath: kubeconfigPath,
				Username:       username,
				BaseURL:        apiURL,
				SpaceNames:     spaces,
			}
		} else if pType == "cf" {
			// platformConfig = map[string]any{}
			cfg = &cfProvider.Config{
				CFConfigPath:      cfConfigPath,
				Username:          username,
				Token:             cfToken,
				APIEndpoint:       apiURL,
				SkipSslValidation: skipSslValidation,
				SpaceNames:        spaces,
			}
		}
		apps, err := pInterfaces.Discover[[]dTypes.Application](cfg)
		if err != nil {
			return err
		}
		log.Println(apps)
		err = OutputAppManifestsYAML(writer, apps)
		if err != nil {
			return err
		}
	}
	return err
}

func OutputAppManifestsYAML(writer io.Writer, apps []dTypes.Application) error {
	if apps != nil && outputFolder != "" {
		for _, appManifest := range apps {
			b, err := yaml.Marshal(appManifest)
			if err != nil {
				return err
			}
			fmt.Printf("discovered manifest: %v\n", appManifest)
			fmt.Fprintf(writer, "%s", b)
		}
	}
	return nil
}
