package cmd

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
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

// assume we always want to exclude /target/ in Java projects to avoid duplicate incidents
func (a *analyzeCommand) walkJavaPathForTarget(root string) ([]interface{}, error) {
	var targetPaths []interface{}
	var err error
	if a.isFileInput {
		root, err = a.getJavaBinaryProjectDir(filepath.Dir(root))
		if err != nil {
			return nil, err
		}
		// for binaries, wait for "target" folder to decompile
		err = a.waitForTargetDir(root)
		if err != nil {
			return nil, err
		}
	}
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

func (a *analyzeCommand) getJavaBinaryProjectDir(root string) (string, error) {
	var foundDir string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && strings.Contains(info.Name(), "java-project-") {
			foundDir = path
			return nil
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	return foundDir, nil
}

func (a *analyzeCommand) waitForTargetDir(path string) error {
	// worst case we timeout
	// may need to increase
	timeout := 20 * time.Second
	timeoutChan := time.After(timeout)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()
	err = watcher.Add(path)
	if err != nil {
		return err
	}
	a.log.V(7).Info("waiting for target directory in decompiled Java project")

	for {
		select {
		case event := <-watcher.Events:
			if event.Op&fsnotify.Create == fsnotify.Create {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() && event.Name == filepath.Join(path, "target") {
					a.log.Info("target sub-folder detected:", "folder", event.Name)
					return nil
				}
			}
		case err := <-watcher.Errors:
			return err
		case <-timeoutChan:
			return fmt.Errorf("timeout waiting for target folder")
		}
	}
}
