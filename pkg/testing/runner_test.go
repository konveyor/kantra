package testing

import (
	"os"
	"testing"
)

// ENV_RUN_LOCAL enables the runner to run analysis locally instead of a container
// this must be set in CI containers to make sure we are not launching containers
const ENV_RUN_LOCAL = "RUN_LOCAL"
const RUNNER_IMG = "RUNNER_IMG"

func Test_defaultRunner_Run(t *testing.T) {
	tests := []struct {
		name       string
		testFiles  []string
		wantErr    bool
		wantResult []Result
	}{
		{
			name: "simple test",
			testFiles: []string{
				"./examples/ruleset/discovery.test.yaml",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFiles, err := Parse(tt.testFiles, nil)
			if err != nil {
				t.Errorf("failed setting up test")
			}
			_, runLocal := os.LookupEnv(ENV_RUN_LOCAL)
			r := NewRunner()
			_, err = r.Run(testFiles, TestOptions{
				RunLocal:       runLocal,
				ContainerImage: os.Getenv(RUNNER_IMG),
			})
			if (err == nil) != tt.wantErr {
				t.Errorf("runner.Run() expected no error, got error %v", err)
			}
		})
	}
}
