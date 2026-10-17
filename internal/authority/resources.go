package authority

import (
	"slices"
	"strconv"
)

// resourceKind is the provider family a resource belongs to.
type resourceKind int

const (
	kindFiles resourceKind = iota
	kindShell
	kindWeb
	kindMCP
)

// resourceInfo is one resource before its operations are resolved.
type resourceInfo struct {
	res  Resource
	kind resourceKind
	// pattern is the normalized file pattern, "**" for every file.
	pattern string
	// shell is the command pattern; nil stands for any command.
	shell *shellPattern
	// domain is the web domain, empty for all web access.
	domain string
	// mcp is the server of an MCP resource; root is its file root, or nil
	// for an undeclared scope.
	mcp     *mcpInfo
	root    *rootInfo
	actions []actionInfo
}

// actionInfo is one operation a resource offers.
type actionInfo struct {
	action string
	label  string
}

const allFiles = "**"

// discovery gathers what establishes resources, in the order the specs
// name it: settings rules first, then the subject's own tool list, then
// protected paths.
type discovery struct {
	caps      []capability
	protected []string
}

func (bd *builder) discover(subject *subjectInfo) discovery {
	var d discovery
	for _, r := range bd.patternRules {
		d.caps = append(d.caps, r.caps...)
	}
	for _, entry := range subject.caps {
		d.caps = append(d.caps, entry...)
	}
	for _, p := range bd.patternProtect {
		d.protected = append(d.protected, p.path)
	}
	return d
}

// enumerateResources lists every resource the view resolves for
// subject, provider by provider.
func (bd *builder) enumerateResources(subject *subjectInfo, domains map[string][]string) []*resourceInfo {
	d := bd.discover(subject)
	ids := map[string]bool{}
	var out []*resourceInfo
	out = append(out, fileResources(d, ids, domains[providerFiles])...)
	out = append(out, shellResources(d, ids, domains[providerShell])...)
	out = append(out, webResources(d, ids, domains[providerWeb])...)
	for _, m := range bd.mcps {
		out = append(out, mcpResources(m, d, ids, domains[m.id])...)
	}
	return out
}

func fileResources(d discovery, ids map[string]bool, domains []string) []*resourceInfo {
	patterns := []string{allFiles}
	add := func(p string) {
		if p != "" && !slices.Contains(patterns, p) {
			patterns = append(patterns, p)
		}
	}
	for _, c := range d.caps {
		if c.kind == capRead || c.kind == capEdit {
			add(c.path)
		}
	}
	for _, p := range d.protected {
		add(p)
	}
	out := make([]*resourceInfo, 0, len(patterns))
	for _, p := range patterns {
		r := &resourceInfo{kind: kindFiles, pattern: p, actions: fileActionLabels}
		r.res = Resource{
			ID:            uniqueID(ids, "files-"+slug(p)),
			Label:         p,
			Description:   "Files that match " + p + ".",
			Kind:          ResourceFilePattern,
			ProviderID:    providerFiles,
			DomainIDs:     resourceDomains(domains),
			Identity:      Identity{Pattern: p},
			Basis:         "rule-pattern",
			LabelResolved: true,
		}
		if p == allFiles {
			r.res.Label = "All project files"
			r.res.Description = "Every file in the project."
		}
		out = append(out, r)
	}
	return out
}

func shellResources(d discovery, ids map[string]bool, domains []string) []*resourceInfo {
	anyCommand := &resourceInfo{kind: kindShell, actions: []actionInfo{{action: "run", label: "Run"}}}
	anyCommand.res = Resource{
		ID:            uniqueID(ids, "shell-any"),
		Label:         "Any command",
		Description:   "Every command the tool can run.",
		Kind:          ResourceCommandPattern,
		ProviderID:    providerShell,
		DomainIDs:     resourceDomains(domains),
		Identity:      Identity{Pattern: "*"},
		Basis:         "rule-pattern",
		LabelResolved: true,
	}
	out := []*resourceInfo{anyCommand}
	var seen []shellPattern
	for _, c := range d.caps {
		if c.kind != capShell || c.shell == nil {
			continue
		}
		p := *c.shell
		if !p.exact && p.prefix == "" {
			continue // `shell(*)` is any command
		}
		if slices.ContainsFunc(seen, func(s shellPattern) bool { return s.exact == p.exact && s.prefix == p.prefix }) {
			continue
		}
		seen = append(seen, p)
		r := &resourceInfo{kind: kindShell, shell: &p, actions: anyCommand.actions}
		r.res = Resource{
			Kind:          ResourceCommandPattern,
			ProviderID:    providerShell,
			DomainIDs:     resourceDomains(domains),
			Basis:         "rule-pattern",
			LabelResolved: true,
		}
		if p.exact {
			r.res.ID = uniqueID(ids, "shell-"+slug(p.prefix)+"-exact")
			r.res.Label = p.prefix
			r.res.Description = "Exactly this command."
			r.res.Identity = Identity{Pattern: p.prefix}
		} else {
			r.res.ID = uniqueID(ids, "shell-"+slug(p.prefix))
			r.res.Label = p.prefix + " …"
			r.res.Description = "Commands that start with \"" + p.prefix + "\"."
			r.res.Identity = Identity{Pattern: p.prefix + " *"}
		}
		out = append(out, r)
	}
	return out
}

func webResources(d discovery, ids map[string]bool, domains []string) []*resourceInfo {
	all := &resourceInfo{kind: kindWeb, actions: []actionInfo{{action: "fetch", label: "Fetch pages"}, {action: "search", label: "Search the web"}}}
	all.res = Resource{
		ID:            uniqueID(ids, "web-all"),
		Label:         "Web fetch and search",
		Description:   "Fetching any page and searching the web.",
		Kind:          ResourceWeb,
		ProviderID:    providerWeb,
		DomainIDs:     resourceDomains(domains),
		Identity:      Identity{Pattern: "*"},
		Basis:         "rule-pattern",
		LabelResolved: true,
	}
	out := []*resourceInfo{all}
	var seen []string
	for _, c := range d.caps {
		if c.kind != capWebFetch || c.domain == "" || slices.Contains(seen, c.domain) {
			continue
		}
		seen = append(seen, c.domain)
		r := &resourceInfo{kind: kindWeb, domain: c.domain, actions: []actionInfo{{action: "fetch", label: "Fetch pages"}}}
		r.res = Resource{
			ID:            uniqueID(ids, "web-domain-"+slug(c.domain)),
			Label:         c.domain,
			Description:   "Pages fetched from " + c.domain + ".",
			Kind:          ResourceWeb,
			ProviderID:    providerWeb,
			DomainIDs:     resourceDomains(domains),
			Identity:      Identity{Pattern: "domain:" + c.domain},
			Basis:         "rule-pattern",
			LabelResolved: true,
		}
		out = append(out, r)
	}
	return out
}

// mcpResources lists one declared-root resource per file root, or one
// undeclared scope when the server has none. A root with any other
// scheme establishes nothing; the view reports it as an uncertainty.
func mcpResources(m *mcpInfo, d discovery, ids map[string]bool, domains []string) []*resourceInfo {
	actions := mcpActions(m, d)
	var out []*resourceInfo
	for _, root := range m.fileRoots() {
		root := root
		r := &resourceInfo{kind: kindMCP, mcp: m, root: &root, actions: actions}
		r.res = Resource{
			ID:            uniqueID(ids, m.id+"-root-"+strconv.Itoa(root.index)),
			Label:         root.uri,
			Kind:          ResourceDeclaredRoot,
			ProviderID:    m.id,
			DomainIDs:     resourceDomains(domains),
			Identity:      Identity{URI: root.uri, Server: m.name},
			Basis:         "declared-root",
			Caveat:        rootCaveat,
			LabelResolved: false,
		}
		if root.name != "" {
			r.res.Label, r.res.LabelResolved = root.name, true
		}
		out = append(out, r)
	}
	if len(out) > 0 {
		return out
	}
	r := &resourceInfo{kind: kindMCP, mcp: m, actions: actions}
	r.res = Resource{
		ID:               uniqueID(ids, m.id+"-scope"),
		Label:            "Scope not declared",
		Description:      "The server declares no roots. Credential reach is not inspected.",
		Kind:             ResourceUndeclaredScope,
		ProviderID:       m.id,
		DomainIDs:        resourceDomains(domains),
		Identity:         Identity{Server: m.name},
		Basis:            "undeclared",
		LabelResolved:    true,
		UnresolvedReason: "The server's reach is set by its credential and the provider, which configuration does not show.",
	}
	return append(out, r)
}

// mcpActions names one operation per tool a rule or the subject's tool
// list names, then the operation for every other tool.
func mcpActions(m *mcpInfo, d discovery) []actionInfo {
	var out []actionInfo
	for _, c := range d.caps {
		if c.kind != capMCP || c.server != m.name || c.tool == "" {
			continue
		}
		if slices.ContainsFunc(out, func(a actionInfo) bool { return a.action == c.tool }) {
			continue
		}
		out = append(out, actionInfo{action: c.tool, label: humanizeTool(c.tool)})
	}
	return append(out, actionInfo{action: anyToolAction, label: "Any other tool"})
}

// operationID names an operation on a resource.
func operationID(resourceID, action string) string {
	return resourceID + ":" + action
}

// capabilityName spells the capability an operation needs, for notes
// and agent-scope translation.
func capabilityName(r *resourceInfo, action string) string {
	switch r.kind {
	case kindFiles:
		if r.pattern != allFiles && (action == capRead || action == capEdit) {
			return action + "(" + r.pattern + ")"
		}
		return action
	case kindShell:
		switch {
		case r.shell == nil:
			return capShell
		case r.shell.exact:
			return "shell(" + r.shell.prefix + ")"
		}
		return "shell(" + r.shell.prefix + " *)"
	case kindWeb:
		if r.domain != "" {
			return "WebFetch(domain:" + r.domain + ")"
		}
		return capWeb
	case kindMCP:
		if action == anyToolAction {
			return "mcp:" + r.mcp.name
		}
		return "mcp:" + r.mcp.name + "/" + action
	}
	return action
}
