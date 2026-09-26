package emit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// ProjectLocalEntryPointPath is the project-relative path of the
// git-ignored instructions file in the project-user layer
// (`.agnostic-ai.local/`). Its text extends the shared
// AgnosticEntryPointPath body in every entry-point file sync writes,
// the same way `~/.agnostic-ai/local/AGNOSTIC_AI.md` extends the global
// instructions.
const ProjectLocalEntryPointPath = ".agnostic-ai.local/AGNOSTIC_AI.md"

// Sentinel markers delimiting the project-local instructions inside an
// entry-point file. Import strips the block (StripGeneratedAppendices)
// so private local text never flows back into the committed sources.
const (
	LocalStartMarker = "<!-- agnostic-ai:local:start -->"
	LocalEndMarker   = "<!-- agnostic-ai:local:end -->"
)

// ReadLocalInstructions returns the trimmed text of
// ProjectLocalEntryPointPath, or "" when the file is absent or blank.
// Any generated block pasted into it is dropped so the markers never
// nest.
func ReadLocalInstructions() (string, error) {
	data, err := os.ReadFile(ProjectLocalEntryPointPath)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", ProjectLocalEntryPointPath, err)
	}
	return strings.TrimSpace(StripGeneratedAppendices(StripHeader(string(data)))), nil
}

// AppendLocalInstructions returns body with local wrapped in the local
// sentinel markers and appended after one blank line, replacing any
// earlier block. Returns body unchanged when local is blank.
func AppendLocalInstructions(body, local string) string {
	local = strings.TrimSpace(local)
	if local == "" {
		return body
	}
	block := LocalStartMarker + "\n\n" + local + "\n\n" + LocalEndMarker + "\n"
	body = strings.TrimRight(StripLocalInstructions(body), "\n")
	if body == "" {
		return block
	}
	return body + "\n\n" + block
}

// StripLocalInstructions removes the sentinel-marked local block
// (markers included) from body. Returns body unchanged when no block is
// present.
func StripLocalInstructions(body string) string {
	return stripMarkedBlock(body, LocalStartMarker, LocalEndMarker)
}
