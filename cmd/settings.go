package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/codingconcepts/env"
)

var Settings = &Config{}

type Config struct {
	RootCommandName      string `env:"CMD_NAME" default:"kantra"`
	ContainerBinary      string `env:"CONTAINER_TOOL" default:"/usr/bin/podman"`
	RunnerImage          string `env:"RUNNER_IMG" default:"quay.io/konveyor/kantra"`
	RunLocal             bool   `env:"RUN_LOCAL"`
	JavaProviderImage    string `env:"JAVA_PROVIDER_IMG" default:"quay.io/konveyor/java-external-provider:latest"`
	GenericProviderImage string `env:"GENERIC_PROVIDER_IMG" default:"quay.io/konveyor/generic-external-provider:latest"`
	DotnetProviderImage  string `env:"DOTNET_PROVIDER_IMG" default:"quay.io/konveyor/dotnet-external-provider:latest"`
}

func (c *Config) Load() error {
	if err := c.loadDefaultPodmanBin(); err != nil {
		return err
	}
	if err := c.loadRunnerImg(); err != nil {
		return err
	}
	if err := c.loadCommandName(); err != nil {
		return err
	}
	if err := c.loadProviders(); err != nil {
		return err
	}
	err := env.Set(c)
	if err != nil {
		return err
	}
	return nil
}

func (c *Config) loadDefaultPodmanBin() error {
	// Respect existing CONTAINER_TOOL setting.
	if os.Getenv("CONTAINER_TOOL") != "" {
		return nil
	}
	podmanBin := os.Getenv("PODMAN_BIN")
	if podmanBin != "" {
		os.Setenv("CONTAINER_TOOL", podmanBin)
		return nil
	}
	// Try to use podman. If it's not found, try to use docker.
	found, err := c.trySetDefaultPodmanBin("podman")
	if err != nil {
		return err
	}
	if !found {
		if _, err = c.trySetDefaultPodmanBin("docker"); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) trySetDefaultPodmanBin(file string) (found bool, err error) {
	path, err := exec.LookPath(file)
	// Ignore all errors other than ErrDot.
	if err != nil && errors.Is(err, exec.ErrDot) {
		return false, err
	}
	// If file was found in PATH and it's not already going to be used, specify it in the env var.
	if path != "" && path != c.ContainerBinary {
		os.Setenv("CONTAINER_TOOL", path)
		return true, nil
	}
	return false, nil
}

func (c *Config) loadRunnerImg() error {
	// TODO(maufart): ensure Config struct works/parses it values from ENV and defaults correctly
	// Respect existing RUNNER_IMG setting
	if os.Getenv("RUNNER_IMG") != "" {
		return nil
	}
	// if version tag is given in image
	img := strings.TrimSuffix(RunnerImage, fmt.Sprintf(":%v", Version))
	updatedImg := fmt.Sprintf("%v:%v", img, Version)
	err := os.Setenv("RUNNER_IMG", updatedImg)
	if err != nil {
		return err
	}

	return nil
}

func (c *Config) loadCommandName() error {
	if RootCommandName != "kantra" {
		err := os.Setenv("CMD_NAME", RootCommandName)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) loadProviders() error {
	// if version tag is given in image
	if os.Getenv("JAVA_PROVIDER_IMG") == "" {
		javaImg := strings.TrimSuffix(JavaProviderImage, fmt.Sprintf(":%v", Version))
		updatedJavaImg := fmt.Sprintf("%v:%v", javaImg, Version)
		err := os.Setenv("JAVA_PROVIDER_IMG", updatedJavaImg)
		if err != nil {
			return err
		}
	}

	if os.Getenv("GENERIC_PROVIDER_IMG") == "" {
		// if version tag is given in image
		genericImg := strings.TrimSuffix(GenericProviderImage, fmt.Sprintf(":%v", Version))
		updatedGenericImg := fmt.Sprintf("%v:%v", genericImg, Version)
		err := os.Setenv("GENERIC_PROVIDER_IMG", updatedGenericImg)
		if err != nil {
			return err
		}
	}

	if os.Getenv("DOTNET_PROVIDER_IMG") == "" {
		// if version tag is given in image
		dotnetImg := strings.TrimSuffix(DotnetProviderImage, fmt.Sprintf(":%v", Version))
		updatedDotnetImg := fmt.Sprintf("%v:%v", dotnetImg, Version)
		err := os.Setenv("DOTNET_PROVIDER_IMG", updatedDotnetImg)
		if err != nil {
			return err
		}
	}

	return nil
}
