package provider

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/konveyor-ecosystem/kantra/pkg/util"
)

// returns ruleset directory paths under rulesetsRoot for each configured provider
//
//	that has bundled defaults so only running providers' default rulesets are loaded.
func DefaultRulesetPathsForProviders(rulesetsRoot string, providers []ProviderInfo) []string {
	if rulesetsRoot == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, info := range providers {
		subdir, ok := util.DefaultRulesetDir[info.Name]
		if !ok {
			continue
		}
		p := filepath.Join(rulesetsRoot, subdir)
		st, err := os.Stat(p)
		if err != nil || !st.IsDir() {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
