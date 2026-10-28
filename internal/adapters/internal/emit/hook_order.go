package emit

// OrderByLifecycle returns each event in seen once: those in lifecycle
// first, in lifecycle order, then the rest in the order first seen.
func OrderByLifecycle(seen, lifecycle []string) []string {
	present := make(map[string]bool, len(seen))
	for _, e := range seen {
		present[e] = true
	}
	out := make([]string, 0, len(seen))
	for _, e := range lifecycle {
		if present[e] {
			out = append(out, e)
			delete(present, e)
		}
	}
	for _, e := range seen {
		if present[e] {
			out = append(out, e)
			delete(present, e)
		}
	}
	return out
}
