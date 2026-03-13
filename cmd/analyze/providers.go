package analyze

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/konveyor-ecosystem/kantra/pkg/util"

	"github.com/devfile/alizer/pkg/apis/model"
	"github.com/konveyor/analyzer-lsp/progress"
	progressReporterPkg "github.com/konveyor/analyzer-lsp/progress/reporter"
)

func (a *analyzeCommand) detectJavaProviderFallback() (bool, error) {
	a.log.V(7).Info("language files not found. Using fallback")
	pomPath := filepath.Join(a.input, "pom.xml")
	_, err := os.Stat(pomPath)
	// some other error
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err == nil {
		return true, nil
	}
	// try gradle next
	gradlePath := filepath.Join(a.input, "build.gradle")
	_, err = os.Stat(gradlePath)
	// some other error
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err

		// java project not found
	} else if err != nil && errors.Is(err, os.ErrNotExist) {
		a.log.V(7).Info("language files not found. Only starting builtin provider")
		return false, nil

	} else if err == nil {
		return true, nil
	}

	return false, nil
}

// setupProgressReporter creates and starts a progress reporter for analysis.
// Returns the reporter, done channel, and cancel function.
// If noProgress is true, returns a noop reporter with nil channel and cancel func.
func setupProgressReporter(ctx context.Context, noProgress bool) (
	progressReporter progress.Reporter,
	progressDone chan struct{},
	progressCancel context.CancelFunc,
) {
	if !noProgress {
		// Create channel-based progress reporter
		var progressCtx context.Context
		progressCtx, progressCancel = context.WithCancel(ctx)
		channelReporter := progressReporterPkg.NewChannelReporter(progressCtx)
		progressReporter = channelReporter

		// Start goroutine to consume progress events and render progress bar
		progressDone = make(chan struct{})
		go func() {
			defer close(progressDone)

			// Track cumulative progress across all rulesets
			var cumulativeTotal int
			var completedFromPreviousRulesets int
			var lastRulesetTotal int
			var justPrintedLoadedRules bool

			for event := range channelReporter.Events() {
				switch event.Stage {
				case progress.StageProviderInit:
					// Skip provider init messages - we show them earlier
				case progress.StageProviderPrepare:
					// Display provider preparation progress
					if event.Total > 0 {
						percent := (event.Current * 100) / event.Total
						const barWidth = 25
						filled := (percent * barWidth) / 100
						if filled > barWidth {
							filled = barWidth
						}
						bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
						// Use \r to return to start of line and \033[K to clear to end of line
						fmt.Fprintf(os.Stderr, "\r\033[K  ✓ %s %3d%% |%s| %d/%d files",
							event.Message, percent, bar, event.Current, event.Total)
						// Print newline when prepare progress reaches 100%
						if event.Current >= event.Total {
							fmt.Fprintf(os.Stderr, "\n")
						}
					}
				case progress.StageDependencyResolution:
					fmt.Fprintf(os.Stderr, "  ✓ Resolved %d dependencies\n", event.Total)
				case progress.StageRuleParsing:
					if event.Total > 0 {
						cumulativeTotal += event.Total
						fmt.Fprintf(os.Stderr, "  ✓ Loaded %d rules\n", cumulativeTotal)
						justPrintedLoadedRules = true
					}
				case progress.StageRuleExecution:
					if event.Total > 0 {
						// Initialize cumulativeTotal from first event if not set by rule parsing
						if cumulativeTotal == 0 {
							cumulativeTotal = event.Total
							fmt.Fprintf(os.Stderr, "  ✓ Loaded %d rules\n", cumulativeTotal)
							justPrintedLoadedRules = true
						}

						// Skip first progress bar render right after printing "Loaded rules"
						if justPrintedLoadedRules {
							justPrintedLoadedRules = false
							continue
						}

						// Detect if we've moved to a new ruleset
						// This happens when event.Total changes
						if lastRulesetTotal > 0 && event.Total != lastRulesetTotal {
							// We've moved to a new ruleset
							completedFromPreviousRulesets += lastRulesetTotal
						}
						lastRulesetTotal = event.Total

						// Calculate overall progress
						totalCompleted := completedFromPreviousRulesets + event.Current

						overallPercent := (totalCompleted * 100) / cumulativeTotal
						renderProgressBar(overallPercent, totalCompleted, cumulativeTotal, event.Message)
					} else if event.Total == 0 && cumulativeTotal > 0 {
						// Skip rendering if we get a zero-total event but we've already initialized
						// This prevents spurious escape sequences from being rendered
						continue
					}
				case progress.StageComplete:
					// Move to next line, keeping the progress bar visible
					fmt.Fprintf(os.Stderr, "\n\n")
					fmt.Fprintf(os.Stderr, "Analysis complete!\n")
				}
			}
		}()
	} else {
		// Use noop reporter when progress is disabled
		progressReporter = progress.NewNoopReporter()
	}

	return progressReporter, progressDone, progressCancel
}

func (c *AnalyzeCommandContext) setProviders(providers []string, languages []model.Language, foundProviders []string) ([]string, error) {
	if len(providers) > 0 {
		for _, p := range providers {
			foundProviders = append(foundProviders, p)
		}
		return foundProviders, nil
	}
	for _, l := range languages {
		if l.CanBeComponent {
			c.log.V(5).Info("Got language", "component language", l)
			if l.Name == "C#" {
				foundProviders = append(foundProviders, util.CsharpProvider)
				continue
			}

			// typescript ls supports both TS and JS
			if l.Name == "JavaScript" || l.Name == "TypeScript" {
				foundProviders = append(foundProviders, util.NodeJSProvider)

			} else {
				foundProviders = append(foundProviders, strings.ToLower(l.Name))
			}
		}
	}
	return foundProviders, nil
}
