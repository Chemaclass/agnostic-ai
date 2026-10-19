package authority

import (
	"maps"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Session subject wording.
const (
	sessionLabel       = "Tool session (no agent)"
	sessionDescription = "The coding tool itself, before any agent narrows it."
)

// subjectInfo is one selectable subject with its narrowing parsed.
type subjectInfo struct {
	subject Subject
	// entry is the agent spec, or nil for the session.
	entry *spec.Entry
	// listed reports an agent that sets can or tools; caps holds each
	// entry of that list, parsed.
	listed bool
	caps   [][]capability
	// mcpListed reports an agent that sets mcpServers.
	mcpListed bool
}

func (s *subjectInfo) id() string { return s.subject.ID }

// sessionSubject is the coding tool itself, which no agent narrows.
func sessionSubject() *subjectInfo {
	return &subjectInfo{subject: Subject{
		SubjectSummary: SubjectSummary{
			ID:          SessionSubjectID,
			Label:       sessionLabel,
			Description: sessionDescription,
			Kind:        SubjectSession,
		},
		ToolScope:  ToolScope{Mode: "unrestricted", Declared: []string{}, Capabilities: []string{}},
		MCPServers: MCPScope{Mode: "all", Names: []string{}},
	}}
}

// parseSubjects returns the session subject and one subject per agent
// spec, keeping the first spec of each name.
func parseSubjects(agents []spec.Entry, source func(spec.Entry) *Source) []*subjectInfo {
	out := []*subjectInfo{sessionSubject()}
	seen := map[string]bool{SessionSubjectID: true}
	for i := range agents {
		e := agents[i]
		if seen[e.Name] {
			continue
		}
		seen[e.Name] = true
		out = append(out, agentSubject(e, source(e)))
	}
	return out
}

func agentSubject(e spec.Entry, src *Source) *subjectInfo {
	s := &subjectInfo{entry: &e, subject: Subject{
		SubjectSummary: SubjectSummary{
			ID:          e.Name,
			Label:       e.Name,
			Description: e.Description(),
			Kind:        SubjectAgent,
		},
		Source:     src,
		ToolScope:  ToolScope{Mode: "unrestricted", Declared: []string{}, Capabilities: []string{}},
		MCPServers: MCPScope{Mode: "all", Names: []string{}},
	}}
	for _, field := range []string{"can", "tools"} {
		value, set := e.Meta[field]
		if !set {
			continue
		}
		list := toolEntries(value)
		s.listed = true
		s.subject.ToolScope = ToolScope{Mode: "listed", Field: field, Declared: make([]string, 0, len(list)), Capabilities: make([]string, 0, len(list))}
		for _, raw := range list {
			s.caps = append(s.caps, parseCapability(raw))
			s.subject.ToolScope.Declared = append(s.subject.ToolScope.Declared, preview.RedactRule(raw))
			s.subject.ToolScope.Capabilities = append(s.subject.ToolScope.Capabilities, preview.RedactRule(neutralEntry(raw)))
		}
		break
	}
	s.subject.Readonly = e.Meta["readonly"] == true
	if names, ok := mcpServerNames(e.Meta["mcpServers"]); ok {
		s.mcpListed = true
		s.subject.MCPServers = MCPScope{Mode: "listed", Names: names}
	}
	return s
}

// toolEntries reads a can or tools value: a list, or a comma-separated
// string as Qoder and Trae write tools.
func toolEntries(value any) []string {
	out := []string{}
	switch v := value.(type) {
	case string:
		for _, name := range spec.SplitToolList(v) {
			if name != "" {
				out = append(out, name)
			}
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case []string:
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	}
	return out
}

// neutralEntry returns the neutral capability a tool-list entry stands
// for, or the entry as written when no capability maps to it alone.
func neutralEntry(raw string) string {
	names, problem := spec.CapabilityTools([]string{raw})
	if problem == "" && len(names) == 1 {
		if c, ok := spec.NeutralCapability(names[0]); ok {
			return c
		}
	}
	return raw
}

// mcpServerNames reads an mcpServers list of names. An inline
// definition counts by its name field, or by its keys.
func mcpServerNames(value any) ([]string, bool) {
	list, ok := value.([]any)
	if !ok {
		return nil, false
	}
	names := []string{}
	for _, item := range list {
		switch v := item.(type) {
		case string:
			names = append(names, v)
		case map[string]any:
			if name, ok := v["name"].(string); ok {
				names = append(names, name)
				continue
			}
			names = append(names, slices.Sorted(maps.Keys(v))...)
		}
	}
	return names, true
}

// listCovers reports whether the subject's own tool list includes the
// capability for action on r.
func (s *subjectInfo) listCovers(r *resourceInfo, action string) bool {
	for _, entry := range s.caps {
		for _, c := range entry {
			if ok, _ := c.covers(r, action, true); ok {
				return true
			}
		}
	}
	return false
}

// reaches reports whether the subject's mcpServers list lets it reach
// server.
func (s *subjectInfo) reaches(server string) bool {
	return !s.mcpListed || slices.Contains(s.subject.MCPServers.Names, server)
}

// listRule spells the subject's tool list as the spec writes it.
func (s *subjectInfo) listRule() string {
	return s.subject.ToolScope.Field + ": [" + strings.Join(s.subject.ToolScope.Declared, ", ") + "]"
}

// summaries lists every subject for the selector.
func summaries(subjects []*subjectInfo) []SubjectSummary {
	out := make([]SubjectSummary, 0, len(subjects))
	for _, s := range subjects {
		out = append(out, s.subject.SubjectSummary)
	}
	return out
}
