package testing

import (
	"reflect"
	"strings"
	"testing"
)

var one = int(1)
var seven = int(7)
var codeSnipOne = "file://common.properties"
var fileTwo = "./test-data/java/src/main/resources/persistence.properties"
var discoveryTests = TestsFile{
	Providers: []ProviderConfig{
		{Name: "builtin", DataPath: "./test-data/python/"},
		{Name: "java", DataPath: "./test-data/java/"},
		{Name: "python", DataPath: "./test-data/python/"},
	},
	RulesPath: "examples/ruleset/discovery.yaml",
	Tests: []Test{
		{
			RuleID: "language-discovery",
			TestCases: []TestCase{{
				Name:    "tc-00",
				RuleID:  "language-discovery",
				HasTags: []string{"Python"},
			}},
		},
		{
			RuleID: "kube-api-usage",
			TestCases: []TestCase{
				{
					Name:    "tc-00",
					RuleID:  "language-discovery",
					HasTags: []string{"Kubernetes"},
				},
				{
					Name:   "tc-01",
					RuleID: "language-discovery",
					HasIncidents: &IncidentVerification{
						CountBased: &CountBasedVerification{
							Exactly: &one,
						},
					},
				},
			},
		},
	},
}
var localStorageTests = TestsFile{
	Providers: []ProviderConfig{
		{Name: "builtin", DataPath: "./test-data/"},
		{Name: "java", DataPath: "./test-data/java/"},
		{Name: "python", DataPath: "./test-data/python/"},
	},
	RulesPath: "examples/ruleset/local-storage.yml",
	Tests: []Test{
		{
			RuleID: "storage-000",
			TestCases: []TestCase{{
				Name:   "tc-00",
				RuleID: "storage-00",
				HasIncidents: &IncidentVerification{
					LocationBased: &LocationBasedVerification{
						Locations: []LocationVerification{
							{
								FileURI:         &fileTwo,
								LineNumber:      &seven,
								MessageMatches:  &codeSnipOne,
								CodeSnipMatches: &codeSnipOne,
							},
						},
					},
				},
			}},
		},
	},
}

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		inputPaths  []string
		inputFilter TestsFilter
		want        []TestsFile
		wantErr     bool
	}{
		{
			name: "pass ruleset as input",
			inputPaths: []string{
				"./examples/ruleset/",
			},
			want: []TestsFile{
				discoveryTests,
				localStorageTests,
			},
		},
		{
			name: "pass multiple test files as input",
			inputPaths: []string{
				"./examples/ruleset/local-storage.test.yml",
				"./examples/ruleset/discovery.test.yaml",
			},
			want: []TestsFile{
				localStorageTests,
				discoveryTests,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.inputPaths, tt.inputFilter)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("Parse() expected %d tests files, got %d", len(tt.want), len(got))
				return
			}
			for idx, gotTests := range got {
				wantTests := tt.want[idx]
				if !reflect.DeepEqual(gotTests.Providers, wantTests.Providers) {
					t.Errorf("Parse() Tests[%d] expected provider config %v, got %v", idx, wantTests.Providers, gotTests.Providers)
					return
				}
				if len(gotTests.Tests) != len(wantTests.Tests) {
					t.Errorf("Parse() Tests[%d] expected total tests %d, got %d", idx, len(wantTests.Tests), len(gotTests.Tests))
					return
				}
				if !strings.Contains(gotTests.RulesPath, wantTests.RulesPath) {
					t.Errorf("Parse() Tests[%d] expected RulesPath %s, got %s", idx, wantTests.RulesPath, gotTests.RulesPath)
					return
				}
				for jdx, gotTest := range gotTests.Tests {
					wantTest := wantTests.Tests[jdx]
					if len(gotTest.TestCases) != len(wantTest.TestCases) {
						t.Errorf("Parse() Tests[%d].Tests[%d] expected test cases %d, got %d", idx, jdx, len(wantTest.TestCases), len(gotTest.TestCases))
						return
					}

					if wantTest.RuleID != gotTest.RuleID {
						t.Errorf("Parse() Tests[%d].Tests[%d] expected ruleID %s, got %s", idx, jdx, wantTest.RuleID, gotTest.RuleID)
						return
					}
					for kdx, gotTc := range gotTest.TestCases {
						wantTc := wantTest.TestCases[kdx]
						if !reflect.DeepEqual(wantTc.AnalysisParams, gotTc.AnalysisParams) {
							t.Errorf("Parse() Tests[%d].Tests[%d].TestCases[%d] expected params %v, got %v", idx, jdx, kdx, wantTc.AnalysisParams, gotTc.AnalysisParams)
							return
						}
						if wantTc.Name != gotTc.Name {
							t.Errorf("Parse() Tests[%d].Tests[%d].TestCases[%d] expected name %s, got %s", idx, jdx, kdx, wantTc.Name, gotTc.Name)
							return
						}
						if gotTc.HasIncidents != nil {
							if wantTc.HasIncidents == nil {
								t.Errorf("Parse() Tests[%d].Tests[%d].TestCases[%d] expected hasIncidents <nil>, got %v", idx, jdx, kdx, gotTc.HasIncidents)
								return
							}
							if gotTc.HasIncidents.CountBased != nil {
								if wantTc.HasIncidents.CountBased == nil {
									t.Errorf("Parse() Tests[%d].Tests[%d].TestCases[%d] expected hasIncidents <nil>, got %v", idx, jdx, kdx, gotTc.HasIncidents.CountBased)
									return
								}
								if !reflect.DeepEqual(gotTc.HasIncidents.CountBased, wantTc.HasIncidents.CountBased) {
									t.Errorf("Parse() Tests[%d].Tests[%d].TestCases[%d] expected hasIncidents %v, got %v", idx, jdx, kdx, wantTc.HasIncidents.CountBased, gotTc.HasIncidents.CountBased)
									return
								}
							}
							if gotTc.HasIncidents.LocationBased != nil {
								if wantTc.HasIncidents.LocationBased == nil {
									t.Errorf("Parse() Tests[%d].Tests[%d].TestCases[%d] expected hasIncidents.locations <nil>, got %v", idx, jdx, kdx, gotTc.HasIncidents.LocationBased)
									return
								}
								for ldx, gotLocation := range gotTc.HasIncidents.LocationBased.Locations {
									wantLocation := wantTc.HasIncidents.LocationBased.Locations[ldx]
									if !reflect.DeepEqual(gotLocation, wantLocation) {
										t.Errorf("Parse() Tests[%d].Tests[%d].TestCases[%d] expected hasIncidents.locations[%d] %v, got %v", idx, jdx, kdx, ldx, wantLocation, gotLocation)
										return
									}
								}
							}
						}
						if !reflect.DeepEqual(gotTc.HasTags, wantTc.HasTags) {
							t.Errorf("Parse() Tests[%d].Tests[%d].TestCases[%d] expected %v hasTags, got %v", idx, jdx, kdx, wantTc.HasTags, gotTc.HasTags)
							return
						}
					}
				}
			}
		})
	}
}

func Test_mergeProviderConfig(t *testing.T) {
	tests := []struct {
		name      string
		mergeInto []ProviderConfig
		mergeFrom []ProviderConfig
		want      []ProviderConfig
	}{
		{
			name: "mergeFrom must take precedance when conflicting values",
			mergeInto: []ProviderConfig{
				{
					DataPath: "./test/",
					Name:     "go",
				},
			},
			mergeFrom: []ProviderConfig{
				{
					DataPath: "./test/go/",
					Name:     "go",
				},
			},
			want: []ProviderConfig{
				{
					DataPath: "./test/go/",
					Name:     "go",
				},
			},
		},
		{
			name: "mergeInto has more items than mergeFrom, they should be kept as-is",
			mergeInto: []ProviderConfig{
				{
					DataPath: "./test/",
					Name:     "go",
				},
				{
					DataPath: "./test/",
					Name:     "builtin",
				},
			},
			mergeFrom: []ProviderConfig{
				{
					DataPath: "./test/go/",
					Name:     "go",
				},
			},
			want: []ProviderConfig{
				{
					DataPath: "./test/",
					Name:     "builtin",
				},
				{
					DataPath: "./test/go/",
					Name:     "go",
				},
			},
		},
		{
			name: "mergeFrom has more items than mergeInto, they should be kept as-is",
			mergeInto: []ProviderConfig{
				{
					DataPath: "./test/",
					Name:     "go",
				},
			},
			mergeFrom: []ProviderConfig{
				{
					DataPath: "./test/go/",
					Name:     "go",
				},
				{
					DataPath: "./test/",
					Name:     "builtin",
				},
			},
			want: []ProviderConfig{
				{
					DataPath: "./test/",
					Name:     "builtin",
				},
				{
					DataPath: "./test/go/",
					Name:     "go",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeProviderConfig(tt.mergeInto, tt.mergeFrom); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeProviderConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func NewTest(modifier ...func(t *Test)) Test {
	t := Test{}
	for _, fn := range modifier {
		fn(&t)
	}
	for idx := range t.TestCases {
		tc := &t.TestCases[idx]
		tc.RuleID = t.RuleID
	}
	return t
}

func WithTC(name string, modifiers ...func(tc *TestCase)) func(tc *Test) {
	testCase := TestCase{Name: name}
	for _, mod := range modifiers {
		mod(&testCase)
	}
	return func(tc *Test) {
		tc.TestCases = append(tc.TestCases, testCase)
	}
}

func WithRuleID(ruleID string) func(t *Test) {
	return func(tc *Test) {
		tc.RuleID = ruleID
	}
}

func WithAnalysisParams(a AnalysisParams) func(tc *TestCase) {
	return func(tc *TestCase) {
		tc.AnalysisParams = a
	}
}

func Test_inlineNameBasedFilter_Filter(t *testing.T) {
	tests := []struct {
		name         string
		filterString string
		inputTests   []Test
		wantTests    []Test
	}{
		{
			name:         "filter string is empty, include everything",
			filterString: "",
			inputTests: []Test{
				NewTest(
					WithRuleID("rule-000"),
					WithTC("tc-00"),
					WithTC("tc-01"),
				),
			},
			wantTests: []Test{
				NewTest(
					WithRuleID("rule-000"),
					WithTC("tc-00"),
					WithTC("tc-01"),
				),
			},
		},
		{
			name:         "filter string specifies only a tc, include that test with only that tc",
			filterString: "rule-000#tc-01",
			inputTests: []Test{
				NewTest(
					WithRuleID("rule-000"),
					WithTC("tc-00"),
					WithTC("tc-01"),
				),
				NewTest(
					WithRuleID("rule-001"),
					WithTC("tc-00"),
					WithTC("tc-01"),
				),
			},
			wantTests: []Test{
				NewTest(
					WithRuleID("rule-000"),
					WithTC("tc-01"),
				),
			},
		},
		{
			name:         "filter string has a test and a test case from that same test, include entire test",
			filterString: "rule-000,rule-000#tc-01",
			inputTests: []Test{
				NewTest(
					WithRuleID("rule-000"),
					WithTC("tc-00"),
					WithTC("tc-01"),
				),
				NewTest(
					WithRuleID("rule-001"),
					WithTC("tc-00"),
					WithTC("tc-01"),
				),
			},
			wantTests: []Test{
				NewTest(
					WithRuleID("rule-000"),
					WithTC("tc-00"),
					WithTC("tc-01"),
				),
			},
		},
		{
			name:         "filter string has a test and a test case from another test",
			filterString: "rule-000,rule-002#tc-00",
			inputTests: []Test{
				NewTest(
					WithRuleID("rule-000"),
					WithTC("tc-00"),
					WithTC("tc-01"),
				),
				NewTest(
					WithRuleID("rule-001"),
					WithTC("tc-00"),
					WithTC("tc-01"),
				),
				NewTest(
					WithRuleID("rule-002"),
					WithTC("tc-00"),
					WithTC("tc-01"),
				),
			},
			wantTests: []Test{
				NewTest(
					WithRuleID("rule-000"),
					WithTC("tc-00"),
					WithTC("tc-01"),
				),
				NewTest(
					WithRuleID("rule-002"),
					WithTC("tc-00"),
				),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewInlineNameBasedFilter(tt.filterString).Filter(tt.inputTests)
			if !reflect.DeepEqual(tt.wantTests, got) {
				t.Errorf("inlineNameBasedFilter.IncludeTest() = %v, want %v", got, tt.wantTests)
			}
		})
	}
}
