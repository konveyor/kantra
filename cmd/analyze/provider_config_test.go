package analyze

import (
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/require"
)

func Test_analyzeCommand_validateProviders(t *testing.T) {
	tests := []struct {
		name      string
		providers []string
		wantErr   bool
	}{
		{
			name:      "valid single provider",
			providers: []string{"java"},
			wantErr:   false,
		},
		{
			name:      "valid multiple providers",
			providers: []string{"java", "go", "python"},
			wantErr:   false,
		},
		{
			name:      "all valid providers",
			providers: []string{"java", "go", "python", "nodejs", "csharp"},
			wantErr:   false,
		},
		{
			name:      "empty providers",
			providers: []string{},
			wantErr:   false,
		},
		{
			name:      "unsupported provider returns error",
			providers: []string{"ruby"},
			wantErr:   true,
		},
		{
			name:      "mix of valid and invalid providers",
			providers: []string{"java", "invalid-provider"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &analyzeCommand{
				AnalyzeCommandContext: AnalyzeCommandContext{log: testr.NewWithOptions(t, testr.Options{Verbosity: 5})},
			}

			err := a.validateProviders(tt.providers)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
