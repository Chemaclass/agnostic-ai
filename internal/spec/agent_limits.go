package spec

import "fmt"

// AgentMaxTurns returns an agent's `maxTurns` when it is a positive
// integer.
func AgentMaxTurns(value any) (int, bool) {
	switch n := value.(type) {
	case int:
		return n, n > 0
	case int64:
		return int(n), n > 0
	case uint64:
		return int(n), n > 0
	}
	return 0, false
}

// AgentLimitProblems returns why the portable agent fields `maxTurns`
// and `background` cannot be written, by field. Values under
// `x-<target>` are the tool's own and are not checked.
func AgentLimitProblems(meta map[string]any) map[string]string {
	out := map[string]string{}
	if v, set := meta["maxTurns"]; set {
		if _, ok := AgentMaxTurns(v); !ok {
			out["maxTurns"] = fmt.Sprintf("maxTurns is %v; write a whole number of 1 or more", v)
		}
	}
	if v, set := meta["background"]; set {
		if _, ok := v.(bool); !ok {
			out["background"] = fmt.Sprintf("background is %v; write true or false", v)
		}
	}
	return out
}
