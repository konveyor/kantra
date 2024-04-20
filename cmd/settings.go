package cmd

import (
	"errors"
	"os"
	"os/exec"

	"github.com/codingconcepts/env"
)

var Settings = &Config{}

const (
	RulesetPath            = "/opt/rulesets"
	OpenRewriteRecipesPath = "/opt/openrewrite"
	InputPath              = "/opt/input"
	OutputPath             = "/opt/output"
	XMLRulePath            = "/opt/xmlrules"
	ShimOutputPath         = "/opt/shimoutput"
	CustomRulePath         = "/opt/input/rules"
	JavaBundlesLocation    = "/jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar"
)

type Config struct {
	RootCommandName      string `env:"CMD_NAME" default:"kantra"`
	PodmanBinary         string `env:"PODMAN_BIN" default:"/usr/bin/podman"`
	RunnerImage          string `env:"RUNNER_IMG" default:"quay.io/konveyor/kantra"`
	JvmMaxMem            string `env:"JVM_MAX_MEM" default:""`
	RunLocal             bool   `env:"RUN_LOCAL"`
	JavaProviderImage    string `env:"JAVA_PROVIDER_IMG" default:"quay.io/konveyor/java-external-provider:latest"`
	GenericProviderImage string `env:"GENERIC_PROVIDER_IMG" default:"quay.io/konveyor/generic-external-provider:latest"`
	DotNetProviderImage  string `env:"DOTNET_PROVIDER_IMG" default:"quay.io/konveyor/dotnet-external-provider:latest"`
	YQProviderImage      string `env:"YQ_PROVIDER_IMG" default:"quay.io/konveyor/yq-external-provider:latest"`
}

func (c *Config) Load() error {
	if err := c.loadDefaultPodmanBin(); err != nil {
		return err
	}

	err := env.Set(c)
	if err != nil {
		return err
	}
	return nil
}

func (c *Config) loadDefaultPodmanBin() error {
	// Respect existing PODMAN_BIN setting.
	if os.Getenv("PODMAN_BIN") != "" {
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
	if path != "" && path != c.PodmanBinary {
		os.Setenv("PODMAN_BIN", path)
		return true, nil
	}
	return false, nil
}
