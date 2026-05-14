package emit

import (
	"bytes"
	"encoding/json"
)

// MarshalJSONIndent renders v as indented JSON without HTML escaping.
//
// Go's default json.MarshalIndent escapes `&`, `<`, `>` as `&`,
// `<`, `>` for browser safety. That mangles shell commands
// in settings files (e.g. `cmd1 && cmd2` becomes `cmd1 && cmd2`),
// which the user must then read in escaped form. Adapters write files
// consumed by CLIs, not browsers, so HTML escaping is off by default.
func MarshalJSONIndent(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	out := buf.Bytes()
	if n := len(out); n > 0 && out[n-1] == '\n' {
		out = out[:n-1]
	}
	return out, nil
}
