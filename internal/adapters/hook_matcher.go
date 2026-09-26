package adapters

// HookMatcherFolder is implemented by a target that may write a hook
// matcher in another spelling than the spec declares, such as a
// reordered alternation.
type HookMatcherFolder interface {
	// HookMatcherKey returns the form two matchers share when the
	// target treats them as one.
	HookMatcherKey(matcher string) string
}

// HookMatcherKey returns the form target folds matcher to, or matcher
// itself when target writes it verbatim. Import compares a native
// matcher with a spec's through it.
func HookMatcherKey(target, matcher string) string {
	if folder, ok := registry[target].(HookMatcherFolder); ok {
		return folder.HookMatcherKey(matcher)
	}
	return matcher
}
