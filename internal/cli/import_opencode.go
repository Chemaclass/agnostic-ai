package cli

import (
	"os"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

var (
	// opencodeEntryPointFile is the file OpenCode's instruction lookup
	// walks up for (opencode.ai/docs/rules, and `fs.up({ targets:
	// ["AGENTS.md"] })` in the vendor's
	// packages/core/src/instruction-context.ts on branch `dev`), and
	// where `sync` writes opencode's entry point since #623.
	opencodeEntryPointFile = "AGENTS.md"
	// opencodeLegacyEntryPointFile is the pre-#623 path. `sync` no
	// longer writes it and sweeps a managed copy, but a project that
	// has not re-synced still keeps its rules there, so import falls
	// back to it rather than dropping them.
	opencodeLegacyEntryPointFile = filepath.Join(".opencode", "AGENTS.md")
	opencodeAgentsDir            = filepath.Join(".opencode", "agents")
	opencodeSkillsDir            = filepath.Join(".opencode", "skills")
	opencodeCommandsDir          = filepath.Join(".opencode", "commands")
)

// opencodeMainFile returns the single entry-point import reads rules and
// the shared body from: the root `AGENTS.md` when present, else the
// pre-#623 `.opencode/AGENTS.md`. Reading one and not both keeps a
// project that carries both from slicing the same rules twice.
func opencodeMainFile(root string) string {
	if _, err := os.Stat(filepath.Join(root, opencodeEntryPointFile)); err == nil {
		return opencodeEntryPointFile
	}
	if _, err := os.Stat(filepath.Join(root, opencodeLegacyEntryPointFile)); err == nil {
		return opencodeLegacyEntryPointFile
	}
	return opencodeEntryPointFile
}

const (
	opencodeMCPFile = "opencode.json"
	opencodeMCPKey  = "mcp"
)

// importFromOpencode reads an existing OpenCode (SST) project
// (`AGENTS.md` or the pre-#623 `.opencode/AGENTS.md`,
// `.opencode/agents/`, `.opencode/skills/`, `.opencode/commands/`,
// `opencode.json`) under root and writes specs into the configured
// source directories.
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
	if _, err := mirrorMainFile(root, opencodeMainFile(root)); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d commands, %d mcps\n", rules, agents, skills, commands, mcps)
	printImportNextSteps(root, "opencode")
	return nil
}

// importOpencodeRules reconstructs rule specs by slicing OpenCode's
// entry-point file on `## ` headings; OpenCode has no dedicated rules
// directory, and `.opencode/commands/*.md` is a slash-command surface,
// not a rules one.
func importOpencodeRules(root, dstDir string) (int, error) {
	return sliceMainFileByH2(root, opencodeMainFile(root), dstDir)
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
