package approvals

import (
	"maps"
	"path/filepath"
	"strings"
)

// NativeRequest maps a tool call that target names its own way into the
// call names and input fields the gate reads. For a target it maps, a
// call it cannot read gets no name, so the gate asks for it.
func NativeRequest(target string, r Request) Request {
	switch target {
	case "kiro":
		return kiroRequest(r)
	case "qwen":
		return qwenRequest(r)
	case "qoder":
		return qoderRequest(r)
	}
	return r
}

// kiroRequest maps the Kiro CLI V3 tools whose input the gate can read
// (kiro.dev/docs/hooks/types.md): execute_bash, whose summary describes
// the command, and a read_file of one path. Every other call, including
// one that borrows a name the gate knows, gets no name, so it asks.
func kiroRequest(r Request) Request {
	switch {
	case r.Call == "execute_bash":
		r.Call, r.Input = "Bash", renamed(r.Input, "summary", "description")
	case r.Call == "read_file" && onlyKeys(r.Input, []string{"path"}):
		r.Call, r.Input = "Read", renamed(r.Input, "path", "file_path")
	default:
		r.Call = ""
	}
	return r
}

// renamed is input with the field from renamed to to. A call that
// already holds to keeps both, so the gate's field check asks.
func renamed(input map[string]any, from, to string) map[string]any {
	value, ok := input[from]
	if !ok {
		return input
	}
	if _, clash := input[to]; clash {
		return input
	}
	out := maps.Clone(input)
	delete(out, from)
	out[to] = value
	return out
}

// qwenModes maps Qwen Code approval modes to the gate's: default asks
// for edits and commands, auto_edit approves edits. Any other mode, or
// none, keeps a name the gate does not approve in.
var qwenModes = map[string]string{"default": "default", "auto_edit": "acceptEdits"}

// qwenRequest maps the Qwen Code tools whose input the gate can read
// (packages/core/src/tools): run_shell_command, whose is_background is
// run_in_background, read_file, edit and write_file. A shell call with a
// directory keeps it, so the gate's field check asks. Every other call
// gets no name, so it asks.
func qwenRequest(r Request) Request {
	if mode, ok := qwenModes[r.PermissionMode]; ok {
		r.PermissionMode = mode
	} else {
		r.PermissionMode = "qwen:" + r.PermissionMode
	}
	switch r.Call {
	case "run_shell_command":
		r.Call, r.Input = "Bash", renamed(r.Input, "is_background", "run_in_background")
	case "read_file":
		r.Call = "Read"
	case "edit":
		r.Call = "Edit"
	case "write_file":
		r.Call = "Write"
	default:
		r.Call = ""
	}
	return r
}

// qoderModes maps Qoder CLI permission modes, which it accepts in snake
// or camel case, to the gate's (docs.qoder.com/cli/permissions).
var qoderModes = map[string]string{"default": "default", "accept_edits": "acceptEdits", "acceptedits": "acceptEdits"}

// qoderRequest keeps the Claude-style names Qoder CLI uses for the tools
// whose input the gate reads: Bash, Read, Edit and Write. Every other
// call gets no name, and a mode other than default or accept edits keeps
// a name the gate does not approve in. Qoder's protected paths, which
// its docs list only by example (.git, .ripgreprc, Git config, shell
// startup files), are dot names, so an Edit or Write through any dot
// folder or file gets no name either.
func qoderRequest(r Request) Request {
	if mode, ok := qoderModes[strings.ToLower(r.PermissionMode)]; ok {
		r.PermissionMode = mode
	} else {
		r.PermissionMode = "qoder:" + r.PermissionMode
	}
	switch r.Call {
	case "Bash", "Read":
	case "Edit", "Write":
		if path, _ := r.Input["file_path"].(string); dotPath(underCwd(path, r.Cwd)) {
			r.Call = ""
		}
	default:
		r.Call = ""
	}
	return r
}

// underCwd is path relative to cwd when it lies there, so a dot folder
// above the session folder does not count.
func underCwd(path, cwd string) string {
	if cwd == "" {
		return path
	}
	if rel, err := filepath.Rel(cwd, path); err == nil && filepath.IsAbs(path) && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}

func dotPath(path string) bool {
	for part := range strings.FieldsFuncSeq(path, func(c rune) bool { return c == '/' || c == '\\' }) {
		if strings.HasPrefix(part, ".") && part != "." && part != ".." {
			return true
		}
	}
	return false
}
