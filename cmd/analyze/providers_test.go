package analyze

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/konveyor-ecosystem/kantra/cmd/internal/settings"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
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

func Test_AnalyzeCommandContext_setProviderInitInfo(t *testing.T) {
	// Initialize Settings so provider images are available
	settings.Settings = &settings.Config{
		JavaProviderImage:    "quay.io/konveyor/java-external-provider:latest",
		GenericProviderImage: "quay.io/konveyor/generic-external-provider:latest",
		CsharpProviderImage:  "quay.io/konveyor/c-sharp-provider:latest",
	}

	tests := []struct {
		name           string
		foundProviders []string
		wantInMap      []string
	}{
		{
			name:           "java provider",
			foundProviders: []string{util.JavaProvider},
			wantInMap:      []string{util.JavaProvider},
		},
		{
			name:           "go provider",
			foundProviders: []string{util.GoProvider},
			wantInMap:      []string{util.GoProvider},
		},
		{
			name:           "python provider",
			foundProviders: []string{util.PythonProvider},
			wantInMap:      []string{util.PythonProvider},
		},
		{
			name:           "nodejs provider",
			foundProviders: []string{util.NodeJSProvider},
			wantInMap:      []string{util.NodeJSProvider},
		},
		{
			name:           "csharp provider",
			foundProviders: []string{util.CsharpProvider},
			wantInMap:      []string{util.CsharpProvider},
		},
		{
			name:           "multiple providers",
			foundProviders: []string{util.JavaProvider, util.GoProvider, util.PythonProvider},
			wantInMap:      []string{util.JavaProvider, util.GoProvider, util.PythonProvider},
		},
		{
			name:           "empty providers",
			foundProviders: []string{},
			wantInMap:      []string{},
		},
		{
			name:           "unknown provider ignored",
			foundProviders: []string{"unknown-provider"},
			wantInMap:      []string{}, // unknown providers are silently skipped
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &AnalyzeCommandContext{
				log:          testr.NewWithOptions(t, testr.Options{Verbosity: 5}),
				providersMap: make(map[string]ProviderInit),
			}

			err := ctx.setProviderInitInfo(tt.foundProviders)
			require.NoError(t, err)

			assert.Len(t, ctx.providersMap, len(tt.wantInMap))

			for _, prov := range tt.wantInMap {
				provInit, ok := ctx.providersMap[prov]
				require.True(t, ok, "providersMap missing provider %q", prov)
				assert.NotZero(t, provInit.port, "provider %q has port 0", prov)
				assert.NotEmpty(t, provInit.image, "provider %q has empty image", prov)
				assert.NotNil(t, provInit.provider, "provider %q has nil provider interface", prov)
			}
		})
	}
}

func Test_AnalyzeCommandContext_setProviderInitInfo_UniquePortsPerProvider(t *testing.T) {
	settings.Settings = &settings.Config{
		JavaProviderImage:    "quay.io/konveyor/java-external-provider:latest",
		GenericProviderImage: "quay.io/konveyor/generic-external-provider:latest",
		CsharpProviderImage:  "quay.io/konveyor/c-sharp-provider:latest",
	}

	ctx := &AnalyzeCommandContext{
		log:          testr.NewWithOptions(t, testr.Options{Verbosity: 5}),
		providersMap: make(map[string]ProviderInit),
	}

	err := ctx.setProviderInitInfo([]string{
		util.JavaProvider, util.GoProvider, util.PythonProvider, util.NodeJSProvider,
	})
	require.NoError(t, err)

	// Verify all ports are unique
	ports := make(map[int]string)
	for name, init := range ctx.providersMap {
		existing, ok := ports[init.port]
		assert.False(t, ok, "providers %q and %q share the same port %d", name, existing, init.port)
		ports[init.port] = name
	}
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

func Test_analyzeCommand_getProviderLogs_EmptyProviders(t *testing.T) {
	a := &analyzeCommand{
		AnalyzeCommandContext: AnalyzeCommandContext{
			log:                    testr.NewWithOptions(t, testr.Options{Verbosity: 5}),
			providerContainerNames: map[string]string{},
		},
	}

	err := a.getProviderLogs(context.Background())
	require.NoError(t, err)
}

func Test_analyzeCommand_getProviderLogs_NeedsBuiltin(t *testing.T) {
	a := &analyzeCommand{
		AnalyzeCommandContext: AnalyzeCommandContext{
			log:                    testr.NewWithOptions(t, testr.Options{Verbosity: 5}),
			needsBuiltin:           true,
			providerContainerNames: map[string]string{"java": "java-container"},
		},
	}

	err := a.getProviderLogs(context.Background())
	require.NoError(t, err)
}
