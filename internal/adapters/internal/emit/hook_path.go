package emit

import "strings"

// hookSiblingPrefixes enumerates the per-tool hook directories the
// rewriter recognizes. Anything outside this list is treated as user
// content and left untouched.
var hookSiblingPrefixes = []string{
	".claude/hooks/",
	".codex/hooks/",
	".gemini/hooks/",
}

// RewriteHookPath translates the project root and sibling hook directories.
func RewriteHookPath(cmd, target string, metadata ...map[string]any) string {
	if cmd == "" || target == "" {
		return cmd
	}
	meta := hookRootMeta(target, metadata)
	return RewriteHookRoot(RewriteHookDirectories(cmd, target, len(StringSlice(meta["args"])) > 0 || target == "augment"), target, metadata...)
}

func RewriteHookDirectories(cmd, target string, literal ...bool) string {
	if cmd == "" || target == "" {
		return cmd
	}
	replacement := "." + target + "/hooks/"
	for _, prefix := range hookSiblingPrefixes {
		if prefix == replacement {
			continue
		}
		cmd = strings.ReplaceAll(cmd, prefix, replacement)
	}
	return RewriteNeutralHookPath(cmd, HookScriptsDir(target), literal...)
}

func HookScriptsDir(target string) string {
	switch target {
	case "goose":
		return ".agents/plugins/agnostic-ai/hooks"
	case "copilot":
		return ".github/hooks/scripts"
	case "antigravity":
		return ".agents/hooks"
	case "windsurf":
		return ".devin/hooks"
	case "kiro":
		return ".kiro/scripts"
	case "cline":
		return ".cline/hooks/scripts"
	default:
		return "." + target + "/hooks"
	}
}

func HasNeutralHookPath(command string) bool {
	return len(neutralHookReferences(command, false)) > 0
}

func RewriteNeutralHookPath(command, dir string, literal ...bool) string {
	const prefix = agnosticScriptsDir + "/"
	var out strings.Builder
	from := 0
	for _, ref := range neutralHookReferences(command, len(literal) > 0 && literal[0]) {
		out.WriteString(command[from:ref.start])
		directory := strings.TrimRight(dir, "/")
		if (len(literal) == 0 || !literal[0]) && strings.IndexFunc(directory, func(r rune) bool { return !isShellWordRune(r) }) >= 0 {
			switch ref.quote {
			case '"':
				directory = strings.NewReplacer("\\", "\\\\", "$", "\\$", "`", "\\`", "\"", "\\\"").Replace(directory)
			case '\'':
				directory = strings.ReplaceAll(directory, "'", "'\\''")
			default:
				directory = ShellQuote(directory)
			}
		}
		out.WriteString(directory + "/")
		out.WriteString(command[ref.start+len(prefix) : ref.end])
		from = ref.end
	}
	out.WriteString(command[from:])
	return out.String()
}

func RewriteGlobalHookPath(command, target, scriptsDir string, metadata ...map[string]any) string {
	meta := hookRootMeta(target, metadata)
	literal := len(StringSlice(meta["args"])) > 0 || target == "augment"
	var out strings.Builder
	from := 0
	for _, ref := range neutralHookReferences(command, literal) {
		out.WriteString(command[from:ref.rootStart])
		out.WriteString(command[ref.start:ref.end])
		from = ref.end
	}
	out.WriteString(command[from:])
	command = RewriteNeutralHookPath(out.String(), scriptsDir, literal)
	return RewriteGlobalHookRoot(command, target, metadata...)
}

func RewriteWindowsNeutralHookPath(command, dir string) string {
	const prefix = agnosticScriptsDir + "/"
	var out strings.Builder
	from := 0
	sep := "/"
	if strings.Contains(dir, `\`) {
		// cmd.exe reads a forward slash as a switch, so keep a native path native.
		sep = `\`
	}
	for _, ref := range neutralHookReferences(command, false) {
		out.WriteString(command[from:ref.start])
		directory := strings.TrimRight(dir, `/\`)
		if ref.quote == 0 {
			path := directory + sep + strings.ReplaceAll(ref.name, "/", sep)
			if strings.ContainsAny(path, " \t&|<>^()") {
				path = `"` + path + `"`
			}
			out.WriteString(path)
		} else {
			if ref.quote == '\'' {
				directory = strings.ReplaceAll(directory, "'", "''")
			}
			out.WriteString(directory + sep + strings.ReplaceAll(command[ref.start+len(prefix):ref.end], "/", sep))
		}
		from = ref.end
	}
	out.WriteString(command[from:])
	return out.String()
}
