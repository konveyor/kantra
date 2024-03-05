package testing

import (
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strings"
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
	justifyLen := 0
	maxInt := func(a ...int) int {
		maxInt := math.MinInt64
		for _, n := range a {
			maxInt = int(math.Max(float64(n), float64(maxInt)))
		}
		return maxInt
	}
	for _, result := range results {
		if result.Error != nil {
			if _, ok := errorsByTestsFile[result.TestsFilePath]; !ok {
				errorsByTestsFile[result.TestsFilePath] = []string{}
			}
			errorsByTestsFile[result.TestsFilePath] = append(errorsByTestsFile[result.TestsFilePath],
				result.Error.Error())
			continue
		}
		justifyLen = maxInt(justifyLen,
			len(filepath.Base(result.TestsFilePath))+3,
			len(result.RuleID)+4, len(result.TestCaseName)+6)
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
	report := []string{}
	for testsFile, resultsByRule := range resultsByTestsFile {
		totalTestsInFile := len(resultsByRule)
		passedTestsInFile := 0
		testsFileReport := []string{}
		testsFileSummary := fmt.Sprintf("- %s%s%%d/%%d PASSED",
			filepath.Base(testsFile), strings.Repeat(" ", justifyLen-len(filepath.Base(testsFile))-2))
		testsReport := []string{}
		for ruleID, resultsByTCs := range resultsByRule {
			totalTestCasesInTest := len(resultsByTCs)
			passedTestCasesInTest := 0
			testReport := []string{}
			testSummary := fmt.Sprintf("%+2s %s%s%%d/%%d PASSED",
				"-", ruleID, strings.Repeat(" ", justifyLen-len(ruleID)-3))
			testCaseReport := []string{}
			for _, tcResult := range resultsByTCs {
				if !tcResult.Passed {
					reasons := []string{}
					for _, reason := range tcResult.FailureReasons {
						reasons = append(reasons, fmt.Sprintf("%+6s %s", "-", reason))
					}
					for _, debugInfo := range tcResult.DebugInfo {
						reasons = append(reasons, fmt.Sprintf("%+6s %s", "-", debugInfo))
					}
					testCaseReport = append(testCaseReport,
						fmt.Sprintf("%+4s %s%sFAILED", "-",
							tcResult.TestCaseName, strings.Repeat(" ", justifyLen-len(tcResult.TestCaseName)-5)))
					testCaseReport = append(testCaseReport, reasons...)
				} else {
					passedTestCasesInTest += 1
				}
			}
			if passedTestCasesInTest == totalTestCasesInTest {
				passedTestsInFile += 1
			}
			testReport = append(testReport,
				fmt.Sprintf(testSummary, passedTestCasesInTest, totalTestCasesInTest))
			testReport = append(testReport, testCaseReport...)
			testsReport = append(testsReport, testReport...)
		}
		testsFileReport = append(testsFileReport,
			fmt.Sprintf(testsFileSummary, passedTestsInFile, totalTestsInFile))
		testsFileReport = append(testsFileReport, testsReport...)
		report = append(report, testsFileReport...)
	}
	for testsFile, errs := range errorsByTestsFile {
		errorReport := []string{fmt.Sprintf("- %s   FAILED", filepath.Base(testsFile))}
		for _, e := range errs {
			errorReport = append(errorReport, fmt.Sprintf("%+2s %s", "-", e))
		}
		report = append(report, errorReport...)
	}
	fmt.Fprintln(w, strings.Join(report, "\n"))
}

func AnyFailed(results []Result) bool {
	for _, res := range results {
		if len(res.FailureReasons) > 0 {
			return true
		}
	}
	return false
}
