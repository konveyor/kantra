package testing

import (
	"os"
	"testing"
)

func Test_resolveDataPath(t *testing.T) {
	tests := []struct {
		name          string
		testsFilePath string
		dataPath      string
		want          string
	}{
		{
			name:          "relative path is joined with test file dir",
			testsFilePath: "/home/user/tests/my.test.yaml",
			dataPath:      "./test-data/python",
			want:          "/home/user/tests/test-data/python",
		},
		{
			name:          "relative path without dot prefix",
			testsFilePath: "/home/user/tests/my.test.yaml",
			dataPath:      "test-data/python",
			want:          "/home/user/tests/test-data/python",
		},
		{
			name:          "absolute path is returned as-is",
			testsFilePath: "/home/user/tests/my.test.yaml",
			dataPath:      "/opt/data/examples/nodejs",
			want:          "/opt/data/examples/nodejs",
		},
		{
			name:          "absolute path is not prepended with test file dir",
			testsFilePath: "/home/user/tests/my.test.yaml",
			dataPath:      "/home/other/git/analyzer-lsp/examples/nodejs",
			want:          "/home/other/git/analyzer-lsp/examples/nodejs",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveDataPath(tt.testsFilePath, tt.dataPath)
			if got != tt.want {
				t.Errorf("resolveDataPath() = %q, want %q", got, tt.want)
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
				RunLocal:        tt.runLocal,
				ContainerBinary: os.Getenv("CONTAINER_TOOL"),
				RunnerImage:     os.Getenv(RUNNER_IMG),
			})
			if (err == nil) != tt.wantErr {
				t.Errorf("runner.Run() expected no error, got error %v", err)
			}
		})
	}
}
