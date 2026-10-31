package emit

import "strings"

// hookSiblingPrefixes enumerates the per-tool hook directories the
// rewriter recognizes. Anything outside this list is treated as user
// content and left untouched.
var hookSiblingPrefixes = []string{
	".codebuddy/hooks/",
	".claude/hooks/",
	".codex/hooks/",
	".gemini/hooks/",
	".qwen/hooks/",
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
		cmd = rewriteSiblingPrefix(cmd, prefix, replacement)
	}
	return RewriteNeutralHookPath(cmd, HookScriptsDir(target), literal...)
}

func rewriteSiblingPrefix(cmd, prefix, replacement string) string {
	var out strings.Builder
	from := 0
	for search := 0; search < len(cmd); {
		i := strings.Index(cmd[search:], prefix)
		if i < 0 {
			break
		}
		at := search + i
		search = at + len(prefix)
		if !projectRelativeHookPrefix(cmd, at) {
			continue
		}
		out.WriteString(cmd[from:at])
		out.WriteString(replacement)
		from = search
	}
	out.WriteString(cmd[from:])
	return out.String()
}

const hookWordBoundary = " \t\r\n\"';|&()"

// projectRelativeHookPrefix reports whether the hook directory at index at
// belongs to the project: at a word start, or under a project root.
func projectRelativeHookPrefix(cmd string, at int) bool {
	if at == 0 || strings.ContainsRune(hookWordBoundary, rune(cmd[at-1])) {
		return true
	}
	if cmd[at-1] != '/' {
		return false
	}
	roots := []string{".", gitHookRoot, "$PWD", "${PWD}", "$(pwd)"}
	for _, variable := range nativeHookRoots {
		roots = append(roots, "$"+variable, "${"+variable+"}")
	}
	for _, root := range roots {
		for _, closer := range []string{"/", `"/`} {
			before := cmd[:at]
			if !strings.HasSuffix(before, root+closer) {
				continue
			}
			rootStart := len(before) - len(root+closer)
			if rootStart == 0 || strings.ContainsRune(hookWordBoundary, rune(cmd[rootStart-1])) {
				return true
			}
		}
	}
	return false
}

func HookScriptsDir(target string) string {
	switch target {
	case "kimi":
		return ".kimi-code/hooks"
	case "goose":
		return ".agents/plugins/agnostic-ai/hooks"
	case "copilot":
		return ".github/hooks/scripts"
	case "antigravity":
		return ".agents/hooks"
	case "windsurf":
		return ".devin/hooks"
	case "grok":
		return ".grok/hook-scripts"
	case "kiro":
		return ".kiro/scripts"
	case "cline":
		return ".cline/hooks/scripts"
	default:
		return "." + target + "/hooks"
	}
}

func RewriteNeutralHookPath(command, dir string, literal ...bool) string {
	return rewriteHookPathPrefix(command, agnosticScriptsDir+"/", dir, false, literal...)
}

func RewriteGrokHookPath(command, dir string, literal ...bool) string {
	command = rewriteHookPathPrefix(command, HookScriptsDir("grok")+"/", dir, true, literal...)
	return rewriteHookPathPrefix(command, agnosticScriptsDir+"/", dir, true, literal...)
}

func rewriteHookPathPrefix(command, prefix, dir string, directPath bool, literal ...bool) string {
	var out strings.Builder
	from := 0
	for _, ref := range hookPathReferences(command, prefix, len(literal) > 0 && literal[0]) {
		out.WriteString(command[from:ref.start])
		directory := strings.TrimRight(dir, "/")
		needsQuote := strings.IndexFunc(directory, func(r rune) bool { return !isShellWordRune(r) && (!directPath || r != '~') }) >= 0 || directPath && strings.HasPrefix(directory, "~")
		if (len(literal) == 0 || !literal[0]) && needsQuote {
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
	command = stripNeutralHookRoots(command, literal)
	if target == "grok" {
		command = stripHookPathRoots(command, HookScriptsDir(target)+"/", literal)
		command = RewriteGrokHookPath(command, scriptsDir, literal)
	} else {
		command = RewriteNeutralHookPath(command, scriptsDir, literal)
	}
	return RewriteGlobalHookRoot(command, target, metadata...)
}

func RewriteGlobalWindowsHookPath(command, scriptsDir string) string {
	return RewriteWindowsNeutralHookPath(stripNeutralHookRoots(command, false), scriptsDir)
}

func stripNeutralHookRoots(command string, literal bool) string {
	return stripHookPathRoots(command, agnosticScriptsDir+"/", literal)
}

func stripHookPathRoots(command, prefix string, literal bool) string {
	var out strings.Builder
	from := 0
	for _, ref := range hookPathReferences(command, prefix, literal) {
		out.WriteString(command[from:ref.rootStart])
		out.WriteString(command[ref.start:ref.end])
		from = ref.end
	}
	out.WriteString(command[from:])
	return out.String()
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
