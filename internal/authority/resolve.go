package authority

import (
	"slices"
	"strconv"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// ruleRef is one permission rule of a settings spec.
type ruleRef struct {
	list string
	raw  string
	// shown is raw, or raw redacted when it holds a credential.
	shown    string
	redacted bool
	caps     []capability
	entry    spec.Entry
	source   Source
}

// protectRef is one path of a settings spec's protected block.
type protectRef struct {
	path     string
	decision string
	reason   string
	entry    spec.Entry
	source   Source
}

// Grant kinds a declaration can be decided by.
const (
	grantRule      = "rule"
	grantProtected = "protected"
	grantNarrowing = "narrowing"
)

// candidate is one declaration that applies to an operation.
type candidate struct {
	kind    string
	list    string
	effect  Effect
	rule    *ruleRef
	protect *protectRef
	grant   Grant
}

// declaration is the canonical decision for one operation: the specs
// alone, before any tool reads them.
type declaration struct {
	declared Effect
	grants   []Grant
	// win is the declaration that decided, or nil when none did.
	win *candidate
	// narrow holds every narrowing of the subject's own spec that leaves
	// the operation out, in the order the spec reads.
	narrow []candidate
	// disabled reports an operation on a server whose spec sets
	// disabled: true.
	disabled bool
	// capability names what the operation needs, as a rule spells it.
	capability string
}

// declare resolves one operation for subject. The subject's own
// narrowing comes first; then the settings lists by neutral precedence,
// deny over ask over allow, where a more specific rule never overrides
// a stronger one.
func (bd *builder) declare(s *subjectInfo, r *resourceInfo, action string) declaration {
	d := declaration{capability: capabilityName(r, action)}
	narrow := bd.narrowing(s, r, action, d.capability)
	var deny, ask, allow []candidate
	for i := range bd.rules {
		rule := &bd.rules[i]
		for _, c := range rule.caps {
			ok, note := c.covers(r, action, false)
			if !ok {
				continue
			}
			cand := candidate{kind: grantRule, list: rule.list, rule: rule, grant: Grant{
				Rule: rule.shown, List: rule.list, Scope: "project", Source: rule.source, Note: note, Redacted: rule.redacted,
			}}
			switch rule.list {
			case "deny":
				cand.effect = EffectDenied
				deny = append(deny, cand)
			case "ask":
				cand.effect = EffectAsk
				ask = append(ask, cand)
			default:
				cand.effect = EffectAllowed
				allow = append(allow, cand)
			}
			break
		}
	}
	if r.kind == kindFiles && (action == capEdit || action == capWrite) {
		for i := range bd.protect {
			p := &bd.protect[i]
			if !protectCovers(p.path, r.pattern) {
				continue
			}
			cand := candidate{kind: grantProtected, list: "protected", protect: p, grant: Grant{
				Rule: p.path, List: "protected", Scope: "project", Source: p.source, Note: protectNote(p),
			}}
			if p.decision == spec.ProtectDeny {
				cand.effect = EffectDenied
				deny = append(deny, cand)
			} else {
				cand.effect = EffectAsk
				ask = append(ask, cand)
			}
		}
	}
	d.narrow = narrow
	// The classes are in precedence order, so the first declaration
	// decides and the grants list the winner first.
	ordered := append(append(append(slices.Clone(narrow), deny...), ask...), allow...)
	switch {
	case r.kind == kindMCP && r.mcp.disabled:
		d.declared, d.disabled = EffectUnavailable, true
	case len(ordered) > 0:
		ordered[0].grant.Wins = true
		d.win = &ordered[0]
		d.declared = d.win.effect
	default:
		d.declared = EffectDefault
	}
	d.grants = make([]Grant, 0, len(ordered))
	for _, c := range ordered {
		d.grants = append(d.grants, c.grant)
	}
	return d
}

// narrowing lists the subject's own declarations that leave the
// operation out: its mcpServers list, its tool list, and readonly.
func (bd *builder) narrowing(s *subjectInfo, r *resourceInfo, action, capability string) []candidate {
	if s.entry == nil {
		return nil
	}
	at := func(key string) Source {
		src := Source{}
		if s.subject.Source != nil {
			src = *s.subject.Source
		}
		src.Line = bd.lines.keyLine(*s.entry, key)
		return src
	}
	var out []candidate
	if r.kind == kindMCP && !s.reaches(r.mcp.name) {
		out = append(out, candidate{kind: grantNarrowing, list: "mcpServers", effect: EffectUnavailable, grant: Grant{
			Rule:   "mcpServers: [" + strings.Join(s.subject.MCPServers.Names, ", ") + "]",
			List:   "mcpServers",
			Scope:  "agent",
			Source: at("mcpServers"),
			Note:   "The agent's mcpServers list does not include " + r.mcp.name,
		}})
	}
	if s.listed && !s.listCovers(r, action) {
		field := s.subject.ToolScope.Field
		out = append(out, candidate{kind: grantNarrowing, list: field, effect: EffectUnavailable, grant: Grant{
			Rule:   s.listRule(),
			List:   field,
			Scope:  "agent",
			Source: at(field),
			Note:   "The agent's " + field + " list does not include " + capability,
		}})
	}
	if s.subject.Readonly && r.kind == kindFiles && (action == capEdit || action == capWrite || action == capDelete) {
		out = append(out, candidate{kind: grantNarrowing, list: "readonly", effect: EffectUnavailable, grant: Grant{
			Rule:   "readonly: true",
			List:   "readonly",
			Scope:  "agent",
			Source: at("readonly"),
			Note:   "readonly: true leaves out edits, writes, and deletes",
		}})
	}
	return out
}

// protectCovers reports whether a protected path covers the file
// pattern. A path covers every file under it.
func protectCovers(path, pattern string) bool {
	return fileCovers(path, pattern) || !strings.HasSuffix(path, allFiles) && fileCovers(path+"/"+allFiles, pattern)
}

func protectNote(p *protectRef) string {
	note := "decision: " + p.decision
	if p.reason != "" {
		note += ". Reason: " + p.reason
	}
	return note
}

// narrowerConditions names the rules that decide part of a file or
// command pattern more narrowly than the operation's own decision, so a
// broad resource never reads as if it decided every file or command.
func (bd *builder) narrowerConditions(r *resourceInfo, action string) []string {
	var names []string
	for _, rule := range bd.rules {
		for _, c := range rule.caps {
			if narrowerThan(c, r, action) {
				names = append(names, rule.list+" "+rule.shown)
				break
			}
		}
	}
	if r.kind == kindFiles && (action == capEdit || action == capWrite) {
		for _, p := range bd.protect {
			if p.path != r.pattern && fileCovers(r.pattern, p.path) && !protectCovers(p.path, r.pattern) {
				names = append(names, "protected "+p.path+" ("+p.decision+")")
			}
		}
	}
	if len(names) == 0 {
		return nil
	}
	const limit = 4
	text := strings.Join(names[:min(limit, len(names))], "; ")
	if len(names) > limit {
		text += "; and " + strconv.Itoa(len(names)-limit) + " more"
	}
	return []string{"Narrower rules decide part of this pattern: " + text + "."}
}

// narrowerThan reports whether c names part of r strictly inside it.
func narrowerThan(c capability, r *resourceInfo, action string) bool {
	switch r.kind {
	case kindFiles:
		kind := action
		if action == capWrite {
			kind = capEdit
		}
		return c.kind == kind && c.path != "" && c.path != r.pattern && fileCovers(r.pattern, c.path) && !fileCovers(c.path, r.pattern)
	case kindShell:
		if c.kind != capShell || c.shell == nil {
			return false
		}
		inside := r.shell == nil || shellCovers(*r.shell, c.shell)
		return inside && !shellCovers(*c.shell, r.shell)
	}
	return false
}
