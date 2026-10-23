package kimi

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	for _, a := range agents {
		if !nativeName.MatchString(a.Name) {
			return fmt.Errorf("kimi: agent %s requires a lowercase kebab-case name", a.Name)
		}
		var reason string
		a, reason = a.NativeToolsFor(target)
		if reason != "" {
			return fmt.Errorf("kimi: agent %s: %s", a.Name, reason)
		}
		m := emit.ResolveMeta(a.Meta, target)
		out := native(a.Meta)
		out["name"] = a.Name
		out["description"] = a.Name
		if d, ok := m["description"]; ok {
			out["description"] = d
		}
		if _, override := out["tools"]; !override {
			if raw, ok := a.Meta["tools"]; ok {
				translated := []string{}
				for _, name := range list(raw) {
					if name == "*" || strings.HasPrefix(name, "mcp__") {
						if strings.HasPrefix(name, "mcp__") && !strings.Contains(strings.TrimPrefix(name, "mcp__"), "__") {
							name += "__*"
						}
						translated = append(translated, name)
						continue
					}
					if strings.Contains(name, "(") {
						return fmt.Errorf("kimi: agent %s cannot express scoped tool %s; use a native tool restriction", a.Name, name)
					}
					if name == "WebFetch" {
						name = "FetchURL"
					}
					switch name {
					case "Read", "Write", "Edit", "Bash", "Glob", "Grep", "FetchURL", "WebSearch":
						translated = append(translated, name)
					default:
						emit.NoteAgentToolsNoOp(target, []spec.Entry{a}, "unsupported tool "+name+" omitted; remaining allowlist stays restrictive")
					}
				}
				out["tools"] = translated
			}
		}
		for _, key := range []string{"model", "effort", "maxTurns"} {
			if _, ok := m[key]; ok {
				emit.NoteFieldNoOp(target, spec.KindAgent, key, 1, "Kimi Code agent frontmatter does not read this field")
			}
			delete(out, key)
		}
		if err := validateAgent(out); err != nil {
			return fmt.Errorf("kimi: agent %s: %w", a.Name, err)
		}
		if strings.TrimSpace(a.Body) == "" {
			return fmt.Errorf("kimi: agent %s requires a prompt body", a.Name)
		}
		if err := sess.WriteFile(filepath.Join(dir, a.Name+".md"), emit.WithHeader(emit.Frontmatter(out)+"\n"+strings.TrimSpace(a.Body)+"\n", emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	return nil
}
func (Adapter) ToolCapabilities() emit.ToolCapabilityTable {
	return emit.ToolCapabilityTable{"read": {Agent: []string{"Read"}}, "write": {Agent: []string{"Write"}}, "edit": {Agent: []string{"Edit"}}, "shell": {Agent: []string{"Bash"}}, "web": {Agent: []string{"FetchURL", "WebSearch"}}}
}
func (Adapter) TranslateAgentCapability(name string) ([]string, bool) {
	if strings.HasPrefix(name, "mcp__") {
		if !strings.Contains(strings.TrimPrefix(name, "mcp__"), "__") {
			name += "__*"
		}
		return []string{name}, true
	}
	if name == "Grep" || name == "Glob" || name == "FetchURL" {
		return []string{name}, true
	}
	if n, ok := emit.CapabilityTool((Adapter{}).ToolCapabilities(), name, false); ok {
		return []string{n}, true
	}
	return nil, false
}
