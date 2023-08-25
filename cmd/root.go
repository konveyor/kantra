/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"log"
	"os"

	"github.com/bombsimon/logrusr/v3"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	// TODO: better descriptions
	Short:        "A cli tool for analysis and transformation of applications",
	Long:         ``,
	SilenceUsage: true,
}

var logLevel int

func init() {
	rootCmd.PersistentFlags().IntVar(&logLevel, "log-level", 5, "log level")

	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	logrusLog.SetLevel(logrus.InfoLevel)
	log := logrusr.New(logrusLog)

	rootCmd.AddCommand(NewTransformCommand(log))
	rootCmd.AddCommand(NewAnalyzeCmd(log))
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := Settings.Load()
	if err != nil {
		log.Fatal("failed to load global settings")
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	rootCmd.Use = Settings.RootCommandName
	err = rootCmd.ExecuteContext(ctx)
	if err != nil {
		os.Exit(1)
	}
}
