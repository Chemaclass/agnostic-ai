package cli

import (
	"strconv"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// importPortableAgentKey renames a native agent key, such as Gemini's
// `max_turns` or Cursor's `is_background`, to the portable field when
// its value is valid there and the file does not set that field too.
// Any other file comes back unchanged, and sync still reads the native
// key.
func importPortableAgentKey(data []byte, native, portable string) []byte {
	meta, _ := splitMdcFrontmatter(data)
	value, set := meta[native]
	if _, both := meta[portable]; !set || both {
		return data
	}
	var raw string
	switch portable {
	case "maxTurns":
		n, ok := spec.AgentMaxTurns(value)
		if !ok {
			return data
		}
		raw = strconv.Itoa(n)
	case "background":
		b, ok := value.(bool)
		if !ok {
			return data
		}
		raw = strconv.FormatBool(b)
	default:
		return data
	}
	out, err := rewriteFrontmatterKeys(string(data), []yamlKeyRewrite{{Key: native, NewKey: portable, Value: raw}})
	if err != nil {
		return data
	}
	return []byte(out)
}
