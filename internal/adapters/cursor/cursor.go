// Package cursor emits .cursor/rules/*.mdc files for the Cursor editor.
//
// Rules emit with alwaysApply=true; agents emit as rules with
// alwaysApply=false. Both honor a frontmatter override.
package cursor

import (
	"fmt"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target         = "cursor"
	defaultDir     = ".cursor/rules"
	defaultExt     = ".mdc"
	defaultMCPFile = ".cursor/mcp.json"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP},
}

// Adapter emits Cursor configs.
type Adapter struct{}

// New returns a Cursor adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one .mdc per rule, agent, and skill, plus an
// `.cursor/mcp.json` when MCP entries exist. When
// `outputs.cursor.commands-dir` is set, also writes one Cursor Custom
// Command per agent at that directory; the rule-form emission still
// happens so users that depend on it keep working.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := emit.RulesDirectory(b, emit.RulesDirOpts{
		Dir:         emit.OutputRulesDir(cfg, target, defaultDir),
		Ext:         defaultExt,
		FormatRule:  func(e spec.Entry) string { return emit.WithHeader(mdc(e, true), emit.FormatMarkdown) },
		FormatAgent: func(e spec.Entry) string { return emit.WithHeader(mdc(e, false), emit.FormatMarkdown) },
		FormatSkill: func(e spec.Entry) string { return emit.WithHeader(mdc(e, false), emit.FormatMarkdown) },
	}, dryRun); err != nil {
		return err
	}
	if commandsDir := emit.OutputCommandsDir(cfg, target, ""); commandsDir != "" {
		for _, a := range b.Agents {
			path := commandsDir + "/" + a.Name + ".md"
			if err := emit.WriteFile(path, emit.WithHeader(command(a), emit.FormatMarkdown), dryRun); err != nil {
				return err
			}
		}
	}
	return emit.WriteMCPFile(b.MCPs, emit.MCPSchemaServersMap, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// command renders a Cursor Custom Command file. The Cursor docs
// describe these as Markdown with optional frontmatter (`description`,
// `model`); the body is the prompt the IDE sends when the user invokes
// the command.
func command(e spec.Entry) string {
	m := emit.ResolveMeta(e.Meta, target)
	desc, _ := m["description"].(string)
	model, _ := m["model"].(string)
	var b strings.Builder
	b.WriteString("---\n")
	if desc != "" {
		b.WriteString("description: " + desc + "\n")
	}
	if model != "" {
		b.WriteString("model: " + model + "\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(e.Body)
	return b.String()
}

func mdc(e spec.Entry, alwaysApplyDefault bool) string {
	m := emit.ResolveMeta(e.Meta, target)
	desc, _ := m["description"].(string)
	globs, _ := m["globs"].(string)
	if globs == "" {
		globs = "**/*"
	}
	always := alwaysApplyDefault
	if v, ok := m["alwaysApply"].(bool); ok {
		always = v
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("description: " + desc + "\n")
	fmt.Fprintf(&b, "globs: %q\n", globs)
	fmt.Fprintf(&b, "alwaysApply: %t\n", always)
	b.WriteString("---\n\n")
	b.WriteString(e.Body)
	return b.String()
}
