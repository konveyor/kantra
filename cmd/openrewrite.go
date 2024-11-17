package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/container"
	"github.com/spf13/cobra"
)

type openRewriteCommand struct {
	listTargets       bool
	input             string
	target            string
	goal              string
	miscOpts          string
	log               logr.Logger
	cleanup           bool
	mavenSettingsFile string
	mavenDebugLog	  string
}

func NewOpenRewriteCommand(log logr.Logger) *cobra.Command {
	openRewriteCmd := &openRewriteCommand{
		log:     log,
		cleanup: true,
	}

	openRewriteCommand := &cobra.Command{
		Use: "openrewrite",

		Short: "Transform application source code using OpenRewrite recipes",
		PreRun: func(cmd *cobra.Command, args []string) {
			if !cmd.Flags().Lookup("list-targets").Changed {
				cmd.MarkFlagRequired("input")
				cmd.MarkFlagRequired("target")
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if val, err := cmd.Flags().GetBool(noCleanupFlag); err == nil {
				openRewriteCmd.cleanup = !val
			}
			err := openRewriteCmd.Validate()
			if err != nil {
				log.Error(err, "failed validating input args")
				return err
			}
			err = openRewriteCmd.Run(cmd.Context())
			if err != nil {
				log.Error(err, "failed executing openrewrite recipe")
				return err
			}
			return nil
		},
	}
	openRewriteCommand.Flags().BoolVarP(&openRewriteCmd.listTargets, "list-targets", "l", false, "list all available OpenRewrite recipes")
	openRewriteCommand.Flags().StringVarP(&openRewriteCmd.target, "target", "t", "", "target openrewrite recipe to use. Run --list-targets to get a list of packaged recipes.")
	openRewriteCommand.Flags().StringVarP(&openRewriteCmd.goal, "goal", "g", "dryRun", "target goal")
	openRewriteCommand.Flags().StringVarP(&openRewriteCmd.input, "input", "i", "", "path to application source code directory")
	openRewriteCommand.Flags().StringVarP(&openRewriteCmd.mavenSettingsFile, "maven-settings", "s", "", "path to a custom maven settings file to use")
	openRewriteCommand.Flags().StringVarP(&openRewriteCmd.mavenDebugLog, "maven debug log level", "x", "", "set maven log to debug")

	return openRewriteCommand
}

func (o *openRewriteCommand) Validate() error {
	if o.listTargets {
		return nil
	}
	stat, err := os.Stat(o.input)
	if err != nil {
		return fmt.Errorf("%w failed to stat input directory %s", err, o.input)
	}
	if !stat.IsDir() {
		return fmt.Errorf("input path %s is not a directory", o.input)
	}

	if o.target == "" {
		return fmt.Errorf("target recipe must be specified")
	}

	if _, found := recipes[o.target]; !found {
		return fmt.Errorf("unsupported target recipe. use --list-targets to get list of all recipes")
	}
	// try to get abs path, if not, continue with relative path
	if absPath, err := filepath.Abs(o.input); err == nil {
		o.input = absPath
	}
	return nil
}

type recipe struct {
	names       []string
	path        string
	description string
}

var recipes = map[string]recipe{
	"eap8-xml": {
		names:       []string{"org.jboss.windup.eap8.FacesWebXml"},
		path:        "eap8/xml/rewrite.yml",
		description: "Transform Faces Web XML for EAP8 migration",
	},
	"jakarta-xml": {
		names:       []string{"org.jboss.windup.jakarta.javax.PersistenceXml"},
		path:        "jakarta/javax/xml/rewrite.yml",
		description: "Transform Persistence XML for Jakarta migration",
	},
	"jakarta-bootstrapping": {
		names:       []string{"org.jboss.windup.jakarta.javax.BootstrappingFiles"},
		path:        "jakarta/javax/bootstrapping/rewrite.yml",
		description: "Transform bootstrapping files for Jakarta migration",
	},
	"jakarta-imports": {
		names:       []string{"org.jboss.windup.JavaxToJakarta"},
		path:        "jakarta/javax/imports/rewrite.yml",
		description: "Transform dependencies and imports for Jakarta migration",
	},
	"quarkus-properties": {
		names:       []string{"org.jboss.windup.sb-quarkus.Properties"},
		path:        "quarkus/springboot/properties/rewrite.yml",
		description: "Migrate Springboot properties to Quarkus",
	},
}

func (o *openRewriteCommand) Run(ctx context.Context) error {
	if o.listTargets {
		fmt.Printf("%-20s\t%s\n", "NAME", "DESCRIPTION")
		for name, recipe := range recipes {
			fmt.Printf("%-20s\t%s\n", name, recipe.description)
		}
		return nil
	}

	volumes := map[string]string{
		o.input: InputPath,
	}
	args := []string{
		"-U", "org.openrewrite.maven:rewrite-maven-plugin:run",
		fmt.Sprintf("-Drewrite.configLocation=%s/%s",
			OpenRewriteRecipesPath, recipes[o.target].path),
		fmt.Sprintf("-Drewrite.activeRecipes=%s",
			strings.Join(recipes[o.target].names, ",")),
	}
	o.log.Info("executing openrewrite recipe",
		"recipe", o.target, "input", o.input, "args", strings.Join(args, " "))

	if o.mavenSettingsFile != "" {
		o.log.Info("using custom maven settings file", "path", o.mavenSettingsFile)
		args = append(args, "-s", o.mavenSettingsFile)
	}
	if o.mavenDebugLog != "" {
		o.log.Info("Setting Maven log to debug")
		args = append(args, "-x")
	}

	err := container.NewContainer().Run(
		ctx,
		container.WithImage(Settings.RunnerImage),
		container.WithLog(o.log.V(1)),
		container.WithEntrypointArgs(args...),
		container.WithEntrypointBin("/usr/bin/openrewrite_entrypoint.sh"),
		container.WithContainerToolBin(Settings.ContainerBinary),
		container.WithVolumes(volumes),
		container.WithWorkDir("/tmp/source-app/input"),
		container.WithCleanup(o.cleanup),
	)
	if err != nil {
		o.log.V(1).Error(err, "error running openrewrite")
		return err
	}
	return nil
}
