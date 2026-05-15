package emit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// OrderedJSON is a JSON object that preserves insertion order on
// round-trip. Adapters use it for files the user hand-edits
// (`.claude/settings.json`, opencode's `mcp` block, etc.) so the
// author's key sequence survives a `sync` instead of being
// alpha-sorted by `encoding/json`'s default map iteration.
//
// Nested objects parsed from disk land in `vals` as `json.RawMessage`
// so their inner key order is preserved byte-for-byte. New values
// added via Set are re-marshaled and so follow Go map iteration
// order (alpha-sorted by `encoding/json`), but the top-level key the
// user authored stays where they put it.
type OrderedJSON struct {
	keys []string
	vals map[string]json.RawMessage
}

// NewOrderedJSON returns an empty OrderedJSON ready for Set / Get.
func NewOrderedJSON() *OrderedJSON {
	return &OrderedJSON{vals: map[string]json.RawMessage{}}
}

// Len reports the number of keys.
func (o *OrderedJSON) Len() int {
	if o == nil {
		return 0
	}
	return len(o.keys)
}

// Keys returns the keys in insertion order. Mutating the returned
// slice does not affect the OrderedJSON.
func (o *OrderedJSON) Keys() []string {
	if o == nil {
		return nil
	}
	out := make([]string, len(o.keys))
	copy(out, o.keys)
	return out
}

// Get returns the raw JSON for key and whether it was present.
func (o *OrderedJSON) Get(key string) (json.RawMessage, bool) {
	if o == nil {
		return nil, false
	}
	v, ok := o.vals[key]
	return v, ok
}

// Set assigns val for key, marshaling val to JSON. New keys append
// at the end; existing keys retain their position.
func (o *OrderedJSON) Set(key string, val any) error {
	raw, err := MarshalJSONIndent(val)
	if err != nil {
		return err
	}
	o.setRaw(key, raw)
	return nil
}

// SetRaw assigns a pre-marshaled json.RawMessage for key. Useful when
// the caller wants to carry over a value byte-for-byte from another
// OrderedJSON.
func (o *OrderedJSON) SetRaw(key string, raw json.RawMessage) {
	o.setRaw(key, append(json.RawMessage(nil), raw...))
}

func (o *OrderedJSON) setRaw(key string, raw json.RawMessage) {
	if o.vals == nil {
		o.vals = map[string]json.RawMessage{}
	}
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = raw
}

// Delete removes key. No-op when key is absent.
func (o *OrderedJSON) Delete(key string) {
	if o == nil {
		return
	}
	if _, ok := o.vals[key]; !ok {
		return
	}
	delete(o.vals, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// UnmarshalJSON parses data as a JSON object and records keys in
// source order. Values are kept as RawMessage so nested key order
// inside untouched subtrees survives byte-for-byte.
func (o *OrderedJSON) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok == nil {
		o.keys = nil
		o.vals = map[string]json.RawMessage{}
		return nil
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("OrderedJSON: expected object, got %v", tok)
	}
	o.keys = o.keys[:0]
	o.vals = map[string]json.RawMessage{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return errors.New("OrderedJSON: non-string key")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return err
		}
		if _, dup := o.vals[key]; !dup {
			o.keys = append(o.keys, key)
		}
		o.vals[key] = raw
	}
	if _, err := dec.Token(); err != nil && err != io.EOF {
		return err
	}
	return nil
}

// MarshalJSON renders the object in insertion order with two-space
// indent, matching MarshalJSONIndent's style. Empty objects render
// as `{}`.
func (o OrderedJSON) MarshalJSON() ([]byte, error) {
	if len(o.keys) == 0 {
		return []byte("{}"), nil
	}
	var buf bytes.Buffer
	buf.WriteString("{\n")
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteString(",\n")
		}
		keyBytes, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.WriteString("  ")
		buf.Write(keyBytes)
		buf.WriteString(": ")
		indentRawJSON(&buf, o.vals[k], "  ")
	}
	buf.WriteString("\n}")
	return buf.Bytes(), nil
}

// indentRawJSON rewrites compact RawMessage bytes (which `encoding/json`
// produces when reading a value via Decode) into the same two-space
// indented form MarshalJSONIndent emits, prepending `prefix` to every
// line after the first so the nested value lines up with its parent.
func indentRawJSON(buf *bytes.Buffer, raw json.RawMessage, prefix string) {
	var tmp bytes.Buffer
	if err := json.Indent(&tmp, raw, prefix, "  "); err != nil {
		// Fall back to the original bytes; never panic on an emit path.
		buf.Write(raw)
		return
	}
	buf.Write(tmp.Bytes())
}
