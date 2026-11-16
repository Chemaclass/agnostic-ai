// Package chromeidbtest writes IndexedDB folders the way Chromium
// does, in their simplest valid form, for tests of code that reads
// them through chromeidb. Nothing here reads a real app's data.
package chromeidbtest

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"unicode/utf16"
)

// WriteRecord writes a LevelDB folder at dir that holds one database
// with one object store, and value under key in it. value is built
// from map[string]any, []any, string, float64, int, bool and nil.
func WriteRecord(t *testing.T, dir, database, store, key string, value any) {
	t.Helper()
	encoded, err := v8Value(value)
	if err != nil {
		t.Fatal(err)
	}
	// IndexedDB's record version, Blink's version and empty trailer
	// offset, then V8's version and the value.
	blink := append([]byte{0xFF, 0x15, 0xFE}, make([]byte, 12)...)
	blink = append(blink, 0xFF, 0x10)
	record := append(binary.AppendUvarint(nil, 1), append(blink, encoded...)...)
	// The database name maps to id 1, as Chromium's EncodeInt writes it.
	writes := [][2][]byte{
		{join([]byte{0, 0, 0, 0, 201}, keyString("https_claude.ai_0@1"), keyString(database)), []byte{1}},
		{[]byte{0, 1, 0, 0, 50, 1, 0}, be16(store)},
		{join([]byte{0, 1, 1, 1, 1}, keyString(key)), record},
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "000003.log"), logRecord(batch(writes)), 0o600); err != nil {
		t.Fatal(err)
	}
}

func join(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func be16(s string) []byte {
	var out []byte
	for _, unit := range utf16.Encode([]rune(s)) {
		out = binary.BigEndian.AppendUint16(out, unit)
	}
	return out
}

func keyString(s string) []byte {
	return join(binary.AppendUvarint(nil, uint64(len(utf16.Encode([]rune(s))))), be16(s))
}

func batch(writes [][2][]byte) []byte {
	out := binary.LittleEndian.AppendUint64(nil, 1)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(writes)))
	for _, w := range writes {
		out = append(out, 1)
		out = binary.AppendUvarint(out, uint64(len(w[0])))
		out = append(out, w[0]...)
		out = binary.AppendUvarint(out, uint64(len(w[1])))
		out = append(out, w[1]...)
	}
	return out
}

// logRecord frames one record into 32 KB log blocks.
func logRecord(record []byte) []byte {
	const blockSize, headerSize = 32 << 10, 7
	var out []byte
	first := true
	for {
		left := blockSize - len(out)%blockSize
		if left < headerSize {
			out = append(out, make([]byte, left)...)
			continue
		}
		fragment := record[:min(left-headerSize, len(record))]
		record = record[len(fragment):]
		last := len(record) == 0
		kind := byte(3)
		switch {
		case first && last:
			kind = 1
		case first:
			kind = 2
		case last:
			kind = 4
		}
		out = append(out, 0, 0, 0, 0)
		out = binary.LittleEndian.AppendUint16(out, uint16(len(fragment)))
		out = append(out, kind)
		out = append(out, fragment...)
		first = false
		if last {
			return out
		}
	}
}

func v8Value(value any) ([]byte, error) {
	switch v := value.(type) {
	case nil:
		return []byte{'0'}, nil
	case bool:
		if v {
			return []byte{'T'}, nil
		}
		return []byte{'F'}, nil
	case int:
		zigzag := uint64(v<<1) ^ uint64(v>>63)
		return binary.AppendUvarint([]byte{'I'}, zigzag), nil
	case float64:
		return binary.LittleEndian.AppendUint64([]byte{'N'}, math.Float64bits(v)), nil
	case string:
		units := utf16.Encode([]rune(v))
		out := binary.AppendUvarint([]byte{'c'}, uint64(2*len(units)))
		for _, unit := range units {
			out = binary.LittleEndian.AppendUint16(out, unit)
		}
		return out, nil
	case []any:
		out := binary.AppendUvarint([]byte{'A'}, uint64(len(v)))
		for _, item := range v {
			encoded, err := v8Value(item)
			if err != nil {
				return nil, err
			}
			out = append(out, encoded...)
		}
		out = append(out, '$')
		out = binary.AppendUvarint(out, 0)
		return binary.AppendUvarint(out, uint64(len(v))), nil
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := []byte{'o'}
		for _, k := range keys {
			name, _ := v8Value(k)
			item, err := v8Value(v[k])
			if err != nil {
				return nil, err
			}
			out = append(append(out, name...), item...)
		}
		out = append(out, '{')
		return binary.AppendUvarint(out, uint64(len(keys))), nil
	default:
		return nil, fmt.Errorf("chromeidbtest: cannot encode %T", value)
	}
}
