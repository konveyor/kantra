package settings

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/codingconcepts/env"
)

// Build-time variables set via ldflags.
//
// Example:
//
//	--ldflags="-X 'github.com/konveyor-ecosystem/kantra/cmd/internal/settings.Version=1.2.3' \
//	           -X 'github.com/konveyor-ecosystem/kantra/cmd/internal/settings.BuildCommit=$(git rev-parse HEAD)'"
var (
	BuildCommit = ""
	Version     = "latest"

	RootCommandName      = "kantra"
	JavaBundlesLocation  = "/jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar"
	JDTLSBinLocation     = "/jdtls/bin/jdtls"
	MavenIndexPath       = "/usr/local/etc/maven-index.txt"
	DepOpenSourceLabels  = "/usr/local/etc/maven.default.index"
	RulesetsLocation     = "rulesets"
	JavaProviderImage    = "quay.io/konveyor/java-external-provider"
	GoProviderImage      = "quay.io/konveyor/go-external-provider"
	PythonProviderImage  = "quay.io/konveyor/python-external-provider"
	NodeJSProviderImage  = "quay.io/konveyor/nodejs-external-provider"
	CsharpProviderImage  = "quay.io/konveyor/c-sharp-provider"
	RunnerImage          = "quay.io/konveyor/kantra"
)

// Settings is the global configuration instance, loaded once at startup.
var Settings = &Config{}

// Config holds runtime configuration populated from environment variables and build-time defaults.
type Config struct {
	RootCommandName     string `env:"CMD_NAME" default:"kantra"`
	ContainerBinary     string `env:"CONTAINER_TOOL" default:"/usr/bin/podman"`
	RunnerImage         string `env:"RUNNER_IMG" default:"quay.io/konveyor/kantra"`
	JvmMaxMem           string `env:"JVM_MAX_MEM" default:""`
	JavaProviderImage   string `env:"JAVA_PROVIDER_IMG" default:"quay.io/konveyor/java-external-provider:latest"`
	GoProviderImage     string `env:"GO_PROVIDER_IMG" default:"quay.io/konveyor/go-external-provider:latest"`
	PythonProviderImage string `env:"PYTHON_PROVIDER_IMG" default:"quay.io/konveyor/python-external-provider:latest"`
	NodeJSProviderImage string `env:"NODEJS_PROVIDER_IMG" default:"quay.io/konveyor/nodejs-external-provider:latest"`
	CsharpProviderImage string `env:"CSHARP_PROVIDER_IMG" default:"quay.io/konveyor/c-sharp-provider:latest"`
}

func (c *Config) Load() error {
	if err := c.loadCommandName(); err != nil {
		return err
	}
	if err := c.loadDefaultPodmanBin(); err != nil {
		return err
	}
	if err := c.loadRunnerImg(); err != nil {
		return err
	}
	if err := c.loadProviders(); err != nil {
		return err
	}
	err := env.Set(c)
	if err != nil {
		return err
	}
	return nil
}

func (c *Config) loadDefaultPodmanBin() error {
	// Respect existing CONTAINER_TOOL setting.
	if os.Getenv("CONTAINER_TOOL") != "" {
		return nil
	}
	podmanBin := os.Getenv("PODMAN_BIN")
	if podmanBin != "" {
		os.Setenv("CONTAINER_TOOL", podmanBin)
		return nil
	}
	// Try to use podman. If it's not found, try to use docker.
	found, err := c.trySetDefaultPodmanBin("podman")
	if err != nil {
		return err
	}
	if !found {
		if _, err = c.trySetDefaultPodmanBin("docker"); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) trySetDefaultPodmanBin(file string) (found bool, err error) {
	path, err := exec.LookPath(file)
	// Ignore all errors other than ErrDot.
	if err != nil && errors.Is(err, exec.ErrDot) {
		return false, err
	}
	// If file was found in PATH and it's not already going to be used, specify it in the env var.
	if path != "" && path != c.ContainerBinary {
		os.Setenv("CONTAINER_TOOL", path)
		return true, nil
	}
	return false, nil
}

func (c *Config) loadRunnerImg() error {
	// TODO(maufart): ensure Config struct works/parses it values from ENV and defaults correctly
	// Respect existing RUNNER_IMG setting
	if os.Getenv("RUNNER_IMG") != "" {
		return nil
	}
	// if version tag is given in image
	img := strings.TrimSuffix(RunnerImage, fmt.Sprintf(":%v", Version))
	updatedImg := fmt.Sprintf("%v:%v", img, Version)
	err := os.Setenv("RUNNER_IMG", updatedImg)
	if err != nil {
		return err
	}

	return nil
}

func (c *Config) loadCommandName() error {
	if RootCommandName != "kantra" {
		err := os.Setenv("CMD_NAME", RootCommandName)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) loadProviders() error {
	// if version tag is given in image
	if os.Getenv("JAVA_PROVIDER_IMG") == "" {
		javaImg := strings.TrimSuffix(JavaProviderImage, fmt.Sprintf(":%v", Version))
		updatedJavaImg := fmt.Sprintf("%v:%v", javaImg, Version)
		err := os.Setenv("JAVA_PROVIDER_IMG", updatedJavaImg)
		if err != nil {
			return err
		}
	}

	if os.Getenv("GO_PROVIDER_IMG") == "" {
		goImg := strings.TrimSuffix(GoProviderImage, fmt.Sprintf(":%v", Version))
		updatedGoImg := fmt.Sprintf("%v:%v", goImg, Version)
		err := os.Setenv("GO_PROVIDER_IMG", updatedGoImg)
		if err != nil {
			return err
		}
	}

	if os.Getenv("PYTHON_PROVIDER_IMG") == "" {
		pythonImg := strings.TrimSuffix(PythonProviderImage, fmt.Sprintf(":%v", Version))
		updatedPythonImg := fmt.Sprintf("%v:%v", pythonImg, Version)
		err := os.Setenv("PYTHON_PROVIDER_IMG", updatedPythonImg)
		if err != nil {
			return err
		}
	}

	if os.Getenv("NODEJS_PROVIDER_IMG") == "" {
		nodejsImg := strings.TrimSuffix(NodeJSProviderImage, fmt.Sprintf(":%v", Version))
		updatedNodeJSImg := fmt.Sprintf("%v:%v", nodejsImg, Version)
		err := os.Setenv("NODEJS_PROVIDER_IMG", updatedNodeJSImg)
		if err != nil {
			return err
		}
	}

	if os.Getenv("CSHARP_PROVIDER_IMG") == "" {
		csharpImg := strings.TrimSuffix(CsharpProviderImage, fmt.Sprintf(":%v", Version))
		updatedCsharpImg := fmt.Sprintf("%v:%v", csharpImg, Version)
		err := os.Setenv("CSHARP_PROVIDER_IMG", updatedCsharpImg)
		if err != nil {
			return err
		}
	}

	return nil
}
