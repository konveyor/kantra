package analyze

import (
	"github.com/go-logr/logr"
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
	networkName string
	volumeName  string
	// mavenCacheVolumeName tracks the persistent Maven cache volume (NOT removed during cleanup)
	mavenCacheVolumeName   string
	providerContainerNames map[string]string

	// for containerless cmd
	kantraDir string

	// StopHook, if set, is called when RunAnalysisContainerless deferred cleanup runs.
	// Used by tests to assert that cleanup executed (e.g. on early return).
	StopHook func()
}
