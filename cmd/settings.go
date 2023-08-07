package cmd

import "github.com/codingconcepts/env"

var Settings = &Config{}

type Config struct {
	RuleSetPath string `env:"RULESET_PATH" default:"/opt/rulesets/"`
	CommandName string `env:"CMD_NAME" default:"kantra"`
}

func (c *Config) Load() error {
	err := env.Set(Settings)
	if err != nil {
		return err
	}
	return nil
}
