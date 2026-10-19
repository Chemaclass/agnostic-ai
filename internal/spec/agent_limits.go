package spec

import (
	"fmt"
	"math"
)

// AgentMaxTurns returns an agent's `maxTurns` when it is a positive
// integer that fits an int.
func AgentMaxTurns(value any) (int, bool) {
	switch n := value.(type) {
	case int:
		return n, n > 0
	case int64:
		if n > 0 && n <= math.MaxInt {
			return int(n), true
		}
	case uint64:
		if n > 0 && n <= math.MaxInt {
			return int(n), true
		}
	}
	return 0, false
}

// AgentBackground returns an agent's `background` when it is a boolean.
func AgentBackground(value any) (bool, bool) {
	b, ok := value.(bool)
	return b, ok
}

// AgentLimitProblems returns why the portable agent fields `maxTurns`
// and `background` cannot be written, by field. Values under
// `x-<target>` are the tool's own and are not checked.
func AgentLimitProblems(meta map[string]any) map[string]string {
	out := map[string]string{}
	if v, set := meta["maxTurns"]; set {
		if _, ok := AgentMaxTurns(v); !ok {
			out["maxTurns"] = fmt.Sprintf("maxTurns is %s; write a whole number of 1 or more", describeValue(v))
			if tooLarge(v) {
				out["maxTurns"] = fmt.Sprintf("maxTurns is %v, which is too large; write a whole number of 1 or more", v)
			}
		}
	}
	if v, set := meta["background"]; set {
		if _, ok := AgentBackground(v); !ok {
			out["background"] = fmt.Sprintf("background is %s; write true or false", describeValue(v))
		}
	}
	return out
}

// tooLarge reports an integer above the largest int.
func tooLarge(v any) bool {
	switch n := v.(type) {
	case int64:
		return n > math.MaxInt
	case uint64:
		return n > math.MaxInt
	}
	return false
}

// describeValue names a frontmatter value with its type when the plain
// form would read as a valid value, such as the text "true".
func describeValue(v any) string {
	switch x := v.(type) {
	case string:
		return fmt.Sprintf("the text %q", x)
	case float64:
		return fmt.Sprintf("the decimal number %v", x)
	case nil:
		return "empty"
	}
	return fmt.Sprint(v)
}
