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

type dumpRulesCommand struct {
	output    string
	overwrite bool
	log       logr.Logger
}

func NewDumpRulesCommand(log logr.Logger) *cobra.Command {
	dumpRulesCmd := &dumpRulesCommand{
		log: log,
	}

	dumpRulesCommand := &cobra.Command{
		Use:   "dump-rules",
		Short: "Dump builtin rulesets",
		RunE: func(cmd *cobra.Command, args []string) error {
			dumpRulesCmd.output = filepath.Join(dumpRulesCmd.output, "default-rulesets.zip")
			err := dumpRulesCmd.handleOutputFile()
			if err != nil {
				return err
			}

			kantraDir, err := util.GetKantraDir()
			if err != nil {
				log.Error(err, "unable to get kantra dir")
				return err
			}

			rulesPath := filepath.Join(kantraDir, RulesetsLocation)
			if _, err := os.Stat(rulesPath); os.IsNotExist(err) {
				log.Error(err, "cannot open rulesets path")
				return nil
			}

			log.Info("rulesets dir found", "dir", rulesPath)
			log.Info("dumping rules")

			file, err := os.Create(dumpRulesCmd.output)
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
			log.Info("rulesets exported successfully ", "file", dumpRulesCmd.output)
			return nil
		},
	}
	dumpRulesCommand.Flags().BoolVar(&dumpRulesCmd.overwrite, "overwrite", false, "overwrite output directory")
	dumpRulesCommand.Flags().StringVarP(&dumpRulesCmd.output, "output", "o", "", "path to the directory for rulesets output")
	err := dumpRulesCommand.MarkFlagRequired("output")
	if err != nil {
		return nil
	}

	return dumpRulesCommand
}

func (d *dumpRulesCommand) handleOutputFile() error {
	stat, err := os.Stat(d.output)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	if !d.overwrite && stat != nil {
		return fmt.Errorf("output file %v already exists and --overwrite not set", d.output)
	}

	if d.overwrite && stat != nil {
		err := os.RemoveAll(d.output)
		if err != nil {
			return err
		}
	}
	return nil
}
