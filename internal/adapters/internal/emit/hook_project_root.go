package emit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const claudeProjectDir = "CLAUDE_PROJECT_DIR"
const gitHookRoot = "$(git rev-parse --show-toplevel)"

var nativeHookRoots = map[string]string{
	"claude":  claudeProjectDir,
	"cursor":  claudeProjectDir,
	"trae":    claudeProjectDir,
	"gemini":  "GEMINI_PROJECT_DIR",
	"qoder":   "QODER_PROJECT_DIR",
	"factory": "FACTORY_PROJECT_DIR",
}

type hookRootReference struct {
	start, end int
	quoted     bool
}

func hookRootReferences(command string) ([]hookRootReference, string) {
	var references []hookRootReference
	var quote byte
	for i := 0; i < len(command); i++ {
		char := command[i]
		if quote == '\'' {
			if char == '\'' {
				quote = 0
			}
			continue
		}
		if char == '\\' {
			i++
			continue
		}
		if char == '\'' && quote == 0 {
			quote = char
			continue
		}
		if char == '"' {
			if quote == 0 {
				quote = char
			} else {
				quote = 0
			}
			continue
		}
		if strings.HasPrefix(command[i:], "%"+claudeProjectDir+"%") {
			return references, "cmd root syntax cannot be translated"
		}
		if char != '$' {
			continue
		}
		rest := command[i:]
		if hookRootVariablePrefix(rest, "${#"+claudeProjectDir) || hookRootVariablePrefix(rest, "${!"+claudeProjectDir) || hookRootVariablePrefix(rest, "$env:"+claudeProjectDir) || strings.HasPrefix(rest, "${env:"+claudeProjectDir+"}") {
			return references, "length, indirect, or PowerShell root syntax cannot be translated"
		}
		length := 0
		switch {
		case strings.HasPrefix(rest, "${"+claudeProjectDir):
			end := len("${" + claudeProjectDir)
			if len(rest) > end && rootVariableRune(rest[end]) {
				continue
			}
			if len(rest) <= end || rest[end] != '}' {
				return references, "parameter operators on CLAUDE_PROJECT_DIR cannot be translated"
			}
			length = end + 1
		case strings.HasPrefix(rest, "$"+claudeProjectDir):
			length = len("$" + claudeProjectDir)
			if len(rest) > length && rootVariableRune(rest[length]) {
				continue
			}
		default:
			continue
		}
		references = append(references, hookRootReference{i, i + length, quote == '"'})
		i += length - 1
	}
	if len(references) > 0 && (quote != 0 || strings.Contains(command, "$(") || strings.Contains(command, "`") || strings.Contains(command, "<<")) {
		return references, "nested shell substitutions or here-documents cannot be translated"
	}
	return references, ""
}

func hookRootVariablePrefix(command, prefix string) bool {
	return strings.HasPrefix(command, prefix) && (len(command) == len(prefix) || !rootVariableRune(command[len(prefix)]))
}

func rootVariableRune(char byte) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_'
}

// posixRootEscape and powershellRootEscape keep a project path literal
// inside a double-quoted string of the shell that runs the hook.
var (
	posixRootEscape      = strings.NewReplacer("\\", "\\\\", "$", "\\$", "`", "\\`", "\"", "\\\"")
	powershellRootEscape = strings.NewReplacer("`", "``", "$", "`$", "\"", "`\"")
)

func projectHookRoot() string {
	return projectHookRootEscaped(posixRootEscape)
}

func projectHookRootEscaped(escape *strings.Replacer) string {
	project, err := os.Getwd()
	if err != nil {
		return ""
	}
	for root := project; ; root = filepath.Dir(root) {
		if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
			prefix, err := filepath.Rel(root, project)
			if err != nil {
				return ""
			}
			if prefix == "." {
				return gitHookRoot
			}
			return gitHookRoot + "/" + escape.Replace(filepath.ToSlash(prefix))
		}
		if filepath.Dir(root) == root {
			return ""
		}
	}
}

// RewriteHookProjectRoot translates shell-form root references; args are literal argv.
func RewriteHookProjectRoot(command, target, root string, args []string) string {
	if nativeHookRoots[target] == claudeProjectDir || len(args) > 0 {
		return command
	}
	if variable := nativeHookRoots[target]; variable != "" {
		root = "${" + variable + "}"
	}
	references, reason := hookRootReferences(command)
	if root == "" || reason != "" || len(references) == 0 {
		return command
	}
	var out strings.Builder
	previous := 0
	for _, ref := range references {
		out.WriteString(command[previous:ref.start])
		if ref.quoted {
			out.WriteString(root)
		} else {
			out.WriteString("\"" + root + "\"")
		}
		previous = ref.end
	}
	out.WriteString(command[previous:])
	return out.String()
}

// ReportHookProjectRoot names references the target cannot safely expand.
func ReportHookProjectRoot(target string, hooks []spec.Entry, mode string, global bool) error {
	if mode == OnUnsupportedSilent || target == "claude" {
		return nil
	}
	root := projectHookRoot()
	if global {
		root = gitHookRoot
	}
	for _, hook := range hooks {
		metadata := []map[string]any{ResolveMeta(hook.Meta, target)}
		if target == "gemini" && !global {
			native, _ := hook.Meta["x-gemini"].(map[string]any)
			if raw, exists := native["hooks"]; exists {
				metadata = HookCommandEntries(raw)
			}
		}
		for _, meta := range metadata {
			if windows, _ := meta["commandWindows"].(string); windows != "" {
				refs, reason := hookRootReferences(windows)
				translated := target == "codex" && !global && reason == "" && root != ""
				if !translated && (len(refs) > 0 || reason != "") {
					if err := reportHookRoot(target, hook, mode, "commandWindows", "Windows root references require a target-specific project root"); err != nil {
						return err
					}
				}
			}
			args := StringSlice(meta["args"])
			commands := HookCommands(meta["command"])
			for _, arg := range args {
				refs, reason := hookRootReferences(arg)
				if len(refs) > 0 || reason != "" {
					commands = append(commands, "$"+claudeProjectDir)
					break
				}
			}
			for _, command := range commands {
				references, reason := hookRootReferences(command)
				if len(references) == 0 && reason == "" {
					continue
				}
				switch {
				case len(args) > 0:
					reason = "exec-form root placeholders are not expanded by this target"
				case nativeHookRoots[target] == claudeProjectDir:
					continue
				case reason != "":
				case !hookPOSIXShell(meta):
					reason = "a POSIX shell is required to translate the root reference"
				case nativeHookRoots[target] == "" && root == "":
					reason = "the project has no Git worktree root to resolve at runtime"
				default:
					continue
				}
				if err := reportHookRoot(target, hook, mode, "command", reason); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}

func hookPOSIXShell(meta map[string]any) bool {
	shell, _ := meta["shell"].(string)
	return shell == "" || shell == "bash" || shell == "sh" || shell == "zsh"
}

func reportHookRoot(target string, hook spec.Entry, mode, field, reason string) error {
	location := hook.Path
	if location == "" {
		location = hook.Name
	}
	message := fmt.Sprintf("%s: %s hook %q uses %s in %s; %s; use a target-specific command or target: claude", location, target, hook.Name, claudeProjectDir, field, reason)
	if mode == OnUnsupportedError {
		return fmt.Errorf("%s", message)
	}
	NoteFieldNoOp(target, spec.KindHook, claudeProjectDir, 1, message)
	return nil
}

func hookRootMeta(target string, metadata []map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	return ResolveMeta(metadata[0], target)
}

func RewriteHookRoot(command, target string, metadata ...map[string]any) string {
	meta := hookRootMeta(target, metadata)
	if !hookPOSIXShell(meta) {
		return command
	}
	return RewriteHookProjectRoot(command, target, projectHookRoot(), StringSlice(meta["args"]))
}

// RewriteWindowsHookRoot translates root references in Codex's
// commandWindows, which PowerShell runs. `$(...)` is a subexpression there
// too, so the root resolves as in the POSIX command.
func RewriteWindowsHookRoot(command, target string) string {
	return RewriteHookProjectRoot(command, target, projectHookRootEscaped(powershellRootEscape), nil)
}

func RewriteGlobalHookRoot(command, target string, metadata ...map[string]any) string {
	meta := hookRootMeta(target, metadata)
	if !hookPOSIXShell(meta) {
		return command
	}
	return RewriteHookProjectRoot(command, target, gitHookRoot, StringSlice(meta["args"]))
}
