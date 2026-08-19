package cli

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

var (
	// opencodeMainFile is where sync wrote the entry point before the
	// path was corrected to the root AGENTS.md. Import still reads it so
	// a project synced by an older release round-trips.
	opencodeLegacyMainFile = filepath.Join(".opencode", "AGENTS.md")
	opencodeMainFile       = "AGENTS.md"
	opencodeAgentsDir      = filepath.Join(".opencode", "agents")
	opencodeSkillsDir      = filepath.Join(".opencode", "skills")
	opencodeCommandsDir    = filepath.Join(".opencode", "commands")
)

const (
	opencodeMCPFile = "opencode.json"
	opencodeMCPKey  = "mcp"
)

// importFromOpencode reads an existing OpenCode (SST) project
// (`AGENTS.md`, `.opencode/agents/`, `.opencode/skills/`,
// `.opencode/commands/`, `opencode.json`) under root and writes specs
// into the configured source directories.
func importFromOpencode(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.Commands, src.MCPs); err != nil {
		return err
	}
	rules, err := importOpencodeRules(root, filepath.Join(root, src.Rules))
	if err != nil {
		return err
	}
	agents, err := importOpencodeMarkdownDir(root, opencodeAgentsDir, filepath.Join(root, src.Agents))
	if err != nil {
		return err
	}
	skills, err := importSkillFolders(filepath.Join(root, opencodeSkillsDir), filepath.Join(root, src.Skills))
	if err != nil {
		return err
	}
	commands, err := importOpencodeMarkdownDir(root, opencodeCommandsDir, filepath.Join(root, src.Commands))
	if err != nil {
		return err
	}
	mcps, err := importOpencodeMCP(root, filepath.Join(root, src.MCPs))
	if err != nil {
		return err
	}
	if _, err := mirrorMainFile(root, opencodeEntryPoint(root)); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d commands, %d mcps\n", rules, agents, skills, commands, mcps)
	printImportNextSteps(root, "opencode")
	return nil
}

// opencodeEntryPoint returns the entry-point file to import from: the
// root AGENTS.md OpenCode actually reads, or the legacy
// `.opencode/AGENTS.md` when only that one is present, so a project
// synced before the path was corrected still round-trips.
func opencodeEntryPoint(root string) string {
	if !fileExists(filepath.Join(root, opencodeMainFile)) &&
		fileExists(filepath.Join(root, opencodeLegacyMainFile)) {
		return opencodeLegacyMainFile
	}
	return opencodeMainFile
}

// importOpencodeRules prefers `.opencode/commands/*.md` is NOT a rules
// source for OpenCode. Rules come from slicing the entry-point file on
// `## ` headings since OpenCode has no dedicated rules directory.
func importOpencodeRules(root, dstDir string) (int, error) {
	return sliceMainFileByH2(root, opencodeEntryPoint(root), dstDir)
}

// importOpencodeMarkdownDir copies every top-level `*.md` in the named
// OpenCode dir byte-for-byte (header stripped) into dstDir. Covers the
// flat per-file surfaces `.opencode/agents/` (native subagents) and
// `.opencode/commands/` (slash commands).
func importOpencodeMarkdownDir(root, dir, dstDir string) (int, error) {
	src := filepath.Join(root, dir)
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
// into `command` + `args`, rename `environment` → `env`, and invert
// `enabled: false` to the spec's own `disabled: true` (#555). Other
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
		case "enabled":
			if enabled, ok := v.(bool); ok && !enabled {
				out["disabled"] = true
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
