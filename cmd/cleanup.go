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
	// Remove source volume
	if c.volumeName != "" {
		cmd := exec.CommandContext(
			ctx,
			Settings.ContainerBinary,
			"volume",
			"rm", c.volumeName)
		c.log.V(1).Info("removing created volume",
			"volume", c.volumeName)
		if err := cmd.Run(); err != nil {
			c.log.V(1).Error(err, "failed to remove volume", "volume", c.volumeName)
		}
	}

	// NOTE: We do NOT remove the Maven cache volume here!
	// The Maven cache volume (maven-cache-volume) is designed to persist across
	// analysis runs to avoid re-downloading dependencies. Removing it would defeat
	// the purpose of caching. The volume maps to the host's ~/.m2/repository and
	// will be reused by subsequent analyses.
	//
	// To manually clean the Maven cache if needed:
	//   podman volume rm maven-cache-volume
	//
	// Or clean the host cache directory:
	//   rm -rf ~/.m2/repository

	return nil
}

func (c *AnalyzeCommandContext) RmProviderContainers(ctx context.Context) error {
	// if multiple provider containers, we need to remove the first created provider container last
	for _, con := range c.providerContainerNames {
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
		cmd = exec.CommandContext(
			ctx,
			Settings.ContainerBinary,
			"rm", con)
		c.log.V(1).Info("removing provider container", "container", con)
		err = cmd.Run()
		if err != nil {
			c.log.V(1).Error(err, "failed to remove container",
				"container", con)
			continue
		}
	}
	return nil
}

func (c *AnalyzeCommandContext) StopProvider(ctx context.Context, provider string) error {
	con, ok := c.providerContainerNames[provider]
	if !ok {
		return nil
	}
	cmd := exec.CommandContext(
		ctx,
		Settings.ContainerBinary,
		"stop", con)
	c.log.V(1).Info("stopping provider container", "container", con)
	err := cmd.Run()
	if err != nil {
		c.log.V(1).Error(err, "failed to stop container",
			"container", con)
		return err
	}
	cmd = exec.CommandContext(
		ctx,
		Settings.ContainerBinary,
		"rm", con)
	c.log.V(1).Info("removing provider container", "container", con)
	err = cmd.Run()
	if err != nil {
		c.log.V(1).Error(err, "failed to remove container",
			"container", con)
		return err
	}
	return nil
}

func (a *analyzeCommand) cleanlsDirs() error {
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
