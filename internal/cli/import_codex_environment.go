package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// codexEnvironmentFile is where the Codex app keeps a project's local
// environment.
var codexEnvironmentFile = filepath.Join(".codex", "environments", "environment.toml")

// codexEnvironmentSpecName names the environment spec the file imports
// into.
const codexEnvironmentSpecName = "codex"

// importCodexEnvironment reads a hand-written
// .codex/environments/environment.toml into an environment spec's
// `setup`, `setup-windows`, `cleanup`, and `dev-commands`, the reverse of
// the codex emit (#1393). A file with a key the spec cannot hold, such as
// a `[setup.darwin]` script or an action's `platform`, is left whole with
// a note, since sync would drop it. So is a file sync wrote, or one whose
// destination spec already exists.
func importCodexEnvironment(root string, src config.Sources) (int, error) {
	if src.Environments == "" {
		return 0, nil
	}
	for _, p := range readStateFile(root).Outputs {
		if filepath.Clean(filepath.FromSlash(p)) == codexEnvironmentFile {
			return 0, nil
		}
	}
	path := filepath.Join(root, codexEnvironmentFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	var doc map[string]any
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}

	leave := func(reason string) (int, error) {
		summaryf("  ! left %s as written: %s, which an environment spec cannot hold, so sync would drop it\n",
			filepath.ToSlash(codexEnvironmentFile), reason)
		return 0, nil
	}
	if problem := codexEnvironmentProblem(doc); problem != "" {
		return leave(problem)
	}

	spec := yaml.Node{Kind: yaml.MappingNode}
	name, _ := doc["name"].(string)
	if name == "" {
		name = codexEnvironmentSpecName
	}
	addYAMLField(&spec, "name", name)
	setup, _ := doc["setup"].(map[string]any)
	cleanup, _ := doc["cleanup"].(map[string]any)
	if s := scriptOf(setup); s != "" {
		addYAMLField(&spec, "setup", s)
	}
	if win, _ := setup["win32"].(map[string]any); scriptOf(win) != "" {
		addYAMLField(&spec, "setup-windows", scriptOf(win))
	}
	if s := scriptOf(cleanup); s != "" {
		addYAMLField(&spec, "cleanup", s)
	}
	var commands []any
	actions, _ := doc["actions"].([]map[string]any)
	for _, a := range actions {
		cmd := yaml.Node{Kind: yaml.MappingNode}
		addYAMLField(&cmd, "name", a["name"])
		cwd, command := actionCwd(strings.TrimRight(a["command"].(string), "\n"))
		addYAMLField(&cmd, "command", command)
		if cwd != "" {
			addYAMLField(&cmd, "cwd", cwd)
		}
		if icon, _ := a["icon"].(string); icon != "" && icon != "run" {
			addYAMLField(&cmd, "icon", icon)
		}
		commands = append(commands, &cmd)
	}
	if len(commands) > 0 {
		addYAMLField(&spec, "dev-commands", commands)
	}
	if len(spec.Content) == 2 {
		return 0, nil
	}

	dstDir := filepath.Join(root, src.Environments)
	out := filepath.Join(dstDir, codexEnvironmentSpecName+".yaml")
	if fileExists(out) {
		summaryf("  ! skipped %s: %s already exists; move its scripts and actions there by hand\n",
			filepath.ToSlash(codexEnvironmentFile), filepath.ToSlash(src.Environments)+"/"+codexEnvironmentSpecName+".yaml")
		return 0, nil
	}
	raw, err := yaml.Marshal(&spec)
	if err != nil {
		return 0, fmt.Errorf("marshal %s: %w", out, err)
	}
	if err := importMkdirAll(dstDir, 0o755); err != nil {
		return 0, fmt.Errorf("mkdir %s: %w", dstDir, err)
	}
	if err := importWriteFile(out, raw, 0o644); err != nil {
		return 0, fmt.Errorf("write %s: %w", out, err)
	}
	return 1, nil
}

// scriptOf reads the `script` of a `[setup]` or `[cleanup]` table,
// without its trailing newline.
func scriptOf(table map[string]any) string {
	s, _ := table["script"].(string)
	return strings.TrimRight(s, "\n")
}

// codexEnvironmentProblem says why the file cannot become an environment
// spec exactly, or returns "" when it can.
func codexEnvironmentProblem(doc map[string]any) string {
	for _, k := range slices.Sorted(maps.Keys(doc)) {
		var ok bool
		switch v := doc[k].(type) {
		case int64:
			if k == "version" && v != 1 {
				return fmt.Sprintf("its version is %d, not 1", v)
			}
			ok = k == "version"
		case string:
			ok = k == "name"
		case map[string]any:
			switch k {
			case "setup":
				if problem := scriptTableProblem(k, v, true); problem != "" {
					return problem
				}
				ok = true
			case "cleanup":
				if problem := scriptTableProblem(k, v, false); problem != "" {
					return problem
				}
				ok = true
			}
		case []map[string]any:
			if k != "actions" {
				return fmt.Sprintf("it sets %q", k)
			}
			seen := map[string]bool{}
			for i, a := range v {
				if problem := codexActionProblem(a); problem != "" {
					return fmt.Sprintf("action %d %s", i+1, problem)
				}
				name, _ := a["name"].(string)
				if seen[name] {
					return fmt.Sprintf("action %q appears twice", name)
				}
				seen[name] = true
			}
			ok = true
		}
		if !ok {
			return fmt.Sprintf("it sets %q", k)
		}
	}
	return ""
}

// scriptTableProblem checks a `[setup]` or `[cleanup]` table. Only setup
// has a Windows form in the spec.
func scriptTableProblem(table string, v map[string]any, windows bool) string {
	for _, k := range slices.Sorted(maps.Keys(v)) {
		switch {
		case k == "script":
			if _, ok := v[k].(string); !ok {
				return fmt.Sprintf("its [%s] script is not a string", table)
			}
		case k == "win32" && windows:
			win, ok := v[k].(map[string]any)
			if !ok || len(win) != 1 {
				return fmt.Sprintf("its [%s.win32] table sets more than a script", table)
			}
			if _, ok := win["script"].(string); !ok {
				return fmt.Sprintf("its [%s.win32] script is not a string", table)
			}
		default:
			return fmt.Sprintf("its [%s] table sets %q", table, k)
		}
	}
	return ""
}

func codexActionProblem(a map[string]any) string {
	for _, k := range slices.Sorted(maps.Keys(a)) {
		if _, ok := a[k].(string); !ok || (k != "name" && k != "icon" && k != "command") {
			return fmt.Sprintf("sets %q", k)
		}
	}
	name, _ := a["name"].(string)
	command, _ := a["command"].(string)
	if name == "" || strings.TrimSpace(command) == "" {
		return "has no name or command"
	}
	return ""
}

// actionCdRE matches the `cd <dir> && ` or `cd <dir> || exit 1` line that
// sync writes for a dev command's cwd, with the directory bare or in
// single quotes.
var actionCdRE = regexp.MustCompile(`^cd (?:'([^']+)'|([A-Za-z0-9_@%+=:,./-]+))(?: && |[ \t]*\|\| exit 1\n)`)

// actionCwd splits the directory an action changes into off its command,
// the reverse of the cd that sync writes for a dev command's cwd.
func actionCwd(command string) (string, string) {
	m := actionCdRE.FindStringSubmatch(command)
	if m == nil {
		return "", command
	}
	rest := command[len(m[0]):]
	if strings.TrimSpace(rest) == "" {
		return "", command
	}
	return m[1] + m[2], rest
}
