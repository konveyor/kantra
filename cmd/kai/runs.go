package kai

import (
	"context"
	"fmt"
	"os"
	"time"

	agenticv1alpha1 "github.com/konveyor/agentic-controller/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// pollInterval is how often run status is refreshed while waiting.
const pollInterval = 3 * time.Second

// isTerminalPhase reports whether a run has reached a final state.
func isTerminalPhase(phase agenticv1alpha1.AgentRunPhase) bool {
	return phase == "Succeeded" || phase == "Failed"
}

// waitForRun polls obj until its phase (read via phaseOf) becomes terminal or
// the context is cancelled, printing each phase transition.
func waitForRun(ctx context.Context, cl client.Client, obj client.Object, phaseOf func() agenticv1alpha1.AgentRunPhase) error {
	key := client.ObjectKeyFromObject(obj)
	var last agenticv1alpha1.AgentRunPhase
	for {
		if err := cl.Get(ctx, key, obj); err != nil {
			return fmt.Errorf("failed to poll run status: %w", err)
		}
		phase := phaseOf()
		if phase != last {
			fmt.Fprintf(os.Stdout, "  phase: %s\n", phase)
			last = phase
		}
		if isTerminalPhase(phase) {
			if phase == "Failed" {
				return fmt.Errorf("run %q failed", key.Name)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
