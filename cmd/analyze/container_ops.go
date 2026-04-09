package analyze

import (
	"fmt"
	"path"
)

var (
	// TODO (pgaikwad): this assumes that the $USER in container is always root, it may not be the case in future
	M2Dir = path.Join("/", "root", ".m2")
	// SourceMountPath is the directory where source code is mounted in the container.
	// This value must not be modified at runtime. Use analyzeCommand.sourceLocationPath
	// for the resolved source location (which may include a filename for file inputs).
	SourceMountPath = path.Join(InputPath, "source")
	// ConfigMountPath analyzer config files
	ConfigMountPath = path.Join(InputPath, "config")
	// RulesMountPath user provided rules path
	RulesMountPath = path.Join(RulesetPath, "input")
	// AnalysisOutputMountPath paths to files in the container
	AnalysisOutputMountPath   = path.Join(OutputPath, "output.yaml")
	DepsOutputMountPath       = path.Join(OutputPath, "dependencies.yaml")
	ProviderSettingsMountPath = path.Join(ConfigMountPath, "settings.json")
)

// analyzer container paths
const (
	RulesetPath    = "/opt/rulesets"
	InputPath      = "/opt/input"
	OutputPath     = "/opt/output"
	CustomRulePath = "/opt/input/rules"
)

func (a *analyzeCommand) getDepsFolders() (map[string]string, []string) {
	vols := map[string]string{}
	dependencyFolders := []string{}
	if len(a.depFolders) != 0 {
		for i := range a.depFolders {
			newDepPath := path.Join(InputPath, fmt.Sprintf("deps%v", i))
			vols[a.depFolders[i]] = newDepPath
			dependencyFolders = append(dependencyFolders, newDepPath)
		}
		return vols, dependencyFolders
	}

	return vols, dependencyFolders
}
