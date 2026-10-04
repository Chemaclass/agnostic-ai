package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/copilot"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

var (
	copilotMainFile        = filepath.Join(".github", "copilot-instructions.md")
	copilotInstructionsDir = filepath.Join(".github", "instructions")
	copilotAgentsDir       = filepath.Join(".github", "agents")
	copilotChatmodesDir    = filepath.Join(".github", "chatmodes")
	copilotHooksDir        = filepath.Join(".github", "hooks")
	copilotMCPFile         = filepath.Join(".vscode", "mcp.json")
	copilotSettingsFile    = filepath.Join(".github", "copilot", "settings.json")
)

// copilotSkillsDirs lists the three project skill directories the
// vendor documents ("create a `.github/skills`, `.claude/skills`, or
// `.agents/skills` directory in your repository"), in precedence order.
// `.github/skills/` is what the adapter emits, so it comes first.
var copilotSkillsDirs = []string{
	filepath.Join(".github", "skills"),
	filepath.Join(".claude", "skills"),
	filepath.Join(".agents", "skills"),
}

const (
	copilotInstructionSuffix = ".instructions.md"
	copilotAgentSuffix       = ".agent.md"
	copilotChatmodeSuffix    = ".chatmode.md"
)

// importFromCopilot reads an existing GitHub Copilot project
// (`.github/copilot-instructions.md`, `.github/instructions/`,
// `.github/chatmodes/`, every directory in copilotSkillsDirs,
// `.github/copilot/settings.json`, and `.vscode/mcp.json`) under root
// and writes specs into the configured source directories.
func importFromCopilot(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.Hooks, src.MCPs, src.Settings); err != nil {
		return err
	}
	counts, err := importCopilotRules(root, src)
	if err != nil {
		return err
	}
	agents, err := importCopilotAgents(root, importSourcePath(root, src.Agents))
	if err != nil {
		return err
	}
	skills := 0
	seenSkills := map[string]bool{}
	for _, skillsDir := range copilotSkillsDirs {
		folderSkills, err := importSkillFoldersWith(root, filepath.Join(root, skillsDir), importSourcePath(root, src.Skills), skillFolderImportOpts{SkipNames: seenSkills})
		if err != nil {
			return err
		}
		skills += folderSkills
	}
	chatmodes, err := importCopilotChatmodes(root, importSourcePath(root, src.Agents))
	if err != nil {
		return err
	}
	mcps, err := importCopilotMCP(root, importSourcePath(root, src.MCPs))
	if err != nil {
		return err
	}
	hooks, err := importCopilotHooks(root, importSourcePath(root, src.Hooks))
	if err != nil {
		return err
	}
	settings, err := importPortableSettings(root, copilotSettingsFile, importSourcePath(root, src.Settings), portableSettingsShape{target: "copilot", effortKey: "effortLevel"})
	if err != nil {
		return err
	}
	if _, err := mirrorMainFile(root, copilotMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d hooks, %d mcps, %d settings\n",
		counts.rules, counts.agents+agents+chatmodes, counts.skills+skills, hooks, mcps, settings)
	printImportNextSteps(root, "copilot")
	return nil
}

// importCopilotHooks reads every repository hook file and writes one
// target-scoped spec per native handler. Copilot's event arrays do not group
// handlers, so separate specs preserve HTTP, prompt, and command payloads.
func importCopilotHooks(root, dstDir string) (int, error) {
	dir := filepath.Join(root, copilotHooksDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", dir, err)
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", path, err)
		}
		var native struct {
			Hooks map[string][]map[string]any `json:"hooks"`
		}
		if err := json.Unmarshal(data, &native); err != nil {
			return count, fmt.Errorf("parse %s: %w", path, err)
		}
		events := make([]string, 0, len(native.Hooks))
		for event := range native.Hooks {
			events = append(events, event)
		}
		sort.Strings(events)
		for _, event := range events {
			for _, handler := range native.Hooks[event] {
				doc := normalizeCopilotHook(event, handler)
				if doc == nil {
					continue
				}
				payload, err := json.Marshal(handler)
				if err != nil {
					return count, fmt.Errorf("marshal %s hook: %w", event, err)
				}
				matcher, _ := handler["matcher"].(string)
				name := hookSpecName(event, matcher, []string{string(payload)})
				doc["name"], doc["target"] = name, "copilot"
				if err := writeHookSpecFile(dstDir, name, doc); err != nil {
					return count, err
				}
				count++
			}
		}
	}
	return count, nil
}

func normalizeCopilotHook(event string, native map[string]any) map[string]any {
	dropHookTargetEnv(native, "copilot")
	kind, _ := native["type"].(string)
	if kind == "" {
		kind = "command"
	}
	doc := map[string]any{"event": event, "type": kind}
	// `cwd` and `env` are Copilot's own command-hook fields, emitted
	// since #888 and read back here so the round trip is closed.
	for _, key := range []string{"matcher", "url", "headers", "allowedEnvVars", "prompt", "cwd", "env"} {
		if value, exists := native[key]; exists {
			doc[key] = value
		}
	}
	if timeout, exists := native["timeoutSec"]; exists {
		doc["timeout"] = timeout
	}
	switch kind {
	case "command":
		cwd, _ := native["cwd"].(string)
		command, _ := native["command"].(string)
		// A wrapped command came from a portable hook; only that form
		// syncs the wrapper back.
		matcher, _ := native["matcher"].(string)
		if on, match, ok := spec.WrappedPortableHook("copilot", event, matcher); ok {
			if inner, wrapped := copilot.UnwrapPortableCommand(command, cwd); wrapped {
				command = inner
				delete(doc, "event")
				delete(doc, "matcher")
				doc["on"] = on
				if match != "" {
					doc["match"] = match
				}
			}
		}
		command = copilot.ScriptFromCwd(command, cwd)
		if command == "" {
			command, _ = native["exec"].(string)
			var args []string
			raw, exists := native["args"]
			if exists {
				doc["args"] = raw
			}
			list, isList := raw.([]any)
			ok := !exists || isList
			for _, item := range list {
				text, isText := item.(string)
				ok = ok && isText
				args = append(args, text)
			}
			if ok {
				var restored []string
				command, restored = copilot.ExecFromCwd(command, args, cwd)
				if exists {
					doc["args"] = restored
				}
			}
		}
		if command == "" {
			return nil
		}
		doc["command"] = command
	case "http":
		if url, _ := native["url"].(string); url == "" {
			return nil
		}
	case "prompt":
		if prompt, _ := native["prompt"].(string); prompt == "" {
			return nil
		}
	default:
		return nil
	}
	return doc
}

var frontmatterNameRE = regexp.MustCompile(`(?m)^name:[ \t]*(.*?)[ \t]*$`)

// copilotAgentIdentity keeps an agent named after its file. A profile's
// `name` is the display name VS Code shows ("If not specified, the file
// name is used"), so `name: Data` in data.md moves to x-copilot.name and
// the spec is named data, which sync writes back to data.md.
func copilotAgentIdentity(doc, stem string) string {
	rest, ok := strings.CutPrefix(doc, "---\n")
	if !ok {
		return doc
	}
	front, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return doc
	}
	m := frontmatterNameRE.FindStringSubmatchIndex(front)
	if m == nil {
		return doc
	}
	display := strings.Trim(front[m[2]:m[3]], `"'`)
	if display == stem || strings.Contains(front, "x-copilot:") {
		return doc
	}
	front = front[:m[0]] + "name: " + stem + front[m[1]:] + "\nx-copilot:\n  name: " + strconv.Quote(display)
	return "---\n" + front + "\n---\n" + body
}

// importCopilotAgents copies every native agent profile under
// `.github/agents/` into the agents source dir. Both documented
// filename forms are accepted (`<name>.agent.md` and `<name>.md`); the
// `.agent` infix is dropped so the spec lands at `<agents>/<name>.md`.
// The agnostic-ai provenance header is stripped when present.
func importCopilotAgents(root, dstDir string) (int, error) {
	src := filepath.Join(root, copilotAgentsDir)
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".md"), ".agent")
		srcPath := filepath.Join(src, e.Name())
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", srcPath, err)
		}
		dst := filepath.Join(dstDir, name+".md")
		body := copilotAgentIdentity(header.Strip(string(data)), name)
		if err := importWriteSpecMarkdown(dst, []byte(body), 0o644, copilotAgentFields); err != nil {
			return count, fmt.Errorf("write %s: %w", dst, err)
		}
		count++
	}
	return count, nil
}

type copilotCounts struct{ rules, agents, skills int }

// importCopilotRules prefers `.github/instructions/*.instructions.md`
// (one file becomes one rule, agent, or skill depending on filename
// prefix; `applyTo:` frontmatter translates to `globs:`). Falls back
// to slicing `.github/copilot-instructions.md` on `## ` headings when
// no instructions dir exists.
func importCopilotRules(root string, src config.Sources) (copilotCounts, error) {
	var c copilotCounts
	instrDir := filepath.Join(root, copilotInstructionsDir)
	if dirExists(instrDir) {
		return importCopilotInstructions(instrDir, root, src)
	}
	n, err := sliceMirroredMainFile(root, copilotMainFile, importSourcePath(root, src.Rules))
	if err != nil {
		return c, err
	}
	c.rules = n
	return c, nil
}

// importCopilotInstructions reads every *.instructions.md under src
// and routes it by filename prefix:
//   - `agent-<name>.instructions.md` → <agents>/<name>.md
//   - `skill-<name>.instructions.md` → <skills>/<name>.md
//   - `<name>.instructions.md`       → <rules>/<name>.md
//
// `applyTo` (Copilot's path glob) translates to `globs`. The catch-all
// `**` is dropped since it carries no scope.
func importCopilotInstructions(src, root string, sources config.Sources) (copilotCounts, error) {
	var c copilotCounts
	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), copilotInstructionSuffix) {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		base := strings.TrimSuffix(d.Name(), copilotInstructionSuffix)
		kind, name := classifyRulesDirFile(base+".md", data)
		dstDir := pickKindDir(kind, sources)
		translated, err := translateCopilotInstruction(name, data)
		if err != nil {
			return fmt.Errorf("translate %s: %w", rel, err)
		}
		out := filepath.Join(importSourcePath(root, dstDir), scopeDir(rel), name+".md")
		if err := importMkdirAll(filepath.Dir(out), 0o755); err != nil {
			return fmt.Errorf("%s: %w", filepath.Dir(out), err)
		}
		if err := importWriteSpecMarkdown(out, translated, 0o644, flattenedKindFields(kind)); err != nil {
			return fmt.Errorf("write %s: %w", out, err)
		}
		switch kind {
		case "agents":
			c.agents++
		case "skills":
			c.skills++
		default:
			c.rules++
		}
		return nil
	})
	if errors.Is(walkErr, fs.ErrNotExist) {
		return copilotCounts{}, nil
	}
	if walkErr != nil {
		return c, walkErr
	}
	return c, nil
}

// translateCopilotInstruction rewrites a Copilot `.instructions.md`
// file as an agnostic rule. `applyTo:` becomes `globs:` (the catch-all
// `**` is dropped), and a file without it, loaded on demand, gets
// `alwaysApply: false`. A `name:` field is injected from the filename when
// absent. A leading single-line italic paragraph (the form the emitter
// writes for `description:`) is lifted out of the body into the
// `description:` frontmatter so a round-trip is loss-free.
func translateCopilotInstruction(name string, data []byte) ([]byte, error) {
	data = []byte(header.Strip(string(data)))
	meta, body := splitMdcFrontmatter(data)
	if _, ok := meta["name"]; !ok {
		meta["name"] = name
	}
	if applyTo, ok := meta["applyTo"].(string); ok {
		delete(meta, "applyTo")
		if applyTo != "" && applyTo != "**" {
			if _, exists := meta["globs"]; !exists {
				meta["globs"] = applyTo
			}
		}
	} else if _, exists := meta["alwaysApply"]; !exists {
		// No applyTo: VS Code attaches the file only on demand, when its
		// description matches the task. alwaysApply: false keeps that.
		meta["alwaysApply"] = false
	}
	if desc, stripped, ok := extractLeadingItalic(body); ok {
		if _, exists := meta["description"]; !exists {
			meta["description"] = desc
		}
		body = stripped
	}
	var fm strings.Builder
	enc := yaml.NewEncoder(&fm)
	enc.SetIndent(2)
	if err := enc.Encode(meta); err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close encoder: %w", err)
	}
	var out strings.Builder
	out.WriteString("---\n")
	out.WriteString(fm.String())
	out.WriteString("---\n\n")
	out.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		out.WriteString("\n")
	}
	return []byte(out.String()), nil
}

// importCopilotChatmodes reads `.github/chatmodes/*.chatmode.md` and
// writes one agent spec per chatmode into dstDir.
func importCopilotChatmodes(root, dstDir string) (int, error) {
	src := filepath.Join(root, copilotChatmodesDir)
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), copilotChatmodeSuffix) {
			continue
		}
		full := filepath.Join(src, e.Name())
		data, err := os.ReadFile(full)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", full, err)
		}
		name := strings.TrimSuffix(e.Name(), copilotChatmodeSuffix)
		data = []byte(header.Strip(string(data)))
		meta, body := splitMdcFrontmatter(data)
		if _, ok := meta["name"]; !ok {
			meta["name"] = name
		}
		translated, err := writeFrontmatter(meta, body)
		if err != nil {
			return count, fmt.Errorf("translate %s: %w", e.Name(), err)
		}
		out := filepath.Join(dstDir, name+".md")
		if err := importWriteSpecMarkdown(out, translated, 0o644, copilotAgentFields); err != nil {
			return count, fmt.Errorf("write %s: %w", out, err)
		}
		count++
	}
	return count, nil
}

// writeFrontmatter re-marshals meta + body as a markdown file with
// `---` frontmatter delimiters. Generic helper for translated specs
// where keys are not known up front.
func writeFrontmatter(meta map[string]any, body string) ([]byte, error) {
	var fm strings.Builder
	enc := yaml.NewEncoder(&fm)
	enc.SetIndent(2)
	if err := enc.Encode(meta); err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close encoder: %w", err)
	}
	var out strings.Builder
	out.WriteString("---\n")
	out.WriteString(fm.String())
	out.WriteString("---\n\n")
	out.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		out.WriteString("\n")
	}
	return []byte(out.String()), nil
}

// importCopilotMCP reads `.vscode/mcp.json` (VS Code's MCP shape:
// `{servers: {name: {...}}}`) and writes one yaml per server.
func importCopilotMCP(root, dstDir string) (int, error) {
	return importJSONMCPMap("copilot", filepath.Join(root, copilotMCPFile), "servers", dstDir)
}
