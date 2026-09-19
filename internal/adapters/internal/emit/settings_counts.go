package emit

import "github.com/chemaclass/agnostic-ai/internal/spec"

// SpecsWithPermissions counts the settings specs carrying at least one
// non-empty permission list, so a caller can fold them into one
// coverage note rather than one note per rule.
//
// A spec counts once however many lists it fills: the note tells the
// user that their policy did not reach this target, and repeating that
// three times for one file helps nobody.
func SpecsWithPermissions(settings []spec.Entry) int {
	n := 0
	for _, entry := range settings {
		permissions, _ := entry.Meta["permissions"].(map[string]any)
		for _, list := range []string{"allow", "deny", "ask"} {
			if len(StringSlice(permissions[list])) > 0 {
				n++
				break
			}
		}
	}
	return n
}
