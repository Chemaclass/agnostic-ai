// Package continueai emits .continue/rules/*.md and .continue/mcpServers/*.yaml for continue.dev.
//
// Every rule file carries Continue's documented activation
// frontmatter: `name`, `globs`, `alwaysApply`, and `description`
// (continuedev/continue, docs/customize/deep-dives/rules.mdx; the docs
// site itself is client-rendered with no markdown mirror, so the repo
// is the fetchable source). `globs` falls back to the rule's
// source-layout scope (`<scope>/**`) when the spec declares none, so a
// rule authored by directory narrows instead of loading everywhere.
// `alwaysApply` and `description` emit only when the spec sets them:
// Continue's undefined default ("Included if no globs exist OR globs
// exist and match") is not the same as an explicit `false`, so
// synthesizing one would change behavior. `regex`, new since the last
// audit ("When files are provided as context and their content matches
// this regex pattern, the rule will be included"), has no generic spec
// field and reaches the file through `x-continue`, along with any key
// Continue adds next. Before this existed no rule file carried
// frontmatter at all, so every scoped rule was always-on (target-audit
// 2026-08-27, #639).
//
// Each MCP file speaks Continue's own dialect, not the spec's.
// `mcpServerSchema` (packages/config-yaml/src/schemas/mcp/index.ts) is
// a union of a stdio branch and a url branch, and the url branch reads
// `type: z.union([z.literal("sse"), z.literal("streamable-http")])`
// with no `http` member, so the canonical spec spelling `http` is
// translated to `streamable-http` on the way out. That branch extends
// the base fields with `url`, `type`, `apiKey` and `requestOptions`
// only, so headers nest under `requestOptions.headers`
// (`requestOptionsSchema`, packages/config-yaml/src/schemas/models.ts).
// Both matter because `parseBlock` calls `blockSchema.parse`: an
// unlisted `type` literal throws and the file never loads, while an
// unknown top-level key is stripped, which connected the server
// unauthenticated instead (target-audit 2026-09-11, #726 and #730). A
// transport Continue documents nowhere, `ws` today, emits no file at
// all and raises a coverage note, since a server matching neither
// branch throws the same way.
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
		FormatRule:  rule,
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
// up each file as a single server config (per the Continue docs). A
// transport outside Continue's two server shapes writes no file and
// surfaces a coverage note: `mcpServerSchema` is a union of a stdio
// branch (`command` required) and a url branch (`url` required), so a
// server carrying neither throws on load and takes the whole file with
// it rather than being skipped.
func emitMCPServers(sess *emit.Session, mcps []spec.Entry, dir string, dryRun bool) error {
	unmapped := 0
	for _, m := range mcps {
		if m.Name == "" {
			continue
		}
		if !mappedTransport(mcpTransport(m)) {
			unmapped++
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
	emit.NoteCoverageGap(target, spec.KindMCP, unmapped,
		"no Continue MCP server shape for this transport")
	return nil
}

// mcpTransport returns the spec transport, defaulting to stdio the same
// way Continue's schema does: `type` is optional on both branches.
func mcpTransport(e spec.Entry) string {
	transport, _ := e.Meta["type"].(string)
	if transport == "" {
		return "stdio"
	}
	return transport
}

// mappedTransport reports whether Continue has a server shape for the
// transport. `ws` is the one the spec format offers that Continue
// documents nowhere, so it lands here (target-audit 2026-09-11, #726).
func mappedTransport(transport string) bool {
	switch transport {
	case "stdio", "http", "sse", "streamable-http":
		return true
	}
	return false
}

// continueTransport maps the agnostic transport spelling onto a literal
// Continue's url-based server accepts. `http` is the canonical agnostic
// name for Streamable HTTP, but the vendor union is
// `z.union([z.literal("sse"), z.literal("streamable-http")])` and
// `parseBlock` runs `blockSchema.parse`, so an unlisted literal throws
// instead of degrading (target-audit 2026-09-11, #726).
func continueTransport(transport string) string {
	if transport == "http" {
		return "streamable-http"
	}
	return transport
}

// mcpYAML renders one MCP server as a Continue block YAML document.
// Standalone files under `.continue/mcpServers/` require the block
// wrapper (`name` + `version` + `schema: v1`) with the server nested
// under an `mcpServers:` list; a flat single-server file does not load.
// See https://docs.continue.dev/customize/deep-dives/mcp.
// Stdio servers emit command/args/env; remote servers emit
// type/url/requestOptions.
func mcpYAML(e spec.Entry) (string, error) {
	server := map[string]any{"name": e.Name}

	transport := mcpTransport(e)

	switch transport {
	case "stdio":
		if cmd, _ := e.Meta["command"].(string); cmd != "" {
			server["command"] = cmd
		}
		if args := emit.StringSlice(e.Meta["args"]); len(args) > 0 {
			server["args"] = args
		}
	case "http", "sse", "streamable-http":
		server["type"] = continueTransport(transport)
		if url, _ := e.Meta["url"].(string); url != "" {
			server["url"] = url
		}
		// Continue's url branch takes headers only under requestOptions;
		// zod strips a top-level `headers` key, so the server used to
		// connect unauthenticated with no sync-time signal (#730).
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			server["requestOptions"] = map[string]any{"headers": h}
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
