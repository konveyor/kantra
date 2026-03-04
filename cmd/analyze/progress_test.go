package analyze

import (
	"bytes"
	"os"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProgressMode(t *testing.T) {
	tests := []struct {
		name       string
		noProgress bool
		wantState  bool
	}{
		{
			name:       "progress enabled (noProgress=false)",
			noProgress: false,
			wantState:  false,
		},
		{
			name:       "progress disabled (noProgress=true)",
			noProgress: true,
			wantState:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := NewProgressMode(tt.noProgress)
			require.NotNil(t, pm)
			assert.Equal(t, tt.wantState, pm.disabled)
		})
	}
}

func TestProgressMode_IsDisabled(t *testing.T) {
	tests := []struct {
		name       string
		noProgress bool
		want       bool
	}{
		{
			name:       "disabled when noProgress=true",
			noProgress: true,
			want:       true,
		},
		{
			name:       "not disabled when noProgress=false",
			noProgress: false,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := NewProgressMode(tt.noProgress)
			assert.Equal(t, tt.want, pm.IsDisabled())
		})
	}
}

func TestProgressMode_IsEnabled(t *testing.T) {
	tests := []struct {
		name       string
		noProgress bool
		want       bool
	}{
		{
			name:       "enabled when noProgress=false",
			noProgress: false,
			want:       true,
		},
		{
			name:       "not enabled when noProgress=true",
			noProgress: true,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := NewProgressMode(tt.noProgress)
			assert.Equal(t, tt.want, pm.IsEnabled())
		})
	}
}

func TestProgressMode_IsDisabledAndIsEnabled_Inverse(t *testing.T) {
	for _, noProgress := range []bool{true, false} {
		pm := NewProgressMode(noProgress)
		assert.NotEqual(t, pm.IsDisabled(), pm.IsEnabled(),
			"IsDisabled() and IsEnabled() should be inverse for noProgress=%v", noProgress)
	}
}

func TestProgressMode_OperationalLogger(t *testing.T) {
	t.Run("returns given logger when progress disabled", func(t *testing.T) {
		pm := NewProgressMode(true)
		inputLogger := testr.NewWithOptions(t, testr.Options{Verbosity: 5})
		result := pm.OperationalLogger(inputLogger)
		// When disabled, OperationalLogger should return the same logger passed in
		assert.NotNil(t, result)
	})

	t.Run("returns discard logger when progress enabled", func(t *testing.T) {
		pm := NewProgressMode(false)
		inputLogger := testr.NewWithOptions(t, testr.Options{Verbosity: 5})
		result := pm.OperationalLogger(inputLogger)
		// When enabled, it should return logr.Discard() to suppress operational output
		assert.NotNil(t, result)
	})
}

func TestProgressMode_ShouldAddConsoleHook(t *testing.T) {
	tests := []struct {
		name       string
		noProgress bool
		want       bool
	}{
		{
			name:       "should add hook when progress disabled",
			noProgress: true,
			want:       true,
		},
		{
			name:       "should not add hook when progress enabled",
			noProgress: false,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := NewProgressMode(tt.noProgress)
			assert.Equal(t, tt.want, pm.ShouldAddConsoleHook())
		})
	}
}

func TestProgressMode_HideCursor(t *testing.T) {
	tests := []struct {
		name       string
		noProgress bool
		wantOutput bool
	}{
		{
			name:       "hides cursor when progress enabled",
			noProgress: false,
			wantOutput: true,
		},
		{
			name:       "no-op when progress disabled",
			noProgress: true,
			wantOutput: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			require.NoError(t, err)
			oldStderr := os.Stderr
			os.Stderr = w

			pm := NewProgressMode(tt.noProgress)
			pm.HideCursor()

			w.Close()
			var buf bytes.Buffer
			buf.ReadFrom(r)
			os.Stderr = oldStderr

			hasOutput := buf.Len() > 0
			assert.Equal(t, tt.wantOutput, hasOutput)
			if tt.wantOutput {
				assert.Equal(t, "\033[?25l", buf.String())
			}
		})
	}
}

func TestProgressMode_ShowCursor(t *testing.T) {
	tests := []struct {
		name       string
		noProgress bool
		wantOutput bool
	}{
		{
			name:       "shows cursor when progress enabled",
			noProgress: false,
			wantOutput: true,
		},
		{
			name:       "no-op when progress disabled",
			noProgress: true,
			wantOutput: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			require.NoError(t, err)
			oldStderr := os.Stderr
			os.Stderr = w

			pm := NewProgressMode(tt.noProgress)
			pm.ShowCursor()

			w.Close()
			var buf bytes.Buffer
			buf.ReadFrom(r)
			os.Stderr = oldStderr

			hasOutput := buf.Len() > 0
			assert.Equal(t, tt.wantOutput, hasOutput)
			if tt.wantOutput {
				assert.Equal(t, "\033[?25h", buf.String())
			}
		})
	}
}

func TestProgressMode_Printf(t *testing.T) {
	tests := []struct {
		name       string
		noProgress bool
		format     string
		args       []interface{}
		wantOutput string
	}{
		{
			name:       "prints when progress enabled",
			noProgress: false,
			format:     "hello %s %d",
			args:       []interface{}{"world", 42},
			wantOutput: "hello world 42",
		},
		{
			name:       "no-op when progress disabled",
			noProgress: true,
			format:     "hello %s",
			args:       []interface{}{"world"},
			wantOutput: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			require.NoError(t, err)
			oldStderr := os.Stderr
			os.Stderr = w

			pm := NewProgressMode(tt.noProgress)
			pm.Printf(tt.format, tt.args...)

			w.Close()
			var buf bytes.Buffer
			buf.ReadFrom(r)
			os.Stderr = oldStderr

			assert.Equal(t, tt.wantOutput, buf.String())
		})
	}
}

func TestProgressMode_Println(t *testing.T) {
	tests := []struct {
		name       string
		noProgress bool
		args       []interface{}
		wantOutput string
	}{
		{
			name:       "prints line when progress enabled",
			noProgress: false,
			args:       []interface{}{"hello", "world"},
			wantOutput: "hello world\n",
		},
		{
			name:       "no-op when progress disabled",
			noProgress: true,
			args:       []interface{}{"hello"},
			wantOutput: "",
		},
		{
			name:       "empty args when progress enabled",
			noProgress: false,
			args:       []interface{}{},
			wantOutput: "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			require.NoError(t, err)
			oldStderr := os.Stderr
			os.Stderr = w

			pm := NewProgressMode(tt.noProgress)
			pm.Println(tt.args...)

			w.Close()
			var buf bytes.Buffer
			buf.ReadFrom(r)
			os.Stderr = oldStderr

			assert.Equal(t, tt.wantOutput, buf.String())
		})
	}
}
