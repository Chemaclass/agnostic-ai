package cli

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

var (
	opencodeMainFile    = filepath.Join(".opencode", "AGENTS.md")
	opencodeCommandsDir = filepath.Join(".opencode", "commands")
)

const (
	opencodeMCPFile = "opencode.json"
	opencodeMCPKey  = "mcp"
)

// importFromOpencode reads an existing OpenCode (SST) project
// (`.opencode/AGENTS.md`, `.opencode/commands/`, `opencode.json`) under
// root and writes specs into the configured source directories.
func importFromOpencode(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.MCPs); err != nil {
		return err
	}
	rules, err := importOpencodeRules(root, filepath.Join(root, src.Rules))
	if err != nil {
		return err
	}
	agents, err := importOpencodeCommands(root, filepath.Join(root, src.Agents))
	if err != nil {
		return err
	}
	mcps, err := importOpencodeMCP(root, filepath.Join(root, src.MCPs))
	if err != nil {
		return err
	}
	if _, err := mirrorMainFile(root, opencodeMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d mcps\n", rules, agents, mcps)
	printImportNextSteps(root, "opencode")
	return nil
}

// importOpencodeRules prefers `.opencode/commands/*.md` is NOT a rules
// source for OpenCode. Rules come from slicing `.opencode/AGENTS.md`
// on `## ` headings since OpenCode has no dedicated rules directory.
func importOpencodeRules(root, dstDir string) (int, error) {
	return sliceMainFileByH2(root, opencodeMainFile, dstDir)
}

// importOpencodeCommands copies `.opencode/commands/*.md` byte-for-byte
// into the agents source dir. Each command file becomes one agent.
func importOpencodeCommands(root, dstDir string) (int, error) {
	src := filepath.Join(root, opencodeCommandsDir)
	if !dirExists(src) {
		return 0, nil
	}
	return copyMarkdownDir(src, dstDir)
}

// importOpencodeMCP reads `opencode.json` and writes one yaml per
// `mcp.<name>` entry. The OpenCode shape differs from the standard
// `mcpServers` schema: entries carry `type: "local"|"remote"`, `command`
// is an array (`["cmd", "arg1", "arg2"]`), and env vars live under
// `environment`. This translator normalizes each entry into the
// agnostic MCP shape (`command:` string + `args:` slice, `env:` map)
// before writing so the YAML re-emits cleanly.
func importOpencodeMCP(root, dstDir string) (int, error) {
	servers, err := readJSONMapAt(filepath.Join(root, opencodeMCPFile), opencodeMCPKey)
	if err != nil || len(servers) == 0 {
		return 0, err
	}
	normalized := map[string]any{}
	for name, raw := range servers {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		normalized[name] = normalizeOpencodeMCPEntry(entry)
	}
	return writeMCPYAMLs(normalized, dstDir)
}

// normalizeOpencodeMCPEntry maps OpenCode's MCP entry shape to the
// agnostic spec shape: drop `type`, split `command: [cmd, args...]`
// into `command` + `args`, and rename `environment` → `env`. Other
// keys (url, headers, ...) pass through unchanged so HTTP/remote
// entries survive a round-trip.
func normalizeOpencodeMCPEntry(entry map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range entry {
		switch k {
		case "type":
			continue
		case "environment":
			out["env"] = v
		case "command":
			cmd, args := splitOpencodeCommand(v)
			if cmd != "" {
				out["command"] = cmd
			}
			if len(args) > 0 {
				out["args"] = args
			}
		default:
			out[k] = v
		}
	}
	return out
}

// splitOpencodeCommand interprets v as a JSON-decoded array of strings
// (`["cmd", "arg1", ...]`) and returns the first element as the
// command and the rest as args. A plain string value falls back to
// command-only.
func splitOpencodeCommand(v any) (string, []string) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return "", nil
	}
	cmd, _ := list[0].(string)
	args := make([]string, 0, len(list)-1)
	for _, x := range list[1:] {
		if s, ok := x.(string); ok {
			args = append(args, s)
		}
	}
	return cmd, args
}
