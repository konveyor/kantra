package cmd

import (
	"os"
	"testing"
)

// Test RUNNER_IMG settings
func TestRunnerImgDefault(t *testing.T) {
	os.Unsetenv("RUNNER_IMG") // Ensure empty variable
	s := &Config{}
	s.Load()
	if s.RunnerImage != "quay.io/konveyor/kantra:latest" {
		t.Errorf("Unexpected RUNNER_IMG default: %s", s.RunnerImage)
	}
}

func TestRunnerImgCustom(t *testing.T) {
	os.Setenv("RUNNER_IMG", "quay.io/some-contributor/my-kantra")
	s := &Config{}
	s.Load()
	if s.RunnerImage != "quay.io/some-contributor/my-kantra" {
		t.Errorf("Unexpected RUNNER_IMG: %s", s.RunnerImage)
	}
}
