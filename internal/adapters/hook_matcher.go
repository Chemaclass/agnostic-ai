package adapters

// HookMatcherCoverer is implemented by a target that may write a hook
// matcher in another form than the spec declares, such as a reordered
// alternation or one joined with other specs' matchers.
type HookMatcherCoverer interface {
	// HookMatcherCovers reports whether the native matcher can be what
	// the target wrote for a spec declaring the spec matcher.
	HookMatcherCovers(native, spec string) bool
}

// HookMatcherCovers reports whether target could write the native
// matcher for a spec declaring spec. Without the interface, only an
// identical matcher does.
func HookMatcherCovers(target, native, spec string) bool {
	if coverer, ok := registry[target].(HookMatcherCoverer); ok {
		return coverer.HookMatcherCovers(native, spec)
	}
	return native == spec
}

// HookAccepter is implemented by a target that can tell whether it runs
// a hook spec as written.
type HookAccepter interface {
	// AcceptsHook returns why the target would not run the hook whose
	// spec fields are meta as written, or "" when it would.
	AcceptsHook(meta map[string]any) string
}

// AcceptsHook returns why target would not run the hook as written, or
// "" when it would. A target that cannot tell never vouches for one.
func AcceptsHook(target string, meta map[string]any) string {
	if accepter, ok := registry[target].(HookAccepter); ok {
		return accepter.AcceptsHook(meta)
	}
	return "agnostic-ai cannot tell whether " + target + " runs it"
}

// JudgesHooks reports whether target can tell whether it runs a hook.
func JudgesHooks(target string) bool {
	_, ok := registry[target].(HookAccepter)
	return ok
}
