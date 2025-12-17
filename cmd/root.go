/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor-ecosystem/kantra/cmd/asset_generation/discover"
	"github.com/konveyor-ecosystem/kantra/cmd/asset_generation/generate"
	"github.com/konveyor-ecosystem/kantra/cmd/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	noCleanupFlag = "no-cleanup"
	logLevelFlag  = "log-level"
)

var logLevel uint32
var logrusLog *logrus.Logger
var noCleanup bool

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Short:        "A CLI tool for analysis and transformation of applications",
	Long:         ``,
	SilenceUsage: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Parse flags and only ignore errors during test execution
		err := cmd.ParseFlags(args)
		if err != nil {
			// Only silently ignore errors during testing when Go passes test-specific
			// flags like --test.v that our command doesn't recognize
			if testing.Testing() {
				// The logLevel will use its default value (4) if parsing fails in tests
				return
			}
			// In production, report the error to the user
			log.Fatalf("Error parsing flags: %v", err)
		}
		// TODO (pgaikwad): this is a hack to set log level
		// this won't work if any subcommand overrides this func
		logrusLog.SetLevel(logrus.Level(logLevel))
	},
}

func init() {
	rootCmd.PersistentFlags().Uint32Var(&logLevel, logLevelFlag, 4, "log level")
	rootCmd.PersistentFlags().BoolVar(&noCleanup, noCleanupFlag, false, "do not cleanup temporary resources")

	logrusLog = logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})

	assertGenerationGroup := cobra.Group{
		ID:    "assetGeneration",
		Title: "Asset Generation",
	}
	rootCmd.AddGroup(&assertGenerationGroup)

	logger := logrusr.New(logrusLog)
	rootCmd.AddCommand(NewTransformCommand(logger))
	rootCmd.AddCommand(NewAnalyzeCmd(logger))
	rootCmd.AddCommand(NewTestCommand(logger))
	rootCmd.AddCommand(NewDumpRulesCommand(logger))
	rootCmd.AddCommand(NewVersionCommand())
	rootCmd.AddCommand(discover.NewDiscoverCommand(logger))
	rootCmd.AddCommand(generate.NewGenerateCommand(logger))
	rootCmd.AddCommand(config.NewConfigCmd(logger))
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := Settings.Load()
	if err != nil {
		log.Fatal(err, "failed to load global settings")
		os.Exit(1)
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	rootCmd.Use = Settings.RootCommandName
	err = rootCmd.ExecuteContext(ctx)
	if err != nil {
		os.Exit(1)
	}
}
