package claude

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// NoteCursorDropsArgs warns once per run that Cursor, which loads
// .claude/settings.json hooks by default, reads only their `command`:
// an exec-form hook runs there as a bare interpreter, which reads the
// hook's JSON payload as its program. Call it when cursor is a target.
func NoteCursorDropsArgs(hooks []spec.Entry) {
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
	emit.NoteProject(fmt.Sprintf("Cursor runs .claude/settings.json hooks without args, so %d claude %s with args run the bare command there; use a shell-form command, or turn off Cursor's third-party hooks", count, noun))
}
