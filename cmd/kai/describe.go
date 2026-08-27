package kai

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	agenticv1alpha1 "github.com/konveyor/agentic-controller/api/v1alpha1"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getResource fetches obj by name, translating not-found into a friendly error.
func getResource(ctx context.Context, cl client.Client, namespace, name string, obj client.Object, kind string) error {
	if err := cl.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%s %q not found in namespace %q", kind, name, namespace)
		}
		return fmt.Errorf("failed to get %s: %w", kind, err)
	}
	return nil
}

// dash returns s, or "<none>" when empty, for readable key/value output.
func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "<none>"
	}
	return s
}

// timeStr renders an optional timestamp for describe output.
func timeStr(t *metav1.Time) string {
	if t == nil || t.IsZero() {
		return "<none>"
	}
	return t.Time.Format("2006-01-02 15:04:05 MST")
}

// ---- Gateway ----

func newGatewayGetCommand(cfg *kaiConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Show a concise status summary for a Gateway",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			gw := &agenticv1alpha1.Gateway{}
			if err := getResource(cmd.Context(), cl, cfg.namespace, args[0], gw, "gateway"); err != nil {
				return err
			}
			p := newKVPrinter(cmd.OutOrStdout())
			p.kv("Name", gw.Name)
			p.kv("Namespace", gw.Namespace)
			p.kv("Provider", gw.Spec.Provider)
			p.kv("Model", gw.Spec.Model.Name)
			p.kv("Verified", strconv.FormatBool(gw.Status.ConnectionVerified))
			p.kv("Ready", conditionStatus(gw.Status.Conditions, "Ready"))
			p.kv("Age", age(gw.CreationTimestamp))
			p.flush()
			return nil
		},
	}
}

func newGatewayDescribeCommand(cfg *kaiConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "describe <name>",
		Short: "Show full status and configuration for a Gateway",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			gw := &agenticv1alpha1.Gateway{}
			if err := getResource(cmd.Context(), cl, cfg.namespace, args[0], gw, "gateway"); err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			p := newKVPrinter(w)
			p.kv("Name", gw.Name)
			p.kv("Namespace", gw.Namespace)
			p.kv("Provider", gw.Spec.Provider)
			p.kv("Endpoint", dash(gw.Spec.Endpoint))
			p.kv("Model", gw.Spec.Model.Name)
			p.kv("Context Window", strconv.FormatInt(gw.Spec.Model.ContextWindow, 10))
			p.kv("Model Tier", dash(gw.Spec.Model.Tier))
			p.kv("Credential Secret", dash(gw.Spec.CredentialRef.SecretName))
			p.kv("Credential Key", dash(gw.Spec.CredentialRef.Key))
			p.kv("Connection Verified", strconv.FormatBool(gw.Status.ConnectionVerified))
			p.kv("Age", age(gw.CreationTimestamp))
			p.flush()
			section(w, "Conditions:")
			printConditions(w, gw.Status.Conditions)
			return nil
		},
	}
}

// ---- Agent ----

func newAgentGetCommand(cfg *kaiConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Show a concise status summary for an Agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			ag := &agenticv1alpha1.Agent{}
			if err := getResource(cmd.Context(), cl, cfg.namespace, args[0], ag, "agent"); err != nil {
				return err
			}
			p := newKVPrinter(cmd.OutOrStdout())
			p.kv("Name", ag.Name)
			p.kv("Namespace", ag.Namespace)
			p.kv("Image", ag.Spec.Image)
			p.kv("Gateways", dash(strings.Join(agentGatewayRefs(ag), ", ")))
			p.kv("Ready", conditionStatus(ag.Status.Conditions, "Ready"))
			p.kv("Age", age(ag.CreationTimestamp))
			p.flush()
			return nil
		},
	}
}

func newAgentDescribeCommand(cfg *kaiConfig) *cobra.Command {
	var tail int64
	var runName string
	cmd := &cobra.Command{
		Use:   "describe <name>",
		Short: "Show full status for an Agent, including its latest run and logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			ag := &agenticv1alpha1.Agent{}
			if err := getResource(ctx, cl, cfg.namespace, args[0], ag, "agent"); err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			p := newKVPrinter(w)
			p.kv("Name", ag.Name)
			p.kv("Namespace", ag.Namespace)
			p.kv("Image", ag.Spec.Image)
			p.kv("Gateways", dash(strings.Join(agentGatewayRefs(ag), ", ")))
			p.kv("Skill Cards", dash(strings.Join(agentSkillCardRefs(ag), ", ")))
			p.kv("Skill Collections", dash(strings.Join(agentSkillCollectionRefs(ag), ", ")))
			p.kv("Age", age(ag.CreationTimestamp))
			p.flush()

			if p := strings.TrimSpace(ag.Spec.Prompt); p != "" {
				section(w, "Prompt:")
				fmt.Fprintln(w, indent(p))
			}
			if len(ag.Spec.Params) > 0 {
				section(w, "Parameters:")
				printAgentParams(w, ag.Spec.Params)
			}
			section(w, "Conditions:")
			printConditions(w, ag.Status.Conditions)

			// Latest (or requested) run plus its sandbox pod logs.
			var run *agenticv1alpha1.AgentRun
			if runName != "" {
				run = &agenticv1alpha1.AgentRun{}
				if err := getResource(ctx, cl, cfg.namespace, runName, run, "agent run"); err != nil {
					return err
				}
			} else {
				run, err = latestAgentRun(ctx, cl, cfg.namespace, ag.Name)
				if err != nil {
					return fmt.Errorf("failed to look up runs: %w", err)
				}
			}
			section(w, "Latest Run:")
			if run == nil {
				fmt.Fprintln(w, "  <none>")
				return nil
			}
			printAgentRun(w, run)

			cs, err := cfg.newClientset()
			if err != nil {
				return err
			}
			section(w, fmt.Sprintf("Logs (last %d lines):", tail))
			printRunLogs(ctx, cs, w, cfg.namespace, sandboxPod(run), tail)
			return nil
		},
	}
	cmd.Flags().Int64Var(&tail, "tail", defaultLogTailLines, "number of log lines to show from the run's pod")
	cmd.Flags().StringVar(&runName, "run", "", "inspect a specific AgentRun instead of the latest")
	return cmd
}

func newAgentRunsCommand(cfg *kaiConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "runs [agent-name]",
		Short: "List AgentRuns (optionally filtered by agent)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			filter := ""
			if len(args) > 0 {
				filter = args[0]
			}
			var list agenticv1alpha1.AgentRunList
			if err := cl.List(cmd.Context(), &list, client.InNamespace(cfg.namespace)); err != nil {
				return fmt.Errorf("failed to list agent runs: %w", err)
			}
			rows := make([][]string, 0, len(list.Items))
			for i := range list.Items {
				r := &list.Items[i]
				if filter != "" && r.Spec.AgentRef != filter {
					continue
				}
				rows = append(rows, []string{
					r.Name,
					r.Spec.AgentRef,
					dash(r.Spec.Gateway),
					string(r.Status.Phase),
					age(r.CreationTimestamp),
				})
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "no agent runs found in namespace %q\n", cfg.namespace)
				return nil
			}
			table(cmd.OutOrStdout(), []string{"NAME", "AGENT", "GATEWAY", "PHASE", "AGE"}, rows)
			return nil
		},
	}
}

// ---- Workflow ----

func newWorkflowGetCommand(cfg *kaiConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Show a concise status summary for an Agent Workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			wf := &agenticv1alpha1.AgentWorkflow{}
			if err := getResource(cmd.Context(), cl, cfg.namespace, args[0], wf, "workflow"); err != nil {
				return err
			}
			p := newKVPrinter(cmd.OutOrStdout())
			p.kv("Name", wf.Name)
			p.kv("Namespace", wf.Namespace)
			p.kv("Stages", strconv.Itoa(len(wf.Spec.Stages)))
			p.kv("Ready", conditionStatus(wf.Status.Conditions, "Ready"))
			p.kv("Age", age(wf.CreationTimestamp))
			p.flush()
			return nil
		},
	}
}

func newWorkflowDescribeCommand(cfg *kaiConfig) *cobra.Command {
	var tail int64
	var runName string
	cmd := &cobra.Command{
		Use:   "describe <name>",
		Short: "Show full status for an Agent Workflow, including its latest run and logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			wf := &agenticv1alpha1.AgentWorkflow{}
			if err := getResource(ctx, cl, cfg.namespace, args[0], wf, "workflow"); err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			p := newKVPrinter(w)
			p.kv("Name", wf.Name)
			p.kv("Namespace", wf.Namespace)
			p.kv("Stages", strconv.Itoa(len(wf.Spec.Stages)))
			p.kv("Age", age(wf.CreationTimestamp))
			p.flush()

			if g := strings.TrimSpace(wf.Spec.Guide); g != "" {
				section(w, "Guide:")
				fmt.Fprintln(w, indent(g))
			}
			if len(wf.Spec.Stages) > 0 {
				section(w, "Stages:")
				printWorkflowStages(w, wf.Spec.Stages)
			}
			section(w, "Conditions:")
			printConditions(w, wf.Status.Conditions)

			var run *agenticv1alpha1.AgentWorkflowRun
			if runName != "" {
				run = &agenticv1alpha1.AgentWorkflowRun{}
				if err := getResource(ctx, cl, cfg.namespace, runName, run, "workflow run"); err != nil {
					return err
				}
			} else {
				run, err = latestWorkflowRun(ctx, cl, cfg.namespace, wf.Name)
				if err != nil {
					return fmt.Errorf("failed to look up runs: %w", err)
				}
			}
			section(w, "Latest Run:")
			if run == nil {
				fmt.Fprintln(w, "  <none>")
				return nil
			}
			printWorkflowRun(w, run)

			// Resolve the AgentRun backing the current (or last) stage for logs.
			stageRunName := currentStageAgentRun(run)
			if stageRunName == "" {
				section(w, "Logs:")
				fmt.Fprintln(w, "  <no stage has started a run yet>")
				return nil
			}
			stageRun := &agenticv1alpha1.AgentRun{}
			if err := getResource(ctx, cl, cfg.namespace, stageRunName, stageRun, "agent run"); err != nil {
				section(w, "Logs:")
				fmt.Fprintf(w, "  <%v>\n", err)
				return nil
			}
			cs, err := cfg.newClientset()
			if err != nil {
				return err
			}
			section(w, fmt.Sprintf("Logs for stage run %q (last %d lines):", stageRunName, tail))
			printRunLogs(ctx, cs, w, cfg.namespace, sandboxPod(stageRun), tail)
			return nil
		},
	}
	cmd.Flags().Int64Var(&tail, "tail", defaultLogTailLines, "number of log lines to show from the run's pod")
	cmd.Flags().StringVar(&runName, "run", "", "inspect a specific AgentWorkflowRun instead of the latest")
	return cmd
}

func newWorkflowRunsCommand(cfg *kaiConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "runs [workflow-name]",
		Short: "List AgentWorkflowRuns (optionally filtered by workflow)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			filter := ""
			if len(args) > 0 {
				filter = args[0]
			}
			var list agenticv1alpha1.AgentWorkflowRunList
			if err := cl.List(cmd.Context(), &list, client.InNamespace(cfg.namespace)); err != nil {
				return fmt.Errorf("failed to list workflow runs: %w", err)
			}
			rows := make([][]string, 0, len(list.Items))
			for i := range list.Items {
				r := &list.Items[i]
				if filter != "" && r.Spec.WorkflowRef != filter {
					continue
				}
				rows = append(rows, []string{
					r.Name,
					r.Spec.WorkflowRef,
					dash(r.Status.CurrentStage),
					string(r.Status.Phase),
					age(r.CreationTimestamp),
				})
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "no workflow runs found in namespace %q\n", cfg.namespace)
				return nil
			}
			table(cmd.OutOrStdout(), []string{"NAME", "WORKFLOW", "CURRENT-STAGE", "PHASE", "AGE"}, rows)
			return nil
		},
	}
}

// ---- Skill ----

func newSkillGetCommand(cfg *kaiConfig, collection *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Show a concise status summary for a SkillCard (or SkillCollection)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if *collection {
				col := &agenticv1alpha1.SkillCollection{}
				if err := getResource(cmd.Context(), cl, cfg.namespace, args[0], col, "skillcollection"); err != nil {
					return err
				}
				p := newKVPrinter(w)
				p.kv("Name", col.Name)
				p.kv("Namespace", col.Namespace)
				p.kv("Resolved Skills", strconv.Itoa(len(col.Status.ResolvedSkills)))
				p.kv("Ready", conditionStatus(col.Status.Conditions, "Ready"))
				p.kv("Age", age(col.CreationTimestamp))
				p.flush()
				return nil
			}
			card := &agenticv1alpha1.SkillCard{}
			if err := getResource(cmd.Context(), cl, cfg.namespace, args[0], card, "skillcard"); err != nil {
				return err
			}
			p := newKVPrinter(w)
			p.kv("Name", card.Name)
			p.kv("Namespace", card.Namespace)
			p.kv("Type", string(card.Spec.Type))
			p.kv("Delivery Mode", dash(card.Status.DeliveryMode))
			p.kv("Ready", conditionStatus(card.Status.Conditions, "Ready"))
			p.kv("Age", age(card.CreationTimestamp))
			p.flush()
			return nil
		},
	}
}

func newSkillDescribeCommand(cfg *kaiConfig, collection *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "describe <name>",
		Short: "Show full status for a SkillCard (or SkillCollection)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if *collection {
				col := &agenticv1alpha1.SkillCollection{}
				if err := getResource(cmd.Context(), cl, cfg.namespace, args[0], col, "skillcollection"); err != nil {
					return err
				}
				p := newKVPrinter(w)
				p.kv("Name", col.Name)
				p.kv("Namespace", col.Namespace)
				p.kv("Image", dash(col.Spec.Image))
				p.kv("Version", dash(col.Spec.Version))
				p.kv("Age", age(col.CreationTimestamp))
				p.flush()
				if len(col.Status.ResolvedSkills) > 0 {
					section(w, "Resolved Skills:")
					for _, s := range col.Status.ResolvedSkills {
						fmt.Fprintf(w, "  - %s\n", s)
					}
				}
				section(w, "Conditions:")
				printConditions(w, col.Status.Conditions)
				return nil
			}
			card := &agenticv1alpha1.SkillCard{}
			if err := getResource(cmd.Context(), cl, cfg.namespace, args[0], card, "skillcard"); err != nil {
				return err
			}
			p := newKVPrinter(w)
			p.kv("Name", card.Name)
			p.kv("Namespace", card.Namespace)
			p.kv("Type", string(card.Spec.Type))
			p.kv("Display Name", dash(card.Spec.DisplayName))
			p.kv("Version", dash(card.Spec.Version))
			p.kv("Image", dash(card.Spec.Image))
			p.kv("Source", dash(card.Spec.Source))
			p.kv("Ref", dash(card.Spec.Ref))
			p.kv("SubPath", dash(card.Spec.SubPath))
			p.kv("Delivery Mode", dash(card.Status.DeliveryMode))
			p.kv("Resolved Image", dash(card.Status.ResolvedImage))
			p.kv("Tags", dash(strings.Join(card.Spec.Tags, ", ")))
			p.kv("Age", age(card.CreationTimestamp))
			p.flush()
			if d := strings.TrimSpace(card.Spec.Description); d != "" {
				section(w, "Description:")
				fmt.Fprintln(w, indent(d))
			}
			section(w, "Conditions:")
			printConditions(w, card.Status.Conditions)
			return nil
		},
	}
}

// ---- shared formatting helpers ----

func agentGatewayRefs(a *agenticv1alpha1.Agent) []string {
	out := make([]string, 0, len(a.Spec.Gateways))
	for _, g := range a.Spec.Gateways {
		out = append(out, g.Ref)
	}
	return out
}

func agentSkillCardRefs(a *agenticv1alpha1.Agent) []string {
	out := make([]string, 0, len(a.Spec.SkillCards))
	for _, s := range a.Spec.SkillCards {
		out = append(out, s.Ref)
	}
	return out
}

func agentSkillCollectionRefs(a *agenticv1alpha1.Agent) []string {
	out := make([]string, 0, len(a.Spec.SkillCollections))
	for _, s := range a.Spec.SkillCollections {
		out = append(out, s.Ref)
	}
	return out
}

func printAgentParams(w io.Writer, params []agenticv1alpha1.AgentParam) {
	rows := make([][]string, 0, len(params))
	for _, p := range params {
		rows = append(rows, []string{
			"  " + p.Name,
			string(p.Type),
			strconv.FormatBool(p.Required),
			dash(p.Default),
			dash(p.Description),
		})
	}
	table(w, []string{"  NAME", "TYPE", "REQUIRED", "DEFAULT", "DESCRIPTION"}, rows)
}

func printWorkflowStages(w io.Writer, stages []agenticv1alpha1.AgentWorkflowStage) {
	rows := make([][]string, 0, len(stages))
	for i, s := range stages {
		rows = append(rows, []string{
			fmt.Sprintf("  %d", i+1),
			s.Name,
			s.AgentRef,
			dash(s.Instructions),
		})
	}
	table(w, []string{"  #", "STAGE", "AGENT", "INSTRUCTIONS"}, rows)
}

func printAgentRun(w io.Writer, run *agenticv1alpha1.AgentRun) {
	p := newKVPrinter(w)
	p.kv("  Name", run.Name)
	p.kv("  Phase", string(run.Status.Phase))
	p.kv("  Gateway", dash(run.Spec.Gateway))
	p.kv("  Started", timeStr(run.Status.StartTime))
	p.kv("  Completed", timeStr(run.Status.CompletionTime))
	if run.Status.Duration != nil {
		p.kv("  Duration", fmt.Sprintf("%ds", *run.Status.Duration))
	}
	p.kv("  Ready", conditionStatus(run.Status.Conditions, "Ready"))
	p.flush()
}

func printWorkflowRun(w io.Writer, run *agenticv1alpha1.AgentWorkflowRun) {
	p := newKVPrinter(w)
	p.kv("  Name", run.Name)
	p.kv("  Phase", string(run.Status.Phase))
	p.kv("  Current Stage", dash(run.Status.CurrentStage))
	p.kv("  Started", timeStr(run.Status.StartTime))
	p.kv("  Completed", timeStr(run.Status.CompletionTime))
	p.flush()
	if len(run.Status.Stages) > 0 {
		rows := make([][]string, 0, len(run.Status.Stages))
		for _, s := range run.Status.Stages {
			rows = append(rows, []string{"  " + s.Name, string(s.Phase), dash(s.AgentRunName)})
		}
		table(w, []string{"  STAGE", "PHASE", "AGENT-RUN"}, rows)
	}
}

// sandboxPod returns the pod name backing a run: the Sandbox is named after the
// run, so the pod shares that name.
func sandboxPod(run *agenticv1alpha1.AgentRun) string {
	if run.Status.SandboxName != "" {
		return run.Status.SandboxName
	}
	return run.Name
}

// currentStageAgentRun returns the AgentRun name for the workflow's current
// stage, falling back to the last stage that has started a run.
func currentStageAgentRun(run *agenticv1alpha1.AgentWorkflowRun) string {
	if run.Status.CurrentStage != "" {
		for _, s := range run.Status.Stages {
			if s.Name == run.Status.CurrentStage && s.AgentRunName != "" {
				return s.AgentRunName
			}
		}
	}
	last := ""
	for _, s := range run.Status.Stages {
		if s.AgentRunName != "" {
			last = s.AgentRunName
		}
	}
	return last
}

func printRunLogs(ctx context.Context, cs *kubernetes.Clientset, w io.Writer, namespace, podName string, tail int64) {
	if podName == "" {
		fmt.Fprintln(w, "  <no sandbox pod yet>")
		return
	}
	logs, err := podLogs(ctx, cs, namespace, podName, tail)
	if err != nil {
		fmt.Fprintf(w, "  <could not fetch logs from pod %q: %v>\n", podName, err)
		return
	}
	if strings.TrimSpace(logs) == "" {
		fmt.Fprintln(w, "  <no logs>")
		return
	}
	fmt.Fprintln(w, indent(logs))
}
