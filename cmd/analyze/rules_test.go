package analyze

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func Test_AnalyzeCommandContext_createTempRuleSet(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() (string, func())
		rulesetNm string
		wantErr   bool
		wantFile  bool
	}{
		{
			name: "creates ruleset.yaml in existing directory",
			setup: func() (string, func()) {
				dir, _ := os.MkdirTemp("", "ruleset-test-")
				return dir, func() { os.RemoveAll(dir) }
			},
			rulesetNm: "my-ruleset",
			wantErr:   false,
			wantFile:  true,
		},
		{
			name: "returns nil for non-existent directory",
			setup: func() (string, func()) {
				return "/non/existent/path", func() {}
			},
			rulesetNm: "my-ruleset",
			wantErr:   false,
			wantFile:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, cleanup := tt.setup()
			defer cleanup()

			ctx := &AnalyzeCommandContext{
				log: testr.NewWithOptions(t, testr.Options{Verbosity: 5}),
			}

			err := ctx.createTempRuleSet(path, tt.rulesetNm)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tt.wantFile {
				rulesetPath := filepath.Join(path, "ruleset.yaml")
				data, err := os.ReadFile(rulesetPath)
				require.NoError(t, err, "expected ruleset.yaml to exist at %s", rulesetPath)

				var ruleset engine.RuleSet
				err = yaml.Unmarshal(data, &ruleset)
				require.NoError(t, err)

				assert.Equal(t, tt.rulesetNm, ruleset.Name)
				assert.Equal(t, "temp ruleset", ruleset.Description)
			}
		})
	}
}

func Test_AnalyzeCommandContext_handleDir(t *testing.T) {
	tests := []struct {
		name     string
		dirTree  []string // relative paths within basePath to create
		wantErr  bool
		wantDirs []string // expected dirs in tempDir
	}{
		{
			name:     "creates nested directory and ruleset",
			dirTree:  []string{"subdir"},
			wantErr:  false,
			wantDirs: []string{"subdir"},
		},
		{
			name:     "creates deeply nested directory",
			dirTree:  []string{"level1"},
			wantErr:  false,
			wantDirs: []string{"level1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			basePath, err := os.MkdirTemp("", "handledir-base-")
			require.NoError(t, err)
			defer os.RemoveAll(basePath)

			tempDir, err := os.MkdirTemp("", "handledir-temp-")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			ctx := &AnalyzeCommandContext{
				log: testr.NewWithOptions(t, testr.Options{Verbosity: 5}),
			}

			for _, dir := range tt.dirTree {
				fullPath := filepath.Join(basePath, dir)
				err := os.MkdirAll(fullPath, 0777)
				require.NoError(t, err)

				err = ctx.handleDir(fullPath, tempDir, basePath)
				if tt.wantErr {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
			}

			// Verify expected directories were created in tempDir
			for _, dir := range tt.wantDirs {
				expectedPath := filepath.Join(tempDir, dir)
				stat, err := os.Stat(expectedPath)
				require.NoError(t, err, "expected directory %s to exist in tempDir", dir)
				assert.True(t, stat.IsDir(), "expected %s to be a directory", dir)

				// Verify ruleset.yaml was created
				rulesetPath := filepath.Join(expectedPath, "ruleset.yaml")
				assert.FileExists(t, rulesetPath, "expected ruleset.yaml in %s", expectedPath)
			}
		})
	}
}

func Test_AnalyzeCommandContext_handleDir_NestedPath(t *testing.T) {
	basePath, err := os.MkdirTemp("", "handledir-base-")
	require.NoError(t, err)
	defer os.RemoveAll(basePath)

	tempDir, err := os.MkdirTemp("", "handledir-temp-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create level1/level2 in basePath
	nestedDir := filepath.Join(basePath, "level1", "level2")
	err = os.MkdirAll(nestedDir, 0777)
	require.NoError(t, err)

	ctx := &AnalyzeCommandContext{
		log: testr.NewWithOptions(t, testr.Options{Verbosity: 5}),
	}

	// First handle level1
	err = ctx.handleDir(filepath.Join(basePath, "level1"), tempDir, basePath)
	require.NoError(t, err)

	// Then handle level1/level2
	err = ctx.handleDir(nestedDir, tempDir, basePath)
	require.NoError(t, err)

	// Verify level1/level2 exists in tempDir
	expectedPath := filepath.Join(tempDir, "level1", "level2")
	assert.DirExists(t, expectedPath)

	// Verify both levels have ruleset.yaml
	for _, dir := range []string{"level1", filepath.Join("level1", "level2")} {
		rulesetPath := filepath.Join(tempDir, dir, "ruleset.yaml")
		assert.FileExists(t, rulesetPath)
	}
}
