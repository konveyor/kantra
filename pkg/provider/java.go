package provider

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/util"

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

	providerSpecificConfig := map[string]interface{}{
		"lspServerName":                 util.JavaProvider,
		"bundles":                       c.JavaBundleLocation,
		"mavenIndexPath":                "/usr/local/etc/maven-index.txt",
		"depOpenSourceLabelsFile":       "/usr/local/etc/maven.default.index",
		provider.LspServerPathConfigKey: "/jdtls/bin/jdtls",
		"disableMavenSearch":            c.DisableMavenSearch,
	}
	if excludedDir := util.GetProfilesExcludedDir(c.InputPath, true); excludedDir != "" {
		providerSpecificConfig["excludedDirs"] = []interface{}{excludedDir}
	}

	p.config = provider.Config{
		Name:    util.JavaProvider,
		Address: fmt.Sprintf("0.0.0.0:%v", c.Port),
		InitConfig: []provider.InitConfig{
			{
				Location:               mountPath,
				AnalysisMode:           provider.AnalysisMode(c.Mode),
				ProviderSpecificConfig: providerSpecificConfig,
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

// assume we always want to exclude /target/ in Java projects to avoid duplicate incidents
func WalkJavaPathForTarget(log logr.Logger, isFileInput bool, root string) ([]interface{}, error) {
	var targetPaths []interface{}
	var err error
	if isFileInput {
		originalRoot := root
		root, err = GetJavaBinaryProjectDir(filepath.Dir(root))
		if err != nil {
			return nil, err
		}
		// For JAR files, wait for "target" folder to be created during decompilation
		// WAR and EAR files are just exploded (extracted), not compiled, so they don't create a target folder
		ext := filepath.Ext(originalRoot)
		if ext == util.JavaArchive || ext == util.ClassFile {
			// Use 2 minute timeout for large binaries
			err = WaitForTargetDir(log, root, 2*time.Minute)
			if err != nil {
				return nil, err
			}
		}
		// For WAR/EAR files, wait for decompilation to complete by checking for exploded directory
		if ext == util.WebArchive || ext == util.EnterpriseArchive {
			err = WaitForDecompilation(log, root, 2*time.Minute)
			if err != nil {
				return nil, err
			}
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

func GetJavaBinaryProjectDir(root string) (string, error) {
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

func WaitForTargetDir(log logr.Logger, path string, timeout time.Duration) error {
	targetPath := filepath.Join(path, "target")

	// Check if target dir already exists
	if _, err := os.Stat(targetPath); err == nil {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	err = watcher.Add(path)
	if err != nil {
		return err
	}

	// Check again after watcher is set up to close the race window
	// The target directory might have been created between our first check
	// and setting up the watcher
	if _, err := os.Stat(targetPath); err == nil {
		return nil
	}

	log.Info("waiting for target directory in decompiled Java project")
	timeoutChan := time.After(timeout)

	for {
		select {
		case event := <-watcher.Events:
			if event.Op&fsnotify.Create == fsnotify.Create {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() && event.Name == targetPath {
					log.Info("target sub-folder detected", "folder", event.Name)
					return nil
				}
			}
		case err := <-watcher.Errors:
			return err
		case <-timeoutChan:
			return fmt.Errorf("timeout waiting for target folder after %v", timeout)
		}
	}
}

// WaitForDecompilation waits for WAR/EAR decompilation to complete
// by checking for the presence of any subdirectory in the java-project directory
func WaitForDecompilation(log logr.Logger, path string, timeout time.Duration) error {
	// Check if any subdirectory already exists (decompilation complete)
	entries, err := os.ReadDir(path)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				return nil // Decompilation already complete
			}
		}
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	err = watcher.Add(path)
	if err != nil {
		return err
	}

	// Check again after watcher is set up to close the race window
	entries, err = os.ReadDir(path)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				return nil
			}
		}
	}

	log.Info("waiting for WAR/EAR decompilation to complete")
	timeoutChan := time.After(timeout)

	for {
		select {
		case event := <-watcher.Events:
			if event.Op&fsnotify.Create == fsnotify.Create {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					log.Info("decompiled directory detected", "folder", event.Name)
					return nil
				}
			}
		case err := <-watcher.Errors:
			return err
		case <-timeoutChan:
			return fmt.Errorf("timeout waiting for decompilation after %v", timeout)
		}
	}
}
