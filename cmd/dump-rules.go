package cmd

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/spf13/cobra"
)

var (
	output    string
	overwrite bool
)

func NewDumpRulesCommand(log logr.Logger) *cobra.Command {
	dumpRulesCmd := &cobra.Command{
		Use:   "dump-rules",
		Short: "Dump builtin rulesets",
		RunE: func(cmd *cobra.Command, args []string) error {
			output = filepath.Join(output, "default-rulesets.zip")
			err := handleOutputFile(output)
			if err != nil {
				return err
			}

			kantraDir, err := util.GetKantraDir()
			if err != nil {
				log.Error(err, "unable to get kantra dir")
				return err
			}

			rulesPath := filepath.Join(kantraDir, "rulesets")
			if _, err := os.Stat(rulesPath); os.IsNotExist(err) {
				log.Error(err, "cannot open rulesets path")
				return nil
			}

			log.Info("rulesets dir found", "dir", rulesPath)
			log.Info("dumping rules")

			file, err := os.Create(output)
			if err != nil {
				log.Error(err, "error while creating output file")
				return err
			}
			defer func(file *os.File) {
				err := file.Close()
				if err != nil {
					log.Error(err, "an error occurred while closing the output file")
				}
			}(file)

			w := zip.NewWriter(file)
			defer func(w *zip.Writer) {
				err := w.Close()
				if err != nil {
					log.Error(err, "an error occurred while closing the walker")
				}
			}(w)

			walker := func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				file, err := os.Open(path)
				if err != nil {
					return err
				}

				defer func(file *os.File) {
					err := file.Close()
					if err != nil {
						log.Error(err, "an error occurred while closing a file")
						return
					}
				}(file)

				relPath, err := filepath.Rel(rulesPath, path)
				if err != nil {
					return err
				}

				f, err := w.Create(relPath)
				if err != nil {
					return err
				}

				_, err = io.Copy(f, file)
				if err != nil {
					return err
				}

				return nil
			}
			err = filepath.WalkDir(rulesPath, walker)
			if err != nil {
				log.Error(err, "error while exporting rules")
				return err
			}

			return nil
		},
	}
	dumpRulesCmd.Flags().BoolVar(&overwrite, "overwrite", false, "overwrite output directory")
	dumpRulesCmd.Flags().StringVarP(&output, "output", "o", "", "path to the directory for rulesets output")
	err := dumpRulesCmd.MarkFlagRequired("output")
	if err != nil {
		return nil
	}

	return dumpRulesCmd
}

func handleOutputFile(output string) error {
	stat, err := os.Stat(output)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	if !overwrite && stat != nil {
		return fmt.Errorf("output file %v already exists and --overwrite not set", output)
	}

	if overwrite && stat != nil {
		err := os.RemoveAll(output)
		if err != nil {
			return err
		}
	}
	return nil
}
