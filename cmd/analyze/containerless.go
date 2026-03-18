package analyze

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/sirupsen/logrus"
	"go.lsp.dev/uri"
)

type ConsoleHook struct {
	Level logrus.Level
	Log   logr.Logger
}

func (hook *ConsoleHook) Fire(entry *logrus.Entry) error {
	_, err := entry.String()
	if err != nil {
		return nil // Ignore the error
	}

	if entry.Data["logger"] == "process-rule" {
		hook.Log.Info("processing rule", "ruleID", entry.Data["ruleID"])
	}
	return nil
}

func (hook *ConsoleHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// renderProgressBar renders a visual progress bar to stderr
func renderProgressBar(percent int, current, total int, message string) {
	const barWidth = 25
	filled := (percent * barWidth) / 100
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	// Truncate message if too long
	maxMessageLen := 40
	if len(message) > maxMessageLen {
		message = message[:maxMessageLen-3] + "..."
	}

	// Use \r to return to start of line and \033[K to clear to end of line
	fmt.Fprintf(os.Stderr, "\r\033[K  ✓ Processing rules %3d%% |%s| %d/%d  %s",
		percent, bar, current, total, message)
}

func (a *analyzeCommand) buildStaticReportFile(ctx context.Context, staticReportPath string, depsErr bool) error {
	if a.skipStaticReport {
		return nil
	}
	// Prepare report args list with single input analysis
	applicationNames := []string{filepath.Base(a.input)}
	outputAnalyses := []string{filepath.Join(a.output, "output.yaml")}
	outputDeps := []string{filepath.Join(a.output, "dependencies.yaml")}
	outputJSPath := filepath.Join(staticReportPath, "output.js")

	if a.bulk {
		// Scan all available analysis output files to be reported
		applicationNames = nil
		outputAnalyses = nil
		outputDeps = nil
		outputFiles, err := filepath.Glob(filepath.Join(a.output, "output.yaml.*"))
		if err != nil {
			return err
		}
		for i := range outputFiles {
			outputName := filepath.Base(outputFiles[i])
			applicationName := strings.SplitN(outputName, "output.yaml.", 2)[1]
			applicationNames = append(applicationNames, applicationName)
			outputAnalyses = append(outputAnalyses, outputFiles[i])
			deps := fmt.Sprintf("%s.%s", filepath.Join(a.output, "dependencies.yaml"), applicationName)
			// If deps for given application are missing, empty the deps path allowing skip it in static-report
			if _, err := os.Stat(deps); errors.Is(err, os.ErrNotExist) {
				deps = ""
			}
			outputDeps = append(outputDeps, deps)
		}

	}

	if depsErr {
		outputDeps = []string{}
	}
	// create output.js file from analysis output.yaml
	apps, err := validateFlags(outputAnalyses, applicationNames, outputDeps, a.log)
	if err != nil {
		return fmt.Errorf("failed to validate flags: %w", err)
	}

	err = loadApplications(apps)
	if err != nil {
		return fmt.Errorf("failed to load report data from analysis output: %w", err)
	}

	err = generateJSBundle(apps, outputJSPath, a.log)
	if err != nil {
		return fmt.Errorf("failed to generate output.js file from template: %w", err)
	}

	return nil
}

func (a *analyzeCommand) getStaticReportSrcPath() string {
	if a.staticReportPath != "" {
		return a.staticReportPath
	}
	return filepath.Join(a.kantraDir, "static-report")
}

func (a *analyzeCommand) buildStaticReportOutput(ctx context.Context, log *os.File) error {
	outputFolderSrcPath := a.getStaticReportSrcPath()
	outputFolderDestPath := filepath.Join(a.output, "static-report")

	//copy static report files to output folder
	err := util.CopyFolderContents(outputFolderSrcPath, outputFolderDestPath)
	if err != nil {
		return err
	}
	return nil
}

// GenerateStaticReport generates a static HTML report from analysis output.
func (a *analyzeCommand) GenerateStaticReport(ctx context.Context, operationalLog logr.Logger) error {
	if a.skipStaticReport {
		return nil
	}
	operationalLog.Info("generating static report")
	staticReportLogFilePath := filepath.Join(a.output, "static-report.log")
	staticReportLog, err := os.Create(staticReportLogFilePath)
	if err != nil {
		return fmt.Errorf("failed creating provider log file at %s", staticReportLogFilePath)
	}
	defer staticReportLog.Close()

	// it's possible for dependency analysis to fail
	// in this case we still want to generate a static report for successful source analysis
	_, noDepFileErr := os.Stat(filepath.Join(a.output, "dependencies.yaml"))
	if errors.Is(noDepFileErr, os.ErrNotExist) {
		operationalLog.Info("unable to get dependency output in static report. generating static report from source analysis only")

		// some other err
	} else if noDepFileErr != nil && !errors.Is(noDepFileErr, os.ErrNotExist) {
		return noDepFileErr
	}

	if a.bulk {
		a.moveResults()
	}

	err = a.buildStaticReportOutput(ctx, staticReportLog)
	if err != nil {
		return err
	}
	staticReportDestPath := filepath.Join(a.output, "static-report")
	err = a.buildStaticReportFile(ctx, staticReportDestPath, errors.Is(noDepFileErr, os.ErrNotExist))
	if err != nil {
		return err
	}
	uri := uri.File(filepath.Join(a.output, "static-report", "index.html"))
	operationalLog.Info("Static report created. Access it at this URL:", "URL", string(uri))

	return nil
}
