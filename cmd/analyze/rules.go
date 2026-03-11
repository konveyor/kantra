package analyze

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/konveyor-ecosystem/kantra/cmd/internal/settings"
	"github.com/konveyor-ecosystem/kantra/pkg/container"
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

// extractDefaultRulesets extracts default rulesets from the kantra container to the host.
// This allows hybrid mode to use default rulesets without bundling them separately on the host.
//
// The function creates a temporary container from the runner image, copies the /opt/rulesets
// directory to the host at {output}/.rulesets-{version}/, and removes the temporary container.
// On subsequent runs with the same version, the cached rulesets are reused, avoiding the extraction overhead.
// The version suffix ensures cache invalidation when upgrading/downgrading kantra versions.
//
// Parameters:
//   - ctx: Context for container operations and cancellation
//   - containerLogWriter: Writer for container command output (typically analysis.log file)
//
// Returns:
//   - string: Path to the extracted rulesets directory, or empty string if disabled
//   - error: Any error encountered during container creation or file copying
func (a *analyzeCommand) extractDefaultRulesets(ctx context.Context, containerLogWriter io.Writer) (string, error) {
	if !a.enableDefaultRulesets {
		return "", nil
	}

	rulesetsDir := filepath.Join(a.output, fmt.Sprintf(".rulesets-%s", settings.Version))

	// Check if rulesets already extracted (cached from previous run)
	if _, err := os.Stat(rulesetsDir); os.IsNotExist(err) {
		a.log.Info("extracting default rulesets from container to host", "version", settings.Version, "dir", rulesetsDir)

		// Create temp container to extract rulesets
		tempName := fmt.Sprintf("ruleset-extract-%v", container.RandomName())
		createCmd := exec.CommandContext(ctx, settings.Settings.ContainerBinary,
			"create", "--name", tempName, settings.Settings.RunnerImage)
		// Send container output to log file instead of console
		createCmd.Stdout = containerLogWriter
		createCmd.Stderr = containerLogWriter
		if err := createCmd.Run(); err != nil {
			return "", fmt.Errorf("failed to create temp container for ruleset extraction: %w", err)
		}

		// Ensure temp container is removed
		defer func() {
			rmCmd := exec.CommandContext(ctx, settings.Settings.ContainerBinary, "rm", tempName)
			rmCmd.Run()
		}()

		// Copy rulesets from container to host
		copyCmd := exec.CommandContext(ctx, settings.Settings.ContainerBinary,
			"cp", fmt.Sprintf("%s:/opt/rulesets", tempName), rulesetsDir)
		// Send container output to log file instead of console
		copyCmd.Stdout = containerLogWriter
		copyCmd.Stderr = containerLogWriter
		if err := copyCmd.Run(); err != nil {
			return "", fmt.Errorf("failed to copy rulesets from container: %w", err)
		}

		a.log.Info("extracted default rulesets to host", "version", settings.Version, "dir", rulesetsDir)
	} else {
		a.log.V(1).Info("using cached default rulesets", "version", settings.Version, "dir", rulesetsDir)
	}

	return rulesetsDir, nil
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
