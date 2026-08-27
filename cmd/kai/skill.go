package kai

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	agenticv1alpha1 "github.com/konveyor/agentic-controller/api/v1alpha1"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func newSkillCommand(cfg *kaiConfig) *cobra.Command {
	var collection bool
	cmd := &cobra.Command{
		Use:     "skill",
		Aliases: []string{"skc", "skills"},
		Short:   "Manage Skills (SkillCards, or SkillCollections with --collection)",
	}
	cmd.PersistentFlags().BoolVar(&collection, "collection", false, "operate on SkillCollections instead of SkillCards")
	cmd.AddCommand(newSkillCreateCommand(cfg, &collection))
	cmd.AddCommand(newSkillEditCommand(cfg, &collection))
	cmd.AddCommand(newSkillDeleteCommand(cfg, &collection))
	cmd.AddCommand(newSkillListCommand(cfg, &collection))
	cmd.AddCommand(newSkillGetCommand(cfg, &collection))
	cmd.AddCommand(newSkillDescribeCommand(cfg, &collection))
	return cmd
}

func newSkillCreateCommand(cfg *kaiConfig, collection *bool) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a SkillCard (or SkillCollection) via an interactive wizard",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			if *collection {
				return runSkillCollectionCreate(cmd.Context(), cfg, name, dryRun)
			}
			return runSkillCardCreate(cmd.Context(), cfg, name, dryRun)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the resource YAML without creating it")
	return cmd
}

func runSkillCardCreate(ctx context.Context, cfg *kaiConfig, name string, dryRun bool) error {
	if err := requireTerminal(); err != nil {
		return err
	}
	cl, err := cfg.newClient()
	if err != nil {
		return err
	}

	nameVal := name
	mode := "image"
	fields := []huh.Field{}
	if name == "" {
		fields = append(fields, inputField("SkillCard name", "javaee-to-quarkus", &nameVal, requiredValidator("name")))
	}
	fields = append(fields, selectField("Source", []string{"image", "source", "inline"}, &mode))
	if err := runForm(fields...); err != nil {
		return err
	}
	name = strings.TrimSpace(nameVal)

	spec := agenticv1alpha1.SkillCardSpec{}
	var subPath, ref string
	switch mode {
	case "image":
		if err := runForm(
			inputField("Image (OCI ref)", "quay.io/konveyor/skills:latest", &spec.Image, requiredValidator("image")),
			inputField("SubPath (optional)", "", &subPath, nil),
		); err != nil {
			return err
		}
	case "source":
		if err := runForm(
			inputField("Source (git URL)", "https://github.com/org/repo", &spec.Source, requiredValidator("source")),
			inputField("Ref (branch/tag/commit, optional)", "", &ref, nil),
			inputField("SubPath (optional)", "", &subPath, nil),
		); err != nil {
			return err
		}
	case "inline":
		if err := runForm(
			huh.NewText().Title("Inline markdown content").Value(&spec.Inline).Validate(requiredValidator("inline content")),
		); err != nil {
			return err
		}
	}
	spec.SubPath = strings.TrimSpace(subPath)
	spec.Ref = strings.TrimSpace(ref)

	skillType := "skill"
	var displayName, version, description, tags string
	if err := runForm(
		inputField("Display name (optional)", "", &displayName, nil),
		inputField("Version (optional)", "", &version, nil),
		inputField("Description (optional)", "", &description, nil),
		selectField("Type", []string{"skill", "rule"}, &skillType),
		inputField("Tags (comma-separated, optional)", "java,quarkus", &tags, nil),
	); err != nil {
		return err
	}
	spec.DisplayName = strings.TrimSpace(displayName)
	spec.Version = strings.TrimSpace(version)
	spec.Description = strings.TrimSpace(description)
	spec.Type = agenticv1alpha1.SkillCardType(skillType)
	spec.Tags = splitTags(tags)

	card := &agenticv1alpha1.SkillCard{
		TypeMeta:   metav1.TypeMeta{APIVersion: agenticv1alpha1.GroupVersion.String(), Kind: "SkillCard"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cfg.namespace},
		Spec:       spec,
	}
	return previewAndCreate(ctx, cl, card, name, "skillcard", cfg.namespace, dryRun)
}

func runSkillCollectionCreate(ctx context.Context, cfg *kaiConfig, name string, dryRun bool) error {
	if err := requireTerminal(); err != nil {
		return err
	}
	cl, err := cfg.newClient()
	if err != nil {
		return err
	}

	nameVal := name
	mode := "image"
	version := ""
	fields := []huh.Field{}
	if name == "" {
		fields = append(fields, inputField("SkillCollection name", "java-migration", &nameVal, requiredValidator("name")))
	}
	fields = append(fields,
		selectField("Populate from", []string{"image", "skill references"}, &mode),
		inputField("Version (optional)", "", &version, nil),
	)
	if err := runForm(fields...); err != nil {
		return err
	}
	name = strings.TrimSpace(nameVal)

	spec := agenticv1alpha1.SkillCollectionSpec{Version: strings.TrimSpace(version)}
	if mode == "image" {
		skillType := "skill"
		if err := runForm(
			inputField("Image (OCI ref)", "quay.io/konveyor/skills:latest", &spec.Image, requiredValidator("image")),
			selectField("Default type for enumerated skills", []string{"skill", "rule"}, &skillType),
		); err != nil {
			return err
		}
		spec.Type = agenticv1alpha1.SkillCardType(skillType)
	} else {
		existing, err := skillCardNames(ctx, cl, cfg.namespace)
		if err != nil {
			return err
		}
		if len(existing) == 0 {
			return fmt.Errorf("no SkillCards found in namespace %q to reference; create some first or use --collection with an image", cfg.namespace)
		}
		skills, err := collectCollectionSkills(existing)
		if err != nil {
			return err
		}
		if len(skills) == 0 {
			return fmt.Errorf("a skill-reference collection requires at least one skill")
		}
		spec.Skills = skills
	}

	col := &agenticv1alpha1.SkillCollection{
		TypeMeta:   metav1.TypeMeta{APIVersion: agenticv1alpha1.GroupVersion.String(), Kind: "SkillCollection"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cfg.namespace},
		Spec:       spec,
	}
	return previewAndCreate(ctx, cl, col, name, "skillcollection", cfg.namespace, dryRun)
}

// collectCollectionSkills interactively gathers skill references for a
// SkillCollection, each pointing at an existing SkillCard.
func collectCollectionSkills(cards []string) ([]agenticv1alpha1.SkillCollectionSkillRef, error) {
	var skills []agenticv1alpha1.SkillCollectionSkillRef
	for {
		add, err := confirm("Add a skill reference?", len(skills) == 0)
		if err != nil {
			return nil, err
		}
		if !add {
			return skills, nil
		}
		var (
			localName string
			cardRef   = cards[0]
		)
		if err := runForm(
			inputField("Local name", "javaee-to-quarkus", &localName, requiredValidator("local name")),
			selectField("SkillCard reference", cards, &cardRef),
		); err != nil {
			return nil, err
		}
		skills = append(skills, agenticv1alpha1.SkillCollectionSkillRef{
			Name:         strings.TrimSpace(localName),
			SkillCardRef: cardRef,
		})
	}
}

func splitTags(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func newSkillEditCommand(cfg *kaiConfig, collection *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit a SkillCard (or SkillCollection) in your $EDITOR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			var obj client.Object
			if *collection {
				obj = &agenticv1alpha1.SkillCollection{ObjectMeta: metav1.ObjectMeta{Name: args[0], Namespace: cfg.namespace}}
			} else {
				obj = &agenticv1alpha1.SkillCard{ObjectMeta: metav1.ObjectMeta{Name: args[0], Namespace: cfg.namespace}}
			}
			return editResource(cmd.Context(), cl, obj)
		},
	}
}

func newSkillDeleteCommand(cfg *kaiConfig, collection *bool) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a SkillCard (or SkillCollection)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			var obj client.Object
			kind := "skillcard"
			if *collection {
				obj = &agenticv1alpha1.SkillCollection{ObjectMeta: metav1.ObjectMeta{Name: args[0], Namespace: cfg.namespace}}
				kind = "skillcollection"
			} else {
				obj = &agenticv1alpha1.SkillCard{ObjectMeta: metav1.ObjectMeta{Name: args[0], Namespace: cfg.namespace}}
			}
			return deleteResource(cmd.Context(), cl, obj, args[0], kind, yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func newSkillListCommand(cfg *kaiConfig, collection *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List SkillCards (or SkillCollections)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := cfg.newClient()
			if err != nil {
				return err
			}
			if *collection {
				return listSkillCollections(cmd, cl, cfg.namespace)
			}
			return listSkillCards(cmd, cl, cfg.namespace)
		},
	}
}

func listSkillCards(cmd *cobra.Command, cl client.Client, namespace string) error {
	var list agenticv1alpha1.SkillCardList
	if err := cl.List(cmd.Context(), &list, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("failed to list skillcards: %w", err)
	}
	if len(list.Items) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "no skillcards found in namespace %q\n", namespace)
		return nil
	}
	rows := make([][]string, 0, len(list.Items))
	for i := range list.Items {
		s := &list.Items[i]
		rows = append(rows, []string{
			s.Name,
			string(s.Spec.Type),
			s.Status.DeliveryMode,
			conditionStatus(s.Status.Conditions, "Ready"),
			age(s.CreationTimestamp),
		})
	}
	table(cmd.OutOrStdout(), []string{"NAME", "TYPE", "DELIVERY", "READY", "AGE"}, rows)
	return nil
}

func listSkillCollections(cmd *cobra.Command, cl client.Client, namespace string) error {
	var list agenticv1alpha1.SkillCollectionList
	if err := cl.List(cmd.Context(), &list, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("failed to list skillcollections: %w", err)
	}
	if len(list.Items) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "no skillcollections found in namespace %q\n", namespace)
		return nil
	}
	rows := make([][]string, 0, len(list.Items))
	for i := range list.Items {
		s := &list.Items[i]
		rows = append(rows, []string{
			s.Name,
			fmt.Sprintf("%d", len(s.Status.ResolvedSkills)),
			conditionStatus(s.Status.Conditions, "Ready"),
			age(s.CreationTimestamp),
		})
	}
	table(cmd.OutOrStdout(), []string{"NAME", "SKILLS", "READY", "AGE"}, rows)
	return nil
}
