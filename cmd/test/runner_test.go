package test

import (
	"os"
	"testing"
)

func Test_defaultProviderImage(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		want         string
	}{
		{
			name:         "java provider",
			providerName: "java",
			want:         "quay.io/konveyor/java-external-provider:latest",
		},
		{
			name:         "go provider",
			providerName: "go",
			want:         "quay.io/konveyor/go-external-provider:latest",
		},
		{
			name:         "python provider",
			providerName: "python",
			want:         "quay.io/konveyor/python-external-provider:latest",
		},
		{
			name:         "nodejs provider",
			providerName: "nodejs",
			want:         "quay.io/konveyor/nodejs-external-provider:latest",
		},
		{
			name:         "csharp provider",
			providerName: "dotnet",
			want:         "quay.io/konveyor/c-sharp-provider:latest",
		},
		{
			name:         "builtin provider has no image",
			providerName: "builtin",
			want:         "",
		},
		{
			name:         "unknown provider",
			providerName: "unknown",
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultProviderImage(tt.providerName)
			if got != tt.want {
				t.Errorf("defaultProviderImage() = %q, want %q", got, tt.want)
			}
		})
	}
}

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

const RUNNER_IMG = "RUNNER_IMG"

func Test_defaultRunner_Run(t *testing.T) {
	tests := []struct {
		name       string
		testFiles  []string
		wantErr    bool
		wantResult []Result
	}{
		{
			name: "discovery ruleset tests (hybrid providers)",
			testFiles: []string{
				"./examples/builtin/discovery.test.yaml",
			},
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
				RunLocal:        false,
				ContainerBinary: os.Getenv("CONTAINER_TOOL"),
				RunnerImage:     os.Getenv(RUNNER_IMG),
			})
			if (err == nil) != tt.wantErr {
				t.Errorf("runner.Run() expected no error, got error %v", err)
			}
		})
	}
}
