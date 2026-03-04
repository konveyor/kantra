package analyze

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/devfile/alizer/pkg/apis/model"
	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_analyzeCommand_ListAllProviders(t *testing.T) {
	// Capture stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	oldStdout := os.Stdout
	os.Stdout = w

	a := &analyzeCommand{
		AnalyzeCommandContext: AnalyzeCommandContext{
			log: testr.NewWithOptions(t, testr.Options{Verbosity: 5}),
		},
	}
	a.ListAllProviders()

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	output := buf.String()

	// Verify container providers are listed
	containerProviders := []string{"java", "python", "go", "csharp", "nodejs"}
	for _, prov := range containerProviders {
		assert.Contains(t, output, prov, "ListAllProviders() missing container provider %q", prov)
	}

	assert.Contains(t, output, "container analysis supported providers")
	assert.Contains(t, output, "containerless analysis supported providers")
}

func Test_listLanguages(t *testing.T) {
	tests := []struct {
		name      string
		languages []model.Language
		input     string
		wantErr   bool
		wantInOut string
	}{
		{
			name:      "no languages detected returns error",
			languages: []model.Language{},
			input:     "/path/to/app",
			wantErr:   true,
		},
		{
			name: "single language",
			languages: []model.Language{
				{Name: "Java", CanBeComponent: true},
			},
			input:     "/path/to/app",
			wantErr:   false,
			wantInOut: "Java",
		},
		{
			name: "multiple languages",
			languages: []model.Language{
				{Name: "Java", CanBeComponent: true},
				{Name: "Go", CanBeComponent: true},
			},
			input:     "/path/to/app",
			wantErr:   false,
			wantInOut: "Java",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout for non-error cases
			r, w, err := os.Pipe()
			require.NoError(t, err)
			oldStdout := os.Stdout
			os.Stdout = w

			err = listLanguages(tt.languages, tt.input)

			w.Close()
			var buf bytes.Buffer
			buf.ReadFrom(r)
			os.Stdout = oldStdout

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "failed to detect")
				return
			}

			require.NoError(t, err)

			output := buf.String()
			assert.Contains(t, output, tt.wantInOut)
			assert.Contains(t, output, tt.input)
			assert.Contains(t, output, "--list-providers")
		})
	}
}

func Test_listLanguages_EmptyReturnsError(t *testing.T) {
	err := listLanguages([]model.Language{}, "test-input")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "failed to detect"))
}
