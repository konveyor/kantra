package cmd

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/konveyor/analyzer-lsp/provider"
)

type JavaProvider struct {
	config provider.Config
}

func (p *JavaProvider) GetConfigVolume(a *analyzeCommand, tmpDir string) (provider.Config, error) {

	var mountPath = SourceMountPath
	// when input is a file, it means it's probably a binary
	// only java provider can work with binaries, all others
	// continue pointing to the directory instead of file
	if a.isFileInput {
		mountPath = path.Join(SourceMountPath, filepath.Base(a.input))
	}

	p.config = provider.Config{
		Name:    javaProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", a.providersMap[javaProvider].port),
		InitConfig: []provider.InitConfig{
			{
				Location:     mountPath,
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
		err := CopyFileContents(a.mavenSettingsFile, filepath.Join(tmpDir, "settings.xml"))
		if err != nil {
			a.log.V(1).Error(err, "failed copying maven settings file", "path", a.mavenSettingsFile)
			return provider.Config{}, err
		}
		p.config.InitConfig[0].ProviderSpecificConfig["mavenSettingsFile"] = fmt.Sprintf("%s/%s", ConfigMountPath, "settings.xml")
	}
	if Settings.JvmMaxMem != "" {
		p.config.InitConfig[0].ProviderSpecificConfig["jvmMaxMem"] = Settings.JvmMaxMem
	}

	return p.config, nil
}
