package provider

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/konveyor-ecosystem/kantra/pkg/util"
)

// BundledDefaultRulesetSubdir returns the subdirectory name under the rulesets root for
// bundled default rules for this provider name
func BundledDefaultRulesetSubdir(providerName string) string {
	return util.DefaultRulesetDir[providerName]
}

// DefaultRulesetPathsForProviders returns ruleset directory paths under rulesetsRoot for
// each provider whose ProviderInfo.DefaultRulesetSubdir is set and the path exists
func DefaultRulesetPathsForProviders(rulesetsRoot string, providers []ProviderInfo) []string {
	if rulesetsRoot == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, info := range providers {
		if info.DefaultRulesetSubdir == "" {
			continue
		}
		p := filepath.Join(rulesetsRoot, info.DefaultRulesetSubdir)
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
