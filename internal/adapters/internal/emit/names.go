package emit

import (
	"fmt"
	"regexp"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// NameRule is a vendor's documented identifier constraint for an emitted
// spec name. Rule states the constraint in prose and reads as the tail of
// the error message, after "name must ".
type NameRule struct {
	Pattern *regexp.Regexp
	MaxLen  int
	Rule    string
}

// ValidateNames rejects an entry whose name violates the target's
// documented identifier rule, before anything is written. Emitting the
// file instead would produce config the vendor's own rule says is
// invalid: the tool skips it silently, so a sync that looked clean
// leaves the user with a skill or agent that never loads. The error
// names the target, the kind, the offending spec, and the rule, so it
// can be acted on without opening vendor docs.
func ValidateNames(entries []spec.Entry, target, kind string, rule NameRule) error {
	for _, e := range entries {
		if rule.MaxLen > 0 && len(e.Name) > rule.MaxLen {
			return nameRuleError(e, target, kind, rule)
		}
		if rule.Pattern != nil && !rule.Pattern.MatchString(e.Name) {
			return nameRuleError(e, target, kind, rule)
		}
	}
	return nil
}

func nameRuleError(e spec.Entry, target, kind string, rule NameRule) error {
	return fmt.Errorf("%s %s %q: name must %s", target, kind, e.Name, rule.Rule)
}
