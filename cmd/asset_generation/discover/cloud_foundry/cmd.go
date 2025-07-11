package cloud_foundry

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	cfConfig "github.com/cloudfoundry/go-cfclient/v3/config"
	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/cmd/asset_generation/internal/printers"
	providerInterface "github.com/konveyor/asset-generation/pkg/providers/discoverers"
	cfProvider "github.com/konveyor/asset-generation/pkg/providers/discoverers/cloud_foundry"
	providerTypes "github.com/konveyor/asset-generation/pkg/providers/types/provider"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	useLive              bool
	input                string
	outputDir            string
	pType                string
	spaces               []string
	appName              string
	skipSslValidation    bool
	cfConfigPath         string
	logger               logr.Logger
	concealSensitiveData bool
	listApps             bool
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
				if cfConfigPath != "" {
					_, err := os.Stat(cfConfigPath)
					if err != nil {
						return fmt.Errorf("failed to retrieve Cloud Foundry configuration file at %s:%s", cfConfigPath, err)
					}
				}
				return nil
			}
			if input == "" {
				return fmt.Errorf("input flag is required")
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := createOutputDirIfNeeded(); err != nil {
				return fmt.Errorf("failed to create output folder: %w", err)
			}

			p, err := initProviderIfNeeded()
			if err != nil {
				return err
			}

			if listApps {
				return runListApps(p, useLive, cmd.OutOrStderr())
			}
			return discoverManifest(p, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&input, "input", "", "input path of the manifest file or folder to analyze")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Directory where output manifests will be saved (default: standard output). If the directory does not exist, it will be created automatically.")
	cmd.MarkFlagDirname("output-dir")
	cmd.Flags().BoolVar(&concealSensitiveData, "conceal-sensitive-data", false, "Extract sensitive information in the discover manifest into a separate file (default: false).")
	// Live discovery flags
	cmd.Flags().BoolVar(&useLive, "use-live-connection", false, "Enable real-time discovery using live platform connections.")
	cmd.Flags().StringVar(&pType, "platformType", "cloud-foundry", "Platform type for discovery. Allowed value is: \"cloud-foundry\" (default).")
	cmd.Flags().StringVar(&cfConfigPath, "cf-config", "~/.cf/config", "Path to the Cloud Foundry CLI configuration file (default: ~/.cf/config).")
	cmd.Flags().StringSliceVar(&spaces, "spaces", []string{}, "Comma-separated list of Cloud Foundry spaces to analyze (e.g., --spaces=\"space1,space2\"). At least one space is required when using live discovery.")
	cmd.Flags().StringVar(&appName, "app-name", "", "Name of the Cloud Foundry application to discover.")
	cmd.Flags().BoolVar(&skipSslValidation, "skip-ssl-validation", false, "Skip SSL certificate validation for API connections (default: false).")

	cmd.Flags().BoolVar(&listApps, "list-apps", false, "List applications available for each space.")

	cmd.MarkFlagsMutuallyExclusive("use-live-connection", "input")
	cmd.MarkFlagsOneRequired("use-live-connection", "input")
	cmd.MarkFlagsRequiredTogether("use-live-connection", "spaces")
	cmd.MarkFlagsMutuallyExclusive("list-apps", "app-name")
	cmd.MarkFlagsMutuallyExclusive("list-apps", "output-dir")
	return "Cloud Foundry V3 (local manifest)", cmd
}

func createOutputDirIfNeeded() error {
	if outputDir == "" {
		return nil
	}
	return os.MkdirAll(outputDir, 0755)
}

func initProviderIfNeeded() (providerInterface.Provider, error) {
	if !useLive {
		return nil, nil
	}
	return createLiveProvider()
}

func runListApps(p providerInterface.Provider, useLive bool, out io.Writer) error {
	if useLive {
		return listApplicationsLive(p, out)
	}
	return listApplicationsLocal(input, out)
}

func listApplicationsLive(p providerInterface.Provider, out io.Writer) error {
	appListPerSpace, err := p.ListApps()
	if err != nil {
		return fmt.Errorf("failed to list apps by space: %w", err)
	}
	printApps(appListPerSpace, out)

	return nil
}
func listApplicationsLocal(inputPath string, out io.Writer) error {
	filesToProcess, err := getFilesToProcess(inputPath)
	if err != nil {
		return err
	}
	for _, manifestPath := range filesToProcess {
		p, err := createProviderForManifest(manifestPath)
		if err != nil {
			logger.Error(err, "failed to stat input path", "input", input)
			return err
		}
		logger.Info("Analizing manifests file", "Manifest", manifestPath)
		appListPerSpace, err := p.ListApps()
		if err != nil {
			return err
		}
		printApps(appListPerSpace, out)
	}
	return nil
}

func printApps(appListPerSpace map[string][]any, out io.Writer) error {

	for space, appsAny := range appListPerSpace {
		fmt.Fprintf(out, "Space: %s\n", space)
		for _, appAny := range appsAny {
			appRef, ok := appAny.(cfProvider.AppReference)
			if !ok {
				return fmt.Errorf("unexpected type for app list: %T", appAny)
			}
			fmt.Fprintf(out, "  - %s\n", appRef.AppName)
		}
	}
	return nil
}

func discoverManifest(p providerInterface.Provider, out io.Writer) error {
	if useLive {
		return discoverLive(p, out)
	}
	return discoverFromFiles(out)
}

func discoverFromFiles(out io.Writer) error {
	filesToProcess, err := getFilesToProcess(input)
	if err != nil {
		return err
	}

	for _, manifestPath := range filesToProcess {
		p, err := createProviderForManifest(manifestPath)
		if err != nil {
			logger.Error(err, "failed to create provider for manifest", "manifestPath", manifestPath)
			return err
		}

		appListPerSpace, err := p.ListApps()
		if err != nil {
			return err
		}

		for _, appList := range appListPerSpace {
			err = processAppList(p, appList, out)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func createLiveProvider() (providerInterface.Provider, error) {
	if pType != "cloud-foundry" {
		return nil, fmt.Errorf("unsupported platform type: %s", pType)
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
		return nil, err
	}

	cfg := &cfProvider.Config{
		CloudFoundryConfig: cfCfg,
		SpaceNames:         spaces,
	}

	p, err := cfProvider.New(cfg, &logger, concealSensitiveData)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func discoverLive(p providerInterface.Provider, out io.Writer) error {
	appListPerSpace, err := p.ListApps()
	if err != nil {
		return fmt.Errorf("failed to list apps by space: %w", err)
	}

	for _, appList := range appListPerSpace {
		for _, appReferences := range appList {
			appRef, ok := appReferences.(cfProvider.AppReference)
			if !ok {
				return fmt.Errorf("unexpected type for app list: %T", appReferences)
			}
			if appName != "" && appRef.AppName != appName {
				logger.Info("Skipping application: app name does not match target app name", "app name", appRef.AppName, "target app name", appName)
				continue
			}

			discoverResult, err := p.Discover(appRef)
			if err != nil {
				return err
			}
			err = OutputAppManifestsYAML(out, discoverResult, appRef.SpaceName, appRef.AppName)
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
				logger.Info("unsupported nested directory. Skip.", "name", manifestFile.Name())
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
	return cfProvider.New(&cfg, &logger, concealSensitiveData)
}

func processAppList(p providerInterface.Provider, appList []any, out io.Writer) error {

	for _, appReferences := range appList {
		appRef, ok := appReferences.(cfProvider.AppReference)
		if !ok {
			return fmt.Errorf("unexpected type for app list: %T", appReferences)
		}
		if appName != "" && appName != appRef.AppName {
			continue
		}
		app, err := p.Discover(appRef)
		if err != nil {
			return err
		}

		err = OutputAppManifestsYAML(out, app, appRef.SpaceName, appRef.AppName)
		if err != nil {
			return err
		}
	}
	return nil
}

func OutputAppManifestsYAML(out io.Writer, discoverResult *providerTypes.DiscoverResult, spaceName string, appName string) error {
	suffix := "_" + appName
	if spaceName != "" {
		suffix = "_" + spaceName + suffix
	}
	printer := printers.NewOutput(out)
	printFunc := printer.ToStdout
	// Marshal content
	d, err := cfProvider.MarshalUnmarshal[cfProvider.Application](discoverResult.Content)
	if err != nil {
		return err
	}
	contentBytes, err := yaml.Marshal(d)
	if err != nil {
		return err
	}

	if outputDir != "" {
		printFunc = func(filename, contents string) error {
			return printers.ToFile(outputDir, filename, contents)
		}
		contentFileName := fmt.Sprintf("discover_manifest%s.yaml", suffix)
		logger.Info("Writing content to file", "path", contentFileName)
		if err := printFunc(contentFileName, string(contentBytes)); err != nil {
			return err
		}
	} else {
		contentHeader := ""
		// Write to stdout
		if discoverResult.Secret != nil {
			contentHeader = "--- Content Section ---\n"
		}
		printer.ToStdoutWithHeader(contentHeader, string(contentBytes))

	}

	if discoverResult.Secret != nil {
		secretBytes, err := yaml.Marshal(discoverResult.Secret)
		if err != nil {
			return err
		}

		secretStr := string(secretBytes)

		if !isEmptyYamlString(secretStr) {
			if outputDir != "" {
				secretFileName := fmt.Sprintf("secrets%s.yaml", suffix)
				logger.Info("Writing secrets to file", "path", secretFileName)
				if err := printFunc(secretFileName, string(secretStr)); err != nil {
					return err
				}
			} else {
				// Write to stdout
				printer.ToStdoutWithHeader("\n--- Secrets Section ---\n", secretStr)
			}
		}
	}
	return nil
}

func isEmptyYamlString(yamlString string) bool {
	return len(yamlString) == 0 || yamlString == "{}\n" || yamlString == "{}"
}
