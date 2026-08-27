package kai

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	agenticv1alpha1 "github.com/konveyor/agentic-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// defaultLogTailLines is how many trailing pod-log lines describe shows.
const defaultLogTailLines int64 = 50

// kvPrinter accumulates aligned "key: value" lines and flushes them together.
type kvPrinter struct {
	w  io.Writer
	tw *tabwriter.Writer
}

func newKVPrinter(w io.Writer) *kvPrinter {
	return &kvPrinter{w: w, tw: tabwriter.NewWriter(w, 0, 2, 1, ' ', 0)}
}

func (p *kvPrinter) kv(key, value string) {
	fmt.Fprintf(p.tw, "%s:\t%s\n", key, value)
}

func (p *kvPrinter) flush() { _ = p.tw.Flush() }

// section prints a blank-line-separated heading directly to the underlying
// writer (outside the aligned block).
func section(w io.Writer, title string) {
	fmt.Fprintf(w, "\n%s\n", title)
}

// printConditions renders a resource's status conditions in a readable block.
func printConditions(w io.Writer, conditions []metav1.Condition) {
	if len(conditions) == 0 {
		fmt.Fprintln(w, "  <none>")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "  TYPE\tSTATUS\tREASON\tMESSAGE")
	for i := range conditions {
		c := conditions[i]
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", c.Type, c.Status, c.Reason, c.Message)
	}
	_ = tw.Flush()
}

// latestAgentRun returns the most recently created AgentRun that targets the
// named agent, or nil when the agent has never been run.
func latestAgentRun(ctx context.Context, cl client.Client, namespace, agentName string) (*agenticv1alpha1.AgentRun, error) {
	var list agenticv1alpha1.AgentRunList
	if err := cl.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	var latest *agenticv1alpha1.AgentRun
	for i := range list.Items {
		r := &list.Items[i]
		if r.Spec.AgentRef != agentName {
			continue
		}
		if latest == nil || r.CreationTimestamp.After(latest.CreationTimestamp.Time) {
			latest = r
		}
	}
	return latest, nil
}

// latestWorkflowRun returns the most recently created AgentWorkflowRun that
// targets the named workflow, or nil when it has never been run.
func latestWorkflowRun(ctx context.Context, cl client.Client, namespace, workflowName string) (*agenticv1alpha1.AgentWorkflowRun, error) {
	var list agenticv1alpha1.AgentWorkflowRunList
	if err := cl.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	var latest *agenticv1alpha1.AgentWorkflowRun
	for i := range list.Items {
		r := &list.Items[i]
		if r.Spec.WorkflowRef != workflowName {
			continue
		}
		if latest == nil || r.CreationTimestamp.After(latest.CreationTimestamp.Time) {
			latest = r
		}
	}
	return latest, nil
}

// podLogs returns the trailing tail lines of a pod's logs. The sandbox that
// backs a run is a single pod named after the run/sandbox.
func podLogs(ctx context.Context, cs *kubernetes.Clientset, namespace, podName string, tail int64) (string, error) {
	opts := &corev1.PodLogOptions{}
	if tail > 0 {
		opts.TailLines = &tail
	}
	req := cs.CoreV1().Pods(namespace).GetLogs(podName, opts)
	rc, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// indent prefixes every non-empty line of s with two spaces for readable
// nesting under a section heading.
func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n")
}
