package authority

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Secret explanations.
const (
	secretReadExplanation   = "The configuration references the value by name only. Whether the subject can read it another way, through a shell command or a file, depends on the Filesystem & Execution rules."
	secretManageExplanation = "Rotation and revocation happen in the provider, outside this configuration."
)

// secretRef is one `${NAME}` an MCP server spec references.
type secretRef struct {
	name  string
	field string
}

// mcpSecretRefs lists every environment reference in a server's env and
// headers values, args, and url, in that order. Editor variables such as
// `${workspaceFolder}` name no secret and are skipped.
func mcpSecretRefs(e spec.Entry) []secretRef {
	var refs []secretRef
	add := func(field, value string) {
		for _, t := range spec.EnvRefTokens(value) {
			if t.Known() && !t.EditorVariable() {
				refs = append(refs, secretRef{name: t.Name, field: field})
			}
		}
	}
	for _, field := range []string{"env", "headers"} {
		values, _ := e.Meta[field].(map[string]any)
		for _, key := range slices.Sorted(maps.Keys(values)) {
			if s, ok := values[key].(string); ok {
				add(field+"."+key, s)
			}
		}
	}
	args, _ := e.Meta["args"].([]any)
	for i, arg := range args {
		if s, ok := arg.(string); ok {
			add("args["+strconv.Itoa(i)+"]", s)
		}
	}
	if u, ok := e.Meta["url"].(string); ok {
		add("url", u)
	}
	return refs
}

// buildSecrets returns each referenced secret with the subject's rights
// over it, and records each provider's secret ids on providers.
func buildSecrets(mcps []*mcpInfo, providers map[string]*Provider, subject *subjectInfo, source func(spec.Entry) *Source) []Secret {
	var out []Secret
	index := map[string]int{}
	for _, m := range mcps {
		p := providers[m.id]
		for _, ref := range mcpSecretRefs(m.entry) {
			id := "secret-" + ref.name
			i, ok := index[id]
			if !ok {
				i = len(out)
				index[id] = i
				out = append(out, Secret{
					ID:           id,
					Name:         ref.name,
					ProviderIDs:  []string{},
					ReferencedBy: []SecretRef{},
					Rights:       SecretRights{Discover: "allowed", ReadValue: "unknown", Use: "unavailable", Manage: "unknown"},
					Explanation:  secretReadExplanation + " " + secretManageExplanation,
				})
			}
			s := &out[i]
			if !slices.Contains(s.ProviderIDs, m.id) {
				s.ProviderIDs = append(s.ProviderIDs, m.id)
			}
			s.ReferencedBy = append(s.ReferencedBy, SecretRef{Source: *source(m.entry), Field: ref.field})
			if !m.disabled && subject.reaches(m.name) {
				s.Rights.Use = "allowed"
			}
			if p != nil && !slices.Contains(p.SecretIDs, id) {
				p.SecretIDs = append(p.SecretIDs, id)
			}
		}
	}
	for i := range out {
		out[i].Label = secretLabel(out[i], providers)
	}
	return out
}

func secretLabel(s Secret, providers map[string]*Provider) string {
	labels := make([]string, 0, len(s.ProviderIDs))
	for _, id := range s.ProviderIDs {
		if p := providers[id]; p != nil {
			labels = append(labels, p.Label)
		}
	}
	if len(labels) == 1 {
		return s.Name + " (" + labels[0] + " credential)"
	}
	return s.Name + " (credential for " + strings.Join(labels, ", ") + ")"
}
