/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"log"
	"math"
	"os"

	"github.com/bombsimon/logrusr/v3"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	noCleanupFlag = "no-cleanup"
)

var logLevel uint32
var noCleanup bool

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	// TODO: better descriptions
	Short:        "A cli tool for analysis and transformation of applications",
	Long:         ``,
	SilenceUsage: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logrusLog := logrus.New()
		logrusLog.SetOutput(os.Stdout)
		logrusLog.SetFormatter(&logrus.TextFormatter{})
		logrusLog.SetLevel(logrus.Level(uint32(math.Min(6, float64(logLevel)))))
		log := logrusr.New(logrusLog)
		cmd.AddCommand(NewTransformCommand(log))
		cmd.AddCommand(NewAnalyzeCmd(log))
	},
}

func init() {
	rootCmd.PersistentFlags().Uint32Var(&logLevel, "log-level", 3, "log level")
	rootCmd.PersistentFlags().BoolVar(&noCleanup, noCleanupFlag, false, "log level")
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := Settings.Load()
	if err != nil {
		log.Fatal(err, "failed to load global settings")
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	rootCmd.Use = Settings.RootCommandName
	err = rootCmd.ExecuteContext(ctx)
	if err != nil {
		os.Exit(1)
	}
}
