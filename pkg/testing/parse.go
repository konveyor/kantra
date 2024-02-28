package testing

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// TestsFilter filters in-out tests/test cases and returns final list of things to run
type TestsFilter interface {
	Filter([]Test) []Test
}

func Parse(paths []string, filter TestsFilter) ([]TestsFile, error) {
	tests := []TestsFile{}
	for _, path := range paths {
		err := filepath.Walk(path, func(path string, info fs.FileInfo, e error) error {
			if e != nil {
				return e
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(info.Name(), ".test.yaml") &&
				!strings.HasSuffix(info.Name(), ".test.yml") {
				return nil
			}
			// attempt tp parse ruleset level provider config
			providerConfig := parseRulesetConfig(
				filepath.Join(filepath.Dir(path), RULESET_TEST_CONFIG_GOLDEN_FILE))
			// parse the tests file
			t, err := parseFile(path, filter)
			if err != nil {
				return fmt.Errorf("failed to load tests from path %s (%w)", path, err)
			}
			t.Path = path
			if val, err := filepath.Abs(t.Path); err == nil {
				t.Path = val
			}
			if t.RulesPath == "" {
				t.RulesPath = strings.Replace(path, ".test.yaml", ".yaml", -1)
				t.RulesPath = strings.Replace(t.RulesPath, ".test.yml", ".yml", -1)
				if val, err := filepath.Abs(t.RulesPath); err == nil {
					t.RulesPath = val
				}
			} else {
				t.RulesPath = filepath.Join(filepath.Dir(path), t.RulesPath)
			}
			// merge ruleset level config with test specific config
			t.Providers = mergeProviderConfig(providerConfig, t.Providers)
			// validate
			err = t.Validate()
			if err != nil {
				return fmt.Errorf("invalid tests file %s (%w)", path, err)
			}
			// apply filters
			if filter != nil {
				t.Tests = filter.Filter(t.Tests)
			}
			if len(t.Tests) == 0 {
				// everything filtered out
				return nil
			}
			tests = append(tests, t)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return tests, nil
}

func parseRulesetConfig(path string) []ProviderConfig {
	providerConfig := struct {
		Providers []ProviderConfig `yaml:"providers" json:"providers"`
	}{
		Providers: []ProviderConfig{},
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return providerConfig.Providers
	}
	err = yaml.Unmarshal(content, &providerConfig)
	if err != nil {
		return providerConfig.Providers
	}
	return providerConfig.Providers
}

func parseFile(path string, f TestsFilter) (TestsFile, error) {
	t := TestsFile{}
	content, err := os.ReadFile(path)
	if err != nil {
		return t, err
	}
	err = yaml.Unmarshal(content, &t)
	if err != nil {
		return t, err
	}
	for idx := range t.Tests {
		test := &t.Tests[idx]
		for jdx := range test.TestCases {
			tc := &test.TestCases[jdx]
			tc.RuleID = test.RuleID
		}
	}
	return t, nil
}

// mergeProviderConfig merge values in p2 into p1, p2 takes precedance
func mergeProviderConfig(p1, p2 []ProviderConfig) []ProviderConfig {
	merged := []ProviderConfig{}
	seen := map[string]*ProviderConfig{}
	for idx, conf := range p1 {
		seen[conf.Name] = &p1[idx]
	}
	for idx, conf := range p2 {
		if _, ok := seen[conf.Name]; ok {
			seen[conf.Name].DataPath = conf.DataPath
		} else {
			seen[conf.Name] = &p2[idx]
		}
	}
	for _, v := range seen {
		merged = append(merged, *v)
	}
	// sorting for stability of unit tests
	sort.Slice(merged, func(i, j int) bool {
		return strings.Compare(merged[i].Name, merged[j].Name) < 0
	})
	return merged
}

// NewInlineNameBasedFilter works on an input string containing a comma
// separated list of test names and test case names to include
func NewInlineNameBasedFilter(names string) TestsFilter {
	if names == "" {
		return &inlineNameBasedFilter{}
	}
	includedNames := map[string]interface{}{}
	for _, val := range strings.Split(names, ",") {
		if val != "" {
			includedNames[val] = nil
		}
	}
	return &inlineNameBasedFilter{
		includedNames: includedNames,
	}
}

type inlineNameBasedFilter struct {
	includedNames map[string]interface{}
}

func (i inlineNameBasedFilter) Filter(tests []Test) []Test {
	if i.includedNames == nil {
		return tests
	}
	filterTCs := func(tcs []TestCase) []TestCase {
		filteredTCs := []TestCase{}
		for _, tc := range tcs {
			tcName := fmt.Sprintf("%s#%s", tc.RuleID, tc.Name)
			if _, tcOk := i.includedNames[tcName]; tcOk {
				filteredTCs = append(filteredTCs, tc)
			}
		}
		return filteredTCs
	}
	filtered := []Test{}
	for _, test := range tests {
		if _, ok := i.includedNames[test.RuleID]; ok {
			// entire test is included
			filtered = append(filtered, test)
		} else {
			// one or more test cases in a test are included
			filteredTest := test
			filteredTest.TestCases = filterTCs(test.TestCases)
			if len(filteredTest.TestCases) > 0 {
				filtered = append(filtered, filteredTest)
			}
		}
	}
	return filtered
}
