package cmd

import (
	"context"
	"os"
	"os/exec"
)

func (a *analyzeCommand) CleanAnalysisResources(ctx context.Context) error {
	if !a.cleanup || a.needsBuiltin {
		return nil
	}
	a.log.V(1).Info("removing temp dirs")
	for _, path := range a.tempDirs {
		err := os.RemoveAll(path)
		if err != nil {
			a.log.V(1).Error(err, "failed to delete temporary dir", "dir", path)
			continue
		}
	}
	err := a.RmProviderContainers(ctx)
	if err != nil {
		a.log.Error(err, "failed to remove provider container")
	}
	err = a.RmNetwork(ctx)
	if err != nil {
		a.log.Error(err, "failed to remove network", "network", a.networkName)
	}
	err = a.RmVolumes(ctx)
	if err != nil {
		a.log.Error(err, "failed to remove volume", "volume", a.volumeName)
	}
	return nil
}

func (a *analyzeCommand) RmNetwork(ctx context.Context) error {
	if a.networkName == "" {
		return nil
	}
	a.log.V(1).Info("removing container network",
		"network", a.networkName)
	cmd := exec.CommandContext(
		ctx,
		Settings.PodmanBinary,
		"network",
		"rm", a.networkName)
	a.log.V(1).Info("removing container network",
		"network", a.networkName)
	return cmd.Run()
}

func (a *analyzeCommand) RmVolumes(ctx context.Context) error {
	if a.volumeName == "" {
		return nil
	}
	cmd := exec.CommandContext(
		ctx,
		Settings.PodmanBinary,
		"volume",
		"rm", a.volumeName)
	a.log.V(1).Info("removing created volume",
		"volume", a.volumeName)
	return cmd.Run()
}

func (a *analyzeCommand) RmProviderContainers(ctx context.Context) error {
	// if multiple provider containers, we need to remove the first created provider container last
	for i := len(a.providerContainerNames) - 1; i >= 0; i-- {
		con := a.providerContainerNames[i]
		// because we are using the --rm option when we start the provider container,
		// it will immediately be removed after it stops
		cmd := exec.CommandContext(
			ctx,
			Settings.PodmanBinary,
			"stop", con)
		a.log.V(1).Info("stopping provider container", "container", con)
		err := cmd.Run()
		if err != nil {
			a.log.V(1).Error(err, "failed to stop container",
				"container", con)
			continue
		}
	}
	return nil
}
