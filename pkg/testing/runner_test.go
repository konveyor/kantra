package testing

import (
	"os"
	"testing"
)

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
