package authority

import (
	"strconv"
	"strings"
)

// Uncertainty severities.
const (
	severityInfo    = "info"
	severityWarning = "warning"
)

// uncertainties lists everything the view cannot determine, in a fixed
// order: runtime, installed state, each server, patterns, each target,
// routes, and secrets.
func (bd *builder) uncertainties(built []*builtResource, defaulted map[string]bool, hasSecrets bool) []Uncertainty {
	out := []Uncertainty{{
		ID:         UncertaintyRuntimeNotObserved,
		Code:       UncertaintyRuntimeNotObserved,
		Severity:   severityInfo,
		Message:    "No running tool was observed. Every effect is what the specs declare and what sync writes, not what a tool enforced.",
		RelatedIDs: []string{},
	}}
	add := func(id, code, severity, message string, related []string) {
		if related == nil {
			related = []string{}
		}
		out = append(out, Uncertainty{ID: id, Code: code, Severity: severity, Message: message, RelatedIDs: related})
	}
	if bd.installedUnchecked {
		add(UncertaintyInstalledNotChecked, UncertaintyInstalledNotChecked, severityInfo,
			"Generated files were not compared with disk, so an effect that depends on them may describe a state sync has not written yet.", []string{bd.target})
	}
	scopes := map[string][]string{}
	for _, br := range built {
		if br.info.kind == kindMCP {
			scopes[br.info.mcp.id] = append(scopes[br.info.mcp.id], br.res.ID)
		}
	}
	for _, m := range bd.mcps {
		resources := scopes[m.id]
		for _, root := range m.roots {
			if root.file {
				continue
			}
			named := root.uri
			if root.name != "" {
				named += " (" + root.name + ")"
			}
			add(UncertaintyRootNotFilesystem+":"+m.id+":"+strconv.Itoa(root.index), UncertaintyRootNotFilesystem, severityWarning,
				m.name+" declares root "+named+", which is not a file URI. MCP roots are informational filesystem guidance, so this root names no resource and the server's scope stays undeclared.",
				append([]string{m.id}, resources...))
		}
		if len(m.fileRoots()) == 0 {
			add(UncertaintyScopeUndeclared+":"+m.id, UncertaintyScopeUndeclared, severityWarning,
				m.name+" declares no file roots, so what it reaches is not declared.", append([]string{m.id}, resources...))
			add(UncertaintyCredentialReach+":"+m.id, UncertaintyCredentialReach, severityInfo,
				"What the credential of "+m.name+" can reach is set by the provider and is not inspected.", append([]string{m.id}, resources...))
		}
		if defaulted[m.id] {
			add(UncertaintyDomainDefaulted+":"+m.id, UncertaintyDomainDefaulted, severityInfo,
				m.name+" matches no curated domain, so it is shown under AI & Agent Ops.", []string{m.id})
		}
		var anyTool []string
		for _, id := range resources {
			anyTool = append(anyTool, operationID(id, anyToolAction))
		}
		add(UncertaintyToolListUnknown+":"+m.id, UncertaintyToolListUnknown, severityInfo,
			"The tool list of "+m.name+" is not known; tools no rule names are grouped under Any other tool.", append([]string{m.id}, anyTool...))
	}
	if bd.scopedFileRules() {
		add(UncertaintyPatternApproximate, UncertaintyPatternApproximate, severityInfo,
			"File patterns are compared as text, so whether one rule's pattern contains another's is approximate.", []string{})
	}
	for _, t := range bd.cfg.Targets {
		if ids := bd.notExpressed[t]; len(ids) > 0 {
			add(UncertaintyTargetNotExpressed+":"+t, UncertaintyTargetNotExpressed, severityWarning,
				TargetLabel(t)+" receives no rule for "+count(len(ids), "declared decision", "declared decisions")+"; its own defaults apply there.", ids)
		}
	}
	for _, t := range bd.cfg.Targets {
		if ids := bd.unverified[t]; len(ids) > 0 {
			add(UncertaintyMatchingUnverified+":"+t, UncertaintyMatchingUnverified, severityWarning,
				TargetLabel(t)+" receives "+count(len(ids), "rule", "rules")+" whose matching this repository does not document; their effect stays unresolved.", ids)
		}
	}
	// Routes beyond shell and local files are never modeled, so this
	// holds even when no checked route is open; it relates the
	// operations whose checked routes are open.
	var routed []string
	for _, br := range built {
		for _, op := range br.res.Operations {
			if len(op.AlternateRoutes) > 0 {
				routed = append(routed, op.ID)
			}
		}
	}
	add(UncertaintyAlternateRoute, UncertaintyAlternateRoute, severityWarning,
		"Only shell access and local files are checked as alternate routes. "+sentence(listWords(notModeledRoutes))+" are not modeled.", routed)
	if hasSecrets {
		add(UncertaintySecretReadUnknown, UncertaintySecretReadUnknown, severityInfo,
			"Whether a subject can read a secret's value another way, through a shell command or a file, is not determined.", []string{})
	}
	return out
}

// scopedFileRules reports any file rule or protected path scoped to a
// pattern, whose containment the view only approximates.
func (bd *builder) scopedFileRules() bool {
	if len(bd.protect) > 0 {
		return true
	}
	for _, r := range bd.rules {
		for _, c := range r.caps {
			if c.path != "" {
				return true
			}
		}
	}
	for _, entry := range bd.subject.caps {
		for _, c := range entry {
			if c.path != "" {
				return true
			}
		}
	}
	return false
}

func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// listWords joins items as a sentence does: "a, b, and c".
func listWords(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	}
	return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
}
