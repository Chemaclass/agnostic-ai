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
// plus one .yaml per MCP entry under `.continue/mcpServers/`.
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
	return emitMCPServers(b.MCPs, emit.OutputMCPDir(cfg, target, defaultMCPDir), dryRun)
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
		if args := stringSlice(e.Meta["args"]); len(args) > 0 {
			doc["args"] = args
		}
	case "http", "sse", "streamable-http":
		doc["type"] = transport
		if url, _ := e.Meta["url"].(string); url != "" {
			doc["url"] = url
		}
		if h := stringMap(e.Meta["headers"]); len(h) > 0 {
			doc["headers"] = h
		}
	}

	if env := stringMap(e.Meta["env"]); len(env) > 0 {
		doc["env"] = env
	}

	raw, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal mcp %s: %w", e.Name, err)
	}
	return string(raw), nil
}

func stringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, x := range raw {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func stringMap(v any) map[string]string {
	raw, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, x := range raw {
		if s, ok := x.(string); ok {
			out[k] = s
		}
	}
	return out
}
