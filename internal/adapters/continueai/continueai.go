// Package continueai emits .continue/rules/*.md and .continue/mcpServers/*.yaml for continue.dev.
//
// The package name is suffixed because `continue` is a Go keyword.
package continueai

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target        = "continue"
	defaultDir    = ".continue/rules"
	defaultMCPDir = ".continue/mcpServers"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP},
}

// Adapter emits Continue configs.
type Adapter struct{}

// New returns a Continue adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one .md per rule and per agent into the rules directory,
// plus one .yaml per MCP entry under `.continue/mcpServers/`. When
// `outputs.continue.assistants-dir` is set, each agent additionally
// emits as a Continue local Assistant YAML at `<dir>/<name>.yaml`.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := emit.RulesDirectory(b, emit.RulesDirOpts{
		Dir:         emit.OutputRulesDir(cfg, target, defaultDir),
		AgentPrefix: "agent-",
	}, dryRun); err != nil {
		return err
	}
	if err := emitAssistants(b, cfg, dryRun); err != nil {
		return err
	}
	return emitMCPServers(b.MCPs, emit.OutputMCPDir(cfg, target, defaultMCPDir), dryRun)
}

// emitAssistants writes one Continue Assistant YAML per agent into the
// configured assistants dir. No-op when the dir is unset.
func emitAssistants(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	dir := emit.OutputAssistantsDir(cfg, target, "")
	if dir == "" {
		return nil
	}
	for _, a := range b.Agents {
		doc, err := assistantYAML(a)
		if err != nil {
			return err
		}
		path := filepath.Join(dir, a.Name+".yaml")
		if err := emit.WriteFile(path, doc, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// assistantYAML renders one agent as a Continue local Assistant per the
// hub assistants schema (v1). The agent body becomes a single named
// prompt; fields like models or `rules:` are intentionally omitted so
// Continue inherits the user's configured defaults.
func assistantYAML(e spec.Entry) (string, error) {
	m := emit.ResolveMeta(e.Meta, target)
	desc, _ := m["description"].(string)
	version, _ := m["version"].(string)
	if version == "" {
		version = "0.0.1"
	}

	doc := map[string]any{
		"name":    e.Name,
		"version": version,
		"schema":  "v1",
	}
	if desc != "" {
		doc["description"] = desc
	}
	prompt := map[string]any{
		"name":   e.Name,
		"prompt": e.Body,
	}
	if desc != "" {
		prompt["description"] = desc
	}
	doc["prompts"] = []any{prompt}

	raw, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal assistant %s: %w", e.Name, err)
	}
	return string(raw), nil
}

// emitMCPServers writes one YAML per MCP entry. Continue's loader picks
// up each file as a single server config (per the Continue docs).
func emitMCPServers(mcps []spec.Entry, dir string, dryRun bool) error {
	for _, m := range mcps {
		if m.Name == "" {
			continue
		}
		doc, err := mcpYAML(m)
		if err != nil {
			return err
		}
		path := filepath.Join(dir, m.Name+".yaml")
		if err := emit.WriteFile(path, doc, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// mcpYAML renders one MCP server as a YAML document. Stdio entries
// emit command/args/env; HTTP/SSE entries emit type/url/headers.
func mcpYAML(e spec.Entry) (string, error) {
	doc := map[string]any{"name": e.Name}

	transport, _ := e.Meta["type"].(string)
	if transport == "" {
		transport = "stdio"
	}

	switch transport {
	case "stdio":
		if cmd, _ := e.Meta["command"].(string); cmd != "" {
			doc["command"] = cmd
		}
		if args := emit.StringSlice(e.Meta["args"]); len(args) > 0 {
			doc["args"] = args
		}
	case "http", "sse", "streamable-http":
		doc["type"] = transport
		if url, _ := e.Meta["url"].(string); url != "" {
			doc["url"] = url
		}
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			doc["headers"] = h
		}
	}

	if env := emit.StringMap(e.Meta["env"]); len(env) > 0 {
		doc["env"] = env
	}

	raw, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal mcp %s: %w", e.Name, err)
	}
	return string(raw), nil
}
