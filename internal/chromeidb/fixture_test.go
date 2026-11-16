package chromeidb

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

// This file writes the formats the package reads, in their simplest
// valid form, so tests build folders without any real app's data.

func uvarint(n uint64) []byte { return binary.AppendUvarint(nil, n) }

func join(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// literalSnappy encodes data as one Snappy stream of literals.
func literalSnappy(data []byte) []byte {
	out := uvarint(uint64(len(data)))
	for len(data) > 0 {
		chunk := min(len(data), 60)
		out = append(out, byte(chunk-1)<<2)
		out = append(out, data[:chunk]...)
		data = data[chunk:]
	}
	return out
}

type write struct {
	key, value []byte
	deleted    bool
}

// batch encodes writes as one write batch starting at sequence.
func batch(sequence uint64, writes ...write) []byte {
	out := binary.LittleEndian.AppendUint64(nil, sequence)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(writes)))
	for _, w := range writes {
		if w.deleted {
			out = append(out, kindDeletion)
			out = append(out, uvarint(uint64(len(w.key)))...)
			out = append(out, w.key...)
			continue
		}
		out = append(out, kindValue)
		out = append(out, uvarint(uint64(len(w.key)))...)
		out = append(out, w.key...)
		out = append(out, uvarint(uint64(len(w.value)))...)
		out = append(out, w.value...)
	}
	return out
}

// logFile frames records into 32 KB blocks, splitting the ones that
// cross a block boundary. Checksums are written as zero: the reader
// does not check them.
func logFile(records ...[]byte) []byte {
	var out []byte
	for _, record := range records {
		first := true
		for {
			left := logBlockSize - len(out)%logBlockSize
			if left < logHeaderSize {
				out = append(out, make([]byte, left)...)
				continue
			}
			room := left - logHeaderSize
			fragment := record[:min(room, len(record))]
			record = record[len(fragment):]
			last := len(record) == 0
			kind := byte(logMiddle)
			switch {
			case first && last:
				kind = logFull
			case first:
				kind = logFirst
			case last:
				kind = logLast
			}
			out = append(out, 0, 0, 0, 0)
			out = binary.LittleEndian.AppendUint16(out, uint16(len(fragment)))
			out = append(out, kind)
			out = append(out, fragment...)
			first = false
			if last {
				break
			}
		}
	}
	return out
}

type tableEntry struct {
	key      []byte
	sequence uint64
	value    []byte
	deleted  bool
}

// block encodes entries with shared key prefixes and one restart.
func block(entries [][2][]byte) []byte {
	var out, previous []byte
	for _, e := range entries {
		shared := 0
		for shared < len(previous) && shared < len(e[0]) && previous[shared] == e[0][shared] {
			shared++
		}
		out = append(out, uvarint(uint64(shared))...)
		out = append(out, uvarint(uint64(len(e[0])-shared))...)
		out = append(out, uvarint(uint64(len(e[1])))...)
		out = append(out, e[0][shared:]...)
		out = append(out, e[1]...)
		previous = e[0]
	}
	out = binary.LittleEndian.AppendUint32(out, 0)
	return binary.LittleEndian.AppendUint32(out, 1)
}

// tableFile writes one data block, compressed when asked, an empty
// metaindex block, an index block and the footer.
func tableFile(compress bool, entries ...tableEntry) []byte {
	var rows [][2][]byte
	for _, e := range entries {
		kind := uint64(kindValue)
		if e.deleted {
			kind = kindDeletion
		}
		internal := binary.LittleEndian.AppendUint64(append([]byte(nil), e.key...), e.sequence<<8|kind)
		rows = append(rows, [2][]byte{internal, e.value})
	}
	var out []byte
	appendBlock := func(b []byte, snappy bool) (offset, size uint64) {
		offset = uint64(len(out))
		kind := byte(blockPlain)
		if snappy {
			b = literalSnappy(b)
			kind = blockSnappy
		}
		out = append(out, b...)
		out = append(out, kind, 0, 0, 0, 0)
		return offset, uint64(len(b))
	}
	dataOffset, dataSize := appendBlock(block(rows), compress)
	metaOffset, metaSize := appendBlock(block(nil), false)
	handle := join(uvarint(dataOffset), uvarint(dataSize))
	indexOffset, indexSize := appendBlock(block([][2][]byte{{[]byte("\xff"), handle}}), false)
	footer := join(uvarint(metaOffset), uvarint(metaSize), uvarint(indexOffset), uvarint(indexSize))
	footer = append(footer, make([]byte, tableFooterSize-8-len(footer))...)
	footer = binary.LittleEndian.AppendUint64(footer, tableMagic)
	return append(out, footer...)
}

// prefix encodes an IndexedDB key prefix: a byte of id lengths, then
// each id little-endian in as few bytes as it takes.
func prefix(database, store, index uint64) []byte {
	db, st, ix := idbInt(database), idbInt(store), idbInt(index)
	lengths := byte(len(db)-1)<<5 | byte(len(st)-1)<<2 | byte(len(ix)-1)
	return join([]byte{lengths}, db, st, ix)
}

// idbInt encodes n as Chromium's EncodeInt does: little-endian, in as
// few bytes as it takes, at least one.
func idbInt(n uint64) []byte {
	out := []byte{byte(n)}
	for n >>= 8; n > 0; n >>= 8 {
		out = append(out, byte(n))
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
	return join(uvarint(uint64(len(utf16.Encode([]rune(s))))), be16(s))
}

func namedDatabaseKey(name string) []byte {
	return join(prefix(0, 0, 0), []byte{databaseNameKey}, keyString("https_app.example_0@1"), keyString(name))
}

func storeNameKey(database, store uint64) []byte {
	return join(prefix(database, 0, 0), []byte{objectStoreMetaKey}, uvarint(store), []byte{objectStoreName})
}

func recordKey(database, store, index uint64, key string) []byte {
	return join(prefix(database, store, index), []byte{keyTypeString}, keyString(key))
}

// v8String encodes a one-byte string.
func v8String(s string) []byte {
	return join([]byte{v8OneByteString}, uvarint(uint64(len(s))), []byte(s))
}

func v8Number(f float64) []byte {
	return binary.LittleEndian.AppendUint64([]byte{v8Double}, math.Float64bits(f))
}

// v8Object encodes alternating keys and values, already encoded.
func v8Object(fields ...[]byte) []byte {
	out := []byte{v8BeginObject}
	for _, f := range fields {
		out = append(out, f...)
	}
	out = append(out, v8EndObject)
	return append(out, uvarint(uint64(len(fields)/2))...)
}

func v8Dense(items ...[]byte) []byte {
	out := join([]byte{v8BeginDense}, uvarint(uint64(len(items))))
	for _, item := range items {
		out = append(out, item...)
	}
	return join(out, []byte{v8EndDense}, uvarint(0), uvarint(uint64(len(items))))
}

// blinkValue wraps V8 bytes as Blink writes a small value: its version,
// an empty trailer, then V8's own version.
func blinkValue(v8 []byte) []byte {
	return join([]byte{blinkVersionTag, 0x15, blinkTrailerTag}, make([]byte, blinkTrailerSize), []byte{v8Version, 0x10}, v8)
}

// record is a record's stored value: IndexedDB's version, then Blink's.
func record(blink []byte) []byte { return join(uvarint(1), blink) }

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
