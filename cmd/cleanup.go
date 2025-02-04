package cmd

import (
	"context"
	"os"
	"os/exec"
	"runtime"
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

func (c *AnalyzeCommandContext) RmNetwork(ctx context.Context) error {
	if c.networkName == "" {
		return nil
	}
	cmd := exec.CommandContext(
		ctx,
		Settings.ContainerBinary,
		"network",
		"rm", c.networkName)
	c.log.V(1).Info("removing container network",
		"network", c.networkName)
	return cmd.Run()
}

func (c *AnalyzeCommandContext) RmVolumes(ctx context.Context) error {
	if c.volumeName == "" {
		return nil
	}
	cmd := exec.CommandContext(
		ctx,
		Settings.ContainerBinary,
		"volume",
		"rm", c.volumeName)
	c.log.V(1).Info("removing created volume",
		"volume", c.volumeName)
	return cmd.Run()
}

func (c *AnalyzeCommandContext) RmProviderContainers(ctx context.Context) error {
	// if multiple provider containers, we need to remove the first created provider container last
	for i := len(c.providerContainerNames) - 1; i >= 0; i-- {
		con := c.providerContainerNames[i]
		// because we are using the --rm option when we start the provider container,
		// it will immediately be removed after it stops
		cmd := exec.CommandContext(
			ctx,
			Settings.ContainerBinary,
			"stop", con)
		c.log.V(1).Info("stopping provider container", "container", con)
		err := cmd.Run()
		if err != nil {
			c.log.V(1).Error(err, "failed to stop container",
				"container", con)
			continue
		}
	}
	return nil
}

func (a *analyzeCommand) cleanlsDirs() error {
	// TODO clean this up for windows
	// currently a perm issue with deleting these dirs
	if runtime.GOOS == "windows" {
		return nil
	}
	a.log.V(7).Info("removing language server dirs")
	// this assumes dirs created in wd
	lsDirs := []string{
		"org.eclipse.core.runtime",
		"org.eclipse.equinox.app",
		"org.eclipse.equinox.launcher",
		"org.eclipse.osgi",
	}
	for _, path := range lsDirs {
		err := os.RemoveAll(path)
		if err != nil {
			a.log.Error(err, "failed to delete temporary dir", "dir", path)
			continue
		}
	}
	return nil
}
