package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/devfile/alizer/pkg/apis/model"
	"github.com/go-logr/logr"
	"github.com/konveyor-ecosystem/kantra/pkg/container"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/phayes/freeport"
	"gopkg.in/yaml.v2"
)

type AnalyzeCommandContext struct {
	providersMap map[string]ProviderInit

	// tempDirs list of temporary dirs created, used for cleanup
	tempDirs []string
	log      logr.Logger
	// isFileInput is set when input points to a file and not a dir
	isFileInput  bool
	needsBuiltin bool
	// used for cleanup
	networkName            string
	volumeName             string
	providerContainerNames []string

	// for containerless cmd
	reqMap    map[string]string
	kantraDir string
}

func (c *AnalyzeCommandContext) setProviders(providers []string, languages []model.Language, foundProviders []string) ([]string, error) {
	if len(providers) > 0 {
		for _, p := range providers {
			foundProviders = append(foundProviders, p)
			return foundProviders, nil
		}
	}
	for _, l := range languages {
		if l.CanBeComponent {
			c.log.V(5).Info("Got language", "component language", l)
			if l.Name == "C#" {
				for _, item := range l.Frameworks {
					supported, ok := DotnetFrameworks[item]
					if ok {
						if !supported {
							err := fmt.Errorf("unsupported .NET Framework version")
							c.log.Error(err, ".NET Framework version must be greater or equal 'v4.5'")
							return foundProviders, err
						}
						return []string{dotnetFrameworkProvider}, nil
					}
				}
				foundProviders = append(foundProviders, dotnetProvider)
				continue
			}
			// typescript ls supports both TS and JS
			if l.Name == "JavaScript" || l.Name == "TypeScript" {
				foundProviders = append(foundProviders, nodeJSProvider)
			} else {
				foundProviders = append(foundProviders, strings.ToLower(l.Name))
			}
		}
	}
	return foundProviders, nil
}

func (c *AnalyzeCommandContext) setProviderInitInfo(foundProviders []string) error {
	for _, prov := range foundProviders {
		port, err := freeport.GetFreePort()
		if err != nil {
			return err
		}
		switch prov {
		case javaProvider:
			c.providersMap[javaProvider] = ProviderInit{
				port:     port,
				image:    Settings.JavaProviderImage,
				provider: &JavaProvider{},
			}
		case goProvider:
			c.providersMap[goProvider] = ProviderInit{
				port:     port,
				image:    Settings.GenericProviderImage,
				provider: &GoProvider{},
			}
		case pythonProvider:
			c.providersMap[pythonProvider] = ProviderInit{
				port:     port,
				image:    Settings.GenericProviderImage,
				provider: &PythonProvider{},
			}
		case nodeJSProvider:
			c.providersMap[nodeJSProvider] = ProviderInit{
				port:     port,
				image:    Settings.GenericProviderImage,
				provider: &NodeJsProvider{},
			}
		case dotnetProvider:
			c.providersMap[dotnetProvider] = ProviderInit{
				port:     port,
				image:    Settings.DotnetProviderImage,
				provider: &DotNetProvider{},
			}
		}
	}
	return nil
}

func (c *AnalyzeCommandContext) handleDir(p string, tempDir string, basePath string) error {
	newDir, err := filepath.Rel(basePath, p)
	if err != nil {
		return err
	}
	tempDir = filepath.Join(tempDir, newDir)
	c.log.Info("creating nested tmp dir", "tempDir", tempDir, "newDir", newDir)
	err = os.Mkdir(tempDir, 0777)
	if err != nil {
		return err
	}
	c.log.V(5).Info("create temp rule set for dir", "dir", tempDir)
	err = c.createTempRuleSet(tempDir, filepath.Base(p))
	if err != nil {
		c.log.V(1).Error(err, "failed to create temp ruleset", "path", tempDir)
		return err
	}
	return err
}

func (c *AnalyzeCommandContext) createTempRuleSet(path string, name string) error {
	c.log.Info("creating temp ruleset ", "path", path, "name", name)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	tempRuleSet := engine.RuleSet{
		Name:        name,
		Description: "temp ruleset",
	}
	yamlData, err := yaml.Marshal(&tempRuleSet)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(path, "ruleset.yaml"), yamlData, os.ModePerm)
	if err != nil {
		return err
	}
	return nil
}

func (c *AnalyzeCommandContext) createContainerNetwork() (string, error) {
	networkName := fmt.Sprintf("network-%v", container.RandomName())
	args := []string{
		"network",
		"create",
		networkName,
	}

	cmd := exec.Command(Settings.ContainerBinary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	c.log.V(1).Info("created container network", "network", networkName)
	// for cleanup
	c.networkName = networkName
	return networkName, nil
}

// TODO: create for each source input once accepting multiple apps is completed
func (c *AnalyzeCommandContext) createContainerVolume(inputPath string) (string, error) {
	volName := fmt.Sprintf("volume-%v", container.RandomName())
	input, err := filepath.Abs(inputPath)
	if err != nil {
		return "", err
	}

	if c.isFileInput {
		//create temp dir and move bin file to mount
		file := filepath.Base(input)
		tempDir, err := os.MkdirTemp("", "java-bin-")
		if err != nil {
			c.log.V(1).Error(err, "failed creating temp dir", "dir", tempDir)
			return "", err
		}
		c.log.V(1).Info("created temp directory for Java input file", "dir", tempDir)
		// for cleanup
		c.tempDirs = append(c.tempDirs, tempDir)

		err = CopyFileContents(input, filepath.Join(tempDir, file))
		if err != nil {
			c.log.V(1).Error(err, "failed copying binary file")
			return "", err
		}
		input = tempDir
	}
	if runtime.GOOS == "windows" {
		// TODO(djzager): Thank ChatGPT
		// Extract the volume name (e.g., "C:")
		// Remove the volume name from the path to get the remaining part
		// Convert backslashes to forward slashes
		// Remove the colon from the volume name and convert to lowercase
		volumeName := filepath.VolumeName(input)
		remainingPath := input[len(volumeName):]
		remainingPath = filepath.ToSlash(remainingPath)
		driveLetter := strings.ToLower(strings.TrimSuffix(volumeName, ":"))

		// Construct the Linux-style path
		input = fmt.Sprintf("/mnt/%s%s", driveLetter, remainingPath)
	}

	args := []string{
		"volume",
		"create",
		"--opt",
		"type=none",
		"--opt",
		fmt.Sprintf("device=%v", input),
		"--opt",
		"o=bind",
		volName,
	}
	cmd := exec.Command(Settings.ContainerBinary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return "", err
	}
	c.log.V(1).Info("created container volume", "volume", volName)
	// for cleanup
	c.volumeName = volName
	return volName, nil
}
