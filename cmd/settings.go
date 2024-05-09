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

const (
	RulesetPath            = "/opt/rulesets"
	OpenRewriteRecipesPath = "/opt/openrewrite"
	InputPath              = "/opt/input"
	OutputPath             = "/opt/output"
	XMLRulePath            = "/opt/xmlrules"
	ShimOutputPath         = "/opt/shimoutput"
	CustomRulePath         = "/opt/input/rules"
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
		podmanPath, err := exec.LookPath("podman")
		if err != nil && errors.Is(err, exec.ErrDot) {
			return err
		}
		if podmanPath != c.PodmanBinary && (podmanPath != "" || len(podmanPath) > 0) {
			os.Setenv("PODMAN_BIN", podmanPath)
		}
	}
	if err := c.loadRunnerImg(); err != nil {
		return err
	}
	if err := c.loadCommandName(); err != nil {
		return err
	}
	err := env.Set(c)
	if err != nil {
		return err
	}
	return nil
}

func (c *Config) loadRunnerImg() error {
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
