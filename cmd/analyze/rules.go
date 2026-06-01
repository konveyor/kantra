package analyze

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/cmd/internal/hiddenfile"
	"github.com/konveyor-ecosystem/kantra/pkg/container"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/engine"
	"gopkg.in/yaml.v2"
)

// PrepareRulesVolumes stages custom --rules paths for mounting into the runner container.
// customRuleDir is the basename of the temp dir (for RULE_PATH). volumes maps host dir to container mount.
func PrepareRulesVolumes(log logr.Logger, rules []string) (volumes map[string]string, customRuleDir string, err error) {
	if len(rules) == 0 {
		return nil, "", nil
	}
	tempDir, err := os.MkdirTemp("", "analyze-rules-")
	if err != nil {
		log.V(1).Error(err, "failed to create temp dir", "path", tempDir)
		return nil, "", err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(tempDir)
		}
	}()
	customRuleDir = filepath.Base(tempDir)
	log.V(1).Info("created directory for rules", "dir", tempDir)

	for i, r := range rules {
		stat, err := os.Stat(r)
		if err != nil {
			log.V(1).Error(err, "failed to stat rules", "path", r)
			return nil, "", err
		}
		if !stat.IsDir() {
			destFile := filepath.Join(tempDir, fmt.Sprintf("rules%d.yaml", i))
			if err := util.CopyFileContents(r, destFile); err != nil {
				log.V(1).Error(err, "failed to move rules file", "src", r, "dest", destFile)
				return nil, "", err
			}
			log.V(5).Info("copied file to rule dir", "added file", r, "destFile", destFile)
			if err := createTempRuleSet(log, tempDir, "custom-ruleset"); err != nil {
				return nil, "", err
			}
			continue
		}
		log.V(5).Info("copying dir", "directory", r)
		err = filepath.WalkDir(r, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path == r {
				return nil
			}
			if isHidden, hiddenErr := hiddenfile.IsHidden(path); hiddenErr != nil || isHidden {
				log.V(5).Info("skipping hidden path", "path", path, "error", hiddenErr)
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return copyRuleDir(log, path, tempDir, r)
			}
			relpath, err := filepath.Rel(r, path)
			if err != nil {
				return err
			}
			destFile := filepath.Join(tempDir, relpath)
			log.V(5).Info("copying file main", "source", path, "dest", destFile)
			if err := util.CopyFileContents(path, destFile); err != nil {
				log.V(1).Error(err, "failed to move rules file", "src", r, "dest", destFile)
				return err
			}
			return nil
		})
		if err != nil {
			log.V(1).Error(err, "failed to move rules file", "src", r)
			return nil, "", err
		}
	}

	volumes = map[string]string{
		tempDir: path.Join(container.CustomRulePath, customRuleDir),
	}
	return volumes, customRuleDir, nil
}

func (a *analyzeCommand) getRulesVolumes() (map[string]string, error) {
	volumes, customRuleDir, err := PrepareRulesVolumes(a.log, a.rules)
	if err != nil {
		return nil, err
	}
	if volumes == nil {
		return nil, nil
	}
	a.tempRuleDir = customRuleDir
	for hostPath := range volumes {
		a.tempDirs = append(a.tempDirs, hostPath)
	}
	return volumes, nil
}

func copyRuleDir(log logr.Logger, p string, tempDir string, basePath string) error {
	newDir, err := filepath.Rel(basePath, p)
	if err != nil {
		return err
	}
	nested := filepath.Join(tempDir, newDir)
	log.Info("creating nested tmp dir", "tempDir", nested, "newDir", newDir)
	if err := os.Mkdir(nested, 0777); err != nil {
		return err
	}
	log.V(5).Info("create temp rule set for dir", "dir", nested)
	return createTempRuleSet(log, nested, filepath.Base(p))
}

func createTempRuleSet(log logr.Logger, dir string, name string) error {
	log.Info("creating temp ruleset ", "path", dir, "name", name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
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
	return os.WriteFile(filepath.Join(dir, "ruleset.yaml"), yamlData, os.ModePerm)
}

func (c *AnalyzeCommandContext) handleDir(p string, tempDir string, basePath string) error {
	newDir, err := filepath.Rel(basePath, p)
	if err != nil {
		return err
	}
	tempDir = filepath.Join(tempDir, newDir)
	c.log.Info("creating nested tmp dir", "tempDir", tempDir, "newDir", newDir)
	if err := os.Mkdir(tempDir, 0777); err != nil {
		return err
	}
	c.log.V(5).Info("create temp rule set for dir", "dir", tempDir)
	if err := createTempRuleSet(c.log, tempDir, filepath.Base(p)); err != nil {
		c.log.V(1).Error(err, "failed to create temp ruleset", "path", tempDir)
		return err
	}
	return nil
}

func (c *AnalyzeCommandContext) createTempRuleSet(path string, name string) error {
	return createTempRuleSet(c.log, path, name)
}
