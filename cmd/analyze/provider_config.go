package analyze

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/konveyor-ecosystem/kantra/cmd/internal/settings"
	kantraProvider "github.com/konveyor-ecosystem/kantra/pkg/provider"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	outputv1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"golang.org/x/exp/maps"
	"gopkg.in/yaml.v2"
)

func (a *analyzeCommand) getConfigVolumes() (map[string]string, error) {
	tempDir, err := os.MkdirTemp("", "analyze-config-")
	if err != nil {
		a.log.V(1).Error(err, "failed creating temp dir", "dir", tempDir)
		return nil, err
	}
	a.log.V(1).Info("created directory for provider settings", "dir", tempDir)
	a.tempDirs = append(a.tempDirs, tempDir)

	settingsVols := map[string]string{
		tempDir: util.ConfigMountPath,
	}

	// Maven settings: mount the file directly into the container (no copy needed)
	mavenSettingsPath := ""
	if a.mavenSettingsFile != "" {
		mavenSettingsPath = fmt.Sprintf("%s/%s", util.ConfigMountPath, "settings.xml")
		settingsVols[a.mavenSettingsFile] = mavenSettingsPath
	}

	// Build provider configs using GetConfig
	baseOpts := kantraProvider.BaseOptions{
		Location:     a.sourceLocationPath,
		AnalysisMode: a.mode,
		InputPath:    a.input,
		HTTPProxy:    a.httpProxy,
		HTTPSProxy:   a.httpsProxy,
		NoProxy:      a.noProxy,
	}

	// Builtin provider always runs
	builtinProvider := &kantraProvider.BuiltinProvider{}
	builtinCfg, err := builtinProvider.GetConfig(kantraProvider.ModeContainer, baseOpts)
	if err != nil {
		a.log.V(1).Error(err, "failed to get builtin provider config")
		return nil, err
	}
	provConfig := []provider.Config{builtinCfg}

	if !a.needsBuiltin {
		vols, _ := a.getDepsFolders()
		if len(vols) != 0 {
			maps.Copy(settingsVols, vols)
		}
		for provName, provInfo := range a.providersMap {
			provOpts := baseOpts
			provOpts.Address = fmt.Sprintf("0.0.0.0:%v", a.providersMap[provName].port)

			// Pass provider-specific options via variadic ProviderOption
			var extraOpts []kantraProvider.ProviderOption
			if provName == util.JavaProvider {
				extraOpts = append(extraOpts, kantraProvider.JavaOptions{
					MavenSettingsFile:  mavenSettingsPath,
					JvmMaxMem:          settings.Settings.JvmMaxMem,
					DisableMavenSearch: a.disableMavenSearch,
				})
			}

			cfg, err := provInfo.provider.GetConfig(kantraProvider.ModeContainer, provOpts, extraOpts...)
			if err != nil {
				a.log.V(1).Error(err, "failed creating provider config")
				return nil, err
			}
			provConfig = append(provConfig, cfg)
		}

		for prov := range a.providersMap {
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
	if _, hasJava := a.providersMap[util.JavaProvider]; hasJava {
		if runtime.GOOS == "linux" {
			m2Dir, err := os.MkdirTemp("", "m2-repo-")
			if err != nil {
				a.log.V(1).Error(err, "failed to create m2 repo", "dir", m2Dir)
			} else {
				settingsVols[m2Dir] = util.M2Dir
				a.log.V(1).Info("created directory for maven repo", "dir", m2Dir)
				a.tempDirs = append(a.tempDirs, m2Dir)
			}
		}
	}

	return settingsVols, nil
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
			// return target expression OR'd with default labels
			return fmt.Sprintf("%s || (%s)",
				targetExpr, strings.Join(defaultLabels, " || "))
		}
	}
	if sourceExpr != "" {
		// when only source is specified, OR them all
		return fmt.Sprintf("%s || (%s)",
			sourceExpr, strings.Join(defaultLabels, " || "))
	}
	return ""
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
			if i >= len(seen[conf.Name].InitConfig) {
				break
			}
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
		case k == util.MavenSettingsFile:
			// validate maven settings file
			if _, err := os.Stat(v.(string)); err != nil {
				return nil, fmt.Errorf("%w failed to stat maven settings file at path %s", err, v)
			}
			if absPath, err := filepath.Abs(v.(string)); err == nil {
				seenConf[k] = absPath
			}
			// copy file to mount path
			err := util.CopyFileContents(v.(string), filepath.Join(tempDir, "settings.xml"))
			if err != nil {
				a.log.V(1).Error(err, "failed copying maven settings file", "path", v)
				return nil, err
			}
			seenConf[k] = fmt.Sprintf("%s/%s", util.ConfigMountPath, "settings.xml")
			continue
		// we don't want users to override these options here
		// use --overrideProviderSettings to do so
		case k != util.LspServerPath && k != util.LspServerName && k != util.WorkspaceFolders && k != util.DependencyProviderPath:
			seenConf[k] = v
		}
	}
	return seenConf, nil
}

// loadOverrideProviderSettings loads provider configuration overrides from a JSON file.
// Returns a slice of provider.Config objects that will be merged with default configs.
func (a *analyzeCommand) loadOverrideProviderSettings() ([]provider.Config, error) {
	if a.overrideProviderSettings == "" {
		return nil, nil
	}

	a.log.V(1).Info("loading override provider settings", "file", a.overrideProviderSettings)

	data, err := os.ReadFile(a.overrideProviderSettings)
	if err != nil {
		return nil, fmt.Errorf("failed to read override provider settings file: %w", err)
	}

	var overrideConfigs []provider.Config
	if err := json.Unmarshal(data, &overrideConfigs); err != nil {
		return nil, fmt.Errorf("failed to parse override provider settings JSON: %w", err)
	}

	a.log.V(1).Info("loaded override provider settings", "providers", len(overrideConfigs))
	return overrideConfigs, nil
}

// mergeProviderSpecificConfig merges override config into base config.
// Override values take precedence over base values.
func mergeProviderSpecificConfig(base, override map[string]interface{}) map[string]interface{} {
	if override == nil {
		return base
	}

	result := make(map[string]interface{})
	// Copy base config
	for k, v := range base {
		result[k] = v
	}
	// Apply overrides
	for k, v := range override {
		result[k] = v
	}
	return result
}

// applyProviderOverrides applies override settings to a provider config.
// Returns the modified config with overrides applied.
func applyProviderOverrides(baseConfig provider.Config, overrideConfigs []provider.Config) provider.Config {
	if overrideConfigs == nil {
		return baseConfig
	}

	// Find matching override config by provider name
	for _, override := range overrideConfigs {
		if override.Name != baseConfig.Name {
			continue
		}

		// Apply top-level config overrides
		if override.ContextLines != 0 {
			baseConfig.ContextLines = override.ContextLines
		}
		if override.Proxy != nil {
			baseConfig.Proxy = override.Proxy
		}

		// Merge InitConfig settings
		if len(override.InitConfig) > 0 && len(baseConfig.InitConfig) > 0 {
			// Merge ProviderSpecificConfig from first InitConfig
			if override.InitConfig[0].ProviderSpecificConfig != nil {
				baseConfig.InitConfig[0].ProviderSpecificConfig = mergeProviderSpecificConfig(
					baseConfig.InitConfig[0].ProviderSpecificConfig,
					override.InitConfig[0].ProviderSpecificConfig,
				)
			}
			// Apply other InitConfig fields
			if override.InitConfig[0].AnalysisMode != "" {
				baseConfig.InitConfig[0].AnalysisMode = override.InitConfig[0].AnalysisMode
			}
		}
		break
	}

	return baseConfig
}
