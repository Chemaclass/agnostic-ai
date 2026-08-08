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
// emits as a standalone `config.yaml`-shaped YAML at `<dir>/<name>.yaml`
// (see assistantYAML); whether Continue itself scans that directory is
// unconfirmed, so treat it as an export, not a native discovery path.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir:         emit.OutputRulesDir(cfg, target, defaultDir),
		AgentPrefix: "agent-",
	}, dryRun); err != nil {
		return err
	}
	if err := emitAssistants(sess, b, cfg, dryRun); err != nil {
		return err
	}
	return emitMCPServers(sess, b.MCPs, emit.OutputMCPDir(cfg, target, defaultMCPDir), dryRun)
}

// emitAssistants writes one Continue Assistant YAML per agent into the
// configured assistants dir. No-op when the dir is unset.
func emitAssistants(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
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
		if err := sess.WriteFile(path, emit.WithHeader(doc, emit.FormatYAML), dryRun); err != nil {
			return err
		}
	}
	return nil
}

// assistantYAML renders one agent as a Continue config.yaml: `name`,
// `version`, and `schema: v1` at the top level, with the agent body
// wrapped as a single `prompts: [{name, description, prompt}]` entry.
// That shape matches promptSchema and configYamlSchema in
// continuedev/continue's packages/config-yaml/src/schemas/index.ts, and
// docs.continue.dev/reference documents the same name/version/schema
// top level (target-audit 2026-08-08, #563; the prior citation here,
// docs.continue.dev/hub/assistants/intro, 404s and the whole /hub/
// namespace is gone). No doc confirms Continue scans
// outputs.continue.assistants-dir as a directory, so this is an export
// in Continue's own file shape, loadable explicitly (`cn --config
// <path>`), not a vendor-confirmed native discovery surface. Fields
// like models or `rules:` are intentionally omitted so Continue
// inherits the user's configured defaults.
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
func emitMCPServers(sess *emit.Session, mcps []spec.Entry, dir string, dryRun bool) error {
	for _, m := range mcps {
		if m.Name == "" {
			continue
		}
		doc, err := mcpYAML(m)
		if err != nil {
			return err
		}
		path := filepath.Join(dir, m.Name+".yaml")
		if err := sess.WriteFile(path, emit.WithHeader(doc, emit.FormatYAML), dryRun); err != nil {
			return err
		}
	}
	return nil
}

// mcpYAML renders one MCP server as a Continue block YAML document.
// Standalone files under `.continue/mcpServers/` require the block
// wrapper (`name` + `version` + `schema: v1`) with the server nested
// under an `mcpServers:` list; a flat single-server file does not load.
// See https://docs.continue.dev/customize/deep-dives/mcp.
// Stdio servers emit command/args/env; HTTP/SSE emit type/url/headers.
func mcpYAML(e spec.Entry) (string, error) {
	server := map[string]any{"name": e.Name}

	transport, _ := e.Meta["type"].(string)
	if transport == "" {
		transport = "stdio"
	}

	switch transport {
	case "stdio":
		if cmd, _ := e.Meta["command"].(string); cmd != "" {
			server["command"] = cmd
		}
		if args := emit.StringSlice(e.Meta["args"]); len(args) > 0 {
			server["args"] = args
		}
	case "http", "sse", "streamable-http":
		server["type"] = transport
		if url, _ := e.Meta["url"].(string); url != "" {
			server["url"] = url
		}
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			server["headers"] = h
		}
	}

	if env := emit.StringMap(e.Meta["env"]); len(env) > 0 {
		server["env"] = env
	}

	version, _ := emit.ResolveMeta(e.Meta, target)["version"].(string)
	if version == "" {
		version = "0.0.1"
	}
	doc := map[string]any{
		"name":       e.Name,
		"version":    version,
		"schema":     "v1",
		"mcpServers": []any{server},
	}

	raw, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal mcp %s: %w", e.Name, err)
	}
	return string(raw), nil
}
