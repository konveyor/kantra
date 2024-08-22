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
	"slices"
	"sort"
	"strings"

	"github.com/devfile/alizer/pkg/apis/model"
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
	DotnetFrameworks          = map[string]bool{
		"v1.0":   false,
		"v1.1":   false,
		"v2.0":   false,
		"v3.0":   false,
		"v3.5":   false,
		"v4":     false,
		"v4.5":   true,
		"v4.5.1": true,
		"v4.5.2": true,
		"v4.6":   true,
		"v4.6.1": true,
		"v4.6.2": true,
		"v4.7":   true,
		"v4.7.1": true,
		"v4.7.2": true,
		"v4.8":   true,
		"v4.8.1": true,
	}
)

// supported providers
const (
	javaProvider            = "java"
	goProvider              = "go"
	pythonProvider          = "python"
	nodeJSProvider          = "nodejs"
	dotnetProvider          = "dotnet"
	dotnetFrameworkProvider = "dotnetframework"
)

// valid java file extensions
const (
	JavaArchive       = ".jar"
	WebArchive        = ".war"
	EnterpriseArchive = ".ear"
	ClassFile         = ".class"
)

// provider config options
const (
	mavenSettingsFile      = "mavenSettingsFile"
	lspServerPath          = "lspServerPath"
	lspServerName          = "lspServerName"
	workspaceFolders       = "workspaceFolders"
	dependencyProviderPath = "dependencyProviderPath"
)

// TODO add network and volume w/ interface
type ProviderInit struct {
	port  int
	image string
	// used for failed provider container retry attempts
	isRunning     bool
	containerName string
}

// kantra analyze flags
type analyzeCommand struct {
	listSources              bool
	listTargets              bool
	listProviders            bool
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
	provider                 []string
	providersMap             map[string]ProviderInit

	// tempDirs list of temporary dirs created, used for cleanup
	tempDirs []string
	log      logr.Logger
	// isFileInput is set when input points to a file and not a dir
	isFileInput  bool
	needsBuiltin bool
	logLevel     *uint32
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
				!cmd.Flags().Lookup("list-targets").Changed &&
				!cmd.Flags().Lookup("list-providers").Changed {
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
				if analyzeCmd.listProviders {
					err := analyzeCmd.ListSupportedProviders(cmd.Context())
					if err != nil {
						log.Error(err, "failed to list providers")
						return err
					}
					return nil
				}
				if analyzeCmd.providersMap == nil {
					analyzeCmd.providersMap = make(map[string]ProviderInit)
				}
				languages, err := recognizer.Analyze(analyzeCmd.input)
				if err != nil {
					log.Error(err, "Failed to determine languages for input")
					return err
				}
				foundProviders := []string{}
				// file input means a binary was given which only the java provider can use
				if analyzeCmd.isFileInput {
					foundProviders = append(foundProviders, javaProvider)
				} else {
					foundProviders, err = analyzeCmd.setProviders(languages, foundProviders)
					if err != nil {
						log.Error(err, "failed to set provider info")
						return err
					}
					err = analyzeCmd.validateProviders(foundProviders)
					if err != nil {
						return err
					}
				}
				if len(foundProviders) == 1 && foundProviders[0] == dotnetFrameworkProvider {
					return analyzeCmd.analyzeDotnetFramework(cmd.Context())
				}

				// default rulesets are only java rules
				// may want to change this in the future
				if len(foundProviders) > 0 && len(analyzeCmd.rules) == 0 && !slices.Contains(foundProviders, javaProvider) {
					return fmt.Errorf("No providers found with default rules. Use --rules option")
				}

				xmlOutputDir, err := analyzeCmd.ConvertXML(cmd.Context())
				if err != nil {
					log.Error(err, "failed to convert xml rules")
					return err
				}
				// alizer does not detect certain files such as xml
				// in this case we need to run only the analyzer to use builtin provider
				if len(foundProviders) == 0 {
					analyzeCmd.needsBuiltin = true
					return analyzeCmd.RunAnalysis(cmd.Context(), xmlOutputDir, analyzeCmd.input)
				}

				err = analyzeCmd.setProviderInitInfo(foundProviders)
				if err != nil {
					log.Error(err, "failed to set provider init info")
					return err
				}
				// defer cleaning created resources here instead of PostRun
				// if Run returns an error, PostRun does not run
				defer func() {
					if err := analyzeCmd.CleanAnalysisResources(cmd.Context()); err != nil {
						log.Error(err, "failed to clean temporary directories")
					}
				}()
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
				// allow for 5 retries of running provider in the case of port in use
				err = analyzeCmd.RunProviders(cmd.Context(), containerNetworkName, containerVolName, 5)
				if err != nil {
					log.Error(err, "failed to run provider")
					return err
				}
				err = analyzeCmd.RunAnalysis(cmd.Context(), xmlOutputDir, containerVolName)
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
	analyzeCommand.Flags().BoolVar(&analyzeCmd.listProviders, "list-providers", false, "list available supported providers")
	analyzeCommand.Flags().StringArrayVarP(&analyzeCmd.sources, "source", "s", []string{}, "source technology to consider for analysis. Use multiple times for additional sources: --source <source1> --source <source2> ...")
	analyzeCommand.Flags().StringArrayVarP(&analyzeCmd.targets, "target", "t", []string{}, "target technology to consider for analysis. Use multiple times for additional targets: --target <target1> --target <target2> ...")
	analyzeCommand.Flags().StringVarP(&analyzeCmd.labelSelector, "label-selector", "l", "", "run rules based on specified label selector expression")
	analyzeCommand.Flags().StringArrayVar(&analyzeCmd.rules, "rules", []string{}, "filename or directory containing rule files. Use multiple times for additional rules: --rules <rule1> --rules <rule2> ...")
	analyzeCommand.Flags().StringVarP(&analyzeCmd.input, "input", "i", "", "path to application source code or a binary")
	analyzeCommand.Flags().StringVarP(&analyzeCmd.output, "output", "o", "", "path to the directory for analysis output")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.skipStaticReport, "skip-static-report", false, "do not generate static report")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.analyzeKnownLibraries, "analyze-known-libraries", false, "analyze known open-source libraries")
	analyzeCommand.Flags().StringVar(&analyzeCmd.mavenSettingsFile, "maven-settings", "", "path to a custom maven settings file to use")
	analyzeCommand.Flags().StringVarP(&analyzeCmd.mode, "mode", "m", string(provider.FullAnalysisMode), "analysis mode. Must be one of 'full' (source + dependencies) or 'source-only'")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.jsonOutput, "json-output", false, "create analysis and dependency output as json")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.overwrite, "overwrite", false, "overwrite output directory")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.bulk, "bulk", false, "running multiple analyze commands in bulk will result to combined static report")
	analyzeCommand.Flags().StringVar(&analyzeCmd.jaegerEndpoint, "jaeger-endpoint", "", "jaeger endpoint to collect traces")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.enableDefaultRulesets, "enable-default-rulesets", true, "run default rulesets with analysis")
	analyzeCommand.Flags().IntVar(&analyzeCmd.contextLines, "context-lines", 100, "number of lines of source code to include in the output for each incident")
	analyzeCommand.Flags().StringVar(&analyzeCmd.incidentSelector, "incident-selector", "", "an expression to select incidents based on custom variables. ex: (!package=io.konveyor.demo.config-utils)")
	analyzeCommand.Flags().StringArrayVarP(&analyzeCmd.depFolders, "dependency-folders", "d", []string{}, "directory for dependencies")
	analyzeCommand.Flags().StringVar(&analyzeCmd.overrideProviderSettings, "override-provider-settings", "", "override the provider settings, the analysis pod will be run on the host network and no providers with be started up")
	analyzeCommand.Flags().StringArrayVar(&analyzeCmd.provider, "provider", []string{}, "specify which provider(s) to run")

	return analyzeCommand
}

func (a *analyzeCommand) Validate(ctx context.Context) error {
	if a.listSources || a.listTargets || a.listProviders {
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
			// validate file types
			fileExt := filepath.Ext(a.input)
			switch fileExt {
			case JavaArchive, WebArchive, EnterpriseArchive, ClassFile:
				a.log.V(5).Info("valid java file found")
			default:
				return fmt.Errorf("invalid file type %v", fileExt)
			}
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

func (a *analyzeCommand) setProviders(languages []model.Language, foundProviders []string) ([]string, error) {
	if len(a.provider) > 0 {
		for _, p := range a.provider {
			foundProviders = append(foundProviders, p)
			return foundProviders, nil
		}
	}
	for _, l := range languages {
		if l.CanBeComponent {
			a.log.V(5).Info("Got language", "component language", l)
			if l.Name == "C#" {
				for _, item := range l.Frameworks {
					supported, ok := DotnetFrameworks[item]
					if ok {
						if !supported {
							err := fmt.Errorf("Unsupported .NET Framework version")
							a.log.Error(err, ".NET Framework version must be greater or equal 'v4.5'")
							return foundProviders, err
						}
						return []string{dotnetFrameworkProvider}, nil
					}
				}
				foundProviders = append(foundProviders, dotnetProvider)
				continue
			}
			if l.Name == "JavaScript" {
				for _, item := range l.Tools {
					if item == "NodeJs" || item == "Node.js" || item == "nodejs" {
						foundProviders = append(foundProviders, nodeJSProvider)
						// only need one instance of provider
						break
					}
				}
			} else {
				foundProviders = append(foundProviders, strings.ToLower(l.Name))
			}
		}
	}
	return foundProviders, nil
}

func (a *analyzeCommand) setProviderInitInfo(foundProviders []string) error {
	for _, prov := range foundProviders {
		port, err := freeport.GetFreePort()
		if err != nil {
			return err
		}
		switch prov {
		case javaProvider:
			a.providersMap[javaProvider] = ProviderInit{
				port:  port,
				image: Settings.JavaProviderImage,
			}
		case goProvider:
			a.providersMap[goProvider] = ProviderInit{
				port:  port,
				image: Settings.GenericProviderImage,
			}
		case pythonProvider:
			a.providersMap[pythonProvider] = ProviderInit{
				port:  port,
				image: Settings.GenericProviderImage,
			}
		case nodeJSProvider:
			a.providersMap[nodeJSProvider] = ProviderInit{
				port:  port,
				image: Settings.GenericProviderImage,
			}
		case dotnetProvider:
			a.providersMap[dotnetProvider] = ProviderInit{
				port:  port,
				image: Settings.DotnetProviderImage,
			}
		}
	}
	return nil
}

func (a *analyzeCommand) validateProviders(providers []string) error {
	validProvs := []string{
		javaProvider,
		pythonProvider,
		goProvider,
		nodeJSProvider,
		dotnetProvider,
		dotnetFrameworkProvider,
	}
	for _, prov := range providers {
		//validate other providers
		if !slices.Contains(validProvs, prov) {
			return fmt.Errorf("provider %v not supported. Use --providerOverride or --provider option", prov)
		}
	}
	return nil
}

func (a *analyzeCommand) ListSupportedProviders(ctx context.Context) error {
	return a.fetchProviders(ctx, os.Stdout)
}

func (a *analyzeCommand) fetchProviders(ctx context.Context, out io.Writer) error {
	runMode := "RUN_MODE"
	runModeContainer := "container"
	if os.Getenv(runMode) == runModeContainer {
		a.listAllProviders(out)
		return nil
	} else {
		args := []string{"analyze",
			"--list-providers",
		}
		err := container.NewContainer().Run(
			ctx,
			container.WithImage(Settings.RunnerImage),
			container.WithLog(a.log.V(1)),
			container.WithEnv(runMode, runModeContainer),
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

func (a *analyzeCommand) listAllProviders(out io.Writer) {
	supportedProvs := []string{
		"java",
		"python",
		"go",
		"dotnet",
		"nodejs",
	}
	fmt.Fprintln(out, "\navailable supported providers:")
	for _, prov := range supportedProvs {
		fmt.Fprintln(out, prov)
	}
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

func (a *analyzeCommand) getConfigVolumes() (map[string]string, error) {
	tempDir, err := os.MkdirTemp("", "analyze-config-")
	if err != nil {
		a.log.V(1).Error(err, "failed creating temp dir", "dir", tempDir)
		return nil, err
	}
	a.log.V(1).Info("created directory for provider settings", "dir", tempDir)
	a.tempDirs = append(a.tempDirs, tempDir)

	otherProvsMountPath := SourceMountPath
	// when input is a file, it means it's probably a binary
	// only java provider can work with binaries, all others
	// continue pointing to the directory instead of file
	if a.isFileInput {
		SourceMountPath = path.Join(SourceMountPath, filepath.Base(a.input))
	}

	javaConfig := provider.Config{
		Name:    javaProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", a.providersMap[javaProvider].port),
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
		Address: fmt.Sprintf("0.0.0.0:%v", a.providersMap[goProvider].port),
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
		Address: fmt.Sprintf("0.0.0.0:%v", a.providersMap[pythonProvider].port),
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
		Address: fmt.Sprintf("0.0.0.0:%v", a.providersMap[nodeJSProvider].port),
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

	dotnetConfig := provider.Config{
		Name:    dotnetProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", a.providersMap[dotnetProvider].port),
		InitConfig: []provider.InitConfig{
			{
				Location:     SourceMountPath,
				AnalysisMode: provider.SourceOnlyAnalysisMode,
				ProviderSpecificConfig: map[string]interface{}{
					provider.LspServerPathConfigKey: "/opt/app-root/.dotnet/tools/csharp-ls",
				},
			},
		},
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

	settingsVols := map[string]string{
		tempDir: ConfigMountPath,
	}
	if !a.needsBuiltin {
		vols, dependencyFolders := a.getDepsFolders()
		if len(vols) != 0 {
			maps.Copy(settingsVols, vols)
		}
		for prov, _ := range a.providersMap {
			switch prov {
			case javaProvider:
				provConfig = append(provConfig, javaConfig)
			case goProvider:
				provConfig = append(provConfig, goConfig)
			case pythonProvider:
				if len(dependencyFolders) != 0 {
					pythonConfig.InitConfig[0].ProviderSpecificConfig["dependencyFolders"] = dependencyFolders
				}
				provConfig = append(provConfig, pythonConfig)
			case nodeJSProvider:
				if len(dependencyFolders) != 0 {
					nodeJSConfig.InitConfig[0].ProviderSpecificConfig["dependencyFolders"] = dependencyFolders
				}
				provConfig = append(provConfig, nodeJSConfig)
			case dotnetProvider:
				provConfig = append(provConfig, dotnetConfig)
			}
		}
		for prov, _ := range a.providersMap {
			err = a.getProviderOptions(tempDir, provConfig, prov)
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
		}
	}
	err = a.writeProvConfig(tempDir, provConfig)
	if err != nil {
		return nil, err
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
	if runtime.GOOS == "windows" {
		// TODO(djzager): Thank ChatGPT
		// Extract the volume name (e.g., "C:")
		// Remove the volume name from the path to get the remaining part
		// Convert backslashes to forward slashes
		// Remove the colon from the volume name and convert to lowercase
		volumeName := filepath.VolumeName(input)
		remainingPath := input[len(volumeName):]
		remainingPath = filepath.ToSlash(remainingPath)
		driveLetter := strings.ToLower(strings.TrimSuffix(volumeName, ":"))

		// Construct the Linux-style path
		input = fmt.Sprintf("/mnt/%s%s", driveLetter, remainingPath)
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

func (a *analyzeCommand) retryProviderContainer(ctx context.Context, networkName string, volName string, retry int) error {
	if retry == 0 {
		return fmt.Errorf("too many provider container retry attempts")
	}
	retry--

	err := a.RunProviders(ctx, networkName, volName, retry)
	if err != nil {
		return fmt.Errorf("error retrying run provider %v", err)
	}
	return nil
}

func (a *analyzeCommand) RunProviders(ctx context.Context, networkName string, volName string, retry int) error {
	volumes := map[string]string{
		// application source code
		volName: SourceMountPath,
	}
	vols, _ := a.getDepsFolders()
	if len(vols) != 0 {
		maps.Copy(volumes, vols)
	}
	firstProvRun := false
	for prov, init := range a.providersMap {
		// if retrying provider, skip providers already running
		if init.isRunning {
			continue
		}
		args := []string{fmt.Sprintf("--port=%v", init.port)}
		// we have to start the fist provider separately to create the shared
		// container network to then add other providers to the network
		if !firstProvRun {
			a.log.Info("starting first provider", "provider", prov)
			con := container.NewContainer()
			err := con.Run(
				ctx,
				container.WithImage(init.image),
				container.WithLog(a.log.V(1)),
				container.WithVolumes(volumes),
				container.WithContainerToolBin(Settings.PodmanBinary),
				container.WithEntrypointArgs(args...),
				container.WithDetachedMode(true),
				container.WithCleanup(a.cleanup),
				container.WithNetwork(networkName),
			)
			if err != nil {
				err := a.retryProviderContainer(ctx, networkName, volName, retry)
				if err != nil {
					return err
				}
			}
			a.providerContainerNames = append(a.providerContainerNames, con.Name)
			init.isRunning = true
		}
		// start additional providers
		if firstProvRun && len(a.providersMap) > 1 {
			a.log.Info("starting provider", "provider", prov)
			con := container.NewContainer()
			err := con.Run(
				ctx,
				container.WithImage(init.image),
				container.WithLog(a.log.V(1)),
				container.WithVolumes(volumes),
				container.WithContainerToolBin(Settings.PodmanBinary),
				container.WithEntrypointArgs(args...),
				container.WithDetachedMode(true),
				container.WithCleanup(a.cleanup),
				container.WithNetwork(fmt.Sprintf("container:%v", a.providerContainerNames[0])),
			)
			if err != nil {
				err := a.retryProviderContainer(ctx, networkName, volName, retry)
				if err != nil {
					return err
				}
			}
			a.providerContainerNames = append(a.providerContainerNames, con.Name)
			init.isRunning = true
		}
		firstProvRun = true
	}
	return nil
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

func (a *analyzeCommand) RunAnalysis(ctx context.Context, xmlOutputDir string, volName string) error {
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
	configVols, err := a.getConfigVolumes()
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
	if a.logLevel != nil {
		args = append(args, fmt.Sprintf("--verbose=%d", *a.logLevel))
	}
	labelSelector := a.getLabelSelector()
	if labelSelector != "" {
		args = append(args, fmt.Sprintf("--label-selector=%s", labelSelector))
	}

	// as of now only java & go have dep capability
	_, hasJava := a.providersMap[javaProvider]
	_, hasGo := a.providersMap[goProvider]
	// TODO currently cannot run these dep options with providers
	// other than java and go
	if (hasJava || hasGo) && len(a.providersMap) == 1 && a.mode == string(provider.FullAnalysisMode) {
		if !a.analyzeKnownLibraries {
			args = append(args,
				fmt.Sprintf("--dep-label-selector=(!%s=open-source)", provider.DepSourceLabel))
		}
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

	var networkName string
	if !a.needsBuiltin {
		networkName = fmt.Sprintf("container:%v", a.providerContainerNames[0])
		// only running builtin provider
	} else {
		networkName = "none"
	}
	c := container.NewContainer()
	// TODO (pgaikwad): run analysis & deps in parallel
	err = c.Run(
		ctx,
		container.WithImage(Settings.RunnerImage),
		container.WithLog(a.log.V(1)),
		container.WithVolumes(volumes),
		container.WithStdout(analysisLog),
		container.WithStderr(analysisLog),
		container.WithEntrypointArgs(args...),
		container.WithEntrypointBin("/usr/local/bin/konveyor-analyzer"),
		container.WithNetwork(networkName),
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

	// as of now only java & go have dep capability
	_, hasJava := a.providersMap[javaProvider]
	_, hasGo := a.providersMap[goProvider]
	if (hasJava || hasGo) && len(a.providersMap) == 1 && a.mode == string(provider.FullAnalysisMode) {
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

	args := []string{}
	staticReportArgs := []string{"/usr/local/bin/js-bundle-generator",
		fmt.Sprintf("--output-path=%s", path.Join("/usr/local/static-report/output.js"))}
	// Prepare report args list with single input analysis
	applicationNames := []string{filepath.Base(a.input)}
	outputAnalyses := []string{AnalysisOutputMountPath}
	outputDeps := []string{DepsOutputMountPath}

	if a.bulk {
		a.moveResults()
		// Scan all available analysis output files to be reported
		applicationNames = nil
		outputAnalyses = nil
		outputDeps = nil
		outputFiles, err := filepath.Glob(fmt.Sprintf("%s/output.yaml.*", a.output))
		if err != nil {
			return err
		}
		for i := range outputFiles {
			outputName := filepath.Base(outputFiles[i])
			applicationName := strings.SplitN(outputName, "output.yaml.", 2)[1]
			applicationNames = append(applicationNames, applicationName)
			outputAnalyses = append(outputAnalyses, strings.ReplaceAll(outputFiles[i], a.output, OutputPath)) // re-map paths to container mounts
			outputDeps = append(outputDeps, fmt.Sprintf("%s.%s", DepsOutputMountPath, applicationName))
		}
		for i := range outputDeps {
			_, depErr := os.Stat(outputDeps[i])
			if a.mode != string(provider.FullAnalysisMode) || depErr != nil {
				// Remove not existing dependency files from statis report generator list
				outputDeps[i] = ""
			}
		}
		staticReportArgs = append(staticReportArgs,
			fmt.Sprintf("--deps-output-list=%s", strings.Join(outputDeps, ",")))
	}

	staticReportArgs = append(staticReportArgs,
		fmt.Sprintf("--analysis-output-list=%s", strings.Join(outputAnalyses, ",")),
		fmt.Sprintf("--application-name-list=%s", strings.Join(applicationNames, ",")))

	// as of now, only java and go providers has dep capability
	_, hasJava := a.providersMap[javaProvider]
	_, hasGo := a.providersMap[goProvider]
	if (hasJava || hasGo) && a.mode == string(provider.FullAnalysisMode) && len(a.providersMap) == 1 {
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
	a.log.Info("Static report created. Access it at this URL:", "URL", string(uri))

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
	err = os.Remove(outputPath)
	if err != nil {
		return err
	}
	err = copyFileContents(analysisLogFilePath, fmt.Sprintf("%s.%s", analysisLogFilePath, a.inputShortName()))
	if err != nil {
		return err
	}
	err = os.Remove(analysisLogFilePath)
	if err != nil {
		return err
	}
	err = copyFileContents(depsPath, fmt.Sprintf("%s.%s", analysisLogFilePath, a.inputShortName()))
	if err == nil { // dependencies file presence is optional
		err = os.Remove(depsPath)
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
		if _, ok := seen[conf.Name]; !ok {
			continue
		}
		// set provider config options
		if conf.ContextLines != 0 {
			seen[conf.Name].ContextLines = conf.ContextLines
		}
		if conf.Proxy != nil {
			seen[conf.Name].Proxy = conf.Proxy
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
	if !a.cleanup || a.needsBuiltin {
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
	if a.networkName == "" {
		return nil
	}
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
	if a.volumeName == "" {
		return nil
	}
	cmd := exec.CommandContext(
		ctx,
		Settings.PodmanBinary,
		"volume",
		"rm", a.volumeName)
	a.log.V(1).Info("removing created volume",
		"volume", a.volumeName)
	return cmd.Run()
}

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

func (a *analyzeCommand) getProviderLogs(ctx context.Context) error {
	if len(a.providerContainerNames) == 0 || a.needsBuiltin {
		return nil
	}
	providerLogFilePath := filepath.Join(a.output, "provider.log")
	providerLog, err := os.Create(providerLogFilePath)
	if err != nil {
		return fmt.Errorf("failed creating provider log file at %s", providerLogFilePath)
	}
	defer providerLog.Close()
	for i := range a.providerContainerNames {
		a.log.V(1).Info("getting provider container logs",
			"container", a.providerContainerNames[i])

		cmd := exec.CommandContext(
			ctx,
			Settings.PodmanBinary,
			"logs",
			a.providerContainerNames[i])

		cmd.Stdout = providerLog
		cmd.Stderr = providerLog
		return cmd.Run()
	}

	return nil
}

func (a *analyzeCommand) analyzeDotnetFramework(ctx context.Context) error {
	if runtime.GOOS != "windows" {
		err := fmt.Errorf("Unsupported OS")
		a.log.Error(err, "Analysis of .NET Framework projects is only supported on Windows")
		return err
	}

	if a.bulk {
		err := fmt.Errorf("Unsupported option")
		a.log.Error(err, "Bulk analysis not supported for .NET Framework projects")
		return err
	}

	if a.mode == string(provider.FullAnalysisMode) {
		a.log.V(1).Info("Only source mode analysis is supported")
		a.mode = string(provider.SourceOnlyAnalysisMode)
	}

	var err error

	// Check configuration
	var systemInfo struct {
		Plugins struct {
			Network []string `json:"network"`
		} `json:"plugins"`
	}
	cmd := exec.Command(Settings.PodmanBinary, []string{"system", "info", "--format=json"}...)
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	if err = json.Unmarshal(out, &systemInfo); err != nil {
		return err
	}
	a.log.V(5).Info("container network plugins", "plugins", systemInfo)
	if !slices.Contains(systemInfo.Plugins.Network, "nat") {
		err := fmt.Errorf("Unsupported container client configuration")
		a.log.Error(err, ".NET Framework projects must be analyzed using docker configured to run Windows containers")
		return err
	}

	// Create network
	networkName := container.RandomName()
	cmd = exec.Command(Settings.PodmanBinary, []string{"network", "create", "-d", "nat", networkName}...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return err
	}
	a.log.V(1).Info("created container network", "network", networkName)
	a.networkName = networkName
	// end create network

	// Create volume
	// opts aren't supported on Windows
	// containerVolName, err := a.createContainerVolume()
	// if err != nil {
	// 	a.log.Error(err, "failed to create container volume")
	// 	return err
	// }

	// Run provider
	//foundProviders := []string{dotnetFrameworkProvider}
	//providerPorts, err := a.RunProviders(ctx, networkName, containerVolName, foundProviders, 5)
	//if err != nil {
	//	a.log.Error(err, "failed to run provider")
	//	return err
	//}
	input, err := filepath.Abs(a.input)
	if err != nil {
		return err
	}
	port, err := freeport.GetFreePort()
	if err != nil {
		return err
	}
	a.log.V(1).Info("Starting dotnet-external-provider")
	providerContainer := container.NewContainer()
	err = providerContainer.Run(
		ctx,
		container.WithImage(Settings.DotnetProviderImage),
		container.WithLog(a.log.V(1)),
		container.WithVolumes(map[string]string{
			input: "C:" + filepath.FromSlash(SourceMountPath),
		}),
		container.WithContainerToolBin(Settings.PodmanBinary),
		container.WithEntrypointArgs([]string{fmt.Sprintf("--port=%v", port)}...),
		container.WithDetachedMode(true),
		container.WithCleanup(a.cleanup),
		container.WithNetwork(networkName),
	)
	if err != nil {
		return err
	}
	a.providerContainerNames = append(a.providerContainerNames, providerContainer.Name)
	a.log.V(1).Info("Provider started")
	// end run provider

	// Run analysis
	// err = a.RunAnalysis(ctx, "", containerVolName, foundProviders, providerPorts)
	// if err != nil {
	// 	a.log.Error(err, "failed to run analysis")
	// 	return err
	// }
	tempDir, err := os.MkdirTemp("", "analyze-config-")
	if err != nil {
		a.log.V(1).Error(err, "failed creating temp dir", "dir", tempDir)
		return err
	}
	a.log.V(1).Info("created directory for provider settings", "dir", tempDir)
	a.tempDirs = append(a.tempDirs, tempDir)

	// Set the IP!!!
	provConfig := []provider.Config{
		{
			Name: "builtin",
			InitConfig: []provider.InitConfig{
				{
					Location:     "C:" + filepath.FromSlash(SourceMountPath),
					AnalysisMode: provider.AnalysisMode(a.mode),
				},
			},
		},
		{
			Name:    dotnetProvider,
			Address: fmt.Sprintf("%v:%v", providerContainer.Name, port),
			InitConfig: []provider.InitConfig{
				{
					Location:     "C:" + filepath.FromSlash(SourceMountPath),
					AnalysisMode: provider.AnalysisMode(a.mode),
					ProviderSpecificConfig: map[string]interface{}{
						provider.LspServerPathConfigKey: "C:/Users/ContainerAdministrator/.dotnet/tools/csharp-ls.exe",
					},
				},
			},
		},
	}

	jsonData, err := json.MarshalIndent(&provConfig, "", "	")
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

	volumes := map[string]string{
		tempDir:  "C:" + filepath.FromSlash(ConfigMountPath),
		input:    "C:" + filepath.FromSlash(SourceMountPath),
		a.output: "C:" + filepath.FromSlash(OutputPath),
	}

	args := []string{
		fmt.Sprintf("--provider-settings=%s", "C:"+filepath.FromSlash(ProviderSettingsMountPath)),
		fmt.Sprintf("--output-file=%s", "C:"+filepath.FromSlash(AnalysisOutputMountPath)),
		fmt.Sprintf("--context-lines=%d", a.contextLines),
	}

	if a.enableDefaultRulesets {
		args = append(args, fmt.Sprintf("--rules=C:%s", filepath.FromSlash(RulesetPath)))
	}

	if len(a.rules) > 0 {
		ruleVols, err := a.getRulesVolumes()
		if err != nil {
			a.log.V(1).Error(err, "failed to get rule volumes for analysis")
			return err
		}
		for key, value := range ruleVols {
			volumes[key] = "C:" + filepath.FromSlash(value)
		}

		args = append(args, fmt.Sprintf("--rules=C:%s", filepath.FromSlash(CustomRulePath)))
	}

	if a.jaegerEndpoint != "" {
		args = append(args, "--enable-jaeger")
		args = append(args, "--jaeger-endpoint")
		args = append(args, a.jaegerEndpoint)
	}

	if a.logLevel != nil {
		args = append(args, fmt.Sprintf("--verbose=%d", *a.logLevel))
	}
	labelSelector := a.getLabelSelector()
	if labelSelector != "" {
		args = append(args, fmt.Sprintf("--label-selector=%s", labelSelector))
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

	c := container.NewContainer()
	err = c.Run(
		ctx,
		container.WithImage(Settings.RunnerImage),
		container.WithLog(a.log.V(1)),
		container.WithVolumes(volumes),
		container.WithStdout(analysisLog),
		container.WithStderr(analysisLog),
		container.WithEntrypointArgs(args...),
		container.WithEntrypointBin(`C:\app\konveyor-analyzer.exe`),
		container.WithNetwork(networkName),
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
	// end run analysis

	// Create json output
	err = a.CreateJSONOutput()
	if err != nil {
		a.log.Error(err, "failed to create json output file")
		return err
	}

	// Generate Static Report
	if a.skipStaticReport {
		return nil
	}

	err = container.NewContainer().Run(
		ctx,
		container.WithImage(Settings.RunnerImage),
		container.WithLog(a.log.V(1)),
		container.WithContainerToolBin(Settings.PodmanBinary),
		container.WithEntrypointBin("powershell"),
		container.WithEntrypointArgs("Copy-Item", `C:\app\static-report\`, "-Recurse", filepath.FromSlash(OutputPath)),
		container.WithVolumes(volumes),
		container.WithCleanup(a.cleanup),
	)
	if err != nil {
		return err
	}

	staticReportArgs := []string{
		fmt.Sprintf(`-output-path=C:\%s\static-report\output.js`, filepath.FromSlash(OutputPath)),
		fmt.Sprintf("-analysis-output-list=C:%s", filepath.FromSlash(AnalysisOutputMountPath)),
		fmt.Sprintf("-application-name-list=%s", filepath.Base(a.input)),
	}

	//staticReportContainer := container.NewContainer()
	a.log.Info("generating static report", "output", a.output, "args", staticReportArgs)
	err = container.NewContainer().Run(
		ctx,
		container.WithImage(Settings.RunnerImage),
		container.WithLog(a.log.V(1)),
		container.WithContainerToolBin(Settings.PodmanBinary),
		container.WithEntrypointBin(`C:\app\js-bundle-generator`),
		container.WithEntrypointArgs(staticReportArgs...),
		container.WithVolumes(volumes),
		container.WithCleanup(a.cleanup),
	)
	if err != nil {
		return err
	}

	uri := uri.File(filepath.Join(a.output, "static-report", "index.html"))
	a.log.Info("Static report created. Access it at this URL:", "URL", string(uri))

	return nil
}
