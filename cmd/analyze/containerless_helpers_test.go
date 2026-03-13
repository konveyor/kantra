package analyze

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_renderProgressBar(t *testing.T) {
	tests := []struct {
		name    string
		percent int
		current int
		total   int
		message string
		wantIn  []string
	}{
		{
			name:    "0 percent",
			percent: 0,
			current: 0,
			total:   100,
			message: "testing",
			wantIn:  []string{"0%", "0/100", "testing"},
		},
		{
			name:    "50 percent",
			percent: 50,
			current: 50,
			total:   100,
			message: "halfway",
			wantIn:  []string{"50%", "50/100", "halfway"},
		},
		{
			name:    "100 percent",
			percent: 100,
			current: 100,
			total:   100,
			message: "done",
			wantIn:  []string{"100%", "100/100", "done"},
		},
		{
			name:    "percent over 100 capped",
			percent: 150,
			current: 150,
			total:   100,
			message: "over",
			wantIn:  []string{"150/100"},
		},
		{
			name:    "long message truncated",
			percent: 10,
			current: 10,
			total:   100,
			message: "this is a very long message that should be truncated because it exceeds the maximum length",
			wantIn:  []string{"..."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			require.NoError(t, err)
			oldStderr := os.Stderr
			os.Stderr = w

			renderProgressBar(tt.percent, tt.current, tt.total, tt.message)

			w.Close()
			var buf bytes.Buffer
			buf.ReadFrom(r)
			os.Stderr = oldStderr

			output := buf.String()
			for _, want := range tt.wantIn {
				assert.Contains(t, output, want)
			}
		})
	}
}

func Test_renderProgressBar_ContainsProgressBarChars(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	oldStderr := os.Stderr
	os.Stderr = w

	renderProgressBar(50, 50, 100, "test")

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stderr = oldStderr

	output := buf.String()
	assert.True(t, strings.Contains(output, "█"), "progress bar should contain filled character")
	assert.True(t, strings.Contains(output, "░"), "progress bar should contain empty character")
}

func TestConsoleHook_Fire(t *testing.T) {
	tests := []struct {
		name string
		data logrus.Fields
	}{
		{
			name: "process-rule logger triggers log",
			data: logrus.Fields{"logger": "process-rule", "ruleID": "rule-001"},
		},
		{
			name: "non-process-rule logger does nothing",
			data: logrus.Fields{"logger": "other-logger"},
		},
		{
			name: "no logger field",
			data: logrus.Fields{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := &ConsoleHook{
				Level: logrus.InfoLevel,
				Log:   testr.NewWithOptions(t, testr.Options{Verbosity: 5}),
			}

			entry := &logrus.Entry{
				Logger:  logrus.New(),
				Data:    tt.data,
				Level:   logrus.InfoLevel,
				Message: "test",
			}

			err := hook.Fire(entry)
			require.NoError(t, err)
		})
	}
}

func TestConsoleHook_Levels(t *testing.T) {
	hook := &ConsoleHook{
		Level: logrus.InfoLevel,
		Log:   testr.NewWithOptions(t, testr.Options{Verbosity: 5}),
	}

	levels := hook.Levels()
	require.NotEmpty(t, levels)
	assert.Len(t, levels, len(logrus.AllLevels))
}
