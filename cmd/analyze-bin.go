package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	java "github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/parser"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/konveyor/analyzer-lsp/tracing"
	"github.com/sirupsen/logrus"
	"go.lsp.dev/uri"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/yaml.v2"

	"github.com/spf13/cobra"
)

type analyzeBinCommand struct {
	listSources           bool
	listTargets           bool
	skipStaticReport      bool
	analyzeKnownLibraries bool
	jsonOutput            bool
	overwrite             bool
	mavenSettingsFile     string
	sources               []string
	targets               []string
	labelSelector         string
	input                 string
	output                string
	mode                  string
	httpProxy             string
	httpsProxy            string
	noProxy               string
	rules                 []string
	jaegerEndpoint        string
	enableDefaultRulesets bool
	contextLines          int
	incidentSelector      string

	//for cleaning
	binMap        map[string]string
	homeKantraDir string
	log           logr.Logger
	// isFileInput is set when input points to a file and not a dir
	isFileInput bool
	logLevel    *uint32
	cleanup     bool
}

// analyzeCmd represents the analyze command
func NewAnalyzeBinCmd(log logr.Logger) *cobra.Command {
	analyzeBinCmd := &analyzeBinCommand{
		log:     log,
		cleanup: true,
	}
	analyzeBinCommand := &cobra.Command{
		Use:   "analyze-bin",
		Short: "Analyze Java application source code",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Lookup("list-sources").Changed &&
				!cmd.Flags().Lookup("list-targets").Changed {
				//cmd.MarkFlagRequired("input")
				cmd.MarkFlagRequired("output")
				if err := cmd.ValidateRequiredFlags(); err != nil {
					return err
				}
			}
			err := analyzeBinCmd.setKantraDir()
			if err != nil {
				log.Error(err, "unable to get binaries")
				os.Exit(1)
			}
			err = analyzeBinCmd.Validate(cmd.Context())
			if err != nil {
				log.Error(err, "failed to validate flags")
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if val, err := cmd.Flags().GetUint32(logLevelFlag); err == nil {
				analyzeBinCmd.logLevel = &val
			}
			if val, err := cmd.Flags().GetBool(noCleanupFlag); err == nil {
				analyzeBinCmd.cleanup = !val
			}
			if analyzeBinCmd.listSources || analyzeBinCmd.listTargets {
				err := analyzeBinCmd.listLabels(cmd.Context())
				if err != nil {
					log.Error(err, "failed to list rule labels")
					return err
				}
				return nil
			}

			if analyzeBinCmd.binMap == nil {
				analyzeBinCmd.binMap = make(map[string]string)
			}

			defer os.Remove(filepath.Join(analyzeBinCmd.output, "settings.json"))

			ctx, cancelFunc := context.WithCancel(context.Background())
			defer cancelFunc()

			analysisLogFilePath := filepath.Join(analyzeBinCmd.output, "analysis.log")
			analysisLog, err := os.Create(analysisLogFilePath)
			if err != nil {
				return fmt.Errorf("failed creating provider log file at %s", analysisLogFilePath)
			}
			defer analysisLog.Close()

			logrusLog := logrus.New()
			logrusLog.SetOutput(analysisLog)
			logrusLog.SetFormatter(&logrus.TextFormatter{})
			// need to do research on mapping in logrusr to level here TODO
			logrusLog.SetLevel(logrus.Level(logLevel))
			log := logrusr.New(logrusLog)

			logrusErrLog := logrus.New()
			logrusErrLog.SetOutput(analysisLog)
			errLog := logrusr.New(logrusErrLog)

			fmt.Println("running analysis")
			labelSelectors := analyzeBinCmd.getLabelSelector()

			selectors := []engine.RuleSelector{}
			if labelSelectors != "" {
				selector, err := labels.NewLabelSelector[*engine.RuleMeta](labelSelectors, nil)
				if err != nil {
					errLog.Error(err, "failed to create label selector from expression", "selector", labelSelectors)
					os.Exit(1)
				}
				selectors = append(selectors, selector)
			}

			err = analyzeBinCmd.setBins()
			if err != nil {
				log.Error(err, "unable to get binaries")
				os.Exit(1)
			}

			// Get the configs
			finalConfigs, err := analyzeBinCmd.createProviderConfigs()
			if err != nil {
				errLog.Error(err, "unable to get Java configuration")
				os.Exit(1)
			}

			providers, providerLocations := analyzeBinCmd.setInternalProviders(log, finalConfigs)

			engineCtx, engineSpan := tracing.StartNewSpan(ctx, "rule-engine")
			//start up the rule eng
			eng := engine.CreateRuleEngine(engineCtx,
				10,
				log,
				engine.WithContextLines(analyzeBinCmd.contextLines),
				engine.WithIncidentSelector(analyzeBinCmd.incidentSelector),
				engine.WithLocationPrefixes(providerLocations),
			)

			parser := parser.RuleParser{
				ProviderNameToClient: providers,
				Log:                  log.WithName("parser"),
			}

			ruleSets := []engine.RuleSet{}
			needProviders := map[string]provider.InternalProviderClient{}

			if analyzeBinCmd.enableDefaultRulesets {
				analyzeBinCmd.rules = append(analyzeBinCmd.rules, filepath.Join(analyzeBinCmd.homeKantraDir, RulesetsLocation))
			}
			for _, f := range analyzeBinCmd.rules {
				internRuleSet, internNeedProviders, err := parser.LoadRules(f)
				if err != nil {
					log.Error(err, "unable to parse all the rules for ruleset", "file", f)
				}
				ruleSets = append(ruleSets, internRuleSet...)
				for k, v := range internNeedProviders {
					needProviders[k] = v
				}
			}
			err = analyzeBinCmd.startProviders(ctx, needProviders)
			if err != nil {
				os.Exit(1)
			}

			// start dependency analysis for full analysis mode only
			wg := &sync.WaitGroup{}
			var depSpan trace.Span
			if analyzeBinCmd.mode == string(provider.FullAnalysisMode) {
				var depCtx context.Context
				depCtx, depSpan = tracing.StartNewSpan(ctx, "dep")
				wg.Add(1)

				fmt.Println("running dependency analysis")
				go analyzeBinCmd.DependencyOutput(depCtx, providers, log, errLog, "dependencies.yaml", wg)
			}

			// This will already wait
			rulesets := eng.RunRules(ctx, ruleSets, selectors...)
			engineSpan.End()
			wg.Wait()
			if depSpan != nil {
				depSpan.End()
			}
			eng.Stop()

			for _, provider := range needProviders {
				provider.Stop()
			}

			sort.SliceStable(rulesets, func(i, j int) bool {
				return rulesets[i].Name < rulesets[j].Name
			})

			// Write results out to CLI
			b, err := yaml.Marshal(rulesets)
			if err != nil {
				return err
			}

			fmt.Println("writing analysis results to output", "output", analyzeBinCmd.output)
			err = os.WriteFile(filepath.Join(analyzeBinCmd.output, "output.yaml"), b, 0644)
			if err != nil {
				os.Exit(1) // Treat the error as a fatal error
			}

			err = analyzeBinCmd.createJSONOutput()
			if err != nil {
				log.Error(err, "failed to create json output file")
				return err
			}

			err = analyzeBinCmd.GenerateStaticReport(cmd.Context())
			if err != nil {
				log.Error(err, "failed to generate static report")
				return err
			}

			return nil
		},
	}
	analyzeBinCommand.Flags().BoolVar(&analyzeBinCmd.listSources, "list-sources", false, "list rules for available migration sources")
	analyzeBinCommand.Flags().BoolVar(&analyzeBinCmd.listTargets, "list-targets", false, "list rules for available migration targets")
	analyzeBinCommand.Flags().StringArrayVarP(&analyzeBinCmd.sources, "source", "s", []string{}, "source technology to consider for analysis. Use multiple times for additional sources: --source <source1> --source <source2> ...")
	analyzeBinCommand.Flags().StringArrayVarP(&analyzeBinCmd.targets, "target", "t", []string{}, "target technology to consider for analysis. Use multiple times for additional targets: --target <target1> --target <target2> ...")
	analyzeBinCommand.Flags().StringVarP(&analyzeBinCmd.labelSelector, "label-selector", "l", "", "run rules based on specified label selector expression")
	analyzeBinCommand.Flags().StringArrayVar(&analyzeBinCmd.rules, "rules", []string{}, "filename or directory containing rule files. Use multiple times for additional rules: --rules <rule1> --rules <rule2> ...")
	analyzeBinCommand.Flags().StringVarP(&analyzeBinCmd.input, "input", "i", "", "path to application source code or a binary")
	analyzeBinCommand.Flags().StringVarP(&analyzeBinCmd.output, "output", "o", "", "path to the directory for analysis output")
	analyzeBinCommand.Flags().BoolVar(&analyzeBinCmd.skipStaticReport, "skip-static-report", false, "do not generate static report")
	analyzeBinCommand.Flags().BoolVar(&analyzeBinCmd.analyzeKnownLibraries, "analyze-known-libraries", false, "analyze known open-source libraries")
	analyzeBinCommand.Flags().StringVar(&analyzeBinCmd.mavenSettingsFile, "maven-settings", "", "path to a custom maven settings file to use")
	analyzeBinCommand.Flags().StringVarP(&analyzeBinCmd.mode, "mode", "m", string(provider.FullAnalysisMode), "analysis mode. Must be one of 'full' (source + dependencies) or 'source-only'")
	analyzeBinCommand.Flags().BoolVar(&analyzeBinCmd.jsonOutput, "json-output", false, "create analysis and dependency output as json")
	analyzeBinCommand.Flags().BoolVar(&analyzeBinCmd.overwrite, "overwrite", false, "overwrite output directory")
	analyzeBinCommand.Flags().StringVar(&analyzeBinCmd.httpProxy, "http-proxy", loadEnvInsensitive("http_proxy"), "HTTP proxy string URL")
	analyzeBinCommand.Flags().StringVar(&analyzeBinCmd.httpsProxy, "https-proxy", loadEnvInsensitive("https_proxy"), "HTTPS proxy string URL")
	analyzeBinCommand.Flags().StringVar(&analyzeBinCmd.noProxy, "no-proxy", loadEnvInsensitive("no_proxy"), "proxy excluded URLs (relevant only with proxy)")
	analyzeBinCommand.Flags().StringVar(&analyzeBinCmd.jaegerEndpoint, "jaeger-endpoint", "", "jaeger endpoint to collect traces")
	analyzeBinCommand.Flags().BoolVar(&analyzeBinCmd.enableDefaultRulesets, "enable-default-rulesets", true, "run default rulesets with analysis")
	analyzeBinCommand.Flags().IntVar(&analyzeBinCmd.contextLines, "context-lines", 10, "number of lines of source code to include in the output for each incident")
	analyzeBinCommand.Flags().StringVar(&analyzeBinCmd.incidentSelector, "incident-selector", "", "an expression to select incidents based on custom variables. ex: (!package=io.konveyor.demo.config-utils)")

	return analyzeBinCommand
}

func (b *analyzeBinCommand) Validate(ctx context.Context) error {
	// Validate .kantra in home directory and its content (containerless)
	requiredDirs := []string{b.homeKantraDir, filepath.Join(b.homeKantraDir, RulesetsLocation), filepath.Join(b.homeKantraDir, JavaBundlesLocation), filepath.Join(b.homeKantraDir, JDTLSBinLocation)}
	for _, path := range requiredDirs {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			b.log.Error(err, "cannot open required path, ensure that container-less dependencies are installed")
			return err
		}
	}

	// Print-only methods
	if b.listSources || b.listTargets {
		return nil
	}
	if b.labelSelector != "" && (len(b.sources) > 0 || len(b.targets) > 0) {
		return fmt.Errorf("must not specify label-selector and sources or targets")
	}
	//Validate source labels
	if len(b.sources) > 0 {
		var sourcesRaw bytes.Buffer
		b.fetchLabels(ctx, true, false, &sourcesRaw)
		knownSources := strings.Split(sourcesRaw.String(), "\n")
		for _, source := range b.sources {
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
	// Validate target labels
	if len(b.targets) > 0 {
		var targetRaw bytes.Buffer
		b.fetchLabels(ctx, false, true, &targetRaw)
		knownTargets := strings.Split(targetRaw.String(), "\n")
		for _, source := range b.targets {
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
	if b.input != "" {
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
		stat, err := os.Stat(b.input)
		if err != nil {
			return fmt.Errorf("%w failed to stat input path %s", err, b.input)
		}
		// when input isn't a dir, it's pointing to a binary
		if !stat.Mode().IsDir() {
			// validate file types
			fileExt := filepath.Ext(b.input)
			switch fileExt {
			case JavaArchive, WebArchive, EnterpriseArchive, ClassFile:
				b.log.V(5).Info("valid java file found")
			default:
				return fmt.Errorf("invalid file type %v", fileExt)
			}
			b.isFileInput = true
		}
	}
	err := b.CheckOverwriteOutput()
	if err != nil {
		return err
	}
	stat, err := os.Stat(b.output)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = os.MkdirAll(b.output, os.ModePerm)
			if err != nil {
				return fmt.Errorf("%w failed to create output dir %s", err, b.output)
			}
		} else {
			return fmt.Errorf("failed to stat output directory %s", b.output)
		}
	}
	if stat != nil && !stat.IsDir() {
		return fmt.Errorf("output path %s is not a directory", b.output)
	}

	if b.mode != string(provider.FullAnalysisMode) &&
		b.mode != string(provider.SourceOnlyAnalysisMode) {
		return fmt.Errorf("mode must be one of 'full' or 'source-only'")
	}
	if _, err := os.Stat(b.mavenSettingsFile); b.mavenSettingsFile != "" && err != nil {
		return fmt.Errorf("%w failed to stat maven settings file at path %s", err, b.mavenSettingsFile)
	}
	// try to get abs path, if not, continue with relative path
	if absPath, err := filepath.Abs(b.output); err == nil {
		b.output = absPath
	}
	if absPath, err := filepath.Abs(b.input); err == nil {
		b.input = absPath
	}
	if absPath, err := filepath.Abs(b.mavenSettingsFile); b.mavenSettingsFile != "" && err == nil {
		b.mavenSettingsFile = absPath
	}
	if !b.enableDefaultRulesets && len(b.rules) == 0 {
		return fmt.Errorf("must specify rules if default rulesets are not enabled")
	}
	return nil
}

func (b *analyzeBinCommand) listLabels(ctx context.Context) error {
	return b.fetchLabels(ctx, b.listSources, b.listTargets, os.Stdout)
}

func (b *analyzeBinCommand) fetchLabels(ctx context.Context, listSources, listTargets bool, out io.Writer) error {
	// reserved labels
	sourceLabel := outputv1.SourceTechnologyLabel
	targetLabel := outputv1.TargetTechnologyLabel

	if listSources {
		sourceSlice, err := b.walkRuleFilesForLabels(sourceLabel)
		if err != nil {
			b.log.Error(err, "failed to read rule labels")
			return err
		}
		listOptionsFromLabels(sourceSlice, sourceLabel, out)
		return nil
	}
	if listTargets {
		targetsSlice, err := b.walkRuleFilesForLabels(targetLabel)
		if err != nil {
			b.log.Error(err, "failed to read rule labels")
			return err
		}
		listOptionsFromLabels(targetsSlice, targetLabel, out)
		return nil
	}

	return nil
}

func (b *analyzeBinCommand) walkRuleFilesForLabels(label string) ([]string, error) {
	labelsSlice := []string{}
	path := filepath.Join(b.homeKantraDir, RulesetsLocation)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		b.log.Error(err, "cannot open provided path")
		return nil, err
	}
	err := filepath.WalkDir(path, walkRuleSets(path, label, &labelsSlice))
	if err != nil {
		return nil, err
	}
	return labelsSlice, nil
}

func (a *analyzeBinCommand) CheckOverwriteOutput() error {
	// default overwrite to false so check for already existing output dir
	stat, err := os.Stat(a.output)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
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

func (b *analyzeBinCommand) setKantraDir() error {
	var homeDir string
	var set bool
	ops := runtime.GOOS
	if ops == "linux" {
		homeDir, set = os.LookupEnv("XDG_CONFIG_HOME")
	}
	if ops != "linux" || homeDir == "" || !set {
		// on Unix, including macOS, this returns the $HOME environment variable. On Windows, it returns %USERPROFILE%
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return err
		}
	}
	b.homeKantraDir = filepath.Join(homeDir, ".kantra")
	return nil
}

func (b *analyzeBinCommand) setBins() error {
	b.binMap["bundle"] = filepath.Join(b.homeKantraDir, JavaBundlesLocation)
	b.binMap["jdtls"] = filepath.Join(b.homeKantraDir, JDTLSBinLocation)

	// validate
	for _, v := range b.binMap {
		stat, err := os.Stat(v)
		if err != nil {
			return fmt.Errorf("%w failed to stat bin %s", err, v)
		}
		if stat.Mode().IsDir() {
			return fmt.Errorf("unable to find expected file at %s", v)
		}
	}
	return nil
}

func (b *analyzeBinCommand) createProviderConfigs() ([]provider.Config, error) {
	javaConfig := provider.Config{
		Name:       javaProvider,
		BinaryPath: b.binMap["jdtls"],
		InitConfig: []provider.InitConfig{
			{
				Location:     b.input,
				AnalysisMode: provider.AnalysisMode(b.mode),
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 javaProvider,
					"bundles":                       b.binMap["bundle"],
					provider.LspServerPathConfigKey: b.binMap["jdtls"],
				},
			},
		},
	}
	if b.mavenSettingsFile != "" {
		javaConfig.InitConfig[0].ProviderSpecificConfig["mavenSettingsFile"] = b.mavenSettingsFile
	}

	provConfig := []provider.Config{
		{
			Name: "builtin",
			InitConfig: []provider.InitConfig{
				{
					Location:     b.input,
					AnalysisMode: provider.AnalysisMode(b.mode),
				},
			},
		},
	}
	provConfig = append(provConfig, javaConfig)

	for i := range provConfig {
		// Set proxy to providers
		if b.httpProxy != "" || b.httpsProxy != "" {
			proxy := provider.Proxy{
				HTTPProxy:  b.httpProxy,
				HTTPSProxy: b.httpsProxy,
				NoProxy:    b.noProxy,
			}

			provConfig[i].Proxy = &proxy
		}
		provConfig[i].ContextLines = b.contextLines
	}

	jsonData, err := json.MarshalIndent(&provConfig, "", "	")
	if err != nil {
		b.log.V(1).Error(err, "failed to marshal provider config")
		return nil, err
	}
	err = os.WriteFile(filepath.Join(b.output, "settings.json"), jsonData, os.ModePerm)
	if err != nil {
		b.log.V(1).Error(err,
			"failed to write provider config", "dir", b.output, "file", "settings.json")
		return nil, err
	}
	configs := b.setConfigs(provConfig)
	return configs, nil
}

func (b *analyzeBinCommand) setConfigs(configs []provider.Config) []provider.Config {
	// we add builtin configs by default for all locations
	defaultBuiltinConfigs := []provider.InitConfig{}
	seenBuiltinConfigs := map[string]bool{}
	finalConfigs := []provider.Config{}
	for _, config := range configs {
		if config.Name != "builtin" {
			finalConfigs = append(finalConfigs, config)
		}
		for _, initConf := range config.InitConfig {
			if _, ok := seenBuiltinConfigs[initConf.Location]; !ok {
				if initConf.Location != "" {
					if stat, err := os.Stat(initConf.Location); err == nil && stat.IsDir() {
						builtinLocation, err := filepath.Abs(initConf.Location)
						if err != nil {
							builtinLocation = initConf.Location
						}
						seenBuiltinConfigs[builtinLocation] = true
						builtinConf := provider.InitConfig{Location: builtinLocation}
						if config.Name == "builtin" {
							builtinConf.ProviderSpecificConfig = initConf.ProviderSpecificConfig
						}
						defaultBuiltinConfigs = append(defaultBuiltinConfigs, builtinConf)
					}
				}
			}
		}
	}

	finalConfigs = append(finalConfigs, provider.Config{
		Name:       "builtin",
		InitConfig: defaultBuiltinConfigs,
	})

	return finalConfigs
}

func (b *analyzeBinCommand) setInternalProviders(log logr.Logger, finalConfigs []provider.Config) (map[string]provider.InternalProviderClient, []string) {
	providers := map[string]provider.InternalProviderClient{}
	providerLocations := []string{}
	for _, config := range finalConfigs {
		config.ContextLines = b.contextLines
		for _, ind := range config.InitConfig {
			providerLocations = append(providerLocations, ind.Location)
		}
		// IF analsyis mode is set from the CLI, then we will override this for each init config
		if b.mode != "" {
			inits := []provider.InitConfig{}
			for _, i := range config.InitConfig {
				i.AnalysisMode = provider.AnalysisMode(b.mode)
				inits = append(inits, i)
			}
			config.InitConfig = inits
		}
		var prov provider.InternalProviderClient
		var err error
		// only create java and builtin providers
		if config.Name == javaProvider {
			prov = java.NewJavaProvider(log, "java", b.contextLines, config)

		} else if config.Name == "builtin" {
			prov, err = lib.GetProviderClient(config, log)
			if err != nil {
				log.Error(err, "failed to create builtin provider")
				os.Exit(1)
			}
		}
		providers[config.Name] = prov
	}
	return providers, providerLocations
}

func (b *analyzeBinCommand) startProviders(ctx context.Context, needProviders map[string]provider.InternalProviderClient) error {
	// Now that we have all the providers, we need to start them.
	additionalBuiltinConfigs := []provider.InitConfig{}
	for name, provider := range needProviders {
		switch name {
		// other providers can return additional configs for the builtin provider
		// therefore, we initiate builtin provider separately at the end
		case "builtin":
			continue
		default:
			initCtx, initSpan := tracing.StartNewSpan(ctx, "init",
				attribute.Key("provider").String(name))
			additionalBuiltinConfs, err := provider.ProviderInit(initCtx, nil)
			if err != nil {
				b.log.Error(err, "unable to init the providers", "provider", name)
				os.Exit(1)
			}
			if additionalBuiltinConfs != nil {
				additionalBuiltinConfigs = append(additionalBuiltinConfigs, additionalBuiltinConfs...)
			}
			initSpan.End()
		}
	}

	if builtinClient, ok := needProviders["builtin"]; ok {
		if _, err := builtinClient.ProviderInit(ctx, additionalBuiltinConfigs); err != nil {
			return err
		}
	}
	return nil
}

func (b *analyzeBinCommand) DependencyOutput(ctx context.Context, providers map[string]provider.InternalProviderClient, log logr.Logger, errLog logr.Logger, depOutputFile string, wg *sync.WaitGroup) {
	defer wg.Done()
	var depsFlat []konveyor.DepsFlatItem
	var depsTree []konveyor.DepsTreeItem
	var err error

	for _, prov := range providers {
		deps, err := prov.GetDependencies(ctx)
		if err != nil {
			errLog.Error(err, "failed to get list of dependencies for provider", "provider", "java")
		}
		for u, ds := range deps {
			newDeps := ds
			depsFlat = append(depsFlat, konveyor.DepsFlatItem{
				Provider:     "java",
				FileURI:      string(u),
				Dependencies: newDeps,
			})
		}

		if depsFlat == nil && depsTree == nil {
			errLog.Info("failed to get dependencies from all given providers")
			return
		}
	}

	var by []byte
	// Sort depsFlat
	sort.SliceStable(depsFlat, func(i, j int) bool {
		if depsFlat[i].Provider == depsFlat[j].Provider {
			return depsFlat[i].FileURI < depsFlat[j].FileURI
		} else {
			return depsFlat[i].Provider < depsFlat[j].Provider
		}
	})

	by, err = yaml.Marshal(depsFlat)
	if err != nil {
		errLog.Error(err, "failed to marshal dependency data as yaml")
		return
	}

	err = os.WriteFile(filepath.Join(b.output, depOutputFile), by, 0644)
	if err != nil {
		errLog.Error(err, "failed to write dependencies to output file", "file", depOutputFile)
		return
	}

}

func (b *analyzeBinCommand) createJSONOutput() error {
	if !b.jsonOutput {
		return nil
	}
	outputPath := filepath.Join(b.output, "output.yaml")
	depPath := filepath.Join(b.output, "dependencies.yaml")

	data, err := os.ReadFile(outputPath)
	if err != nil {
		return err
	}
	ruleOutput := &[]outputv1.RuleSet{}
	err = yaml.Unmarshal(data, ruleOutput)
	if err != nil {
		b.log.V(1).Error(err, "failed to unmarshal output yaml")
		return err
	}

	jsonData, err := json.MarshalIndent(ruleOutput, "", "	")
	if err != nil {
		b.log.V(1).Error(err, "failed to marshal output file to json")
		return err
	}
	err = os.WriteFile(filepath.Join(b.output, "output.json"), jsonData, os.ModePerm)
	if err != nil {
		b.log.V(1).Error(err, "failed to write json output", "dir", b.output, "file", "output.json")
		return err
	}

	// in case of no dep output
	_, noDepFileErr := os.Stat(filepath.Join(b.output, "dependencies.yaml"))
	if errors.Is(noDepFileErr, os.ErrNotExist) || b.mode == string(provider.SourceOnlyAnalysisMode) {
		b.log.Info("skipping dependency output for json output")
		return nil
	}
	depData, err := os.ReadFile(depPath)
	if err != nil {
		return err
	}
	depOutput := &[]outputv1.DepsFlatItem{}
	err = yaml.Unmarshal(depData, depOutput)
	if err != nil {
		b.log.V(1).Error(err, "failed to unmarshal dependencies yaml")
		return err
	}

	jsonDataDep, err := json.MarshalIndent(depOutput, "", "	")
	if err != nil {
		b.log.V(1).Error(err, "failed to marshal dependencies file to json")
		return err
	}
	err = os.WriteFile(filepath.Join(b.output, "dependencies.json"), jsonDataDep, os.ModePerm)
	if err != nil {
		b.log.V(1).Error(err, "failed to write json dependencies output", "dir", b.output, "file", "dependencies.json")
		return err
	}

	return nil
}

func (b *analyzeBinCommand) buildStaticReportFile(ctx context.Context, staticReportPath string, depsErr bool) error {
	// Prepare report args list with single input analysis
	applicationName := []string{filepath.Base(b.input)}
	outputAnalysis := []string{filepath.Join(b.output, "output.yaml")}
	outputDeps := []string{filepath.Join(b.output, "dependencies.yaml")}
	outputJSPath := filepath.Join(staticReportPath, "output.js")

	if depsErr {
		outputDeps = []string{}
	}
	// create output.js file from analysis output.yaml
	apps, err := validateFlags(outputAnalysis, applicationName, outputDeps)
	if err != nil {
		log.Fatalln("failed to validate flags", err)
	}

	err = loadApplications(apps)
	if err != nil {
		log.Fatalln("failed to load report data from analysis output", err)
	}

	err = generateJSBundle(apps, outputJSPath)
	if err != nil {
		log.Fatalln("failed to generate output.js file from template", err)
	}

	return nil
}

func (b *analyzeBinCommand) buildStaticReportOutput(ctx context.Context, log *os.File) error {
	outputFileDestPath := filepath.Join(b.homeKantraDir, "static-report")

	// move build dir to user output dir
	cmd := exec.Command("cp", "-r", outputFileDestPath, b.output)
	cmd.Stdout = log
	err := cmd.Run()
	if err != nil {
		return err
	}

	return nil
}

func (b *analyzeBinCommand) GenerateStaticReport(ctx context.Context) error {
	if b.skipStaticReport {
		return nil
	}
	staticReportLogFilePath := filepath.Join(b.output, "static-report.log")
	staticReportLog, err := os.Create(staticReportLogFilePath)
	if err != nil {
		return fmt.Errorf("failed creating provider log file at %s", staticReportLogFilePath)
	}
	defer staticReportLog.Close()

	// it's possible for dependency analysis to fail
	// in this case we still want to generate a static report for successful source analysis
	_, noDepFileErr := os.Stat(filepath.Join(b.output, "dependencies.yaml"))
	if errors.Is(noDepFileErr, os.ErrNotExist) {
		b.log.Info("unable to get dependency output in static report. generating static report from source analysis only")

		// some other err
	} else if noDepFileErr != nil && !errors.Is(noDepFileErr, os.ErrNotExist) {
		return noDepFileErr
	}

	staticReportAanlyzePath := filepath.Join(b.homeKantraDir, "static-report")
	err = b.buildStaticReportFile(ctx, staticReportAanlyzePath, errors.Is(noDepFileErr, os.ErrNotExist))
	if err != nil {
		return err
	}
	err = b.buildStaticReportOutput(ctx, staticReportLog)
	if err != nil {
		return err
	}
	uri := uri.File(filepath.Join(b.output, "static-report", "index.html"))
	b.log.Info("Static report created. Access it at this URL:", "URL", string(uri))

	return nil
}

func (b *analyzeBinCommand) getLabelSelector() string {
	if b.labelSelector != "" {
		return b.labelSelector
	}
	if (b.sources == nil || len(b.sources) == 0) &&
		(b.targets == nil || len(b.targets) == 0) {
		return ""
	}
	// default labels are applied everytime either a source or target is specified
	defaultLabels := []string{"discovery"}
	targets := []string{}
	for _, target := range b.targets {
		targets = append(targets,
			fmt.Sprintf("%s=%s", outputv1.TargetTechnologyLabel, target))
	}
	sources := []string{}
	for _, source := range b.sources {
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
