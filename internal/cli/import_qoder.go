package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	// qoderRulesDir is the native per-rule directory the qoder adapter
	// writes. Activation frontmatter survives through the generic walker.
	qoderRulesDir = ".qoder/rules"
	// qoderAgentsDir is the native per-agent directory the qoder adapter
	// writes (docs.qoder.com/extensions/subagent).
	qoderAgentsDir = ".qoder/agents"
	// qoderSkillsDir is Qoder's native Agent Skills folder tree
	// (docs.qoder.com/extensions/skills, target-audit 2026-08-08, #558):
	// a folder per skill holding a SKILL.md, the layout the qoder
	// adapter writes today. Project scope only; this importer has no
	// reach into the doc's user-level `~/.qoder/skills/` tier.
	qoderSkillsDir = ".qoder/skills"
	// qoderCommandsDir is the project-level custom-command path
	// docs.qoder.com/cli/commands tables: `.qoder/commands/<command_name>.md`.
	// Project scope only; this importer has no reach into the doc's
	// user-level `~/.qoder/commands/` tier, and the CLI page states
	// that tier takes precedence over this one for a same-named
	// command (#630).
	qoderCommandsDir  = ".qoder/commands"
	qoderSettingsFile = ".qoder/settings.json"
)

func importFromQoder(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.Commands, src.Settings, src.MCPs); err != nil {
		return err
	}
	c, err := importRulesDirectoryWith(root, qoderRulesDir, src, rulesDirImportOpts{
		NativeTarget: "qoder", NativeKeys: []string{"trigger", "glob", "paths"},
	})
	if err != nil {
		return err
	}
	agents, err := importQoderAgents(root, importSourcePath(root, src.Agents))
	if err != nil {
		return err
	}
	skills, err := importSkillFolders(root, filepath.Join(root, qoderSkillsDir), importSourcePath(root, src.Skills))
	if err != nil {
		return err
	}
	commands, err := importQoderCommands(root, importSourcePath(root, src.Commands))
	if err != nil {
		return err
	}
	mcps, err := importQoderMCP(root, importSourcePath(root, src.MCPs))
	if err != nil {
		return err
	}
	settings, err := importPortableSettings(root, qoderSettingsFile, importSourcePath(root, src.Settings), portableSettingsShape{target: "qoder", nestedModel: true, permissions: true})
	if err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d commands, %d mcps, %d settings (from qoder)\n", c.rules, agents, skills, commands, mcps, settings)
	return nil
}

// importQoderCommands copies `.qoder/commands/*.md` byte-for-byte into
// the commands source dir. Each command file becomes one command spec.
// A missing directory imports nothing.
func importQoderCommands(root, dstDir string) (int, error) {
	return copyMarkdownDir(filepath.Join(root, qoderCommandsDir), dstDir)
}

// importQoderAgents copies every `.qoder/agents/<name>.md` file into
// the agents source dir, rewriting `tools:` from Qoder's comma-
// separated string (`tools: Read, Grep, Bash`) into agnostic-ai's
// generic list form (`tools: [Read, Grep, Bash]`) along the way. Every
// other frontmatter key (name, description, model, skills, mcpServers,
// x-qoder, ...) copies through unchanged. A missing directory imports
// nothing.
//
// The rewrite matters: without it, a re-synced spec's `tools` would be
// a single string everywhere, not just qoder, silently breaking every
// other target's generic tools passthrough on the very next sync.
func importQoderAgents(root, dstDir string) (int, error) {
	return importFlatMarkdownFilesAt(filepath.Join(root, qoderAgentsDir), dstDir, qoderAgentFields, func(data []byte, dst string) ([]byte, error) {
		scoped, err := importScopedTargetKeys(data, dst, "qoder")
		if err != nil {
			return nil, err
		}
		return []byte(rewriteQoderAgentTools(string(scoped))), nil
	})
}

// rewriteQoderAgentTools rewrites the `tools:` line in body's leading
// YAML frontmatter, if present, from Qoder's documented comma-separated
// string into agnostic-ai's generic list form. A value that is already
// bracketed, quoted, or a block sequence (multi-line `- item` form) is
// left untouched: it is already list form. A body with no frontmatter
// or no `tools:` key returns unchanged.
func rewriteQoderAgentTools(body string) string {
	if !strings.HasPrefix(body, "---\n") {
		return body
	}
	rest := body[len("---\n"):]
	before, after, ok := strings.Cut(rest, "\n---")
	if !ok {
		return body
	}
	front := before
	afterMarker := after
	lines := strings.Split(front, "\n")
	changed := false
	for i, line := range lines {
		value := strings.TrimPrefix(line, "tools:")
		if value == line {
			continue // line does not start with "tools:"
		}
		value = strings.TrimSpace(value)
		if value == "" || strings.HasPrefix(value, "[") ||
			strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'") {
			continue // empty (block sequence follows), or already list/quoted form
		}
		items := splitAndTrimQoderTools(value)
		if len(items) == 0 {
			continue
		}
		lines[i] = "tools: [" + strings.Join(items, ", ") + "]"
		changed = true
	}
	if !changed {
		return body
	}
	return "---\n" + strings.Join(lines, "\n") + "\n---" + afterMarker
}

// splitAndTrimQoderTools splits Qoder's comma-separated tools value
// (`Read, Grep, Bash`) into trimmed, non-empty items.
func splitAndTrimQoderTools(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func importQoderMCP(root, dstDir string) (int, error) {
	path := filepath.Join(root, qoderSettingsFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	data, _ = adapters.StripJSONC(data)
	var native map[string]any
	if err := json.Unmarshal(data, &native); err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	servers, _ := native["mcpServers"].(map[string]any)
	if len(servers) == 0 {
		return 0, nil
	}
	clean, skipped := withoutMCPOAuthCredentials(servers)
	for _, raw := range clean {
		server, _ := raw.(map[string]any)
		normalizeQoderMCP(server)
	}
	count, err := writeMCPYAMLs("qoder", clean, dstDir)
	if err != nil {
		return count, err
	}
	reportMCPLiteralRefs(skipped)
	return count, nil
}

func normalizeQoderMCP(server map[string]any) {
	if revision, ok := server["protocolVersion"].(string); ok {
		native, _ := server["x-qoder"].(map[string]any)
		if native == nil {
			native = map[string]any{}
		}
		native["protocolVersion"] = revision
		server["x-qoder"] = native
		delete(server, "protocolVersion")
	}
}
