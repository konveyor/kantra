package analyze

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	kantraProvider "github.com/konveyor-ecosystem/kantra/pkg/provider"

	"slices"

	"github.com/devfile/alizer/pkg/apis/recognizer"
	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/provider"

	"github.com/spf13/cobra"
)

// kantra analyze flags
type analyzeCommand struct {
	listSources              bool
	listTargets              bool
	listProviders            bool
	listLanguages            bool
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
	noDepRules               bool
	rules                    []string
	tempRuleDir              string
	jaegerEndpoint           string
	enableDefaultRulesets    bool
	httpProxy                string
	httpsProxy               string
	noProxy                  string
	contextLines             int
	incidentSelector         string
	depFolders               []string
	provider                 []string
	logLevel                 *uint32
	cleanup                  bool
	runLocal                 bool
	disableMavenSearch       bool
	noProgress               bool
	overrideProviderSettings string
	profileDir               string
	profilePath              string
	AnalyzeCommandContext
}

// analyzeCmd represents the analyze command
func NewAnalyzeCmd(log logr.Logger) *cobra.Command {
	analyzeCmd := &analyzeCommand{
		cleanup: true,
	}
	analyzeCmd.log = log

	analyzeCommand := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze application source code",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// TODO (pgaikwad): this is nasty
			if !cmd.Flags().Lookup("list-sources").Changed &&
				!cmd.Flags().Lookup("list-targets").Changed &&
				!cmd.Flags().Lookup("list-providers").Changed &&
				!cmd.Flags().Lookup("list-languages").Changed &&
				!cmd.Flags().Lookup("profile-dir").Changed {
				cmd.MarkFlagRequired("input")
				cmd.MarkFlagRequired("output")
				if err := cmd.ValidateRequiredFlags(); err != nil {
					return err
				}
			}
			if cmd.Flags().Lookup("list-languages").Changed {
				cmd.MarkFlagRequired("input")
			}
			kantraDir, err := util.GetKantraDir()
			if err != nil {
				analyzeCmd.log.Error(err, "unable to get analyze reqs")
				return err
			}
			analyzeCmd.kantraDir = kantraDir
			analyzeCmd.log.Info("found kantra dir", "dir", kantraDir)

			foundProfile, err := analyzeCmd.ValidateAndLoadProfile()
			if err != nil {
				return err
			}
			if analyzeCmd.profileDir == "" && foundProfile == nil {
				analyzeCmd.log.V(7).Info("did not find profile in default path")
			}
			if analyzeCmd.profilePath != "" {
				analyzeCmd.log.Info("using profile", "profile", analyzeCmd.profilePath)
				if err := analyzeCmd.applyProfileSettings(analyzeCmd.profilePath, cmd); err != nil {
					analyzeCmd.log.Error(err, "failed to get settings from profile")
					return err
				}
			}

			err = analyzeCmd.Validate(cmd.Context(), cmd, foundProfile)
			if err != nil {
				log.Error(err, "failed to validate flags")
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if val, err := cmd.Flags().GetUint32("log-level"); err == nil {
				analyzeCmd.logLevel = &val
			}
			if val, err := cmd.Flags().GetBool("no-cleanup"); err == nil {
				analyzeCmd.cleanup = !val
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			if analyzeCmd.listProviders {
				analyzeCmd.ListAllProviders()
				return nil
			}

			// skip container mode check
			if analyzeCmd.listLanguages {
				analyzeCmd.runLocal = false
			}

			if analyzeCmd.listSources || analyzeCmd.listTargets {
				// list sources/targets in containerless mode
				if analyzeCmd.runLocal {
					err := analyzeCmd.listLabelsContainerless(ctx)
					if err != nil {
						analyzeCmd.log.Error(err, "failed to list rule labels")
						return err
					}
					return nil
				}
				// list sources/targets in container mode
				err := analyzeCmd.ListLabels(cmd.Context())
				if err != nil {
					log.Error(err, "failed to list rule labels")
					return err
				}
				return nil
			}
			languages, err := recognizer.Analyze(analyzeCmd.input)
			if err != nil {
				log.Error(err, "Failed to determine languages for input")
				return err
			}
			if analyzeCmd.listLanguages {
				// for binaries, assume Java application
				if analyzeCmd.isFileInput {
					fmt.Fprintln(os.Stdout, "found languages for input application:", util.JavaProvider)
					return nil
				}
				err := listLanguages(languages, analyzeCmd.input)
				if err != nil {
					return err
				}
				return nil
			}

			foundProviders := []string{}
			// file input means a binary was given which only the java provider can use
			if analyzeCmd.isFileInput {
				foundProviders = append(foundProviders, util.JavaProvider)
			} else {
				foundProviders, err = analyzeCmd.setProviders(analyzeCmd.provider, languages, foundProviders)
				if err != nil {
					log.Error(err, "failed to set provider info")
					return err
				}
				err = analyzeCmd.validateProviders(foundProviders)
				if err != nil {
					return err
				}
			}

			// default to run container mode if no Java provider found
			if len(foundProviders) > 0 && !slices.Contains(foundProviders, util.JavaProvider) {
				log.V(1).Info("detected non-Java providers, switching to hybrid mode", "providers", foundProviders)
				analyzeCmd.runLocal = false
			}

			// --- Determine execution mode ---
			mode := kantraProvider.ModeNetwork
			if analyzeCmd.runLocal {
				mode = kantraProvider.ModeLocal
			}

			// For hybrid mode with non-Java providers: check default rules availability
			if mode == kantraProvider.ModeNetwork {
				if len(foundProviders) > 0 && len(analyzeCmd.rules) == 0 && !slices.Contains(foundProviders, util.JavaProvider) {
					return fmt.Errorf("no providers found with default rules. Use --rules option")
				}

				// alizer does not detect certain files such as xml
				// in this case, we can first check for a java project
				// if not found, only start builtin provider
				if len(foundProviders) == 0 {
					foundJava, err := analyzeCmd.detectJavaProviderFallback()
					if err != nil {
						return err
					}
					if foundJava {
						foundProviders = append(foundProviders, util.JavaProvider)
					}
				}
			}

			// default rulesets exist for java, nodejs, and csharp
			hasProviderWithDefaultRules := false
			for _, p := range foundProviders {
				if _, ok := util.DefaultRulesetDir[p]; ok {
					hasProviderWithDefaultRules = true
					break
				}
			}
			if len(foundProviders) > 0 && len(analyzeCmd.rules) == 0 && !hasProviderWithDefaultRules {
				return fmt.Errorf("no providers found with default rules. Use --rules option")
			}

			log.V(1).Info("running analysis", "mode", mode, "providers", foundProviders)

			// Run unified analysis pipeline
			cmdCtx, cancelFunc := context.WithCancel(ctx)
			defer cancelFunc()
			err = analyzeCmd.runAnalysis(cmdCtx, mode, foundProviders)
			if err != nil {
				log.Error(err, "analysis failed")
				return err
			}
			return nil
		},
	}
	analyzeCommand.Flags().BoolVar(&analyzeCmd.listSources, "list-sources", false, "list rules for available migration sources")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.listTargets, "list-targets", false, "list rules for available migration targets")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.listProviders, "list-providers", false, "list available supported providers")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.listLanguages, "list-languages", false, "list found application language(s)")
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
	analyzeCommand.Flags().BoolVar(&analyzeCmd.noDepRules, "no-dependency-rules", false, "disable dependency analysis rules")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.jsonOutput, "json-output", false, "create analysis and dependency output as json")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.overwrite, "overwrite", false, "overwrite output directory")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.bulk, "bulk", false, "running multiple analyze commands in bulk will result to combined static report")
	analyzeCommand.Flags().StringVar(&analyzeCmd.jaegerEndpoint, "jaeger-endpoint", "", "jaeger endpoint to collect traces")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.enableDefaultRulesets, "enable-default-rulesets", true, "run default rulesets with analysis")
	analyzeCommand.Flags().StringVar(&analyzeCmd.httpProxy, "http-proxy", util.LoadEnvInsensitive("http_proxy"), "HTTP proxy string URL")
	analyzeCommand.Flags().StringVar(&analyzeCmd.httpsProxy, "https-proxy", util.LoadEnvInsensitive("https_proxy"), "HTTPS proxy string URL")
	analyzeCommand.Flags().StringVar(&analyzeCmd.noProxy, "no-proxy", util.LoadEnvInsensitive("no_proxy"), "proxy excluded URLs (relevant only with proxy)")
	analyzeCommand.Flags().IntVar(&analyzeCmd.contextLines, "context-lines", 100, "number of lines of source code to include in the output for each incident")
	analyzeCommand.Flags().StringVar(&analyzeCmd.incidentSelector, "incident-selector", "", "an expression to select incidents based on custom variables. ex: (!package=io.konveyor.demo.config-utils)")
	analyzeCommand.Flags().StringArrayVarP(&analyzeCmd.depFolders, "dependency-folders", "d", []string{}, "directory for dependencies")
	analyzeCommand.Flags().StringArrayVar(&analyzeCmd.provider, "provider", []string{}, "specify which provider(s) to run")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.runLocal, "run-local", true, "run Java analysis in containerless mode")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.disableMavenSearch, "disable-maven-search", false, "disable maven search for dependencies")
	analyzeCommand.Flags().BoolVar(&analyzeCmd.noProgress, "no-progress", false, "disable progress reporting (useful for scripting)")
	analyzeCommand.Flags().StringVar(&analyzeCmd.overrideProviderSettings, "override-provider-settings", "", "override provider settings with custom provider config file")
	analyzeCommand.Flags().StringVar(&analyzeCmd.profileDir, "profile-dir", "", "path to a directory containing analysis profiles")
	return analyzeCommand
}
