package testing

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"
)

// Result is a result of a test run
type Result struct {
	Passed         bool
	TestsFilePath  string
	RuleID         string
	TestCaseName   string
	DebugInfo      []string
	FailureReasons []string
	Error          error
}

// ResultPrinter is a function to print given results to a given place
type ResultPrinter func(io.WriteCloser, []Result)

type summary struct {
	total  int
	passed int
}

// PrintSummary prints statistical summary from given results
func PrintSummary(w io.WriteCloser, results []Result) {
	summaryByRules := map[string]*summary{}
	tcSummary := summary{}
	rulesSummary := summary{}
	for _, result := range results {
		if _, found := summaryByRules[result.RuleID]; !found {
			summaryByRules[result.RuleID] = &summary{}
		}
		summaryByRules[result.RuleID].total += 1
		tcSummary.total += 1
		if result.Passed {
			summaryByRules[result.RuleID].passed += 1
			tcSummary.passed += 1
		}
	}
	for _, summary := range summaryByRules {
		rulesSummary.total += 1
		if summary.passed == summary.total {
			rulesSummary.passed += 1
		}
	}
	fmt.Fprintln(w, strings.Repeat("-", 60))
	fmt.Fprintf(w, "  Rules Summary:      %d/%d (%.2f%%) PASSED\n",
		rulesSummary.passed, rulesSummary.total, float64(rulesSummary.passed)/float64(rulesSummary.total)*100)
	fmt.Fprintf(w, "  Test Cases Summary: %d/%d (%.2f%%) PASSED\n",
		tcSummary.passed, tcSummary.total, float64(tcSummary.passed)/float64(tcSummary.total)*100)
	fmt.Fprintln(w, strings.Repeat("-", 60))
}

// PrintProgress prints detailed information from given results
func PrintProgress(w io.WriteCloser, results []Result) {
	// results grouped by their tests files, then rules, then test cases
	resultsByTestsFile := map[string]map[string][]Result{}
	errorsByTestsFile := map[string][]string{}
	for _, result := range results {
		if result.Error != nil {
			if _, ok := errorsByTestsFile[result.TestsFilePath]; !ok {
				errorsByTestsFile[result.TestsFilePath] = []string{}
			}
			errorsByTestsFile[result.TestsFilePath] = append(errorsByTestsFile[result.TestsFilePath],
				result.Error.Error())
			continue
		}
		if _, ok := resultsByTestsFile[result.TestsFilePath]; !ok {
			resultsByTestsFile[result.TestsFilePath] = map[string][]Result{}
		}
		if result.RuleID != "" {
			if _, ok := resultsByTestsFile[result.TestsFilePath][result.RuleID]; !ok {
				resultsByTestsFile[result.TestsFilePath][result.RuleID] = []Result{}
			}
			if result.TestCaseName != "" {
				resultsByTestsFile[result.TestsFilePath][result.RuleID] = append(
					resultsByTestsFile[result.TestsFilePath][result.RuleID], result)
			}
		}
	}
	prettyWriter := tabwriter.NewWriter(w, 1, 1, 1, ' ', tabwriter.StripEscape)
	for testsFile, resultsByRule := range resultsByTestsFile {
		testsResult := ""
		passedRules := 0
		for test, testCases := range resultsByRule {
			passedTCs := 0
			tcsResult := ""
			for _, tcResult := range testCases {
				// only output failed test cases
				if !tcResult.Passed {
					tcsResult = fmt.Sprintf("%s    %s\tFAILED\n", tcsResult, tcResult.TestCaseName)
					for _, reason := range tcResult.FailureReasons {
						tcsResult = fmt.Sprintf("%s    - %s\t\n", tcsResult, reason)
					}
					for _, debugInfo := range tcResult.DebugInfo {
						tcsResult = fmt.Sprintf("%s    - %s\t\n", tcsResult, debugInfo)
					}
				} else {
					passedTCs += 1
				}
			}
			if passedTCs == len(testCases) {
				passedRules += 1
			}
			testStat := fmt.Sprintf("%d/%d PASSED", passedTCs, len(testCases))
			testsResult = fmt.Sprintf("%s  %s\t%s\n", testsResult, test, testStat)
			if tcsResult != "" {
				testsResult = fmt.Sprintf("%s%s", testsResult, tcsResult)
			}
		}
		testsFileStat := fmt.Sprintf("%d/%d PASSED", passedRules, len(resultsByRule))
		fmt.Fprintf(prettyWriter,
			"%s\t%s\n%s", filepath.Base(testsFile), testsFileStat, testsResult)
	}
	prettyWriter.Flush()
}

func AnyFailed(results []Result) bool {
	for _, res := range results {
		if len(res.FailureReasons) > 0 {
			return true
		}
	}
	return false
}
