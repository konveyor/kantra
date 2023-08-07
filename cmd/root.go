/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	// TODO: better descriptions
	Short: "A cli tool for analysis and transformation of applications",
	Long:  ``,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := Settings.Load()
	if err != nil {
		log.Fatal("failed to load global settings")
	}

	rootCmd.Use = Settings.CommandName
	err = rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
