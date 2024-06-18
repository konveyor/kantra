package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"runtime"

	"path/filepath"
	"sort"
	"strings"

	"github.com/devfile/alizer/pkg/apis/recognizer"
	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/cmd/internal/hiddenfile"
	"github.com/konveyor-ecosystem/kantra/pkg/container"
	"github.com/konveyor/analyzer-lsp/engine"
	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/phayes/freeport"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"

	"github.com/spf13/cobra"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

var (
	// TODO (pgaikwad): this assumes that the $USER in container is always root, it may not be the case in future
	M2Dir = path.Join("/", "root", ".m2")
	// application source path inside the container
	SourceMountPath = path.Join(InputPath, "source")
	// analyzer config files
	ConfigMountPath = path.Join(InputPath, "config")
	// user provided rules path
	RulesMountPath = path.Join(RulesetPath, "input")
	// paths to files in the container
	AnalysisOutputMountPath   = path.Join(OutputPath, "output.yaml")
	DepsOutputMountPath       = path.Join(OutputPath, "dependencies.yaml")
	ProviderSettingsMountPath = path.Join(ConfigMountPath, "settings.json")
)

const (
	javaProvider   = "java"
	goProvider     = "go"
	pythonProvider = "python"
	nodeJSProvider = "javascript"
)

// provider config options
const (
	mavenSettingsFile      = "mavenSettingsFile"
	lspServerPath          = "lspServerPath"
	lspServerName          = "lspServerName"
	workspaceFolders       = "workspaceFolders"
	dependencyProviderPath = "dependencyProviderPath"
)

// kantra analyze flags
type analyzeCommand struct {
	listSources              bool
	listTargets              bool
	skipStaticReport         bool
	analyzeKnownLibraries    bool
	jsonOutput               bool
	overwrite                bool
	bulk                     bool
  mavenSettingsFile        string
	sources                  []string
	targets                  []string
	labelSelector            string
	input                    string
	output                   string
	mode                     string
	rules                    []string
	jaegerEndpoint           string
	enableDefaultRulesets    bool
	contextLines             int
	incidentSelector         string
	depFolders               []string
	overrideProviderSettings string

	// tempDirs list of temporary dirs created, used for cleanup
	tempDirs []string
	log      logr.Logger
	// isFileInput is set when input points to a file and not a dir
	isFileInput bool
	logLevel    *uint32
	// used for cleanup
	networkName            string
	volumeName             string
	providerContainerNames []string
	cleanup                bool
}

// analyzeCmd represents the analyze command
func NewAnalyzeCmd(log logr.Logger) *cobra.Command {
	analyzeCmd := &analyzeCommand{
		log:     log,
		cleanup: true,
	}

	analyzeCommand := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze application source code",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// TODO (pgaikwad): this is nasty
			if !cmd.Flags().Lookup("list-sources").Changed &&
				!cmd.Flags().Lookup("list-targets").Changed {
				//cmd.MarkFlagRequired("input")
				cmd.MarkFlagRequired("output")
				if err := cmd.ValidateRequiredFlags(); err != nil {
					return err
				}
			}
			err := analyzeCmd.Validate(cmd.Context())
			if err != nil {
				log.Error(err, "failed to validate flags")
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if val, err := cmd.Flags().GetUint32(logLevelFlag); err == nil {
				analyzeCmd.logLevel = &val
			}
			if val, err := cmd.Flags().GetBool(noCleanupFlag); err == nil {
				analyzeCmd.cleanup = !val
			}
			if analyzeCmd.overrideProviderSettings == "" {
				if analyzeCmd.listSources || analyzeCmd.listTargets {
					err := analyzeCmd.ListLabels(cmd.Context())
					if err != nil {
						log.Error(err, "failed to list rule labels")
						return err
					}
					return nil
				}
				xmlOutputDir, err := analyzeCmd.ConvertXML(cmd.Context())
				if err != nil {
					log.Error(err, "failed to convert xml rules")
					return err
				}
				components, err := recognizer.DetectComponents(analyzeCmd.input)
				if err != nil {
					log.Error(err, "Failed to determine languages for input")
					return err
				}
				foundProviders := []string{}
				if analyzeCmd.isFileInput {
					foundProviders = append(foundProviders, javaProvider)
				} else {
					for _, c := range components {
						log.Info("Got component", "component language", c.Languages, "path", c.Path)
						for _, l := range c.Languages {
							foundProviders = append(foundProviders, strings.ToLower(l.Name))
						}
					}
				}
				containerNetworkName, err := analyzeCmd.createContainerNetwork()
				if err != nil {
					log.Error(err, "failed to create container network")
					return err
				}
				// share source app with provider and engine containers
				containerVolName, err := analyzeCmd.createContainerVolume()
				if err != nil {
					log.Error(err, "failed to create container volume")
					return err
				}
				// defer cleaning created resources here instead of PostRun
				// if Run returns an error, PostRun does not run
				defer func() {
					if err := analyzeCmd.CleanAnalysisResources(cmd.Context()); err != nil {
						log.Error(err, "failed to clean temporary directories")
					}
				}()
				// allow for 5 retries of running provider in the case of port in use
				providerPorts, err := analyzeCmd.RunProviders(cmd.Context(), containerNetworkName, containerVolName, foundProviders, 5)
				if err != nil {
					log.Error(err, "failed to run provider")
					return err
				}
				err = analyzeCmd.RunAnalysis(cmd.Context(), xmlOutputDir, containerVolName, foundProviders, providerPorts)
				if err != nil {
					log.Error(err, "failed to run analysis")
					return err
				}
			} else {
				err := analyzeCmd.RunAnalysisOverrideProviderSettings(cmd.Context())
				if err != nil {
					log.Error(err, "failed to run analysis")
					return err
				}
			}
			err := analyzeCmd.CreateJSONOutput()
			if err != nil {
				log.Error(err, "failed to create json output file")
				return err
			}
			err = analyzeCmd.GenerateStaticReport(cmd.Context())
			if err != nil {
				log.Error(err, "failed to generate static report")
				return err
			}

			return nil
		},
	}
	analyzeCommand.Flags().BoolVar(&analyzeCmd.listSources, "list-sources", false, "list rules for available migration sources")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.listTargets, "list-targets", false, "list rules for available migration targets")
	analyzeCommand.Flags().StringArrayVarP(&analyzeCmd.sources, "source", "s", []string{}, "source technology to consider for analysis. Use multiple times for additional sources: --source <source1> --source <source2> ...")
	analyzeCommand.Flags().StringArrayVarP(&analyzeCmd.targets, "target", "t", []string{}, "target technology to consider for analysis. Use multiple times for additional targets: --target <target1> --target <target2> ...")
	analyzeCommand.Flags().StringVarP(&analyzeCmd.labelSelector, "label-selector", "l", "", "run rules based on specified label selector expression")
	analyzeCommand.Flags().StringArrayVar(&analyzeCmd.rules, "rules", []string{}, "filename or directory containing rule files. Use multiple times for additional rules: --rules <rule1> --rules <rule2> ...")
	analyzeCommand.Flags().StringVarP(&analyzeCmd.input, "input", "i", "", "path to application source code or a binary")
	analyzeCommand.Flags().StringVarP(&analyzeCmd.output, "output", "o", "", "path to the directory for analysis output")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.skipStaticReport, "skip-static-report", false, "do not generate static report")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.analyzeKnownLibraries, "analyze-known-libraries", false, "analyze known open-source libraries")
	analyzeCommand.Flags().StringVar(&analyzeCmd.mavenSettingsFile, "maven-settings", "", "path to a custom maven settings file to use")
	analyzeCommand.Flags().StringVarP(&analyzeCmd.mode, "mode", "m", string(provider.FullAnalysisMode), "analysis mode. Must be one of 'full' or 'source-only'")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.jsonOutput, "json-output", false, "create analysis and dependency output as json")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.overwrite, "overwrite", false, "overwrite output directory")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.bulk, "bulk", false, "running multiple analyze commands in bulk will result to combined static report")
	analyzeCommand.Flags().StringVar(&analyzeCmd.jaegerEndpoint, "jaeger-endpoint", "", "jaeger endpoint to collect traces")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.enableDefaultRulesets, "enable-default-rulesets", true, "run default rulesets with analysis")
	analyzeCommand.Flags().IntVar(&analyzeCmd.contextLines, "context-lines", 100, "number of lines of source code to include in the output for each incident")
	analyzeCommand.Flags().StringVar(&analyzeCmd.incidentSelector, "incident-selector", "", "an expression to select incidents based on custom variables. ex: (!package=io.konveyor.demo.config-utils)")
	analyzeCommand.Flags().StringArrayVarP(&analyzeCmd.depFolders, "dependency-folders", "d", []string{}, "directory for dependencies")
	analyzeCommand.Flags().StringVar(&analyzeCmd.overrideProviderSettings, "override-provider-settings", "", "override the provider settings, the analysis pod will be run on the host network and no providers with be started up")

	return analyzeCommand
}

func (a *analyzeCommand) Validate(ctx context.Context) error {
	if a.listSources || a.listTargets {
		return nil
	}
	if a.labelSelector != "" && (len(a.sources) > 0 || len(a.targets) > 0) {
		return fmt.Errorf("must not specify label-selector and sources or targets")
	}
	// Validate source labels
	if len(a.sources) > 0 {
		var sourcesRaw bytes.Buffer
		a.fetchLabels(ctx, true, false, &sourcesRaw)
		knownSources := strings.Split(sourcesRaw.String(), "\n")
		for _, source := range a.sources {
			found := false
			for _, knownSource := range knownSources {
				if source == knownSource {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("unknown source: \"%s\"", source)
			}
		}
	}
	// Validate source labels
	if len(a.targets) > 0 {
		var targetRaw bytes.Buffer
		a.fetchLabels(ctx, false, true, &targetRaw)
		knownTargets := strings.Split(targetRaw.String(), "\n")
		for _, source := range a.targets {
			found := false
			for _, knownTarget := range knownTargets {
				if source == knownTarget {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("unknown target: \"%s\"", source)
			}
		}
	}

	if a.overrideProviderSettings != "" {
		stat, err := os.Stat(a.overrideProviderSettings)
		if err != nil {
			return fmt.Errorf("%w failed to stat overriden provider settings %s", err, a.overrideProviderSettings)
		}
		if stat.IsDir() {
			return fmt.Errorf("provider settings must be a file")
		}
		a.overrideProviderSettings, err = filepath.Abs(a.overrideProviderSettings)
		if err != nil {
			return fmt.Errorf("%w failed to get absolute path for override provider settings %s", err, a.overrideProviderSettings)
		}
	} else if a.input != "" {
		// do not allow multiple input applications
		inputNum := 0
		for _, arg := range os.Args {
			if arg == "-i" || strings.Contains(arg, "--input") {
				inputNum += 1
				if inputNum > 1 {
					return fmt.Errorf("must specify only one input source")
				}
			}
		}
		stat, err := os.Stat(a.input)
		if err != nil {
			return fmt.Errorf("%w failed to stat input path %s", err, a.input)
		}
		// when input isn't a dir, it's pointing to a binary
		// we need abs path to mount the file correctly
		if !stat.Mode().IsDir() {
			a.input, err = filepath.Abs(a.input)
			if err != nil {
				return fmt.Errorf("%w failed to get absolute path for input file %s", err, a.input)
			}
			// make sure we mount a file and not a dir
			SourceMountPath = path.Join(SourceMountPath, filepath.Base(a.input))
			a.isFileInput = true
		}
	}
	err := a.CheckOverwriteOutput()
	if err != nil {
		return err
	}
	stat, err := os.Stat(a.output)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = os.MkdirAll(a.output, os.ModePerm)
			if err != nil {
				return fmt.Errorf("%w failed to create output dir %s", err, a.output)
			}
		} else {
			return fmt.Errorf("failed to stat output directory %s", a.output)
		}
	}
	if stat != nil && !stat.IsDir() {
		return fmt.Errorf("output path %s is not a directory", a.output)
	}
	if len(a.depFolders) != 0 {
		for i := range a.depFolders {
			stat, err := os.Stat(a.depFolders[i])
			if err != nil {
				return fmt.Errorf("%w failed to stat dependency folder %v", err, a.depFolders[i])
			}
			if stat != nil && !stat.IsDir() {
				return fmt.Errorf("depdendecy folder %v is not a directory", a.depFolders[i])
			}
		}
	}
	if a.mode != string(provider.FullAnalysisMode) &&
		a.mode != string(provider.SourceOnlyAnalysisMode) {
		return fmt.Errorf("mode must be one of 'full' or 'source-only'")
	}
	if _, err := os.Stat(a.mavenSettingsFile); a.mavenSettingsFile != "" && err != nil {
		return fmt.Errorf("%w failed to stat maven settings file at path %s", err, a.mavenSettingsFile)
	}
	// try to get abs path, if not, continue with relative path
	if absPath, err := filepath.Abs(a.output); err == nil {
		a.output = absPath
	}
	if absPath, err := filepath.Abs(a.input); err == nil {
		a.input = absPath
	}
	if absPath, err := filepath.Abs(a.mavenSettingsFile); a.mavenSettingsFile != "" && err == nil {
		a.mavenSettingsFile = absPath
	}
	if !a.enableDefaultRulesets && len(a.rules) == 0 {
		return fmt.Errorf("must specify rules if default rulesets are not enabled")
	}
	return nil
}

func (a *analyzeCommand) CheckOverwriteOutput() error {
	// default overwrite to false so check for already existing output dir
	stat, err := os.Stat(a.output)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if a.bulk {
		lockStat, _ := os.Stat(filepath.Join(a.output, "analysis.log"))
		if lockStat != nil {
			return fmt.Errorf("output dir %v already contains 'analysis.log', it was used for single application analysis or there is running --bulk analysis, try another output dir", a.output)
		}
		sameInputStat, _ := os.Stat(fmt.Sprintf("%s.%s", filepath.Join(a.output, "output.yaml"), a.inputShortName()))
		if sameInputStat != nil {
			return fmt.Errorf("output dir %v already contains analysis report for provided input '%v', try another input or change output dir", a.output, a.inputShortName())
		}
	} else {
		if !a.overwrite && stat != nil {
			return fmt.Errorf("output dir %v already exists and --overwrite not set", a.output)
		}
	}
	if a.overwrite && stat != nil {
		err := os.RemoveAll(a.output)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *analyzeCommand) ListLabels(ctx context.Context) error {
	return a.fetchLabels(ctx, a.listSources, a.listTargets, os.Stdout)
}

func (a *analyzeCommand) fetchLabels(ctx context.Context, listSources, listTargets bool, out io.Writer) error {
	// reserved labels
	sourceLabel := outputv1.SourceTechnologyLabel
	targetLabel := outputv1.TargetTechnologyLabel
	runMode := "RUN_MODE"
	runModeContainer := "container"
	if os.Getenv(runMode) == runModeContainer {
		if listSources {
			sourceSlice, err := readRuleFilesForLabels(sourceLabel)
			if err != nil {
				a.log.Error(err, "failed to read rule labels")
				return err
			}
			listOptionsFromLabels(sourceSlice, sourceLabel, out)
			return nil
		}
		if listTargets {
			targetsSlice, err := readRuleFilesForLabels(targetLabel)
			if err != nil {
				a.log.Error(err, "failed to read rule labels")
				return err
			}
			listOptionsFromLabels(targetsSlice, targetLabel, out)
			return nil
		}
	} else {
		volumes, err := a.getRulesVolumes()
		if err != nil {
			a.log.Error(err, "failed getting rules volumes")
			return err
		}
		args := []string{"analyze"}
		if listSources {
			args = append(args, "--list-sources")
		} else {
			args = append(args, "--list-targets")
		}
		err = container.NewContainer().Run(
			ctx,
			container.WithImage(Settings.RunnerImage),
			container.WithLog(a.log.V(1)),
			container.WithEnv(runMode, runModeContainer),
			container.WithVolumes(volumes),
			container.WithEntrypointBin(fmt.Sprintf("/usr/local/bin/%s", Settings.RootCommandName)),
			container.WithContainerToolBin(Settings.PodmanBinary),
			container.WithEntrypointArgs(args...),
			container.WithStdout(out),
			container.WithCleanup(a.cleanup),
		)
		if err != nil {
			a.log.Error(err, "failed listing labels")
			return err
		}
	}
	return nil
}

func readRuleFilesForLabels(label string) ([]string, error) {
	labelsSlice := []string{}
	err := filepath.WalkDir(RulesetPath, walkRuleSets(RulesetPath, label, &labelsSlice))
	if err != nil {
		return nil, err
	}
	return labelsSlice, nil
}

func walkRuleSets(root string, label string, labelsSlice *[]string) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			*labelsSlice, err = readRuleFile(path, labelsSlice, label)
			if err != nil {
				return err
			}
		}
		return err
	}
}

func readRuleFile(filePath string, labelsSlice *[]string, label string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanWords)

	for scanner.Scan() {
		// add source/target labels to slice
		label := getSourceOrTargetLabel(scanner.Text(), label)
		if len(label) > 0 && !slices.Contains(*labelsSlice, label) {
			*labelsSlice = append(*labelsSlice, label)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return *labelsSlice, nil
}

func getSourceOrTargetLabel(text string, label string) string {
	if strings.Contains(text, label) {
		return text
	}
	return ""
}

func listOptionsFromLabels(sl []string, label string, out io.Writer) {
	var newSl []string
	l := label + "="

	for _, label := range sl {
		newSt := strings.TrimPrefix(label, l)

		if newSt != label {
			newSt = strings.TrimSuffix(newSt, "+")
			newSt = strings.TrimSuffix(newSt, "-")

			if !slices.Contains(newSl, newSt) {
				newSl = append(newSl, newSt)

			}
		}
	}
	sort.Strings(newSl)

	if label == outputv1.SourceTechnologyLabel {
		fmt.Fprintln(out, "available source technologies:")
	} else {
		fmt.Fprintln(out, "available target technologies:")
	}
	for _, tech := range newSl {
		fmt.Fprintln(out, tech)
	}
}

func (a *analyzeCommand) getDepsFolders() (map[string]string, []string) {
	vols := map[string]string{}
	dependencyFolders := []string{}
	if len(a.depFolders) != 0 {
		for i := range a.depFolders {
			newDepPath := path.Join(InputPath, fmt.Sprintf("deps%v", i))
			vols[a.depFolders[i]] = newDepPath
			dependencyFolders = append(dependencyFolders, newDepPath)
		}
		return vols, dependencyFolders
	}

	return vols, dependencyFolders
}

func (a *analyzeCommand) getConfigVolumes(providers []string, ports map[string]int) (map[string]string, error) {
	tempDir, err := os.MkdirTemp("", "analyze-config-")
	if err != nil {
		a.log.V(1).Error(err, "failed creating temp dir", "dir", tempDir)
		return nil, err
	}
	a.log.V(1).Info("created directory for provider settings", "dir", tempDir)
	a.tempDirs = append(a.tempDirs, tempDir)

	var foundJava bool
	var foundGolang bool
	var foundPython bool
	var foundNode bool
	switch providers[0] {
	case javaProvider:
		foundJava = true
	case goProvider:
		foundGolang = true
	case pythonProvider:
		foundPython = true
	case nodeJSProvider:
		foundNode = true
	default:
		return nil, fmt.Errorf("unable to find config for provider %v", providers[0])
	}
	if !foundJava && a.isFileInput {
		foundJava = true
	}

	otherProvsMountPath := SourceMountPath
	// when input is a file, it means it's probably a binary
	// only java provider can work with binaries, all others
	// continue pointing to the directory instead of file
	if a.isFileInput {
		SourceMountPath = path.Join(SourceMountPath, filepath.Base(a.input))
	}

	javaConfig := provider.Config{
		Name:    javaProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", ports[javaProvider]),
		InitConfig: []provider.InitConfig{
			{
				Location:     SourceMountPath,
				AnalysisMode: provider.AnalysisMode(a.mode),
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 javaProvider,
					"bundles":                       JavaBundlesLocation,
					"depOpenSourceLabelsFile":       "/usr/local/etc/maven.default.index",
					provider.LspServerPathConfigKey: "/jdtls/bin/jdtls",
				},
			},
		},
	}

	if a.mavenSettingsFile != "" {
		err := copyFileContents(a.mavenSettingsFile, filepath.Join(tempDir, "settings.xml"))
		if err != nil {
			a.log.V(1).Error(err, "failed copying maven settings file", "path", a.mavenSettingsFile)
			return nil, err
		}
		javaConfig.InitConfig[0].ProviderSpecificConfig["mavenSettingsFile"] = fmt.Sprintf("%s/%s", ConfigMountPath, "settings.xml")
	}

	goConfig := provider.Config{
		Name:    goProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", ports[goProvider]),
		InitConfig: []provider.InitConfig{
			{
				AnalysisMode: provider.FullAnalysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 "generic",
					"workspaceFolders":              []string{fmt.Sprintf("file://%s", otherProvsMountPath)},
					"dependencyProviderPath":        "/usr/local/bin/golang-dependency-provider",
					provider.LspServerPathConfigKey: "/root/go/bin/gopls",
				},
			},
		},
	}

	pythonConfig := provider.Config{
		Name:    pythonProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", ports[pythonProvider]),
		InitConfig: []provider.InitConfig{
			{
				AnalysisMode: provider.SourceOnlyAnalysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 "generic",
					"workspaceFolders":              []string{fmt.Sprintf("file://%s", otherProvsMountPath)},
					provider.LspServerPathConfigKey: "/usr/local/bin/pylsp",
				},
			},
		},
	}

	nodeJSConfig := provider.Config{
		Name:    nodeJSProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", ports[nodeJSProvider]),
		InitConfig: []provider.InitConfig{
			{
				AnalysisMode: provider.SourceOnlyAnalysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 "nodejs",
					"workspaceFolders":              []string{fmt.Sprintf("file://%s", otherProvsMountPath)},
					provider.LspServerPathConfigKey: "/usr/local/bin/typescript-language-server",
				},
			},
		},
	}

	vols, dependencyFolders := a.getDepsFolders()
	if len(dependencyFolders) != 0 {
		if providers[0] == pythonProvider {
			pythonConfig.InitConfig[0].ProviderSpecificConfig["dependencyFolders"] = dependencyFolders
		}
		if providers[0] == nodeJSProvider {
			nodeJSConfig.InitConfig[0].ProviderSpecificConfig["dependencyFolders"] = dependencyFolders
		}
	}

	provConfig := []provider.Config{
		{
			Name: "builtin",
			InitConfig: []provider.InitConfig{
				{
					Location:     otherProvsMountPath,
					AnalysisMode: provider.AnalysisMode(a.mode),
				},
			},
		},
	}
	switch {
	case foundJava:
		provConfig = append(provConfig, javaConfig)
	case foundGolang && a.mode == string(provider.FullAnalysisMode):
		provConfig = append(provConfig, goConfig)
	case foundPython:
		provConfig = append(provConfig, pythonConfig)
	case foundNode:
		provConfig = append(provConfig, nodeJSConfig)
	}

	err = a.getProviderOptions(tempDir, provConfig, providers[0])
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			a.log.V(5).Info("provider options config not found, using default options")
			err := a.writeProvConfig(tempDir, provConfig)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	settingsVols := map[string]string{
		tempDir: ConfigMountPath,
	}
	if len(vols) != 0 {
		maps.Copy(settingsVols, vols)
	}

	// attempt to create a .m2 directory we can use to speed things a bit
	// this will be shared between analyze and dep command containers
	// TODO: when this is fixed on mac and windows for podman machine volume access remove this check.
	if runtime.GOOS == "linux" {
		m2Dir, err := os.MkdirTemp("", "m2-repo-")
		if err != nil {
			a.log.V(1).Error(err, "failed to create m2 repo", "dir", m2Dir)
		} else {
			settingsVols[m2Dir] = M2Dir
			a.log.V(1).Info("created directory for maven repo", "dir", m2Dir)
			a.tempDirs = append(a.tempDirs, m2Dir)
		}
	}

	return settingsVols, nil
}

func (a *analyzeCommand) getRulesVolumes() (map[string]string, error) {
	if a.rules == nil || len(a.rules) == 0 {
		return nil, nil
	}
	rulesVolumes := make(map[string]string)
	tempDir, err := os.MkdirTemp("", "analyze-rules-")
	if err != nil {
		a.log.V(1).Error(err, "failed to create temp dir", "path", tempDir)
		return nil, err
	}
	a.log.V(1).Info("created directory for rules", "dir", tempDir)
	a.tempDirs = append(a.tempDirs, tempDir)
	for i, r := range a.rules {
		stat, err := os.Stat(r)
		if err != nil {
			a.log.V(1).Error(err, "failed to stat rules", "path", r)
			return nil, err
		}
		// move rules files passed into dir to mount
		if !stat.IsDir() {
			// XML rules are handled outside of this func
			if isXMLFile(r) {
				continue
			}
			destFile := filepath.Join(tempDir, fmt.Sprintf("rules%d.yaml", i))
			err := copyFileContents(r, destFile)
			if err != nil {
				a.log.V(1).Error(err, "failed to move rules file", "src", r, "dest", destFile)
				return nil, err
			}
			a.log.V(5).Info("copied file to rule dir", "added file", r, "destFile", destFile)
			err = a.createTempRuleSet(tempDir, "custom-ruleset")
			if err != nil {
				return nil, err
			}
		} else {
			a.log.V(5).Info("coping dir", "directory", r)
			err = filepath.WalkDir(r, func(path string, d fs.DirEntry, err error) error {
				if path == r {
					return nil
				}
				if d.IsDir() {
					// This will create the new dir
					a.handleDir(path, tempDir, r)
				} else {
					// If we are unable to get the file attributes, probably safe to assume this is not a
					// valid rule or ruleset and lets skip it for now.
					if isHidden, err := hiddenfile.IsHidden(d.Name()); isHidden || err != nil {
						a.log.V(5).Info("skipping hidden file", "path", path, "error", err)
						return nil
					}
					relpath, err := filepath.Rel(r, path)
					if err != nil {
						return err
					}
					destFile := filepath.Join(tempDir, relpath)
					a.log.V(5).Info("copying file main", "source", path, "dest", destFile)
					err = copyFileContents(path, destFile)
					if err != nil {
						a.log.V(1).Error(err, "failed to move rules file", "src", r, "dest", destFile)
						return err
					}
				}
				return nil
			})
			if err != nil {
				a.log.V(1).Error(err, "failed to move rules file", "src", r)
				return nil, err
			}
		}
	}
	rulesVolumes[tempDir] = path.Join(CustomRulePath, filepath.Base(tempDir))

	return rulesVolumes, nil
}

func (a *analyzeCommand) handleDir(p string, tempDir string, basePath string) error {
	newDir, err := filepath.Rel(basePath, p)
	if err != nil {
		return err
	}
	tempDir = filepath.Join(tempDir, newDir)
	a.log.Info("creating nested tmp dir", "tempDir", tempDir, "newDir", newDir)
	err = os.Mkdir(tempDir, 0777)
	if err != nil {
		return err
	}
	a.log.V(5).Info("create temp rule set for dir", "dir", tempDir)
	err = a.createTempRuleSet(tempDir, filepath.Base(p))
	if err != nil {
		a.log.V(1).Error(err, "failed to create temp ruleset", "path", tempDir)
		return err
	}
	return err
}

func copyFileContents(src string, dst string) (err error) {
	source, err := os.Open(src)
	if err != nil {
		return nil
	}
	defer source.Close()
	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()
	_, err = io.Copy(destination, source)
	if err != nil {
		return err
	}
	return nil
}

func (a *analyzeCommand) createTempRuleSet(path string, name string) error {
	a.log.Info("creating temp ruleset ", "path", path, "name", name)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	tempRuleSet := engine.RuleSet{
		Name:        name,
		Description: "temp ruleset",
	}
	yamlData, err := yaml.Marshal(&tempRuleSet)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(path, "ruleset.yaml"), yamlData, os.ModePerm)
	if err != nil {
		return err
	}
	return nil
}

func (a *analyzeCommand) createContainerNetwork() (string, error) {
	networkName := container.RandomName()
	args := []string{
		"network",
		"create",
		networkName,
	}

	cmd := exec.Command(Settings.PodmanBinary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	a.log.V(1).Info("created container network", "network", networkName)
	// for cleanup
	a.networkName = networkName
	return networkName, nil
}

// TODO: create for each source input once accepting multiple apps is completed
func (a *analyzeCommand) createContainerVolume() (string, error) {
	volName := container.RandomName()
	input, err := filepath.Abs(a.input)
	if err != nil {
		return "", err
	}
	if a.isFileInput {
		input = filepath.Dir(input)
	}
	args := []string{
		"volume",
		"create",
		"--opt",
		"type=none",
		"--opt",
		fmt.Sprintf("device=%v", input),
		"--opt",
		"o=bind",
		volName,
	}
	cmd := exec.Command(Settings.PodmanBinary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return "", err
	}
	a.log.V(1).Info("created container volume", "volume", volName)
	// for cleanup
	a.volumeName = volName
	return volName, nil
}

func (a *analyzeCommand) retryProviderContainer(ctx context.Context, networkName string, volName string, providers []string, retry int) error {
	if retry == 0 {
		return fmt.Errorf("too many provider container retry attempts")
	}
	retry--

	_, err := a.RunProviders(ctx, networkName, volName, providers, retry)
	if err != nil {
		return fmt.Errorf("error retrying run provider %v", err)
	}
	return nil
}

func (a *analyzeCommand) RunProviders(ctx context.Context, networkName string, volName string, providers []string, retry int) (map[string]int, error) {
	providerPorts := map[string]int{}
	port, err := freeport.GetFreePort()
	if err != nil {
		return nil, err
	}
	volumes := map[string]string{
		// application source code
		volName: SourceMountPath,
	}
	vols, _ := a.getDepsFolders()
	if len(vols) != 0 {
		maps.Copy(volumes, vols)
	}

	var providerImage string
	switch providers[0] {
	case javaProvider:
		providerImage = Settings.JavaProviderImage
		providerPorts[javaProvider] = port
	case goProvider:
		providerImage = Settings.GenericProviderImage
		providerPorts[goProvider] = port
	case pythonProvider:
		providerImage = Settings.GenericProviderImage
		providerPorts[pythonProvider] = port
	case nodeJSProvider:
		providerImage = Settings.GenericProviderImage
		providerPorts[nodeJSProvider] = port
	default:
		return nil, fmt.Errorf("unable to run unsupported provider %v", providers[0])
	}
	args := []string{fmt.Sprintf("--port=%v", port)}
	a.log.Info("starting provider", "provider", providers[0])

	con := container.NewContainer()
	err = con.Run(
		ctx,
		container.WithImage(providerImage),
		container.WithLog(a.log.V(1)),
		container.WithVolumes(volumes),
		container.WithContainerToolBin(Settings.PodmanBinary),
		container.WithEntrypointArgs(args...),
		container.WithDetachedMode(true),
		container.WithCleanup(a.cleanup),
		container.WithNetwork(networkName),
	)
	if err != nil {
		err := a.retryProviderContainer(ctx, networkName, volName, providers, retry)
		if err != nil {
			return nil, err
		}
	}

	a.providerContainerNames = append(a.providerContainerNames, con.Name)
	return providerPorts, nil
}

func (a *analyzeCommand) RunAnalysisOverrideProviderSettings(ctx context.Context) error {

	volumes := map[string]string{
		// output directory
		a.output:                   OutputPath,
		a.overrideProviderSettings: ProviderSettingsMountPath,
	}

	if len(a.rules) > 0 {
		ruleVols, err := a.getRulesVolumes()
		if err != nil {
			a.log.V(1).Error(err, "failed to get rule volumes for analysis")
			return err
		}
		maps.Copy(volumes, ruleVols)
	}

	args := []string{
		fmt.Sprintf("--provider-settings=%s", ProviderSettingsMountPath),
		fmt.Sprintf("--output-file=%s", AnalysisOutputMountPath),
		fmt.Sprintf("--context-lines=%d", a.contextLines),
	}

	a.enableDefaultRulesets = false
	if a.enableDefaultRulesets {
		args = append(args,
			fmt.Sprintf("--rules=%s/", RulesetPath))
	}

	if a.incidentSelector != "" {
		args = append(args,
			fmt.Sprintf("--incident-selector=%s", a.incidentSelector))
	}

	if len(a.rules) > 0 {
		args = append(args,
			fmt.Sprintf("--rules=%s/", CustomRulePath))
	}

	if a.jaegerEndpoint != "" {
		args = append(args, "--enable-jaeger")
		args = append(args, "--jaeger-endpoint")
		args = append(args, a.jaegerEndpoint)
	}
	if !a.analyzeKnownLibraries {
		args = append(args,
			fmt.Sprintf("--dep-label-selector=(!%s=open-source)", provider.DepSourceLabel))
	}
	if a.logLevel != nil {
		args = append(args, fmt.Sprintf("--verbose=%d", *a.logLevel))
	}
	labelSelector := a.getLabelSelector()
	if labelSelector != "" {
		args = append(args, fmt.Sprintf("--label-selector=%s", labelSelector))
	}
	if a.mode == string(provider.FullAnalysisMode) {
		a.log.Info("running dependency retrieval during analysis")
		args = append(args, fmt.Sprintf("--dep-output-file=%s", DepsOutputMountPath))
	}

	analysisLogFilePath := filepath.Join(a.output, "analysis.log")
	// create log files
	analysisLog, err := os.Create(analysisLogFilePath)
	if err != nil {
		return fmt.Errorf("failed creating analysis log file at %s", analysisLogFilePath)
	}
	defer analysisLog.Close()

	a.log.Info("running source code analysis", "log", analysisLogFilePath,
		"input", a.input, "output", a.output, "args", strings.Join(args, " "), "volumes", volumes)
	a.log.Info("generating analysis log in file", "file", analysisLogFilePath)
	// TODO (pgaikwad): run analysis & deps in parallel

	c := container.NewContainer()
	err = c.Run(
		ctx,
		container.WithImage(Settings.RunnerImage),
		container.WithLog(a.log.V(1)),
		container.WithVolumes(volumes),
		container.WithStdout(analysisLog),
		container.WithStderr(analysisLog),
		container.WithEntrypointArgs(args...),
		container.WithEntrypointBin("/usr/local/bin/konveyor-analyzer"),
		container.WithNetwork("host"),
		container.WithContainerToolBin(Settings.PodmanBinary),
		container.WithCleanup(a.cleanup),
	)
	if err != nil {
		return err
	}
	err = a.getProviderLogs(ctx)
	if err != nil {
		a.log.Error(err, "failed to get provider container logs")
	}

	return nil
}

func (a *analyzeCommand) RunAnalysis(ctx context.Context, xmlOutputDir string, volName string, providers []string, ports map[string]int) error {
	volumes := map[string]string{
		// application source code
		volName: SourceMountPath,
		// output directory
		a.output: OutputPath,
	}
	var convertPath string
	if xmlOutputDir != "" {
		if !a.enableDefaultRulesets {
			convertPath = path.Join(CustomRulePath, "convert")
		} else {
			convertPath = path.Join(RulesetPath, "convert")
		}
		volumes[xmlOutputDir] = convertPath
		// for cleanup purposes
		a.tempDirs = append(a.tempDirs, xmlOutputDir)
	}

	configVols, err := a.getConfigVolumes(providers, ports)
	if err != nil {
		a.log.V(1).Error(err, "failed to get config volumes for analysis")
		return err
	}
	maps.Copy(volumes, configVols)

	if len(a.rules) > 0 {
		ruleVols, err := a.getRulesVolumes()
		if err != nil {
			a.log.V(1).Error(err, "failed to get rule volumes for analysis")
			return err
		}
		maps.Copy(volumes, ruleVols)
	}

	args := []string{
		fmt.Sprintf("--provider-settings=%s", ProviderSettingsMountPath),
		fmt.Sprintf("--output-file=%s", AnalysisOutputMountPath),
		fmt.Sprintf("--context-lines=%d", a.contextLines),
	}
	// TODO update for running multiple apps
	if providers[0] != javaProvider {
		a.enableDefaultRulesets = false
	}

	if a.enableDefaultRulesets {
		args = append(args,
			fmt.Sprintf("--rules=%s/", RulesetPath))
	}

	if a.incidentSelector != "" {
		args = append(args,
			fmt.Sprintf("--incident-selector=%s", a.incidentSelector))
	}

	if len(a.rules) > 0 {
		args = append(args,
			fmt.Sprintf("--rules=%s/", CustomRulePath))
	}

	if a.jaegerEndpoint != "" {
		args = append(args, "--enable-jaeger")
		args = append(args, "--jaeger-endpoint")
		args = append(args, a.jaegerEndpoint)
	}

	// python and node providers do not yet support dep analysis
	if !a.analyzeKnownLibraries && (providers[0] != pythonProvider && providers[0] != nodeJSProvider) {
		args = append(args,
			fmt.Sprintf("--dep-label-selector=(!%s=open-source)", provider.DepSourceLabel))
	}
	if a.logLevel != nil {
		args = append(args, fmt.Sprintf("--verbose=%d", *a.logLevel))
	}
	labelSelector := a.getLabelSelector()
	if labelSelector != "" {
		args = append(args, fmt.Sprintf("--label-selector=%s", labelSelector))
	}

	switch true {
	case a.mode == string(provider.FullAnalysisMode) && providers[0] == pythonProvider:
		a.mode = string(provider.SourceOnlyAnalysisMode)
	case a.mode == string(provider.FullAnalysisMode) && providers[0] == nodeJSProvider:
		a.mode = string(provider.SourceOnlyAnalysisMode)
	default:
		a.log.Info("running dependency retrieval during analysis")
		args = append(args, fmt.Sprintf("--dep-output-file=%s", DepsOutputMountPath))
	}

	analysisLogFilePath := filepath.Join(a.output, "analysis.log")
	// create log files
	analysisLog, err := os.Create(analysisLogFilePath)
	if err != nil {
		return fmt.Errorf("failed creating analysis log file at %s", analysisLogFilePath)
	}
	defer analysisLog.Close()

	a.log.Info("running source code analysis", "log", analysisLogFilePath,
		"input", a.input, "output", a.output, "args", strings.Join(args, " "), "volumes", volumes)
	a.log.Info("generating analysis log in file", "file", analysisLogFilePath)
	// TODO (pgaikwad): run analysis & deps in parallel

	c := container.NewContainer()
	err = c.Run(
		ctx,
		container.WithImage(Settings.RunnerImage),
		container.WithLog(a.log.V(1)),
		container.WithVolumes(volumes),
		container.WithStdout(analysisLog),
		container.WithStderr(analysisLog),
		container.WithEntrypointArgs(args...),
		container.WithEntrypointBin("/usr/local/bin/konveyor-analyzer"),
		container.WithNetwork(fmt.Sprintf("container:%v", a.providerContainerNames[0])),
		container.WithContainerToolBin(Settings.PodmanBinary),
		container.WithCleanup(a.cleanup),
	)
	if err != nil {
		return err
	}
	err = a.getProviderLogs(ctx)
	if err != nil {
		a.log.Error(err, "failed to get provider container logs")
	}

	return nil
}

func (a *analyzeCommand) CreateJSONOutput() error {
	if !a.jsonOutput {
		return nil
	}
	outputPath := filepath.Join(a.output, "output.yaml")
	depPath := filepath.Join(a.output, "dependencies.yaml")

	data, err := os.ReadFile(outputPath)
	if err != nil {
		return err
	}
	ruleOutput := &[]outputv1.RuleSet{}
	err = yaml.Unmarshal(data, ruleOutput)
	if err != nil {
		a.log.V(1).Error(err, "failed to unmarshal output yaml")
		return err
	}

	jsonData, err := json.MarshalIndent(ruleOutput, "", "	")
	if err != nil {
		a.log.V(1).Error(err, "failed to marshal output file to json")
		return err
	}
	err = os.WriteFile(filepath.Join(a.output, "output.json"), jsonData, os.ModePerm)
	if err != nil {
		a.log.V(1).Error(err, "failed to write json output", "dir", a.output, "file", "output.json")
		return err
	}

	depData, err := os.ReadFile(depPath)
	if err != nil {
		return err
	}
	depOutput := &[]outputv1.DepsFlatItem{}
	err = yaml.Unmarshal(depData, depOutput)
	if err != nil {
		a.log.V(1).Error(err, "failed to unmarshal dependencies yaml")
		return err
	}

	jsonDataDep, err := json.MarshalIndent(depOutput, "", "	")
	if err != nil {
		a.log.V(1).Error(err, "failed to marshal dependencies file to json")
		return err
	}
	err = os.WriteFile(filepath.Join(a.output, "dependencies.json"), jsonDataDep, os.ModePerm)
	if err != nil {
		a.log.V(1).Error(err, "failed to write json dependencies output", "dir", a.output, "file", "dependencies.json")
		return err
	}

	return nil
}

func (a *analyzeCommand) GenerateStaticReport(ctx context.Context) error {
	if a.skipStaticReport {
		return nil
	}
	volumes := map[string]string{
		a.input:  SourceMountPath,
		a.output: OutputPath,
	}

	// Prepare report args list with single input analysis
	applicationNames := []string{filepath.Base(a.input)}
	outputAnalyses := []string{AnalysisOutputMountPath}

	if a.bulk {
		a.moveResults()
		// Scan all available analysis output files to be reported
		applicationNames = nil
		outputAnalyses = nil
		outputFiles, err := filepath.Glob(fmt.Sprintf("%s/output.yaml.*", a.output))
		if err != nil {
			return err
		}
		for i := range outputFiles {
			outputName := filepath.Base(outputFiles[i])
			applicationNames = append(applicationNames, strings.SplitN(outputName, "output.yaml.", 2)[1])
			outputAnalyses = append(outputAnalyses, strings.ReplaceAll(outputFiles[i], a.output, OutputPath)) // re-map paths to container mounts
		}
	}

	args := []string{}
	staticReportArgs := []string{"/usr/local/bin/js-bundle-generator",
		fmt.Sprintf("--output-path=%s", path.Join("/usr/local/static-report/output.js")),
		fmt.Sprintf("--analysis-output-list=%s", strings.Join(outputAnalyses, ",")),
		fmt.Sprintf("--application-name-list=%s", strings.Join(applicationNames, ",")),
	}
	if a.mode == string(provider.FullAnalysisMode) {
		staticReportArgs = append(staticReportArgs,
			fmt.Sprintf("--deps-output-list=%s", DepsOutputMountPath))
	}
	cpArgs := []string{"&& cp -r",
		"/usr/local/static-report", OutputPath}

	args = append(args, staticReportArgs...)
	args = append(args, cpArgs...)

	joinedArgs := strings.Join(args, " ")
	staticReportCmd := []string{joinedArgs}

	c := container.NewContainer()
	a.log.Info("generating static report",
		"output", a.output, "args", strings.Join(staticReportCmd, " "))
	err := c.Run(
		ctx,
		container.WithImage(Settings.RunnerImage),
		container.WithLog(a.log.V(1)),
		container.WithEntrypointBin("/bin/sh"),
		container.WithContainerToolBin(Settings.PodmanBinary),
		container.WithEntrypointArgs(staticReportCmd...),
		container.WithVolumes(volumes),
		container.WithcFlag(true),
		container.WithCleanup(a.cleanup),
	)
	if err != nil {
		return err
	}
	uri := uri.File(filepath.Join(a.output, "static-report", "index.html"))
	cleanedURI := filepath.Clean(string(uri))
	a.log.Info("Static report created. Access it at this URL:", "URL", cleanedURI)

	return nil
}

func (a *analyzeCommand) moveResults() error {
	outputPath := filepath.Join(a.output, "output.yaml")
	analysisLogFilePath := filepath.Join(a.output, "analysis.log")
	depsPath := filepath.Join(a.output, "dependencies.yaml")
	err := copyFileContents(outputPath, fmt.Sprintf("%s.%s", outputPath, a.inputShortName()))
	if err != nil {
		return err
	}
	err = os.Remove(outputPath);
	if err != nil {
		return err
	}
	err = copyFileContents(analysisLogFilePath, fmt.Sprintf("%s.%s", analysisLogFilePath, a.inputShortName()))
	if err != nil {
		return err
	}
	err = os.Remove(analysisLogFilePath);
	if err != nil {
		return err
	}
	err = copyFileContents(depsPath, fmt.Sprintf("%s.%s", analysisLogFilePath, a.inputShortName()))
	if err == nil {	// dependencies file presence is optional
		err = os.Remove(depsPath);
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *analyzeCommand) inputShortName() string {
	return filepath.Base(a.input)
}

func (a *analyzeCommand) getLabelSelector() string {
	if a.labelSelector != "" {
		return a.labelSelector
	}
	if (a.sources == nil || len(a.sources) == 0) &&
		(a.targets == nil || len(a.targets) == 0) {
		return ""
	}
	// default labels are applied everytime either a source or target is specified
	defaultLabels := []string{"discovery"}
	targets := []string{}
	for _, target := range a.targets {
		targets = append(targets,
			fmt.Sprintf("%s=%s", outputv1.TargetTechnologyLabel, target))
	}
	sources := []string{}
	for _, source := range a.sources {
		sources = append(sources,
			fmt.Sprintf("%s=%s", outputv1.SourceTechnologyLabel, source))
	}
	targetExpr := ""
	if len(targets) > 0 {
		targetExpr = fmt.Sprintf("(%s)", strings.Join(targets, " || "))
	}
	sourceExpr := ""
	if len(sources) > 0 {
		sourceExpr = fmt.Sprintf("(%s)", strings.Join(sources, " || "))
	}
	if targetExpr != "" {
		if sourceExpr != "" {
			// when both targets and sources are present, AND them
			return fmt.Sprintf("(%s && %s) || (%s)",
				targetExpr, sourceExpr, strings.Join(defaultLabels, " || "))
		} else {
			// when target is specified, but source is not
			// use a catch-all expression for source
			return fmt.Sprintf("(%s && %s) || (%s)",
				targetExpr, outputv1.SourceTechnologyLabel, strings.Join(defaultLabels, " || "))
		}
	}
	if sourceExpr != "" {
		// when only source is specified, OR them all
		return fmt.Sprintf("%s || (%s)",
			sourceExpr, strings.Join(defaultLabels, " || "))
	}
	return ""
}

func isXMLFile(rule string) bool {
	return path.Ext(rule) == ".xml"
}

func loadEnvInsensitive(variableName string) string {
	lowerValue := os.Getenv(strings.ToLower(variableName))
	upperValue := os.Getenv(strings.ToUpper(variableName))
	if lowerValue != "" {
		return lowerValue
	} else {
		return upperValue
	}
}

func (a *analyzeCommand) getXMLRulesVolumes(tempRuleDir string) (map[string]string, error) {
	rulesVolumes := make(map[string]string)
	mountTempDir := false
	for _, r := range a.rules {
		stat, err := os.Stat(r)
		if err != nil {
			a.log.V(1).Error(err, "failed to stat rules")
			return nil, err
		}
		// move xml rule files from user into dir to mount
		if !stat.IsDir() {
			if !isXMLFile(r) {
				continue
			}
			mountTempDir = true
			xmlFileName := filepath.Base(r)
			destFile := filepath.Join(tempRuleDir, xmlFileName)
			err := copyFileContents(r, destFile)
			if err != nil {
				a.log.V(1).Error(err, "failed to move rules file from source to destination", "src", r, "dest", destFile)
				return nil, err
			}
		} else {
			rulesVolumes[r] = path.Join(XMLRulePath, filepath.Base(r))
		}
	}
	if mountTempDir {
		rulesVolumes[tempRuleDir] = XMLRulePath
	}
	return rulesVolumes, nil
}

func (a *analyzeCommand) ConvertXML(ctx context.Context) (string, error) {
	if a.rules == nil || len(a.rules) == 0 {
		return "", nil
	}
	tempDir, err := os.MkdirTemp("", "transform-rules-")
	if err != nil {
		a.log.V(1).Error(err, "failed to create temp dir for rules")
		return "", err
	}
	a.log.V(1).Info("created directory for XML rules", "dir", tempDir)
	tempOutputDir, err := os.MkdirTemp("", "transform-output-")
	if err != nil {
		a.log.V(1).Error(err, "failed to create temp dir for rules")
		return "", err
	}
	a.log.V(1).Info("created directory for converted XML rules", "dir", tempOutputDir)
	if a.cleanup {
		defer os.RemoveAll(tempDir)
	}
	volumes := map[string]string{
		tempOutputDir: ShimOutputPath,
	}

	ruleVols, err := a.getXMLRulesVolumes(tempDir)
	if err != nil {
		a.log.V(1).Error(err, "failed to get XML rule volumes for analysis")
		return "", err
	}
	maps.Copy(volumes, ruleVols)

	shimLogPath := filepath.Join(a.output, "shim.log")
	shimLog, err := os.Create(shimLogPath)
	if err != nil {
		return "", fmt.Errorf("failed creating shim log file %s", shimLogPath)
	}
	defer shimLog.Close()

	args := []string{"convert",
		fmt.Sprintf("--outputdir=%v", ShimOutputPath),
		XMLRulePath,
	}
	a.log.Info("running windup shim",
		"output", a.output, "args", strings.Join(args, " "), "volumes", volumes)
	a.log.Info("generating shim log in file", "file", shimLogPath)
	err = container.NewContainer().Run(
		ctx,
		container.WithImage(Settings.RunnerImage),
		container.WithLog(a.log.V(1)),
		container.WithStdout(shimLog),
		container.WithStderr(shimLog),
		container.WithVolumes(volumes),
		container.WithEntrypointArgs(args...),
		container.WithEntrypointBin("/usr/local/bin/windup-shim"),
		container.WithContainerToolBin(Settings.PodmanBinary),
		container.WithCleanup(a.cleanup),
	)
	if err != nil {
		return "", err
	}

	return tempOutputDir, nil
}

func (a *analyzeCommand) writeProvConfig(tempDir string, config []provider.Config) error {
	jsonData, err := json.MarshalIndent(&config, "", "	")
	if err != nil {
		a.log.V(1).Error(err, "failed to marshal provider config")
		return err
	}
	err = os.WriteFile(filepath.Join(tempDir, "settings.json"), jsonData, os.ModePerm)
	if err != nil {
		a.log.V(1).Error(err,
			"failed to write provider config", "dir", tempDir, "file", "settings.json")
		return err
	}
	return nil
}

func (a *analyzeCommand) getProviderOptions(tempDir string, provConfig []provider.Config, prov string) error {
	var confDir string
	var set bool
	ops := runtime.GOOS
	if ops == "linux" {
		confDir, set = os.LookupEnv("XDG_CONFIG_HOME")
	}
	if ops != "linux" || confDir == "" || !set {
		// on Unix, including macOS, this returns the $HOME environment variable. On Windows, it returns %USERPROFILE%
		var err error
		confDir, err = os.UserHomeDir()
		if err != nil {
			return err
		}
	}
	// get provider options from provider settings file
	data, err := os.ReadFile(filepath.Join(confDir, ".kantra", fmt.Sprintf("%v.json", prov)))
	if err != nil {
		return err
	}
	optionsConfig := &[]provider.Config{}
	err = yaml.Unmarshal(data, optionsConfig)
	if err != nil {
		a.log.V(1).Error(err, "failed to unmarshal provider options file")
		return err
	}
	mergedConfig, err := a.mergeProviderConfig(provConfig, *optionsConfig, tempDir)
	if err != nil {
		return err
	}
	err = a.writeProvConfig(tempDir, mergedConfig)
	if err != nil {
		return err
	}
	return nil
}

func (a *analyzeCommand) mergeProviderConfig(defaultConf, optionsConf []provider.Config, tempDir string) ([]provider.Config, error) {
	merged := []provider.Config{}
	seen := map[string]*provider.Config{}

	// find options used for supported providers
	for idx, conf := range defaultConf {
		seen[conf.Name] = &defaultConf[idx]
	}

	for _, conf := range optionsConf {
		if _, ok := seen[conf.Name]; ok {
			// set provider config options
			if conf.ContextLines != 0 {
				seen[conf.Name].ContextLines = conf.ContextLines
			}
			if conf.Proxy != nil {
				seen[conf.Name].Proxy = conf.Proxy
			}
		}
		// set init config options
		for i, init := range conf.InitConfig {
			if len(init.AnalysisMode) != 0 {
				seen[conf.Name].InitConfig[i].AnalysisMode = init.AnalysisMode
			}
			if len(init.ProviderSpecificConfig) != 0 {
				provSpecificConf, err := a.mergeProviderSpecificConfig(init.ProviderSpecificConfig, seen[conf.Name].InitConfig[i].ProviderSpecificConfig, tempDir)
				if err != nil {
					return nil, err
				}
				seen[conf.Name].InitConfig[i].ProviderSpecificConfig = provSpecificConf
			}
		}
	}
	for _, v := range seen {
		merged = append(merged, *v)
	}
	return merged, nil
}

func (a *analyzeCommand) mergeProviderSpecificConfig(optionsConf, seenConf map[string]interface{}, tempDir string) (map[string]interface{}, error) {
	for k, v := range optionsConf {
		switch {
		case optionsConf[k] == "":
			continue
		// special case for maven settings file to mount correctly
		case k == mavenSettingsFile:
			// validate maven settings file
			if _, err := os.Stat(v.(string)); err != nil {
				return nil, fmt.Errorf("%w failed to stat maven settings file at path %s", err, v)
			}
			if absPath, err := filepath.Abs(v.(string)); err == nil {
				seenConf[k] = absPath
			}
			// copy file to mount path
			err := copyFileContents(v.(string), filepath.Join(tempDir, "settings.xml"))
			if err != nil {
				a.log.V(1).Error(err, "failed copying maven settings file", "path", v)
				return nil, err
			}
			seenConf[k] = fmt.Sprintf("%s/%s", ConfigMountPath, "settings.xml")
			continue
		// we don't want users to override these options here
		// use --overrideProviderSettings to do so
		case k != lspServerPath && k != lspServerName && k != workspaceFolders && k != dependencyProviderPath:
			seenConf[k] = v
		}
	}
	return seenConf, nil
}

func (a *analyzeCommand) CleanAnalysisResources(ctx context.Context) error {
	if !a.cleanup {
		return nil
	}
	a.log.V(1).Info("removing temp dirs")
	for _, path := range a.tempDirs {
		err := os.RemoveAll(path)
		if err != nil {
			a.log.V(1).Error(err, "failed to delete temporary dir", "dir", path)
			continue
		}
	}
	err := a.RmProviderContainers(ctx)
	if err != nil {
		a.log.Error(err, "failed to remove provider container")
	}
	err = a.RmNetwork(ctx)
	if err != nil {
		a.log.Error(err, "failed to remove network", "network", a.networkName)
	}
	err = a.RmVolumes(ctx)
	if err != nil {
		a.log.Error(err, "failed to remove volume", "volume", a.volumeName)
	}
	return nil
}

func (a *analyzeCommand) RmNetwork(ctx context.Context) error {
	cmd := exec.CommandContext(
		ctx,
		Settings.PodmanBinary,
		"network",
		"rm", a.networkName)
	a.log.V(1).Info("removing container network",
		"network", a.networkName)
	return cmd.Run()
}

func (a *analyzeCommand) RmVolumes(ctx context.Context) error {
	cmd := exec.CommandContext(
		ctx,
		Settings.PodmanBinary,
		"volume",
		"rm", a.volumeName)
	a.log.V(1).Info("removing created volume",
		"volume", a.volumeName)
	return cmd.Run()
}

// TODO: multiple provider containers
func (a *analyzeCommand) RmProviderContainers(ctx context.Context) error {
	for i := range a.providerContainerNames {
		con := a.providerContainerNames[i]
		// because we are using the --rm option when we start the provider container,
		// it will immediately be removed after it stops
		cmd := exec.CommandContext(
			ctx,
			Settings.PodmanBinary,
			"stop", con)
		a.log.V(1).Info("stopping container", "container", con)
		err := cmd.Run()
		if err != nil {
			a.log.V(1).Error(err, "failed to stop container",
				"container", con)
			continue
		}
	}
	return nil
}

// TODO multiple providers
func (a *analyzeCommand) getProviderLogs(ctx context.Context) error {
	if len(a.providerContainerNames) == 0 {
		return nil
	}
	providerLogFilePath := filepath.Join(a.output, "provider.log")
	providerLog, err := os.Create(providerLogFilePath)
	if err != nil {
		return fmt.Errorf("failed creating provider log file at %s", providerLogFilePath)
	}
	defer providerLog.Close()
	a.log.V(1).Info("getting provider container logs",
		"container", a.providerContainerNames[0])

	cmd := exec.CommandContext(
		ctx,
		Settings.PodmanBinary,
		"logs",
		a.providerContainerNames[0])

	cmd.Stdout = providerLog
	cmd.Stderr = providerLog
	return cmd.Run()
}
