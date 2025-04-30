package provider

import (
	"fmt"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"path"
	"path/filepath"

	"github.com/konveyor/analyzer-lsp/provider"
)

type JavaProvider struct {
	config provider.Config
}

func (p *JavaProvider) GetConfigVolume(c ConfigInput) (provider.Config, error) {

	var mountPath = util.SourceMountPath
	// when input is a file, it means it's probably a binary
	// only java provider can work with binaries, all others
	// continue pointing to the directory instead of file
	if c.IsFileInput {
		mountPath = path.Join(util.SourceMountPath, filepath.Base(c.InputPath))
	}

	p.config = provider.Config{
		Name:    util.JavaProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", c.Port),
		InitConfig: []provider.InitConfig{
			{
				Location:     mountPath,
				AnalysisMode: provider.AnalysisMode(c.Mode),
				ProviderSpecificConfig: map[string]interface{}{
					"lspServerName":                 util.JavaProvider,
					"bundles":                       util.JavaBundlesLocation,
					"depOpenSourceLabelsFile":       "/usr/local/etc/maven.default.index",
					provider.LspServerPathConfigKey: "/jdtls/bin/jdtls",
				},
			},
		},
	}

	if c.MavenSettingsFile != "" {
		err := util.CopyFileContents(c.MavenSettingsFile, filepath.Join(c.TmpDir, "settings.xml"))
		if err != nil {
			c.Log.V(1).Error(err, "failed copying maven settings file", "path", c.MavenSettingsFile)
			return provider.Config{}, err
		}
		p.config.InitConfig[0].ProviderSpecificConfig["mavenSettingsFile"] = fmt.Sprintf("%s/%s", util.ConfigMountPath, "settings.xml")
	}
	if c.JvmMaxMem != "" {
		p.config.InitConfig[0].ProviderSpecificConfig["jvmMaxMem"] = c.JvmMaxMem
	}

	return p.config, nil
}
