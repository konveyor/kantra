package testing

import (
	"os"
	"testing"

	"github.com/konveyor-ecosystem/kantra/pkg/util"
)

func Test_envWithoutKantraDir(t *testing.T) {
	key := util.KantraDirEnv
	tests := []struct {
		name string
		env  []string
		want []string
	}{
		{
			name: "empty env",
			env:  nil,
			want: []string{},
		},
		{
			name: "no KANTRA_DIR",
			env:  []string{"PATH=/usr/bin", "HOME=/home/user"},
			want: []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			name: "single KANTRA_DIR removed",
			env:  []string{"PATH=/usr/bin", key + "=/old/dir", "HOME=/home/user"},
			want: []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			name: "multiple KANTRA_DIR entries all removed",
			env:  []string{key + "=/first", "X=1", key + "=/second", "Y=2"},
			want: []string{"X=1", "Y=2"},
		},
		{
			name: "only KANTRA_DIR",
			env:  []string{key + "=/only"},
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := envWithoutKantraDir(tt.env)
			if len(got) != len(tt.want) {
				t.Errorf("envWithoutKantraDir() returned %d entries, want %d", len(got), len(tt.want))
			}
			for i, e := range tt.want {
				if i >= len(got) || got[i] != e {
					t.Errorf("envWithoutKantraDir()[%d] = %q, want %q", i, got[i], e)
				}
			}
		})
	}
}

// ENV_RUN_LOCAL enables the runner to run analysis locally instead of a container
// this must be set in CI containers to make sure we are not launching containers
const RUNNER_IMG = "RUNNER_IMG"

func Test_defaultRunner_Run(t *testing.T) {
	tests := []struct {
		name       string
		testFiles  []string
		wantErr    bool
		wantResult []Result
		runLocal   bool
	}{
		{
			name: "simple test",
			testFiles: []string{
				"./examples/ruleset/discovery.test.yaml",
			},
			runLocal: false,
		},
		{
			name: "run local test",
			testFiles: []string{
				"./examples/ruleset/discovery.test.yaml",
			},
			runLocal: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFiles, err := Parse(tt.testFiles, nil)
			if err != nil {
				t.Errorf("failed setting up test")
			}
			r := NewRunner()
			_, err = r.Run(testFiles, TestOptions{
				RunLocal:       tt.runLocal,
				ContainerImage: os.Getenv(RUNNER_IMG),
			})
			if (err == nil) != tt.wantErr {
				t.Errorf("runner.Run() expected no error, got error %v", err)
			}
		})
	}
}
