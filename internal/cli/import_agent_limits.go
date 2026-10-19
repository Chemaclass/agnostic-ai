package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// importPortableAgentKey renames a native agent key, such as Gemini's
// `max_turns` or Cursor's `is_background`, to the portable field when
// its value is valid there and the file does not set that field too.
// Any other file comes back unchanged, and sync still reads the native
// key.
//
// When the spec at dst already sets the field under `x-<target>`, the
// imported value goes back there and the spec's shared value is kept,
// so a value meant for one tool does not reach the others.
func importPortableAgentKey(data []byte, dst, target, native, portable string) ([]byte, error) {
	meta, body := splitMdcFrontmatter(data)
	value, set := meta[native]
	if _, both := meta[portable]; !set || both {
		return data, nil
	}
	existing, err := os.ReadFile(dst)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", dst, err)
	}
	have, _ := splitMdcFrontmatter(existing)
	custom, _ := have["x-"+target].(map[string]any)
	for _, key := range []string{native, portable} {
		if _, scoped := custom[key]; !scoped {
			continue
		}
		merged := maps.Clone(custom)
		if own, ok := meta["x-"+target].(map[string]any); ok {
			maps.Copy(merged, own)
		}
		merged[key] = value
		delete(meta, native)
		meta["x-"+target] = merged
		if shared, ok := have[portable]; ok {
			meta[portable] = shared
		}
		front, err := yaml.Marshal(meta)
		if err != nil {
			return nil, fmt.Errorf("marshal %s agent frontmatter: %w", target, err)
		}
		return []byte("---\n" + string(front) + "---\n\n" + strings.TrimRight(body, "\n") + "\n"), nil
	}
	var raw string
	switch portable {
	case "maxTurns":
		n, ok := spec.AgentMaxTurns(value)
		if !ok {
			return data, nil
		}
		raw = strconv.Itoa(n)
	case "background":
		b, ok := spec.AgentBackground(value)
		if !ok {
			return data, nil
		}
		raw = strconv.FormatBool(b)
	default:
		return data, nil
	}
	out, err := rewriteFrontmatterKeys(string(data), []yamlKeyRewrite{{Key: native, NewKey: portable, Value: raw}})
	if err != nil {
		return data, nil
	}
	return []byte(out), nil
}
