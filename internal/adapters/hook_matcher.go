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
