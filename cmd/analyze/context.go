package analyze

import (
	"fmt"
	"strings"
	"unicode"

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

	// parsedContainerRuntime is set during Validate from containerRuntimeFlags on analyzeCommand.
	parsedContainerRuntime []string
}

func parseContainerRuntimeFlags(input string) ([]string, error) {
	var args []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false
	tokenStarted := false

	flush := func() {
		if tokenStarted {
			args = append(args, current.String())
			current.Reset()
			tokenStarted = false
		}
	}

	for _, r := range input {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
			tokenStarted = true
		case r == '\\' && !inSingle:
			escaped = true
			tokenStarted = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
			tokenStarted = true
		case r == '"' && !inSingle:
			inDouble = !inDouble
			tokenStarted = true
		case unicode.IsSpace(r) && !inSingle && !inDouble:
			flush()
		default:
			current.WriteRune(r)
			tokenStarted = true
		}
	}

	if escaped {
		return nil, fmt.Errorf("trailing escape in container runtime flags")
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quote in container runtime flags")
	}
	flush()
	return args, nil
}
