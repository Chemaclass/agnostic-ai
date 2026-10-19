package authority

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Built-in provider ids.
const (
	providerFiles = "local-files"
	providerShell = "shell"
	providerWeb   = "web"
)

// rootCaveat is what a file root does not prove.
const rootCaveat = "informational filesystem guidance; not access control"

// mcpInfo is one MCP server spec, parsed.
type mcpInfo struct {
	entry    spec.Entry
	name     string
	id       string
	disabled bool
	roots    []rootInfo
}

// rootInfo is one entry of an MCP server's roots list.
type rootInfo struct {
	// index is the root's 1-based position in the list.
	index int
	uri   string
	name  string
	// file reports a file URI, the only kind of root that names a
	// filesystem boundary.
	file bool
}

// parseMCPs reads each MCP server spec, keeping the first of each name.
func parseMCPs(entries []spec.Entry) []*mcpInfo {
	var out []*mcpInfo
	seen := map[string]bool{}
	ids := map[string]bool{}
	for _, e := range entries {
		if seen[e.Name] {
			continue
		}
		seen[e.Name] = true
		m := &mcpInfo{entry: e, name: e.Name, disabled: e.Meta["disabled"] == true}
		m.id = uniqueID(ids, "mcp-"+slug(e.Name))
		roots, _ := e.Meta["roots"].([]any)
		for i, raw := range roots {
			fields, _ := raw.(map[string]any)
			uri, _ := fields["uri"].(string)
			name, _ := fields["name"].(string)
			if uri == "" {
				continue
			}
			m.roots = append(m.roots, rootInfo{index: i + 1, uri: uri, name: name, file: fileURI(uri)})
		}
		out = append(out, m)
	}
	return out
}

// fileURI reports whether uri uses the file scheme.
func fileURI(uri string) bool {
	if u, err := url.Parse(uri); err == nil {
		return strings.EqualFold(u.Scheme, "file")
	}
	return strings.HasPrefix(strings.ToLower(uri), "file:")
}

func (m *mcpInfo) fileRoots() []rootInfo {
	var out []rootInfo
	for _, r := range m.roots {
		if r.file {
			out = append(out, r)
		}
	}
	return out
}

// uniqueID returns id, or id with a numeric suffix when it is taken.
func uniqueID(taken map[string]bool, id string) string {
	candidate := id
	for n := 2; taken[candidate]; n++ {
		candidate = id + "-" + strconv.Itoa(n)
	}
	taken[candidate] = true
	return candidate
}

// builtinProviders are the coding tool's own capabilities.
func builtinProviders() []Provider {
	return []Provider{
		{
			ID:               providerFiles,
			Label:            "Local files",
			Description:      "Reading, editing, writing, and deleting files in the project.",
			Kind:             "builtin",
			DomainIDs:        []string{DomainFilesystem},
			DomainResolution: DomainResolutionBuiltin,
			Status:           "configured",
			ResourceIDs:      []string{},
			SecretIDs:        []string{},
			ResourceCoverage: "complete",
			CoverageNote:     "Resources are the patterns permission rules, protected paths, and the agent's tool list name.",
		},
		{
			ID:               providerShell,
			Label:            "Shell commands",
			Description:      "Commands the coding tool runs on this machine.",
			Kind:             "builtin",
			DomainIDs:        []string{DomainFilesystem},
			DomainResolution: DomainResolutionBuiltin,
			Status:           "configured",
			ResourceIDs:      []string{},
			SecretIDs:        []string{},
			ResourceCoverage: "complete",
			CoverageNote:     "Resources are the command patterns permission rules and the agent's tool list name.",
		},
		{
			ID:               providerWeb,
			Label:            "Web access",
			Description:      "Fetching pages and searching the web.",
			Kind:             "builtin",
			DomainIDs:        []string{DomainWeb},
			DomainResolution: DomainResolutionBuiltin,
			Status:           "configured",
			ResourceIDs:      []string{},
			SecretIDs:        []string{},
			ResourceCoverage: "complete",
			CoverageNote:     "Resources are web access as a whole and each domain a WebFetch rule names.",
		},
	}
}

// mcpProvider describes one MCP server for subject.
func mcpProvider(m *mcpInfo, subject *subjectInfo, src *Source) (Provider, bool) {
	domains := mcpDomains(m.entry)
	resolution := DomainResolutionCurated
	if len(domains) == 0 {
		domains, resolution = []string{DomainAgentOps}, DomainResolutionDefault
	}
	status := "configured"
	switch {
	case m.disabled:
		status = "disabled"
	case !subject.reaches(m.name):
		status = "unavailable-to-agent"
	}
	p := Provider{
		ID:               m.id,
		Label:            m.name,
		Description:      m.entry.Description(),
		Kind:             "mcp",
		DomainIDs:        domains,
		DomainResolution: resolution,
		Status:           status,
		Transport:        transport(m.entry),
		Source:           src,
		ResourceIDs:      []string{},
		SecretIDs:        []string{},
		ResourceCoverage: "undeclared",
		CoverageNote:     "The server declares no file roots. Credential reach is not inspected.",
	}
	if len(m.fileRoots()) > 0 {
		p.ResourceCoverage = "declared"
		p.CoverageNote = "File roots are filesystem guidance this project gives the server, not access control. Credential reach is not inspected."
	}
	return p, resolution == DomainResolutionDefault
}

// transport describes how the server is reached: its URL, with any
// credential written into it redacted, or its command. A `${NAME}`
// reference stays as written.
func transport(e spec.Entry) *Transport {
	kind, _ := e.Meta["type"].(string)
	if kind == "" {
		kind = "stdio"
	}
	t := &Transport{Type: kind}
	if u, ok := e.Meta["url"].(string); ok && u != "" {
		t.Endpoint = redactURL(u)
	} else if c, ok := e.Meta["command"].(string); ok {
		t.Endpoint = c
	}
	return t
}

// redactURL keeps a URL's scheme, host, and plain path words, and
// replaces any user information, other path segment, or query and
// fragment value that is not only `${NAME}` references, since such text
// may be a credential.
func redactURL(raw string) string {
	rest, fragment, hasFragment := strings.Cut(raw, "#")
	rest, query, hasQuery := strings.Cut(rest, "?")
	if scheme, after, ok := strings.Cut(rest, "://"); ok {
		authority, path := after, ""
		if i := strings.Index(after, "/"); i >= 0 {
			authority, path = after[:i], after[i:]
		}
		if at := strings.LastIndex(authority, "@"); at >= 0 && !onlyRefs(authority[:at]) {
			authority = preview.Redacted + authority[at:]
		}
		rest = scheme + "://" + authority + redactPath(path)
	}
	out := rest
	if hasQuery {
		out += "?" + redactPairs(query, "&")
	}
	if hasFragment {
		out += "#" + redactPairs(fragment, "&")
	}
	return out
}

// plainPathSegment matches a path word such as mcp, sse, or v1. Webhook
// and bot URLs carry their token as a path segment, which mixes letters
// and digits or runs long, so any other segment is redacted. That errs
// toward hiding: a harmless segment with a digit, such as `oauth2`, or
// one over 32 characters is redacted too.
var plainPathSegment = regexp.MustCompile(`^(?:[A-Za-z][A-Za-z._-]{0,31}|v[0-9]{1,3})$`)

func redactPath(path string) string {
	segments := strings.Split(path, "/")
	for i, s := range segments {
		if s != "" && !onlyRefs(s) && !plainPathSegment.MatchString(s) {
			segments[i] = preview.Redacted
		}
	}
	return strings.Join(segments, "/")
}

func redactPairs(text, sep string) string {
	parts := strings.Split(text, sep)
	for i, part := range parts {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			if part != "" && !onlyRefs(part) {
				parts[i] = preview.Redacted
			}
			continue
		}
		if value != "" && !onlyRefs(value) {
			parts[i] = key + "=" + preview.Redacted
		}
	}
	return strings.Join(parts, sep)
}

// onlyRefs reports whether text is `${NAME}` references and nothing
// else, apart from the colon between a user and a password.
func onlyRefs(text string) bool {
	tokens := spec.EnvRefTokens(text)
	if len(tokens) == 0 {
		return false
	}
	rest := text
	for _, t := range tokens {
		if !t.Known() {
			return false
		}
		rest = strings.Replace(rest, t.Text, "", 1)
	}
	return strings.Trim(rest, ": ") == ""
}
