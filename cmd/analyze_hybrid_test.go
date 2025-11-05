package cmd

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor-ecosystem/kantra/pkg/util"
)

func Test_analyzeCommand_createHybridProviderSettings_JavaProvider(t *testing.T) {
	a := &analyzeCommand{
		input:             "/test/input",
		mode:              "full",
		mavenSettingsFile: "/test/settings.xml",
		AnalyzeCommandContext: AnalyzeCommandContext{
			log: logr.Discard(),
			providersMap: map[string]ProviderInit{
				util.JavaProvider: {
					port:  12345,
					image: "quay.io/konveyor/java-external-provider:latest",
				},
			},
		},
	}

	configs, err := a.createHybridProviderSettings(nil)
	if err != nil {
		t.Fatalf("createHybridProviderSettings() error = %v", err)
	}

	// Should have 2 configs: Java + builtin
	if len(configs) != 2 {
		t.Errorf("createHybridProviderSettings() returned %d configs, want 2 (java + builtin)", len(configs))
	}

	// Find Java provider config
	var javaConfig *provider.Config
	for i := range configs {
		if configs[i].Name == util.JavaProvider {
			javaConfig = &configs[i]
			break
		}
	}

	if javaConfig == nil {
		t.Fatal("Java provider config not found")
	}

	// Verify Java provider settings
	if javaConfig.Address != "localhost:12345" {
		t.Errorf("Java provider address = %v, want localhost:12345", javaConfig.Address)
	}

	if len(javaConfig.InitConfig) != 1 {
		t.Fatalf("Java provider InitConfig length = %d, want 1", len(javaConfig.InitConfig))
	}

	// Check lspServerPath
	lspPath, ok := javaConfig.InitConfig[0].ProviderSpecificConfig[provider.LspServerPathConfigKey]
	if !ok {
		t.Error("Java provider missing lspServerPath")
	}
	if lspPath != "/jdtls/bin/jdtls" {
		t.Errorf("Java provider lspServerPath = %v, want /jdtls/bin/jdtls", lspPath)
	}

	// Check Maven settings
	mavenSettings, ok := javaConfig.InitConfig[0].ProviderSpecificConfig["mavenSettingsFile"]
	if !ok {
		t.Error("Java provider missing mavenSettingsFile")
	}
	if mavenSettings != "/test/settings.xml" {
		t.Errorf("Java provider mavenSettingsFile = %v, want /test/settings.xml", mavenSettings)
	}
}

func Test_analyzeCommand_createHybridProviderSettings_GoProvider(t *testing.T) {
	a := &analyzeCommand{
		input: "/test/input",
		mode:  "full",
		AnalyzeCommandContext: AnalyzeCommandContext{
			log: logr.Discard(),
			providersMap: map[string]ProviderInit{
				util.GoProvider: {
					port:  12346,
					image: "quay.io/konveyor/generic-external-provider:latest",
				},
			},
		},
	}

	configs, err := a.createHybridProviderSettings(nil)
	if err != nil {
		t.Fatalf("createHybridProviderSettings() error = %v", err)
	}

	// Find Go provider config
	var goConfig *provider.Config
	for i := range configs {
		if configs[i].Name == util.GoProvider {
			goConfig = &configs[i]
			break
		}
	}

	if goConfig == nil {
		t.Fatal("Go provider config not found")
	}

	// Verify Go provider settings
	if goConfig.Address != "localhost:12346" {
		t.Errorf("Go provider address = %v, want localhost:12346", goConfig.Address)
	}

	provConfig := goConfig.InitConfig[0].ProviderSpecificConfig

	// Check lspServerPath
	if provConfig[provider.LspServerPathConfigKey] != "/usr/local/bin/gopls" {
		t.Errorf("Go provider lspServerPath = %v, want /usr/local/bin/gopls",
			provConfig[provider.LspServerPathConfigKey])
	}

	// Check lspServerName
	if provConfig["lspServerName"] != "generic" {
		t.Errorf("Go provider lspServerName = %v, want generic", provConfig["lspServerName"])
	}

	// Check dependency provider
	if provConfig["dependencyProviderPath"] != "/usr/local/bin/golang-dependency-provider" {
		t.Errorf("Go provider dependencyProviderPath = %v, want /usr/local/bin/golang-dependency-provider",
			provConfig["dependencyProviderPath"])
	}
}

func Test_analyzeCommand_createHybridProviderSettings_PythonProvider(t *testing.T) {
	a := &analyzeCommand{
		input: "/test/input",
		mode:  "full",
		AnalyzeCommandContext: AnalyzeCommandContext{
			log: logr.Discard(),
			providersMap: map[string]ProviderInit{
				util.PythonProvider: {
					port:  12347,
					image: "quay.io/konveyor/generic-external-provider:latest",
				},
			},
		},
	}

	configs, err := a.createHybridProviderSettings(nil)
	if err != nil {
		t.Fatalf("createHybridProviderSettings() error = %v", err)
	}

	// Find Python provider config
	var pythonConfig *provider.Config
	for i := range configs {
		if configs[i].Name == util.PythonProvider {
			pythonConfig = &configs[i]
			break
		}
	}

	if pythonConfig == nil {
		t.Fatal("Python provider config not found")
	}

	provConfig := pythonConfig.InitConfig[0].ProviderSpecificConfig

	// Check lspServerPath
	if provConfig[provider.LspServerPathConfigKey] != "/usr/local/bin/pylsp" {
		t.Errorf("Python provider lspServerPath = %v, want /usr/local/bin/pylsp",
			provConfig[provider.LspServerPathConfigKey])
	}

	// Check lspServerName
	if provConfig["lspServerName"] != "generic" {
		t.Errorf("Python provider lspServerName = %v, want generic", provConfig["lspServerName"])
	}
}

func Test_analyzeCommand_createHybridProviderSettings_NodeJSProvider(t *testing.T) {
	a := &analyzeCommand{
		input: "/test/input",
		mode:  "full",
		AnalyzeCommandContext: AnalyzeCommandContext{
			log: logr.Discard(),
			providersMap: map[string]ProviderInit{
				util.NodeJSProvider: {
					port:  12348,
					image: "quay.io/konveyor/generic-external-provider:latest",
				},
			},
		},
	}

	configs, err := a.createHybridProviderSettings(nil)
	if err != nil {
		t.Fatalf("createHybridProviderSettings() error = %v", err)
	}

	// Find NodeJS provider config
	var nodejsConfig *provider.Config
	for i := range configs {
		if configs[i].Name == util.NodeJSProvider {
			nodejsConfig = &configs[i]
			break
		}
	}

	if nodejsConfig == nil {
		t.Fatal("NodeJS provider config not found")
	}

	provConfig := nodejsConfig.InitConfig[0].ProviderSpecificConfig

	// Check lspServerPath
	if provConfig[provider.LspServerPathConfigKey] != "/usr/local/bin/typescript-language-server" {
		t.Errorf("NodeJS provider lspServerPath = %v, want /usr/local/bin/typescript-language-server",
			provConfig[provider.LspServerPathConfigKey])
	}

	// Check lspServerName
	if provConfig["lspServerName"] != "nodejs" {
		t.Errorf("NodeJS provider lspServerName = %v, want nodejs", provConfig["lspServerName"])
	}

	// Check lspServerArgs
	args, ok := provConfig["lspServerArgs"].([]string)
	if !ok {
		t.Error("NodeJS provider lspServerArgs not found or wrong type")
	} else if len(args) != 1 || args[0] != "--stdio" {
		t.Errorf("NodeJS provider lspServerArgs = %v, want [--stdio]", args)
	}
}

func Test_analyzeCommand_createHybridProviderSettings_DotnetProvider(t *testing.T) {
	a := &analyzeCommand{
		input: "/test/input",
		mode:  "full",
		AnalyzeCommandContext: AnalyzeCommandContext{
			log: logr.Discard(),
			providersMap: map[string]ProviderInit{
				util.DotnetProvider: {
					port:  12349,
					image: "quay.io/konveyor/dotnet-external-provider:latest",
				},
			},
		},
	}

	configs, err := a.createHybridProviderSettings(nil)
	if err != nil {
		t.Fatalf("createHybridProviderSettings() error = %v", err)
	}

	// Find Dotnet provider config
	var dotnetConfig *provider.Config
	for i := range configs {
		if configs[i].Name == util.DotnetProvider {
			dotnetConfig = &configs[i]
			break
		}
	}

	if dotnetConfig == nil {
		t.Fatal("Dotnet provider config not found")
	}

	provConfig := dotnetConfig.InitConfig[0].ProviderSpecificConfig

	// Check lspServerPath (Windows path)
	expectedPath := "C:/Users/ContainerAdministrator/.dotnet/tools/csharp-ls.exe"
	if provConfig[provider.LspServerPathConfigKey] != expectedPath {
		t.Errorf("Dotnet provider lspServerPath = %v, want %v",
			provConfig[provider.LspServerPathConfigKey], expectedPath)
	}
}

func Test_analyzeCommand_createHybridProviderSettings_BuiltinProvider(t *testing.T) {
	a := &analyzeCommand{
		input: "/test/input",
		mode:  "full",
		AnalyzeCommandContext: AnalyzeCommandContext{
			log:          logr.Discard(),
			providersMap: map[string]ProviderInit{}, // No containerized providers
		},
	}

	// Test with excluded paths
	excludedPaths := []interface{}{"/test/target", "/test/build"}
	configs, err := a.createHybridProviderSettings(excludedPaths)
	if err != nil {
		t.Fatalf("createHybridProviderSettings() error = %v", err)
	}

	// Should have only builtin config
	if len(configs) != 1 {
		t.Errorf("createHybridProviderSettings() returned %d configs, want 1 (builtin only)", len(configs))
	}

	builtinConfig := configs[0]
	if builtinConfig.Name != "builtin" {
		t.Errorf("Provider name = %v, want builtin", builtinConfig.Name)
	}

	// Builtin should have no Address or BinaryPath
	if builtinConfig.Address != "" {
		t.Errorf("Builtin provider Address should be empty, got %v", builtinConfig.Address)
	}

	// Check excluded dirs
	excludedDirs, ok := builtinConfig.InitConfig[0].ProviderSpecificConfig["excludedDirs"]
	if !ok {
		t.Error("Builtin provider missing excludedDirs")
	} else {
		excludedDirsSlice, ok := excludedDirs.([]interface{})
		if !ok || len(excludedDirsSlice) != 2 {
			t.Errorf("Builtin provider excludedDirs = %v, want 2 paths", excludedDirs)
		}
	}
}

func Test_analyzeCommand_createHybridProviderSettings_MultipleProviders(t *testing.T) {
	a := &analyzeCommand{
		input: "/test/input",
		mode:  "full",
		AnalyzeCommandContext: AnalyzeCommandContext{
			log: logr.Discard(),
			providersMap: map[string]ProviderInit{
				util.JavaProvider: {
					port:  12345,
					image: "quay.io/konveyor/java-external-provider:latest",
				},
				util.GoProvider: {
					port:  12346,
					image: "quay.io/konveyor/generic-external-provider:latest",
				},
				util.PythonProvider: {
					port:  12347,
					image: "quay.io/konveyor/generic-external-provider:latest",
				},
			},
		},
	}

	configs, err := a.createHybridProviderSettings(nil)
	if err != nil {
		t.Fatalf("createHybridProviderSettings() error = %v", err)
	}

	// Should have 4 configs: Java, Go, Python + builtin
	if len(configs) != 4 {
		t.Errorf("createHybridProviderSettings() returned %d configs, want 4", len(configs))
	}

	// Verify all providers are present
	providerNames := make(map[string]bool)
	for _, config := range configs {
		providerNames[config.Name] = true
	}

	expectedProviders := []string{util.JavaProvider, util.GoProvider, util.PythonProvider, "builtin"}
	for _, expected := range expectedProviders {
		if !providerNames[expected] {
			t.Errorf("Missing provider config for %s", expected)
		}
	}

	// Verify each has unique port (except builtin)
	ports := make(map[string]bool)
	for _, config := range configs {
		if config.Name != "builtin" {
			if ports[config.Address] {
				t.Errorf("Duplicate port address: %s", config.Address)
			}
			ports[config.Address] = true
		}
	}
}

func Test_analyzeCommand_createHybridProviderSettings_WithProxy(t *testing.T) {
	a := &analyzeCommand{
		input:      "/test/input",
		mode:       "full",
		httpProxy:  "http://proxy.example.com:8080",
		httpsProxy: "https://proxy.example.com:8443",
		noProxy:    "localhost,127.0.0.1",
		AnalyzeCommandContext: AnalyzeCommandContext{
			log: logr.Discard(),
			providersMap: map[string]ProviderInit{
				util.JavaProvider: {
					port:  12345,
					image: "quay.io/konveyor/java-external-provider:latest",
				},
			},
		},
	}

	configs, err := a.createHybridProviderSettings(nil)
	if err != nil {
		t.Fatalf("createHybridProviderSettings() error = %v", err)
	}

	// Check Java provider has proxy
	var javaConfig *provider.Config
	for i := range configs {
		if configs[i].Name == util.JavaProvider {
			javaConfig = &configs[i]
			break
		}
	}

	if javaConfig == nil {
		t.Fatal("Java provider config not found")
	}

	if javaConfig.InitConfig[0].Proxy == nil {
		t.Fatal("Java provider missing proxy config")
	}

	proxy := javaConfig.InitConfig[0].Proxy
	if proxy.HTTPProxy != "http://proxy.example.com:8080" {
		t.Errorf("HTTPProxy = %v, want http://proxy.example.com:8080", proxy.HTTPProxy)
	}
	if proxy.HTTPSProxy != "https://proxy.example.com:8443" {
		t.Errorf("HTTPSProxy = %v, want https://proxy.example.com:8443", proxy.HTTPSProxy)
	}
	if proxy.NoProxy != "localhost,127.0.0.1" {
		t.Errorf("NoProxy = %v, want localhost,127.0.0.1", proxy.NoProxy)
	}

	// Check builtin also has proxy
	var builtinConfig *provider.Config
	for i := range configs {
		if configs[i].Name == "builtin" {
			builtinConfig = &configs[i]
			break
		}
	}

	if builtinConfig == nil {
		t.Fatal("Builtin provider config not found")
	}

	if builtinConfig.InitConfig[0].Proxy == nil {
		t.Error("Builtin provider missing proxy config")
	}
}
