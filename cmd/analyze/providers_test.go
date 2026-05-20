package analyze

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_analyzeCommand_detectJavaProviderFallback_PomXml(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "detect-java-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	err = os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte("<project></project>"), 0644)
	require.NoError(t, err)

	a := &analyzeCommand{
		input:                 tmpDir,
		AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
	}

	found, err := a.detectJavaProviderFallback()
	require.NoError(t, err)
	assert.True(t, found, "should detect pom.xml as Java project")
}

func Test_analyzeCommand_detectJavaProviderFallback_BuildGradle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "detect-java-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	err = os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte("apply plugin: 'java'"), 0644)
	require.NoError(t, err)

	a := &analyzeCommand{
		input:                 tmpDir,
		AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
	}

	found, err := a.detectJavaProviderFallback()
	require.NoError(t, err)
	assert.True(t, found, "should detect build.gradle as Java project")
}

func Test_analyzeCommand_detectJavaProviderFallback_NoJava(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "detect-java-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	err = os.WriteFile(filepath.Join(tmpDir, "main.py"), []byte("print('hello')"), 0644)
	require.NoError(t, err)

	a := &analyzeCommand{
		input:                 tmpDir,
		AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
	}

	found, err := a.detectJavaProviderFallback()
	require.NoError(t, err)
	assert.False(t, found, "should not detect non-Java project")
}

func Test_analyzeCommand_detectJavaProviderFallback_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "detect-java-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	a := &analyzeCommand{
		input:                 tmpDir,
		AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
	}

	found, err := a.detectJavaProviderFallback()
	require.NoError(t, err)
	assert.False(t, found, "should return false for empty directory")
}

func Test_setupProgressReporter_NoProgress(t *testing.T) {
	reporter, done, cancel := setupProgressReporter(context.Background(), true)
	require.NotNil(t, reporter, "reporter should not be nil")
	assert.Nil(t, done, "done channel should be nil when noProgress=true")
	assert.Nil(t, cancel, "cancel should be nil when noProgress=true")
}

func Test_setupProgressReporter_WithProgress(t *testing.T) {
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	reporter, done, cancel := setupProgressReporter(ctx, false)
	require.NotNil(t, reporter, "reporter should not be nil")
	require.NotNil(t, done, "done channel should not be nil when noProgress=false")
	require.NotNil(t, cancel, "cancel should not be nil when noProgress=false")

	// Cancel should close the done channel eventually
	cancel()
	<-done // should not block forever
}

func Test_setupProgressReporter_ShutdownOnCancel(t *testing.T) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	_, done, cancel := setupProgressReporter(ctx, false)
	require.NotNil(t, done)
	require.NotNil(t, cancel)

	// Cancel the progress context
	cancel()

	// done channel must close promptly, not block shutdown
	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("progress goroutine did not shut down within 5s after cancel")
	}

	ctxCancel()
}
