// Package chromeidb reads one record from the IndexedDB data a
// Chromium-based app, such as an Electron desktop app, keeps on disk:
// a <origin>.indexeddb.leveldb folder and its .blob sibling. It reads
// without the app's lock and never writes, so the app can keep running.
//
// The formats it reads are Chromium's and undocumented: LevelDB files
// (leveldb.go), IndexedDB's key layout (this file), Blink's value
// envelope with its Snappy compression and blob files, and V8's value
// serialization (v8.go). What it reads was measured on the Claude
// desktop app on 2026-11-12.
package chromeidb

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// Reason says why a record could not be read.
type Reason string

const (
	// ReasonMissing: the folder, database, object store or key is not
	// there, as when the app was never used on this machine.
	ReasonMissing Reason = "missing"
	// ReasonUnreadable: the files are not in a shape this reader knows.
	ReasonUnreadable Reason = "unreadable"
)

// Error is the one error this package returns.
type Error struct {
	Reason Reason
	Cause  error
}

func (e *Error) Error() string { return fmt.Sprintf("indexeddb %s: %v", e.Reason, e.Cause) }
func (e *Error) Unwrap() error { return e.Cause }

func missing(format string, args ...any) error {
	return &Error{Reason: ReasonMissing, Cause: fmt.Errorf(format, args...)}
}

func unreadable(err error) error {
	return &Error{Reason: ReasonUnreadable, Cause: err}
}

// IndexedDB key prefixes: global metadata has database, object store
// and index id 0; a database's own metadata has object store and index
// id 0; an object store's records have index id 1, and their blob
// lists index id 3.
const (
	indexRecords = 1
	indexBlobs   = 3
)

// Metadata key types.
const (
	databaseNameKey    = 201
	objectStoreMetaKey = 50
	objectStoreName    = 0
)

// IndexedDB key types; only string keys are looked up.
const keyTypeString = 1

// Blink's value envelope. A value that needed processing starts with
// a version tag and a pseudo-version, then says how it was processed.
const (
	blinkVersionTag      = 0xFF
	blinkProcessed       = 0x11
	blinkReplaceWithBlob = 0x01
	blinkSnappy          = 0x02
	blinkTrailerTag      = 0xFE
	// The trailer tag is followed by an 8-byte offset and a 4-byte size.
	blinkTrailerSize = 12
)

// External object types in a record's blob list.
const externalBlob = 0

// Folder is one origin's IndexedDB data, read once.
type Folder struct {
	blobDir string
	values  map[string][]byte
}

// Open reads the LevelDB folder levelDBDir, whose name ends in
// .indexeddb.leveldb.
func Open(levelDBDir string) (*Folder, error) {
	if _, err := os.Stat(levelDBDir); err != nil {
		return nil, missing("%w", err)
	}
	values, err := readLevelDB(levelDBDir)
	if err != nil {
		return nil, unreadable(err)
	}
	blobDir := strings.TrimSuffix(levelDBDir, ".leveldb") + ".blob"
	return &Folder{blobDir: blobDir, values: values}, nil
}

// Get decodes the record stored under the string key in the object
// store of that name, in the database of that name.
func (f *Folder) Get(database, store, key string) (any, error) {
	databaseID, err := f.databaseID(database)
	if err != nil {
		return nil, err
	}
	storeID, err := f.storeID(databaseID, store)
	if err != nil {
		return nil, err
	}
	raw, ok := f.recordValue(databaseID, storeID, indexRecords, key)
	if !ok {
		return nil, missing("no key %q in %s/%s", key, database, store)
	}
	value, err := f.decodeRecord(databaseID, storeID, key, raw)
	if err != nil {
		return nil, unreadable(fmt.Errorf("%s/%s/%s: %w", database, store, key, err))
	}
	return value, nil
}

// keyPrefix is the start of every IndexedDB key: which database,
// object store and index it belongs to.
type keyPrefix struct {
	database, store, index uint64
}

// splitKey reads a key's prefix. Its first byte gives the byte lengths
// of the three ids that follow, little-endian.
func splitKey(key []byte) (keyPrefix, []byte, bool) {
	if len(key) == 0 {
		return keyPrefix{}, nil, false
	}
	databaseLength := int(key[0]>>5) + 1
	storeLength := int(key[0]>>2&7) + 1
	indexLength := int(key[0]&3) + 1
	at := 1
	if at+databaseLength+storeLength+indexLength > len(key) {
		return keyPrefix{}, nil, false
	}
	little := func(n int) uint64 {
		var v uint64
		for i := range n {
			v |= uint64(key[at+i]) << (8 * i)
		}
		at += n
		return v
	}
	prefix := keyPrefix{database: little(databaseLength), store: little(storeLength), index: little(indexLength)}
	return prefix, key[at:], true
}

// databaseID finds a database by name in the global metadata.
func (f *Folder) databaseID(name string) (uint64, error) {
	for key, value := range f.values {
		prefix, rest, ok := splitKey([]byte(key))
		if !ok || prefix != (keyPrefix{}) || len(rest) == 0 || rest[0] != databaseNameKey {
			continue
		}
		r := reader{data: rest, at: 1}
		if _, err := readKeyString(&r); err != nil {
			continue
		}
		databaseName, err := readKeyString(&r)
		if err != nil || databaseName != name {
			continue
		}
		id, err := decodeInt(value)
		if err != nil {
			return 0, unreadable(fmt.Errorf("database %q id: %w", name, err))
		}
		return id, nil
	}
	return 0, missing("no database %q", name)
}

// decodeInt reads an integer as Chromium's EncodeInt writes it: the
// whole value, little-endian, eight bits a byte, in as few bytes as it
// takes. A database id is stored this way, while the ids inside keys
// and metadata use varints.
func decodeInt(raw []byte) (uint64, error) {
	if len(raw) == 0 || len(raw) > 8 {
		return 0, fmt.Errorf("an integer of %d bytes", len(raw))
	}
	var value uint64
	for i, b := range raw {
		value |= uint64(b) << (8 * i)
	}
	return value, nil
}

// storeID finds an object store by name in its database's metadata.
func (f *Folder) storeID(databaseID uint64, name string) (uint64, error) {
	for key, value := range f.values {
		prefix, rest, ok := splitKey([]byte(key))
		if !ok || prefix != (keyPrefix{database: databaseID}) || len(rest) < 3 || rest[0] != objectStoreMetaKey {
			continue
		}
		r := reader{data: rest, at: 1}
		id, err := r.uvarint()
		if err != nil {
			continue
		}
		kind, err := r.byte()
		if err != nil || kind != objectStoreName || !r.done() {
			continue
		}
		storeName, err := utf16BE(value)
		if err == nil && storeName == name {
			return id, nil
		}
	}
	return 0, missing("no object store %q in database %d", name, databaseID)
}

// recordValue finds the value an index of a store holds for a string key.
func (f *Folder) recordValue(databaseID, storeID, index uint64, key string) ([]byte, bool) {
	want := keyPrefix{database: databaseID, store: storeID, index: index}
	for raw, value := range f.values {
		prefix, rest, ok := splitKey([]byte(raw))
		if !ok || prefix != want || len(rest) == 0 || rest[0] != keyTypeString {
			continue
		}
		r := reader{data: rest, at: 1}
		recordKey, err := readKeyString(&r)
		if err == nil && r.done() && recordKey == key {
			return value, true
		}
	}
	return nil, false
}

// decodeRecord unwraps a record's value: a version, then Blink's
// envelope, which may move the value to a blob file or compress it,
// then V8's serialization.
func (f *Folder) decodeRecord(databaseID, storeID uint64, key string, raw []byte) (any, error) {
	r := reader{data: raw}
	if _, err := r.uvarint(); err != nil {
		return nil, fmt.Errorf("record version: %w", err)
	}
	value := raw[r.at:]
	// Blink compresses a value at most once and then moves it to a blob
	// at most once, so each envelope may be unwrapped once. A blob that
	// names a blob again, itself included, is damage, not a value.
	movedToBlob, compressed := false, false
	for {
		switch {
		case bytes.HasPrefix(value, []byte{blinkVersionTag, blinkProcessed, blinkReplaceWithBlob}):
			if movedToBlob || compressed {
				return nil, errors.New("a blob reference inside a blob or a compressed value")
			}
			movedToBlob = true
			blob, err := f.blob(databaseID, storeID, key, value[3:])
			if err != nil {
				return nil, err
			}
			value = blob
		case bytes.HasPrefix(value, []byte{blinkVersionTag, blinkProcessed, blinkSnappy}):
			if compressed {
				return nil, errors.New("a compressed value inside a compressed value")
			}
			compressed = true
			expanded, err := decodeSnappy(value[3:])
			if err != nil {
				return nil, err
			}
			value = expanded
		default:
			return decodeBlinkValue(value)
		}
	}
}

// decodeBlinkValue reads Blink's version header and optional trailer
// offset, then the V8 value.
func decodeBlinkValue(value []byte) (any, error) {
	r := reader{data: value}
	tag, err := r.byte()
	if err != nil {
		return nil, err
	}
	if tag != blinkVersionTag {
		return nil, fmt.Errorf("value starts with %#x, not a version tag", tag)
	}
	if _, err := r.uvarint(); err != nil {
		return nil, err
	}
	if next, err := r.peek(); err == nil && next == blinkTrailerTag {
		if _, err := r.bytes(1 + blinkTrailerSize); err != nil {
			return nil, err
		}
	}
	return decodeV8(value[r.at:])
}

// blob reads the blob file a record's value was moved to. reference
// is the envelope after its tag: the blob's size and its position in
// the record's blob list.
func (f *Folder) blob(databaseID, storeID uint64, key string, reference []byte) ([]byte, error) {
	r := reader{data: reference}
	size, err := r.uvarint()
	if err != nil {
		return nil, fmt.Errorf("blob size: %w", err)
	}
	position, err := r.uvarint()
	if err != nil {
		return nil, fmt.Errorf("blob position: %w", err)
	}
	list, ok := f.recordValue(databaseID, storeID, indexBlobs, key)
	if !ok {
		return nil, errors.New("the value is in a blob, and the record lists none")
	}
	if size > maxDecodedBytes {
		return nil, fmt.Errorf("blob of %d bytes is more than %d", size, maxDecodedBytes)
	}
	number, err := blobNumber(list, position)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(f.blobDir, fmt.Sprintf("%x", databaseID), fmt.Sprintf("%02x", number>>8&0xFF), fmt.Sprintf("%x", number))
	// The size is checked before the read too, so a damaged or swapped
	// file is not read whole.
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if uint64(info.Size()) != size {
		return nil, fmt.Errorf("blob %x holds %d bytes, the record says %d", number, info.Size(), size)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if uint64(len(data)) != size {
		return nil, fmt.Errorf("blob %x holds %d bytes, the record says %d", number, len(data), size)
	}
	return data, nil
}

// blobNumber reads a record's blob list up to the entry at position.
// Each entry is a type, a blob number, a content type and a size; only
// plain blobs are expected.
func blobNumber(list []byte, position uint64) (uint64, error) {
	r := reader{data: list}
	for i := uint64(0); ; i++ {
		kind, err := r.byte()
		if err != nil {
			return 0, fmt.Errorf("blob list ends before entry %d", position)
		}
		if kind != externalBlob {
			return 0, fmt.Errorf("blob list entry %d is of type %d", i, kind)
		}
		number, err := r.uvarint()
		if err != nil {
			return 0, err
		}
		if _, err := readKeyString(&r); err != nil {
			return 0, err
		}
		if _, err := r.uvarint(); err != nil {
			return 0, err
		}
		if i == position {
			return number, nil
		}
	}
}

// readKeyString reads a string as IndexedDB keys store it: a count of
// UTF-16 code units, then the units big-endian.
func readKeyString(r *reader) (string, error) {
	units, err := r.uvarint()
	if err != nil {
		return "", err
	}
	if units > uint64(len(r.data)) {
		return "", fmt.Errorf("string of %d units in %d bytes", units, len(r.data))
	}
	raw, err := r.bytes(2 * units)
	if err != nil {
		return "", err
	}
	return utf16BE(raw)
}

func utf16BE(raw []byte) (string, error) {
	if len(raw)%2 != 0 {
		return "", fmt.Errorf("UTF-16 string of %d bytes", len(raw))
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = uint16(raw[2*i])<<8 | uint16(raw[2*i+1])
	}
	return string(utf16.Decode(units)), nil
}
