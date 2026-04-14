package provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/konveyor-ecosystem/kantra/pkg/util"
)

func TestDefaultRulesetPathsForProviders(t *testing.T) {
	tmpDir := t.TempDir()
	for _, subdir := range []string{"java", "nodejs", "dotnet", "input"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, subdir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name      string
		providers []ProviderInfo
		want      []string
	}{
		{
			name: "java only",
			providers: []ProviderInfo{{
				Name:                 util.JavaProvider,
				DefaultRulesetSubdir: BundledDefaultRulesetSubdir(util.JavaProvider),
			}},
			want: []string{filepath.Join(tmpDir, "java")},
		},
		{
			name: "csharp maps to dotnet",
			providers: []ProviderInfo{{
				Name:                 util.CsharpProvider,
				DefaultRulesetSubdir: BundledDefaultRulesetSubdir(util.CsharpProvider),
			}},
			want: []string{filepath.Join(tmpDir, "dotnet")},
		},
		{
			name: "java and nodejs",
			providers: []ProviderInfo{
				{
					Name:                 util.JavaProvider,
					DefaultRulesetSubdir: BundledDefaultRulesetSubdir(util.JavaProvider),
				},
				{
					Name:                 util.NodeJSProvider,
					DefaultRulesetSubdir: BundledDefaultRulesetSubdir(util.NodeJSProvider),
				},
			},
			want: []string{
				filepath.Join(tmpDir, "java"),
				filepath.Join(tmpDir, "nodejs"),
			},
		},
		{
			name: "go and python have no default subdir mapping",
			providers: []ProviderInfo{
				{Name: util.GoProvider},
				{Name: util.PythonProvider},
			},
			want: nil,
		},
		{
			name: "mixed java and go only adds java",
			providers: []ProviderInfo{
				{
					Name:                 util.JavaProvider,
					DefaultRulesetSubdir: BundledDefaultRulesetSubdir(util.JavaProvider),
				},
				{Name: util.GoProvider},
			},
			want: []string{filepath.Join(tmpDir, "java")},
		},
		{
			name:      "empty providers",
			providers: nil,
			want:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultRulesetPathsForProviders(tmpDir, tt.providers)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d paths %v, want %d %v", len(got), got, len(tt.want), tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestDefaultRulesetPathsForProvidersEmptyRoot(t *testing.T) {
	got := DefaultRulesetPathsForProviders("", []ProviderInfo{{Name: util.JavaProvider}})
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestDefaultRulesetPathsForProvidersMissingSubdir(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "java"), 0o755); err != nil {
		t.Fatal(err)
	}
	providers := []ProviderInfo{
		{
			Name:                 util.JavaProvider,
			DefaultRulesetSubdir: BundledDefaultRulesetSubdir(util.JavaProvider),
		},
		{
			Name:                 util.NodeJSProvider,
			DefaultRulesetSubdir: BundledDefaultRulesetSubdir(util.NodeJSProvider),
		},
	}
	got := DefaultRulesetPathsForProviders(tmpDir, providers)
	want := []string{filepath.Join(tmpDir, "java")}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("got %v, want %v (nodejs missing)", got, want)
	}
}
