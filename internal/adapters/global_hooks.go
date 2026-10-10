package adapters

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// UserHookFileRenderer is implemented by a target whose user-level hooks
// are one file per hook in a directory, rather than entries in a shared
// settings file.
type UserHookFileRenderer interface {
	UserHookFiles(hooks []spec.Entry, sourceDir, scriptsDir string) (map[string]string, []HookScript, error)
}

// UserHookFiles renders hooks as the files of target's user hooks
// directory, keyed by file name, with the shared scripts from sourceDir
// those hooks run, placed under scriptsDir.
func UserHookFiles(target string, hooks []spec.Entry, sourceDir, scriptsDir string) (map[string]string, []HookScript, error) {
	a, err := Resolve(target)
	if err != nil {
		return nil, nil, err
	}
	r, ok := a.(UserHookFileRenderer)
	if !ok {
		return nil, nil, fmt.Errorf("%s: user hook files are unsupported", target)
	}
	return r.UserHookFiles(hooks, sourceDir, scriptsDir)
}
