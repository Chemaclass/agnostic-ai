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

// Emit writes one .mdc per rule and per agent, plus .cursor/mcp.json
// when MCP entries are present.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := emit.RulesDirectory(b, emit.RulesDirOpts{
		Dir:         outDir(cfg),
		Ext:         defaultExt,
		FormatRule:  func(e spec.Entry) string { return mdc(e, true) },
		FormatAgent: func(e spec.Entry) string { return mdc(e, false) },
		FormatSkill: func(e spec.Entry) string { return mdc(e, false) },
	}, dryRun); err != nil {
		return err
	}
	if len(b.MCPs) > 0 {
		doc, err := emit.MCPDocument(b.MCPs, emit.MCPSchemaServersMap)
		if err != nil {
			return err
		}
		if doc != "" {
			if err := emit.WriteFile(outMCPFile(cfg), doc, dryRun); err != nil {
				return err
			}
		}
	}
	return nil
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
	b.WriteString("globs: " + globs + "\n")
	b.WriteString(fmt.Sprintf("alwaysApply: %t\n", always))
	b.WriteString("---\n\n")
	b.WriteString(e.Body)
	return b.String()
}

func outDir(cfg *config.Config) string {
	if o, ok := cfg.Outputs[target]; ok && o.RulesDir != "" {
		return o.RulesDir
	}
	return defaultDir
}

func outMCPFile(cfg *config.Config) string {
	if o, ok := cfg.Outputs[target]; ok && o.MCPFile != "" {
		return o.MCPFile
	}
	return defaultMCPFile
}
