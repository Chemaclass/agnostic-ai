package spec

// Bundle is a pre-bucketed view of loaded entries. Adapters consume this
// directly to avoid repeated Filter calls.
type Bundle struct {
	Agents       []Entry
	Skills       []Entry
	Rules        []Entry
	Hooks        []Entry
	MCPs         []Entry
	Commands     []Entry
	Settings     []Entry
	Reviews      []Entry
	Environments []Entry
	Ignores      []Entry
	// Shadowed holds entries dropped because a peer in the same layer
	// declared the same name. They reach no target; `lint` reports them
	// (#582). Cross-layer overrides are intentional and never listed here.
	Shadowed []Entry
}

// NewBundle groups a flat slice of entries by kind. Useful in tests and
// when adapting external sources of Entry slices.
func NewBundle(entries []Entry) Bundle {
	var b Bundle
	for _, e := range entries {
		switch e.Kind {
		case KindAgent:
			b.Agents = append(b.Agents, e)
		case KindSkill:
			b.Skills = append(b.Skills, e)
		case KindRule:
			b.Rules = append(b.Rules, e)
		case KindHook:
			b.Hooks = append(b.Hooks, e)
		case KindMCP:
			b.MCPs = append(b.MCPs, e)
		case KindCommand:
			b.Commands = append(b.Commands, e)
		case KindSettings:
			b.Settings = append(b.Settings, e)
		case KindReview:
			b.Reviews = append(b.Reviews, e)
		case KindEnvironment:
			b.Environments = append(b.Environments, e)
		case KindIgnore:
			b.Ignores = append(b.Ignores, e)
		}
	}
	return b
}

// All returns every entry in canonical kind order.
func (b Bundle) All() []Entry {
	out := make([]Entry, 0, len(b.Agents)+len(b.Skills)+len(b.Rules)+len(b.Hooks)+len(b.MCPs)+len(b.Commands)+len(b.Settings)+len(b.Reviews)+len(b.Environments)+len(b.Ignores))
	out = append(out, b.Agents...)
	out = append(out, b.Skills...)
	out = append(out, b.Rules...)
	out = append(out, b.Hooks...)
	out = append(out, b.MCPs...)
	out = append(out, b.Commands...)
	out = append(out, b.Settings...)
	out = append(out, b.Reviews...)
	out = append(out, b.Environments...)
	out = append(out, b.Ignores...)
	return out
}

// HooksFor returns the subset of b.Hooks that should emit for target,
// per Entry.EmitsTo. Use this in adapter Emit() loops to honor hook
// `target:` / `targets:` scoping. Empty target returns b.Hooks verbatim.
func (b Bundle) HooksFor(target string) []Entry {
	if target == "" {
		return b.Hooks
	}
	return filterEntriesFor(b.Hooks, target)
}

// For returns a copy of b with every kind filtered by Entry.EmitsTo
// and each surviving entry's body materialized for target via
// Entry.BodyFor. Adapters that loop over `bundle.Agents` / `Skills` /
// `Rules` / `MCPs` / `Commands` see only entries scoped to their
// target via `target:` / `targets:` / `target-exclude:` /
// `targets-exclude:` frontmatter, AND see bodies with `::target` fences
// already resolved for the active target. Empty target string
// short-circuits to identity. Closes #292 + #293.
func (b Bundle) For(target string) Bundle {
	if target == "" {
		return b
	}
	return Bundle{
		Agents:       filterEntriesFor(b.Agents, target),
		Skills:       filterEntriesFor(b.Skills, target),
		Rules:        filterEntriesFor(b.Rules, target),
		Hooks:        filterEntriesFor(b.Hooks, target),
		MCPs:         filterEntriesFor(b.MCPs, target),
		Commands:     filterEntriesFor(b.Commands, target),
		Settings:     filterEntriesFor(b.Settings, target),
		Reviews:      filterEntriesFor(b.Reviews, target),
		Environments: filterEntriesFor(b.Environments, target),
		Ignores:      filterEntriesFor(b.Ignores, target),
	}
}

// filterEntriesFor returns the subset of entries whose EmitsTo(target)
// is true, with each survivor's Body materialized for target via
// BodyFor (a no-op when the body carries no `::target` fences). A
// portable hook arrives in target's native form, or not at all when
// target has no exact mapping for it. An agent's `can:` arrives as the
// `tools:` it stands for; an agent whose `can:` cannot be read is left
// out, so a typo never syncs an agent with every tool. A settings spec's
// permission capabilities arrive as the Claude Code rules they stand for.
func filterEntriesFor(entries []Entry, target string) []Entry {
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if !e.EmitsTo(target) {
			continue
		}
		native, reason := e.NativeHook(target)
		if reason != "" {
			continue
		}
		if native, reason = native.NativeToolsFor(target); reason != "" {
			continue
		}
		if native, reason = native.NativeAllowedToolsFor(target); reason != "" {
			continue
		}
		e = native.NativePermissionsFor(target)
		if resolved := e.BodyFor(target); resolved != e.Body {
			e.Body = resolved
			e.BodyLine = 0
		}
		out = append(out, e)
	}
	return out
}

// Has reports whether the bundle contains any entry of the given kind.
func (b Bundle) Has(k Kind) bool {
	switch k {
	case KindAgent:
		return len(b.Agents) > 0
	case KindSkill:
		return len(b.Skills) > 0
	case KindRule:
		return len(b.Rules) > 0
	case KindHook:
		return len(b.Hooks) > 0
	case KindMCP:
		return len(b.MCPs) > 0
	case KindCommand:
		return len(b.Commands) > 0
	case KindSettings:
		return len(b.Settings) > 0
	case KindReview:
		return len(b.Reviews) > 0
	case KindEnvironment:
		return len(b.Environments) > 0
	case KindIgnore:
		return len(b.Ignores) > 0
	}
	return false
}
