package analyze

import (
	"fmt"
	"path"

	"github.com/konveyor-ecosystem/kantra/pkg/container"
)

func (a *analyzeCommand) getDepsFolders() (map[string]string, []string) {
	vols := map[string]string{}
	dependencyFolders := []string{}
	if len(a.depFolders) != 0 {
		for i := range a.depFolders {
			newDepPath := path.Join(container.InputPath, fmt.Sprintf("deps%v", i))
			vols[a.depFolders[i]] = newDepPath
			dependencyFolders = append(dependencyFolders, newDepPath)
		}
		return vols, dependencyFolders
	}

	return vols, dependencyFolders
}
