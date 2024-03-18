package testing

import (
	"testing"

	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"go.lsp.dev/uri"
)

func TestTestCase_Verify(t *testing.T) {
	two := int(2)
	three := int(3)
	testFileUri := "test"
	mountedUri := "file:///data/test/sample.xml"
	localUri := "./test/sample.xml"
	tests := []struct {
		name       string
		testCase   TestCase
		output     konveyor.RuleSet
		wantErrors int
	}{
		{
			name: "tc checks if a rule is not matched",
			testCase: TestCase{
				RuleID:      "rule",
				IsUnmatched: true,
			},
			output:     konveyor.RuleSet{Unmatched: []string{"rule"}},
			wantErrors: 0,
		},
		{
			name: "tc checks if a tag is present",
			testCase: TestCase{
				HasTags: []string{"Python"},
			},
			output:     konveyor.RuleSet{Tags: []string{"Python"}},
			wantErrors: 0,
		},
		{
			name: "tc checks if a tag is present - negative",
			testCase: TestCase{
				HasTags: []string{"Python"},
			},
			output:     konveyor.RuleSet{Tags: []string{}},
			wantErrors: 1,
		},
		{
			name: "tc uses exactly constraint",
			testCase: TestCase{
				RuleID: "rule",
				HasIncidents: &IncidentVerification{
					CountBased: &CountBasedVerification{
						Exactly: &one,
					},
				},
			},
			output: konveyor.RuleSet{Violations: map[string]konveyor.Violation{
				"rule": {
					Incidents: []konveyor.Incident{
						{URI: "test", Message: "test", CodeSnip: "test"},
					},
				},
			}},
			wantErrors: 0,
		},
		{
			name: "tc uses exactly constraint - negative",
			testCase: TestCase{
				RuleID: "rule",
				HasIncidents: &IncidentVerification{
					CountBased: &CountBasedVerification{
						Exactly: &one,
					},
				},
			},
			output: konveyor.RuleSet{Violations: map[string]konveyor.Violation{
				"rule": {
					Incidents: []konveyor.Incident{
						{URI: "test", Message: "test", CodeSnip: "test"},
						{URI: "test", Message: "test", CodeSnip: "test"},
					},
				},
			}},
			wantErrors: 1,
		},
		{
			name: "tc uses atLeast constraint",
			testCase: TestCase{
				RuleID: "rule",
				HasIncidents: &IncidentVerification{
					CountBased: &CountBasedVerification{
						AtLeast: &one,
					},
				},
			},
			output: konveyor.RuleSet{Violations: map[string]konveyor.Violation{
				"rule": {
					Incidents: []konveyor.Incident{
						{URI: "test", Message: "test", CodeSnip: "test"},
						{URI: "test", Message: "test", CodeSnip: "test"},
					},
				},
			}},
			wantErrors: 0,
		},
		{
			name: "tc uses atLeast constraint - negative",
			testCase: TestCase{
				RuleID: "rule",
				HasIncidents: &IncidentVerification{
					CountBased: &CountBasedVerification{
						AtLeast: &two,
					},
				},
			},
			output: konveyor.RuleSet{Violations: map[string]konveyor.Violation{
				"rule": {
					Incidents: []konveyor.Incident{
						{URI: "test", Message: "test", CodeSnip: "test"},
					},
				},
			}},
			wantErrors: 1,
		},
		{
			name: "tc uses atMost constraint",
			testCase: TestCase{
				RuleID: "rule",
				HasIncidents: &IncidentVerification{
					CountBased: &CountBasedVerification{
						AtMost: &one,
					},
				},
			},
			output: konveyor.RuleSet{Violations: map[string]konveyor.Violation{
				"rule": {
					Incidents: []konveyor.Incident{
						{URI: "test", Message: "test", CodeSnip: "test"},
					},
				},
			}},
			wantErrors: 0,
		},
		{
			name: "tc uses atMost constraint - negative",
			testCase: TestCase{
				RuleID: "rule",
				HasIncidents: &IncidentVerification{
					CountBased: &CountBasedVerification{
						AtMost: &two,
					},
				},
			},
			output: konveyor.RuleSet{Violations: map[string]konveyor.Violation{
				"rule": {
					Incidents: []konveyor.Incident{
						{URI: "test", Message: "test", CodeSnip: "test"},
						{URI: "test", Message: "test", CodeSnip: "test"},
						{URI: "test", Message: "test", CodeSnip: "test"},
					},
				},
			}},
			wantErrors: 1,
		},
		{
			name: "tc uses locationBased constraint",
			testCase: TestCase{
				RuleID: "rule",
				HasIncidents: &IncidentVerification{
					LocationBased: &LocationBasedVerification{
						Locations: []LocationVerification{
							{FileURI: &testFileUri, LineNumber: &one},
							{FileURI: &testFileUri, LineNumber: &three, MessageMatches: &testFileUri, CodeSnipMatches: &testFileUri},
						},
					},
				},
			},
			output: konveyor.RuleSet{Violations: map[string]konveyor.Violation{
				"rule": {
					Incidents: []konveyor.Incident{
						{URI: "test", LineNumber: &one},
						{URI: "test", LineNumber: &three, Message: "test", CodeSnip: "test"},
					},
				},
			}},
			wantErrors: 0,
		},
		{
			name: "tc uses locationBased constraint and has a different file URI coming from the container",
			testCase: TestCase{
				RuleID: "rule",
				HasIncidents: &IncidentVerification{
					LocationBased: &LocationBasedVerification{
						Locations: []LocationVerification{
							{FileURI: &localUri, LineNumber: &one},
						},
					},
				},
			},
			output: konveyor.RuleSet{Violations: map[string]konveyor.Violation{
				"rule": {
					Incidents: []konveyor.Incident{
						{URI: uri.URI(mountedUri), LineNumber: &one},
					},
				},
			}},
			wantErrors: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := tt.testCase
			if got := tr.Verify(tt.output); len(got) != tt.wantErrors {
				t.Errorf("TestCase.Verify() = got verification error %v, want %v, errors %v",
					len(got), tt.wantErrors, got)
			}
		})
	}
}
