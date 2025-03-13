package cmd

import "path"

// analyzer container paths
const (
	RulesetPath            = "/opt/rulesets"
	OpenRewriteRecipesPath = "/opt/openrewrite"
	InputPath              = "/opt/input"
	OutputPath             = "/opt/output"
	XMLRulePath            = "/opt/xmlrules"
	ShimOutputPath         = "/opt/shimoutput"
	CustomRulePath         = "/opt/input/rules"
)

// supported providers
const (
	javaProvider            = "java"
	goProvider              = "go"
	pythonProvider          = "python"
	nodeJSProvider          = "nodejs"
	dotnetProvider          = "dotnet"
	dotnetFrameworkProvider = "dotnetframework"
)

// valid java file extensions
const (
	JavaArchive       = ".jar"
	WebArchive        = ".war"
	EnterpriseArchive = ".ear"
	ClassFile         = ".class"
)

var (
	// TODO (pgaikwad): this assumes that the $USER in container is always root, it may not be the case in future
	M2Dir = path.Join("/", "root", ".m2")
	// application source path inside the container
	SourceMountPath = path.Join(InputPath, "source")
	// analyzer config files
	ConfigMountPath = path.Join(InputPath, "config")
	// user provided rules path
	RulesMountPath = path.Join(RulesetPath, "input")
	// paths to files in the container
	AnalysisOutputMountPath   = path.Join(OutputPath, "output.yaml")
	DepsOutputMountPath       = path.Join(OutputPath, "dependencies.yaml")
	ProviderSettingsMountPath = path.Join(ConfigMountPath, "settings.json")
	DotnetFrameworks          = map[string]bool{
		"v1.0":   false,
		"v1.1":   false,
		"v2.0":   false,
		"v3.0":   false,
		"v3.5":   false,
		"v4":     false,
		"v4.5":   true,
		"v4.5.1": true,
		"v4.5.2": true,
		"v4.6":   true,
		"v4.6.1": true,
		"v4.6.2": true,
		"v4.7":   true,
		"v4.7.1": true,
		"v4.7.2": true,
		"v4.8":   true,
		"v4.8.1": true,
	}
)
