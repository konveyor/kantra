package kai

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	agenticv1alpha1 "github.com/konveyor/agentic-controller/api/v1alpha1"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func newWorkflowCommand(cfg *kaiConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workflow",
		Aliases: []string{"aw", "workflows"},
		Short:   "Manage Agent Workflows",
	}
	cmd.AddCommand(newWorkflowCreateCommand(cfg))
	cmd.AddCommand(newWorkflowEditCommand(cfg))
	cmd.AddCommand(newWorkflowDeleteCommand(cfg))
	cmd.AddCommand(newWorkflowListCommand(cfg))
	cmd.AddCommand(newWorkflowRunCommand(cfg))
	cmd.AddCommand(newWorkflowGetCommand(cfg))
	cmd.AddCommand(newWorkflowDescribeCommand(cfg))
	cmd.AddCommand(newWorkflowRunsCommand(cfg))
	return cmd
}

func newWorkflowCreateCommand(cfg *kaiConfig) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create an Agent Workflow via an interactive wizard",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return runWorkflowCreate(cmd.Context(), cfg, name, dryRun)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the AgentWorkflow YAML without creating it")
	return cmd
}

func runWorkflowCreate(ctx context.Context, cfg *kaiConfig, name string, dryRun bool) error {
	if err := requireTerminal(); err != nil {
		return err
	}
	cl, err := cfg.newClient()
	if err != nil {
		return err
	}

	agents, err := agentNames(ctx, cl, cfg.namespace)
	if err != nil {
		return fmt.Errorf("failed to list agents: %w", err)
	}
	if len(agents) == 0 {
		return fmt.Errorf("no agents found in namespace %q; create one first with 'kantra kai agent create'", cfg.namespace)
	}

	nameVal := name
	guide := ""
	fields := []huh.Field{}
	if name == "" {
		fields = append(fields, inputField("Workflow name", "my-workflow", &nameVal, requiredValidator("name")))
	}
	fields = append(fields, huh.NewText().Title("Guide (optional)").Value(&guide))
	if err := runForm(fields...); err != nil {
		return err
	}
	name = strings.TrimSpace(nameVal)

	stages, err := collectStages(agents)
	if err != nil {
		return err
	}
	if len(stages) == 0 {
		return fmt.Errorf("a workflow requires at least one stage")
	}

	wf := &agenticv1alpha1.AgentWorkflow{
		TypeMeta:   metav1.TypeMeta{APIVersion: agenticv1alpha1.GroupVersion.String(), Kind: "AgentWorkflow"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cfg.namespace},
		Spec: agenticv1alpha1.AgentWorkflowSpec{
			Guide:  strings.TrimSpace(guide),
			Stages: stages,
		},
	}
	return previewAndCreate(ctx, cl, wf, name, "workflow", cfg.namespace, dryRun)
}

// collectStages interactively gathers the ordered workflow stages.
func collectStages(agents []string) ([]agenticv1alpha1.AgentWorkflowStage, error) {
	var stages []agenticv1alpha1.AgentWorkflowStage
	for {
		prompt := "Add a stage?"
		if len(stages) > 0 {
			prompt = "Add another stage?"
		}
		add, err := confirm(prompt, len(stages) == 0)
		if err != nil {
			return nil, err
		}
		if !add {
			return stages, nil
		}
		var (
			sName        string
			agentRef     = agents[0]
			instructions string
		)
		if err := runForm(
			inputField("Stage name", "analyze", &sName, requiredValidator("stage name")),
			selectField("Agent", agents, &agentRef),
			huh.NewText().Title("Instructions (optional)").Value(&instructions),
		); err != nil {
			return nil, err
		}
		stages = append(stages, agenticv1alpha1.AgentWorkflowStage{
			Name:         strings.TrimSpace(sName),
			AgentRef:     agentRef,
			Instructions: strings.TrimSpace(instructions),
		})
	}
}

func newWorkflowEditCommand(cfg *kaiConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit an Agent Workflow in your $EDITOR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			wf := &agenticv1alpha1.AgentWorkflow{ObjectMeta: metav1.ObjectMeta{Name: args[0], Namespace: cfg.namespace}}
			return editResource(cmd.Context(), cl, wf)
		},
	}
}

func newWorkflowDeleteCommand(cfg *kaiConfig) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an Agent Workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			wf := &agenticv1alpha1.AgentWorkflow{ObjectMeta: metav1.ObjectMeta{Name: args[0], Namespace: cfg.namespace}}
			return deleteResource(cmd.Context(), cl, wf, args[0], "workflow", yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func newWorkflowListCommand(cfg *kaiConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List Agent Workflows",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			var list agenticv1alpha1.AgentWorkflowList
			if err := cl.List(cmd.Context(), &list, client.InNamespace(cfg.namespace)); err != nil {
				return fmt.Errorf("failed to list workflows: %w", err)
			}
			if len(list.Items) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "no workflows found in namespace %q\n", cfg.namespace)
				return nil
			}
			rows := make([][]string, 0, len(list.Items))
			for i := range list.Items {
				wf := &list.Items[i]
				rows = append(rows, []string{
					wf.Name,
					fmt.Sprintf("%d", len(wf.Spec.Stages)),
					conditionStatus(wf.Status.Conditions, "Ready"),
					age(wf.CreationTimestamp),
				})
			}
			table(cmd.OutOrStdout(), []string{"NAME", "STAGES", "READY", "AGE"}, rows)
			return nil
		},
	}
}

func newWorkflowRunCommand(cfg *kaiConfig) *cobra.Command {
	var (
		gateway    string
		paramFlags []string
		wait       bool
	)
	cmd := &cobra.Command{
		Use:   "run <workflow-name>",
		Short: "Run an Agent Workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowRun(cmd.Context(), cfg, args[0], gateway, paramFlags, wait)
		},
	}
	cmd.Flags().StringVar(&gateway, "gateway", "", "gateway to use for all stages")
	cmd.Flags().StringArrayVar(&paramFlags, "param", nil, "parameter value as key=value (repeatable)")
	cmd.Flags().BoolVar(&wait, "wait", false, "wait for the run to reach a terminal phase")
	return cmd
}

func runWorkflowRun(ctx context.Context, cfg *kaiConfig, name, gateway string, paramFlags []string, wait bool) error {
	cl, err := cfg.newClient()
	if err != nil {
		return err
	}

	wf := &agenticv1alpha1.AgentWorkflow{}
	if err := cl.Get(ctx, client.ObjectKey{Namespace: cfg.namespace, Name: name}, wf); err != nil {
		return fmt.Errorf("failed to get workflow %q: %w", name, err)
	}

	provided, err := parseParamFlags(paramFlags)
	if err != nil {
		return err
	}
	var params []agenticv1alpha1.AgentRunParam
	for k, v := range provided {
		params = append(params, agenticv1alpha1.AgentRunParam{Name: k, Value: v})
	}

	if gateway == "" && isInteractive() {
		if err := runForm(inputField("Gateway (optional, applies to all stages)", "", &gateway, nil)); err != nil {
			return err
		}
	}

	run := &agenticv1alpha1.AgentWorkflowRun{
		TypeMeta:   metav1.TypeMeta{APIVersion: agenticv1alpha1.GroupVersion.String(), Kind: "AgentWorkflowRun"},
		ObjectMeta: metav1.ObjectMeta{GenerateName: name + "-", Namespace: cfg.namespace},
		Spec: agenticv1alpha1.AgentWorkflowRunSpec{
			WorkflowRef: name,
			Gateway:     strings.TrimSpace(gateway),
			Params:      params,
		},
	}
	if err := cl.Create(ctx, run); err != nil {
		return fmt.Errorf("failed to create AgentWorkflowRun: %w", err)
	}
	fmt.Fprintf(os.Stdout, "workflow run %q created\n", run.Name)

	if wait {
		fmt.Fprintln(os.Stdout, "waiting for run to complete...")
		return waitForRun(ctx, cl, run, func() agenticv1alpha1.AgentRunPhase { return run.Status.Phase })
	}
	return nil
}
