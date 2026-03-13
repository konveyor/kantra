package analyze

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/konveyor/analyzer-lsp/engine"

	"github.com/konveyor-ecosystem/kantra/cmd/internal/hiddenfile"
	"gopkg.in/yaml.v2"
)

func (a *analyzeCommand) getRulesVolumes() (map[string]string, error) {
	if a.rules == nil || len(a.rules) == 0 {
		return nil, nil
	}
	rulesVolumes := make(map[string]string)
	tempDir, err := os.MkdirTemp("", "analyze-rules-")
	if err != nil {
		a.log.V(1).Error(err, "failed to create temp dir", "path", tempDir)
		return nil, err
	}
	a.tempRuleDir = filepath.Base(tempDir)
	a.log.V(1).Info("created directory for rules", "dir", tempDir)
	a.tempDirs = append(a.tempDirs, tempDir)
	for i, r := range a.rules {
		stat, err := os.Stat(r)
		if err != nil {
			a.log.V(1).Error(err, "failed to stat rules", "path", r)
			return nil, err
		}
		// move rules files passed into dir to mount
		if !stat.IsDir() {
			destFile := filepath.Join(tempDir, fmt.Sprintf("rules%d.yaml", i))
			err := util.CopyFileContents(r, destFile)
			if err != nil {
				a.log.V(1).Error(err, "failed to move rules file", "src", r, "dest", destFile)
				return nil, err
			}
			a.log.V(5).Info("copied file to rule dir", "added file", r, "destFile", destFile)
			err = a.createTempRuleSet(tempDir, "custom-ruleset")
			if err != nil {
				return nil, err
			}
		} else {
			a.log.V(5).Info("copying dir", "directory", r)
			err = filepath.WalkDir(r, func(path string, d fs.DirEntry, err error) error {
				if path == r {
					return nil
				}
				if d.IsDir() {
					// This will create the new dir
					if err := a.handleDir(path, tempDir, r); err != nil {
						return err
					}
				} else {
					// If we are unable to get the file attributes, probably safe to assume this is not a
					// valid rule or ruleset and lets skip it for now.
					if isHidden, err := hiddenfile.IsHidden(path); isHidden || err != nil {
						a.log.V(5).Info("skipping hidden file", "path", path, "error", err)
						return nil
					}
					relpath, err := filepath.Rel(r, path)
					if err != nil {
						return err
					}
					destFile := filepath.Join(tempDir, relpath)
					a.log.V(5).Info("copying file main", "source", path, "dest", destFile)
					err = util.CopyFileContents(path, destFile)
					if err != nil {
						a.log.V(1).Error(err, "failed to move rules file", "src", r, "dest", destFile)
						return err
					}
				}
				return nil
			})
			if err != nil {
				a.log.V(1).Error(err, "failed to move rules file", "src", r)
				return nil, err
			}
		}
	}
	rulesVolumes[tempDir] = path.Join(util.CustomRulePath, filepath.Base(tempDir))

	return rulesVolumes, nil
}

func (c *AnalyzeCommandContext) handleDir(p string, tempDir string, basePath string) error {
	newDir, err := filepath.Rel(basePath, p)
	if err != nil {
		return err
	}
	tempDir = filepath.Join(tempDir, newDir)
	c.log.Info("creating nested tmp dir", "tempDir", tempDir, "newDir", newDir)
	err = os.Mkdir(tempDir, 0777)
	if err != nil {
		return err
	}
	c.log.V(5).Info("create temp rule set for dir", "dir", tempDir)
	err = c.createTempRuleSet(tempDir, filepath.Base(p))
	if err != nil {
		c.log.V(1).Error(err, "failed to create temp ruleset", "path", tempDir)
		return err
	}
	return err
}

func (c *AnalyzeCommandContext) createTempRuleSet(path string, name string) error {
	c.log.Info("creating temp ruleset ", "path", path, "name", name)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
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
	err = os.WriteFile(filepath.Join(path, "ruleset.yaml"), yamlData, os.ModePerm)
	if err != nil {
		return err
	}
	return nil
}
