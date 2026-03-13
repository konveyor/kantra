package analyze

import (
	"github.com/go-logr/logr"
)

type AnalyzeCommandContext struct {
	// tempDirs list of temporary dirs created, used for cleanup
	tempDirs []string
	log      logr.Logger
	// isFileInput is set when input points to a file and not a dir
	isFileInput bool

	// for containerless cmd
	kantraDir string
}
