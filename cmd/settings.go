package cmd

import (
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
)

type Config struct {
	RootCommandName string `env:"CMD_NAME" default:"kantra"`
	PodmanBinary    string `env:"PODMAN_BIN" default:"/usr/bin/podman"`
	RunnerImage     string `env:"RUNNER_IMG" default:"quay.io/konveyor/kantra"`
	JvmMaxMem       string `env:"JVM_MAX_MEM" default:""`
}

func (c *Config) Load() error {
	envValue := os.Getenv("PODMAN_BIN")
	if envValue == "" {
		podmanPath, _ := exec.LookPath("podman")
		if podmanPath != c.PodmanBinary && (podmanPath != "" || len(podmanPath) > 0) {
			os.Setenv("PODMAN_BIN", podmanPath)
		}
	}

	err := env.Set(c)
	if err != nil {
		return err
	}
	return nil
}
