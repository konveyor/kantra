package cmd

import (
	"reflect"
	"testing"
)

func Test_analyzeCommand_getLabelSelectorArgs(t *testing.T) {
	tests := []struct {
		name    string
		labelSelector string
		sources []string
		targets []string
		want    string
	}{
		{
			name: "neither sources nor targets must not create label selector",
		},
		{
			name:    "one target specified, return target, catch-all source and default labels",
			targets: []string{"test"},
			want:    "((konveyor.io/target=test) && konveyor.io/source) || (discovery)",
		},
		{
			name:    "one source specified, return source and default labels",
			sources: []string{"test"},
			want:    "(konveyor.io/source=test) || (discovery)",
		},
		{
			name:    "one source & one target specified, return source, target and default labels",
			sources: []string{"test"},
			targets: []string{"test"},
			want:    "((konveyor.io/target=test) && (konveyor.io/source=test)) || (discovery)",
		},
		{
			name:    "multiple sources specified, OR them all with default labels",
			sources: []string{"t1", "t2"},
			want:    "(konveyor.io/source=t1 || konveyor.io/source=t2) || (discovery)",
		},
		{
			name:    "multiple targets specified, OR them all, AND result with catch-all source label, finally OR with default labels",
			targets: []string{"t1", "t2"},
			want:    "((konveyor.io/target=t1 || konveyor.io/target=t2) && konveyor.io/source) || (discovery)",
		},
		{
			name:    "multiple sources & targets specified, OR them within each other, AND result with catch-all source label, finally OR with default labels",
			targets: []string{"t1", "t2"},
			sources: []string{"t1", "t2"},
			want:    "((konveyor.io/target=t1 || konveyor.io/target=t2) && (konveyor.io/source=t1 || konveyor.io/source=t2)) || (discovery)",
		},
		{
			name:    "return the labelSelector when specified",
			labelSelector: "example.io/target=foo",
			want:    "example.io/target=foo",
		},
		{
			name:    "labelSelector should win",
			targets: []string{"t1", "t2"},
			sources: []string{"t1", "t2"},
			labelSelector: "example.io/target=foo",
			want:    "example.io/target=foo",
		},
		{
			name:    "multiple sources & targets specified, OR them within each other, AND result with catch-all source label, finally OR with default labels",
			targets: []string{"t1", "t2"},
			sources: []string{"t1", "t2"},
			labelSelector: "",
			want:    "((konveyor.io/target=t1 || konveyor.io/target=t2) && (konveyor.io/source=t1 || konveyor.io/source=t2)) || (discovery)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &analyzeCommand{
				sources: tt.sources,
				targets: tt.targets,
				labelSelector: tt.labelSelector,
			}
			if got := a.getLabelSelector(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("analyzeCommand.getLabelSelectorArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}
