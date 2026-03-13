package provider

import (
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/util"

	"github.com/konveyor/analyzer-lsp/provider"
)

// JavaProvider generates configuration for the Java analyzer provider.
// Java-specific options (maven settings, JVM memory, bundles) are
// passed through BaseOptions, keeping the provider struct stateless.
type JavaProvider struct {
	baseProvider
}

func (p *JavaProvider) Name() string {
	return util.JavaProvider
}

func (p *JavaProvider) GetConfig(mode ExecutionMode, opts BaseOptions, extra ...ProviderOption) (provider.Config, error) {
	javaOpts, _ := FindOption[JavaOptions](extra)

	// Resolve binary path based on mode
	switch mode {
	case ModeContainer:
		opts.BinaryPath = ContainerJavaProviderBin
	case ModeLocal:
		opts.BinaryPath = filepath.Join(opts.KantraDir, "java-external-provider")
	}

	cfg := NewBaseConfig(util.JavaProvider, mode, opts)
	psc := cfg.InitConfig[0].ProviderSpecificConfig

	// Java-specific config varies by mode
	switch mode {
	case ModeContainer:
		psc["lspServerName"] = util.JavaProvider
		psc["bundles"] = ContainerJavaBundlePath
		psc["depOpenSourceLabelsFile"] = ContainerDepOpenSourceLabels
		psc[provider.LspServerPathConfigKey] = ContainerJDTLSPath

	case ModeLocal:
		kantraDir := opts.KantraDir
		psc["lspServerName"] = util.JavaProvider
		psc["bundles"] = filepath.Join(kantraDir, LocalJavaBundlePath)
		psc[provider.LspServerPathConfigKey] = filepath.Join(kantraDir, LocalJDTLSPath)
		psc["depOpenSourceLabelsFile"] = filepath.Join(kantraDir, "maven.default.index")
		psc["mavenIndexPath"] = kantraDir
		psc["cleanExplodedBin"] = true
		psc["fernFlowerPath"] = filepath.Join(kantraDir, "fernflower.jar")
		psc["gradleSourcesTaskFile"] = filepath.Join(kantraDir, "task.gradle")
		psc["disableMavenSearch"] = javaOpts.DisableMavenSearch

	case ModeNetwork:
		psc["lspServerName"] = util.JavaProvider
		psc[provider.LspServerPathConfigKey] = ContainerJDTLSPath
		psc["bundles"] = ContainerJavaBundlePath
		psc["mavenIndexPath"] = ContainerMavenIndexPath
		psc["depOpenSourceLabelsFile"] = ContainerDepOpenSourceLabels
	}

	// Apply Java-specific overrides from options
	if javaOpts.MavenSettingsFile != "" {
		psc["mavenSettingsFile"] = javaOpts.MavenSettingsFile
	}
	if javaOpts.JvmMaxMem != "" {
		psc["jvmMaxMem"] = javaOpts.JvmMaxMem
	}

	return cfg, nil
}

// assume we always want to exclude /target/ in Java projects to avoid duplicate incidents
func WalkJavaPathForTarget(log logr.Logger, isFileInput bool, root string) ([]interface{}, error) {
	var targetPaths []interface{}
	var err error
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() == "target" {
			targetPaths = append(targetPaths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return targetPaths, nil
}
