package cloud_foundry

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	cfConfig "github.com/cloudfoundry/go-cfclient/v3/config"
	"github.com/go-logr/logr"
	providerInterface "github.com/konveyor/asset-generation/pkg/providers/discoverers"
	cfProvider "github.com/konveyor/asset-generation/pkg/providers/discoverers/cloud_foundry"
	providerTypes "github.com/konveyor/asset-generation/pkg/providers/types/provider"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var (
	useLive           bool
	input             string
	outputFolder      string
	pType             string
	spaces            []string
	appName           string
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
			if useLive {
				if len(spaces) == 0 {
					return fmt.Errorf("at least one space is required")
				}
				if len(cfConfigPath) > 0 {
					_, err := os.Stat(cfConfigPath)
					if err != nil {
						return fmt.Errorf("failed to retrieve Cloud Foundry configuration file at %s:%s", cfConfigPath, err)
					}
				}
				return nil
			}
			if len(input) == 0 {
				return fmt.Errorf("input flag is required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var out io.Writer = cmd.OutOrStdout() // default to stdout

			if outputFolder != "" {

				err := os.MkdirAll(outputFolder, 0755)
				if err != nil {
					return fmt.Errorf("failed to create output file: %w", err)
				}

				outputPath := filepath.Join(outputFolder, "output.txt")
				file, err := os.Create(outputPath)
				if err != nil {
					return fmt.Errorf("failed to create file: %w", err)
				}
				defer file.Close()

				out = file
				logger.Info("Writing output to file: %s\n", outputPath)
			}
			return discoverManifest(out, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&input, "input", "", "input path of the manifest file or folder to analyze")
	cmd.Flags().StringVar(&outputFolder, "output-folder", "", "Directory where output manifests will be saved (default: standard output). If the directory does not exist, it will be created automatically.")

	// Live discovery flags
	cmd.Flags().BoolVar(&useLive, "use-live-connection", false, "Enable real-time discovery using live platform connections.")
	cmd.Flags().StringVar(&pType, "platformType", "cloud-foundry", "Platform type for discovery. Allowed value is: \"cloud-foundry\" (default).")
	cmd.Flags().StringVar(&cfConfigPath, "cf-config", "~/.cf/config", "Path to the Cloud Foundry CLI configuration file (default: ~/.cf/config).")
	cmd.Flags().StringSliceVar(&spaces, "spaces", []string{}, "Comma-separated list of Cloud Foundry spaces to analyze (e.g., --spaces=\"space1,space2\"). At least one space is required when using live discovery.")
	cmd.Flags().StringVar(&appName, "app-name", "", "Name of the Cloud Foundry application to discover.")
	cmd.Flags().BoolVar(&skipSslValidation, "skip-ssl-validation", false, "Skip SSL certificate validation for API connections (default: false).")

	cmd.MarkFlagsMutuallyExclusive("use-live-connection", "input")
	cmd.MarkFlagsRequiredTogether("use-live-connection", "spaces")

	return "Cloud Foundry V3 (local manifest)", cmd
}

type CloudFoundryInputParams struct {
	SpaceName string `json:"spaceName"`
	AppName   string `json:"appName"`
}

func discoverManifest(contentWriter io.Writer, secretWriter io.Writer) error {
	if useLive {
		return discoverLive(contentWriter, secretWriter)
	} else {
		return discoverFromFiles(contentWriter, secretWriter)
	}
}

func discoverFromFiles(contentWriter, secretWriter io.Writer) error {
	filesToProcess, err := getFilesToProcess(input)
	if err != nil {
		return err
	}

	for _, manifestPath := range filesToProcess {
		p, err := createProviderForManifest(manifestPath)
		if err != nil {
			logger.Error(err, "failed to stat input path", "input", input)
			return err
		}

		appListPerSpace, err := p.ListApps()
		if err != nil {
			return err
		}

		for _, appList := range appListPerSpace {
			err = processAppList(p, appList, contentWriter, secretWriter)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func discoverLive(contentWriter, secretWriter io.Writer) error {
	if pType != "cloud-foundry" {
		return fmt.Errorf("unsupported platform type: %s", pType)
	}

	opts := []cfConfig.Option{}
	if skipSslValidation {
		opts = append(opts, cfConfig.SkipTLSValidation())
	}

	var cfCfg *cfConfig.Config
	var err error
	if cfConfigPath != "" {
		cfCfg, err = cfConfig.NewFromCFHomeDir(cfConfigPath, opts...)
	} else {
		cfCfg, err = cfConfig.NewFromCFHome(opts...)
	}
	if err != nil {
		return err
	}

	cfg := &cfProvider.Config{
		CloudFoundryConfig: cfCfg,
		ManifestPath:       input,
		SpaceNames:         spaces,
	}

	p, err := cfProvider.New(cfg, &log.Logger{})
	if err != nil {
		return err
	}

	appListPerSpace, err := p.ListApps()
	if err != nil {
		return fmt.Errorf("failed to list apps by space: %w", err)
	}

	for space, appList := range appListPerSpace {
		for _, appName := range appList {
			input := cfProvider.AppReference{
				SpaceName: space,
				AppName:   fmt.Sprintf("%s", appName),
			}

			discoverResult, err := p.Discover(input)
			if err != nil {
				return err
			}

			err = OutputAppManifestsYAML(contentWriter, secretWriter, discoverResult)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func getFilesToProcess(input string) ([]string, error) {
	var filesToProcess []string
	finfo, err := os.Stat(input)
	if err != nil {
		logger.Error(err, "failed to stat input path", "input", input)
		return nil, err
	}

	if finfo.IsDir() {
		files, err := os.ReadDir(input)
		if err != nil {
			logger.Error(err, "failed to read input folder", "input", input)
			return nil, err
		}

		for _, manifestFile := range files {
			if manifestFile.IsDir() {
				log.Printf("unsupported nested directory. Skipping: %s\n", manifestFile.Name())
				continue
			}
			filesToProcess = append(filesToProcess, filepath.Join(input, manifestFile.Name()))
		}
	} else {
		filesToProcess = append(filesToProcess, input)
	}
	return filesToProcess, nil
}

func createProviderForManifest(manifestPath string) (providerInterface.Provider, error) {
	cfg := cfProvider.Config{
		ManifestPath: manifestPath,
	}
	stdLogger := log.New(os.Stdout, "", log.LstdFlags)
	return cfProvider.New(&cfg, stdLogger)
}

func processAppList(p providerInterface.Provider, appList []any, contentWriter, secretWriter io.Writer) error {
	for _, name := range appList {
		if appName != "" && appName != name {
			continue
		}
		input := cfProvider.AppReference{
			AppName: name.(string),
		}
		app, err := p.Discover(input)
		if err != nil {
			return err
		}

		err = OutputAppManifestsYAML(contentWriter, secretWriter, app)
		if err != nil {
			return err
		}
	}
	return nil
}

func OutputAppManifestsYAML(contentWriter io.Writer, secretWriter io.Writer, discoverResult *providerTypes.DiscoverResult) error {
	// if outputFolder != "" {
	b, err := yaml.Marshal(discoverResult.Content)
	if err != nil {
		return err
	}
	fmt.Fprintf(contentWriter, "%s", b)
	b, err = yaml.Marshal(discoverResult.Secret)
	if err != nil {
		return err
	}
	fmt.Fprintf(secretWriter, "%s", b)
	// }
	return nil
}
