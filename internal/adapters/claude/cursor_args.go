package claude

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// NoteCursorDropsArgs warns once per run that Cursor, which loads the
// Claude settings file at path by default, documents no `args` for those
// hooks: an exec-form hook may run there as a bare interpreter, which
// reads the hook's JSON payload as its program. Call it when cursor is a
// configured target.
func NoteCursorDropsArgs(hooks []spec.Entry, path string) {
	count := 0
	for _, h := range hooks {
		kind, _ := h.Meta["type"].(string)
		if (kind == "" || kind == "command") && len(emit.StringSlice(h.Meta["args"])) > 0 {
			count++
		}
	}
	if count == 0 {
		return
	}
	noun := "hooks"
	if count == 1 {
		noun = "hook"
	}
	emit.NoteProject(fmt.Sprintf("Cursor's third-party hooks docs do not list args, so %d claude %s with args may run as a bare interpreter when Cursor loads %s; use a shell-form command, or turn off Cursor's third-party hooks", count, noun, path))
}
