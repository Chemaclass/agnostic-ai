package approvals

import "maps"

// NativeRequest maps a tool call that target names its own way into the
// call names and input fields the gate reads. For a target it maps, a
// call it cannot read gets no name, so the gate asks for it.
func NativeRequest(target string, r Request) Request {
	switch target {
	case "kiro":
		return kiroRequest(r)
	case "qwen":
		return qwenRequest(r)
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
// for edits and commands, auto_edit approves edits. plan, auto and yolo
// keep their names, so the gate asks.
var qwenModes = map[string]string{"default": "default", "auto_edit": "acceptEdits"}

// qwenRequest maps the Qwen Code tools whose input the gate can read
// (packages/core/src/tools): run_shell_command, whose is_background is
// run_in_background, read_file, edit and write_file. A shell call with a
// directory keeps it, so the gate's field check asks. Every other call
// gets no name, so it asks.
func qwenRequest(r Request) Request {
	if mode, ok := qwenModes[r.PermissionMode]; ok {
		r.PermissionMode = mode
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
