package provider

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	analyzerprovider "github.com/konveyor/analyzer-lsp/provider"
)

// Well-known subdirectory paths within the kantra installation directory.
// These must match the paths used by the kantra install/setup process.
const (
	rulesetsSubdir    = "rulesets"
	javaBundlesSubdir = "jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar"
	jdtlsBinSubdir    = "jdtls/bin/jdtls"
	fernflowerJar     = "fernflower.jar"
)

// Eclipse language server temporary directories created in the working
// directory during Java analysis. Cleaned up by Stop.
var eclipseLSDirs = []string{
	"org.eclipse.core.runtime",
	"org.eclipse.equinox.app",
	"org.eclipse.equinox.launcher",
	"org.eclipse.osgi",
}

// localEnvironment runs providers in-process on the host.
// Used for containerless (Java-only) analysis.
type localEnvironment struct {
	cfg     EnvironmentConfig
	log     logr.Logger
	configs []analyzerprovider.Config
}

func newLocalEnvironment(cfg EnvironmentConfig) *localEnvironment {
	log := cfg.Log
	if log.GetSink() == nil {
		log = logr.Discard()
	}
	return &localEnvironment{
		cfg: cfg,
		log: log,
	}
}

// Start validates that the host has the required tools and kantra
// installation for containerless analysis.
func (e *localEnvironment) Start(ctx context.Context) error {
	// validate input app is not the current dir
	// .metadata cannot initialize in the app root
	currentDir, err := os.Getwd()
	if err != nil {
		return err
	}
	if e.cfg.Input == currentDir {
		return fmt.Errorf("input path %s cannot be the current directory", e.cfg.Input)
	}

	// validate mvn
	if _, err := exec.LookPath("mvn"); err != nil {
		return fmt.Errorf("%w cannot find requirement maven; ensure maven is installed", err)
	}

	// validate java
	cmd := exec.Command("java", "-version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w cannot execute required command java; ensure java is installed", err)
	}
	if strings.Contains(string(output), "openjdk") {
		re := regexp.MustCompile(`openjdk version "(.*?)"`)
		match := re.FindStringSubmatch(string(output))
		if len(match) < 2 {
			return fmt.Errorf("cannot parse openjdk version from output: %s", string(output))
		}
		jdkVersionStr := strings.Split(match[1], ".")
		jdkVersionInt, err := strconv.Atoi(jdkVersionStr[0])
		if err != nil {
			return fmt.Errorf("%w cannot parse java version", err)
		}
		if jdkVersionInt < 17 {
			return fmt.Errorf("cannot find requirement openjdk17+; ensure openjdk17+ is installed")
		}
	}
	if os.Getenv("JAVA_HOME") == "" {
		return fmt.Errorf("JAVA_HOME is not set; ensure JAVA_HOME is set")
	}

	// validate .kantra directory and its contents
	kantraDir := e.cfg.KantraDir
	requiredPaths := []string{
		kantraDir,
		filepath.Join(kantraDir, rulesetsSubdir),
		filepath.Join(kantraDir, javaBundlesSubdir),
		filepath.Join(kantraDir, jdtlsBinSubdir),
		filepath.Join(kantraDir, fernflowerJar),
	}
	for _, p := range requiredPaths {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			e.log.Error(err, "cannot open required path, ensure that container-less dependencies are installed")
			return err
		}
	}

	// generate provider configs now that validation passed
	e.configs = DefaultProviderConfig(ModeLocal, DefaultOptions{
		KantraDir:          e.cfg.KantraDir,
		Location:           e.cfg.Input,
		AnalysisMode:       e.cfg.AnalysisMode,
		DisableMavenSearch: e.cfg.DisableMavenSearch,
		MavenSettingsFile:  e.cfg.MavenSettingsFile,
		JvmMaxMem:          e.cfg.JvmMaxMem,
		ContextLines:       e.cfg.ContextLines,
		HTTPProxy:          e.cfg.HTTPProxy,
		HTTPSProxy:         e.cfg.HTTPSProxy,
		NoProxy:            e.cfg.NoProxy,
	})

	return nil
}

// Stop removes Eclipse language server temporary directories
// created during Java analysis.
func (e *localEnvironment) Stop(ctx context.Context) error {
	e.log.V(7).Info("removing language server dirs")
	for _, dir := range eclipseLSDirs {
		if err := os.RemoveAll(dir); err != nil {
			e.log.Error(err, "failed to delete temporary dir", "dir", dir)
			// continue cleanup even if one fails
		}
	}
	return nil
}

// ProviderConfigs returns ModeLocal provider configurations.
// Must be called after Start.
func (e *localEnvironment) ProviderConfigs() []analyzerprovider.Config {
	return e.configs
}

// Rules returns rule file/directory paths. For local mode, default
// rulesets are at kantraDir/rulesets on the host filesystem.
func (e *localEnvironment) Rules(userRules []string, enableDefaults bool) ([]string, error) {
	rules := make([]string, len(userRules))
	copy(rules, userRules)
	if enableDefaults {
		rulesetsPath := filepath.Join(e.cfg.KantraDir, rulesetsSubdir)
		rules = append(rules, rulesetsPath)
	}
	return rules, nil
}

// ExtraOptions returns no extra options for local mode.
// Path mappings and ignore-additional-builtin-configs are only
// relevant when providers run in containers.
func (e *localEnvironment) ExtraOptions(_ context.Context, _ bool) ExtraEnvironmentOptions {
	return ExtraEnvironmentOptions{}
}

// PostAnalysis is a no-op for local mode.
func (e *localEnvironment) PostAnalysis(_ context.Context) error {
	return nil
}
