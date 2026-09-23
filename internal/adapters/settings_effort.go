package adapters

import (
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// SettingsEffortAccepter is implemented by a target whose settings file
// has a repository effort key.
type SettingsEffortAccepter interface {
	// SettingsEffortLevels lists the values that key accepts. Nil
	// accepts any string.
	SettingsEffortLevels() []string
}

// AcceptsSettingsEffort reports whether value fits target's repository
// effort key. A target without that key accepts nothing.
func AcceptsSettingsEffort(target string, value any) bool {
	a, err := Resolve(target)
	if err != nil {
		return false
	}
	accepter, ok := a.(SettingsEffortAccepter)
	if !ok {
		return false
	}
	level, ok := value.(string)
	if !ok || level == "" {
		return false
	}
	levels := accepter.SettingsEffortLevels()
	return levels == nil || slices.Contains(levels, level)
}

// SettingsEffort mirrors emit.SettingsEffort for the import side: the
// repository effort sync would write for target from entries.
func SettingsEffort(entries []spec.Entry, target string) any {
	return emit.SettingsEffort(entries, target)
}
