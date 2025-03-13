/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/logger"
	"log"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	noCleanupFlag = "no-cleanup"
	logLevelFlag  = "log-level"
)

var logLevel uint32
var analysisLogger *logr.Logger
var noCleanup bool

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Short:        "A CLI tool for analysis and transformation of applications",
	Long:         ``,
	SilenceUsage: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// TODO (pgaikwad): this is a hack to set log level
		// this won't work if any subcommand ovverrides this func
		_ = cmd.ParseFlags(args)
		logger.GetLogrus().SetLevel(logrus.Level(logLevel))
	},
}

func init() {
	rootCmd.PersistentFlags().Uint32Var(&logLevel, logLevelFlag, 4, "log level")
	rootCmd.PersistentFlags().BoolVar(&noCleanup, noCleanupFlag, false, "do not cleanup temporary resources")

	logger.InitLogger()
	analysisLogger = logger.GetLogger()
	rootCmd.AddCommand(NewTransformCommand())
	rootCmd.AddCommand(NewAnalyzeCmd())
	rootCmd.AddCommand(NewTestCommand())
	rootCmd.AddCommand(NewVersionCommand())
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
