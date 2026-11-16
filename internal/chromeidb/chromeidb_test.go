package chromeidb

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"
)

func TestDecodeSnappyCopiesOverlappingRuns(t *testing.T) {
	// "ab", then a 1-byte-offset copy of 6 that repeats it: "abababab".
	stream := join(uvarint(8), []byte{1 << 2, 'a', 'b'}, []byte{(6-4)<<2 | snappyCopy1, 2})
	got, err := decodeSnappy(stream)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abababab" {
		t.Fatalf("got %q", got)
	}
}

func TestDecodeSnappyRejectsDamagedStreams(t *testing.T) {
	for name, tc := range map[string]struct {
		stream []byte
		want   string
	}{
		"copy before any byte": {join(uvarint(4), []byte{0<<2 | snappyCopy1, 1}), "snappy: copy from 1 bytes back, with 0 written"},
		"wrong length":         {join(uvarint(5), []byte{1 << 2, 'a', 'b'}), "snappy: decoded 2 bytes, expected 5"},
		"huge length":          {uvarint(1 << 40), "snappy: 1099511627776 bytes is more than 67108864"},
		"literal cut short":    {join(uvarint(3), []byte{2 << 2, 'a'}), "snappy: literal cut short"},
		"literal past length":  {join(uvarint(1), []byte{1 << 2, 'a', 'b'}), "snappy: 2 more bytes after 0 written overrun the declared 1"},
		// One byte declared, then copies that would grow it without end.
		"copies past length": {join(uvarint(1), []byte{0 << 2, 'a'}, bytes.Repeat([]byte{(11-4)<<2 | snappyCopy1, 1}, 100000)), "snappy: 11 more bytes after 1 written overrun the declared 1"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := decodeSnappy(tc.stream)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDecodeSnappyReadsLongLiterals(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 200)
	// One literal of 200 bytes, its length in one extra byte.
	stream := join(uvarint(200), []byte{60 << 2, 199}, data)
	got, err := decodeSnappy(stream)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("got %d bytes, %v", len(got), err)
	}
}

func TestDecodeV8ReadsDataValues(t *testing.T) {
	// {name: "Café", count: -3, big: 2^32, ok: true, none: null, tags: ["a", same object again]}
	value := join([]byte{v8Version, 0x10}, v8Object(
		v8String("name"), join([]byte{v8TwoByteString}, uvarint(8), []byte{'C', 0, 'a', 0, 'f', 0, 0xE9, 0}),
		v8String("count"), join([]byte{v8Int32}, uvarint(5)),
		v8String("big"), v8Number(1<<32),
		v8String("ok"), []byte{v8True},
		v8String("none"), []byte{v8Null},
		v8String("tags"), v8Dense(v8String("a"), []byte{v8ObjectReference, 0}),
	))
	got, err := decodeV8(value)
	if err != nil {
		t.Fatal(err)
	}
	object := got.(map[string]any)
	tags := object["tags"].([]any)
	if tags[0] != "a" || reflect.ValueOf(tags[1]).Pointer() != reflect.ValueOf(object).Pointer() {
		t.Fatalf("tags = %v", tags)
	}
	delete(object, "tags")
	want := map[string]any{"name": "Café", "count": -3.0, "big": float64(1 << 32), "ok": true, "none": nil}
	if !reflect.DeepEqual(object, want) {
		t.Fatalf("got %v, want %v", object, want)
	}
}

func TestDecodeV8ReadsSparseArrays(t *testing.T) {
	value := join([]byte{v8BeginSparse}, uvarint(3), join([]byte{v8Uint32}, uvarint(2)), v8String("c"), []byte{v8EndSparse}, uvarint(1), uvarint(3))
	got, err := decodeV8(value)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []any{nil, nil, "c"}) {
		t.Fatalf("got %v", got)
	}
}

func TestDecodeV8NamesAnUnsupportedTag(t *testing.T) {
	_, err := decodeV8([]byte{v8Version, 0x10, ';'})
	if err == nil || err.Error() != `v8: unsupported tag ';' at byte 2` {
		t.Fatalf("error %v", err)
	}
}

func TestDecodeV8RejectsAReferenceToNothing(t *testing.T) {
	_, err := decodeV8([]byte{v8ObjectReference, 4})
	if err == nil || err.Error() != "v8: reference to object 4 of 0" {
		t.Fatalf("error %v", err)
	}
}

func TestReadLevelDBKeepsTheNewestWriteOfEveryKey(t *testing.T) {
	dir := t.TempDir()
	// The table holds old values; the log overwrites one and deletes one.
	writeFile(t, filepath.Join(dir, "000005.ldb"), tableFile(true,
		tableEntry{key: []byte("kept"), sequence: 1, value: []byte("table")},
		tableEntry{key: []byte("replaced"), sequence: 2, value: []byte("old")},
		tableEntry{key: []byte("removed"), sequence: 3, value: []byte("gone")},
	))
	big := bytes.Repeat([]byte("v"), 40<<10)
	writeFile(t, filepath.Join(dir, "000006.log"), logFile(
		batch(10, write{key: []byte("replaced"), value: []byte("new")}, write{key: []byte("removed"), deleted: true}),
		// Larger than a block, so it is split across three.
		batch(12, write{key: []byte("large"), value: big}),
	))
	got, err := readLevelDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]byte{"kept": []byte("table"), "replaced": []byte("new"), "large": big}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got keys %v", keysOf(got))
	}
}

func TestReadLevelDBLetsAnOlderLogLose(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "000007.ldb"), tableFile(false, tableEntry{key: []byte("k"), sequence: 20, value: []byte("table")}))
	writeFile(t, filepath.Join(dir, "000003.log"), logFile(batch(4, write{key: []byte("k"), value: []byte("log")})))
	got, err := readLevelDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(got["k"]) != "table" {
		t.Fatalf("k = %q", got["k"])
	}
}

func TestReadLevelDBLeavesOutARecordStillBeingWritten(t *testing.T) {
	dir := t.TempDir()
	log := logFile(batch(1, write{key: []byte("done"), value: []byte("1")}), batch(2, write{key: []byte("half"), value: []byte("2")}))
	writeFile(t, filepath.Join(dir, "000001.log"), log[:len(log)-3])
	got, err := readLevelDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(keysOf(got), []string{"done"}) {
		t.Fatalf("got %v", keysOf(got))
	}
}

// folder writes a Chrome IndexedDB folder holding database "app" with
// object store "things", and the given records in it.
func folder(t *testing.T, records ...write) string {
	t.Helper()
	return folderWithDatabase(t, []byte{2}, 2, records...)
}

// folderWithDatabase writes the folder with database "app" under the
// id id, whose stored value is idValue.
func folderWithDatabase(t *testing.T, idValue []byte, id uint64, records ...write) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "https_app.example_0.indexeddb.leveldb")
	writes := []write{
		{key: namedDatabaseKey("other"), value: []byte{1}},
		{key: namedDatabaseKey("app"), value: idValue},
		{key: storeNameKey(id, 1), value: be16("things")},
		{key: storeNameKey(id, 3), value: be16("others")},
	}
	writes = append(writes, records...)
	writeFile(t, filepath.Join(dir, "000003.log"), logFile(batch(1, writes...)))
	return dir
}

func TestGetDecodesAnInlineRecord(t *testing.T) {
	dir := folder(t, write{key: recordKey(2, 1, indexRecords, "cache"), value: record(blinkValue(v8Object(v8String("title"), v8String("Hello"))))})
	f, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.Get("app", "things", "cache")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]any{"title": "Hello"}) {
		t.Fatalf("got %v", got)
	}
}

func TestGetExpandsACompressedRecord(t *testing.T) {
	compressed := join([]byte{blinkVersionTag, blinkProcessed, blinkSnappy}, literalSnappy(blinkValue(v8String("squeezed"))))
	dir := folder(t, write{key: recordKey(2, 1, indexRecords, "k"), value: record(compressed)})
	f, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.Get("app", "things", "k")
	if err != nil || got != "squeezed" {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestGetReadsARecordMovedToACompressedBlob(t *testing.T) {
	blob := join([]byte{blinkVersionTag, blinkProcessed, blinkSnappy}, literalSnappy(blinkValue(v8String("from the blob"))))
	reference := join([]byte{blinkVersionTag, blinkProcessed, blinkReplaceWithBlob}, uvarint(uint64(len(blob))), uvarint(1))
	// Two blobs listed; the record names the second, number 0x199.
	list := join([]byte{externalBlob}, uvarint(0x12), keyString("application/x"), uvarint(3),
		[]byte{externalBlob}, uvarint(0x199), keyString("application/vnd.blink-idb-value-wrapper"), uvarint(uint64(len(blob))))
	dir := folder(t,
		write{key: recordKey(2, 1, indexRecords, "big"), value: record(reference)},
		write{key: recordKey(2, 1, indexBlobs, "big"), value: list},
	)
	writeFile(t, filepath.Join(strings.TrimSuffix(dir, ".leveldb")+".blob", "2", "01", "199"), blob)
	f, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.Get("app", "things", "big")
	if err != nil || got != "from the blob" {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestGetSaysWhatIsMissing(t *testing.T) {
	dir := folder(t, write{key: recordKey(2, 1, indexRecords, "k"), value: record(blinkValue(v8String("v")))})
	f, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ database, store, key, want string }{
		{"nope", "things", "k", `indexeddb missing: no database "nope"`},
		{"app", "nope", "k", `indexeddb missing: no object store "nope" in database 2`},
		{"app", "things", "nope", `indexeddb missing: no key "nope" in app/things`},
	} {
		_, err := f.Get(tc.database, tc.store, tc.key)
		var e *Error
		if !errors.As(err, &e) || e.Reason != ReasonMissing || err.Error() != tc.want {
			t.Fatalf("Get(%s, %s, %s): %v, want %q", tc.database, tc.store, tc.key, err, tc.want)
		}
	}
}

func TestOpenSaysAMissingFolderIsMissing(t *testing.T) {
	_, err := Open(filepath.Join(t.TempDir(), "none.indexeddb.leveldb"))
	var e *Error
	if !errors.As(err, &e) || e.Reason != ReasonMissing {
		t.Fatalf("error %v", err)
	}
}

func TestGetSaysABrokenValueIsUnreadable(t *testing.T) {
	reference := join([]byte{blinkVersionTag, blinkProcessed, blinkReplaceWithBlob}, uvarint(9), uvarint(0))
	dir := folder(t, write{key: recordKey(2, 1, indexRecords, "k"), value: record(reference)})
	f, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Get("app", "things", "k")
	var e *Error
	if !errors.As(err, &e) || e.Reason != ReasonUnreadable || err.Error() != "indexeddb unreadable: app/things/k: the value is in a blob, and the record lists none" {
		t.Fatalf("error %v", err)
	}
}

func keysOf(values map[string][]byte) []string {
	var keys []string
	for k := range values {
		keys = append(keys, k)
	}
	return keys
}

func TestDecodeV8SkipsLongPaddingWithoutRecursion(t *testing.T) {
	// A small stack makes recursion per padding byte fail fast.
	defer debug.SetMaxStack(debug.SetMaxStack(1 << 20))
	value := join(bytes.Repeat([]byte{v8Padding}, 1<<20), []byte{v8True})
	got, err := decodeV8(value)
	if err != nil || got != true {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestDecodeV8SkipsPaddingBeforeAnEndTag(t *testing.T) {
	value := join([]byte{v8BeginObject}, v8String("a"), []byte{v8True, v8Padding, v8Padding, v8EndObject}, uvarint(1))
	got, err := decodeV8(value)
	if err != nil || !reflect.DeepEqual(got, map[string]any{"a": true}) {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestDecodeV8ReadsSparseArraysV8Wrote(t *testing.T) {
	// Written by Node 24's v8.serialize.
	for name, tc := range map[string]struct {
		hex   string
		check func(array []any) bool
	}{
		"new Array(100000) with a[99999] = \"x\"": {"ff0f61a08d0649be9a0c2201784001a08d06", func(array []any) bool {
			return len(array) == 100000 && array[99999] == "x" && array[0] == nil
		}},
		"[1, , 3]": {"ff0f61034900490249044906400203", func(array []any) bool {
			return reflect.DeepEqual(array, []any{1.0, nil, 3.0})
		}},
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := hex.DecodeString(tc.hex)
			if err != nil {
				t.Fatal(err)
			}
			got, err := decodeV8(raw)
			if err != nil {
				t.Fatal(err)
			}
			if array, ok := got.([]any); !ok || !tc.check(array) {
				t.Fatalf("got %T of %d", got, len(array))
			}
		})
	}
}

func TestDecodeV8RejectsDamagedSparseArrays(t *testing.T) {
	for name, tc := range map[string]struct {
		value []byte
		want  string
	}{
		"length over the budget": {join([]byte{v8BeginSparse}, uvarint(maxV8SparseItems+1)), "v8: sparse array of 4194305 items, with 0 of 4194304 read before"},
		"two arrays over the budget": {join([]byte{v8BeginDense}, uvarint(2),
			[]byte{v8BeginSparse}, uvarint(maxV8SparseItems), []byte{v8EndSparse}, uvarint(0), uvarint(maxV8SparseItems),
			[]byte{v8BeginSparse}, uvarint(1)), "v8: sparse array of 1 items, with 4194304 of 4194304 read before"},
		"wrong length at the end":     {join([]byte{v8BeginSparse}, uvarint(3), []byte{v8EndSparse}, uvarint(0), uvarint(4)), "v8: array ends with 0 properties and length 4, read 0 and 3"},
		"wrong properties at the end": {join([]byte{v8BeginSparse}, uvarint(3), []byte{v8EndSparse}, uvarint(2), uvarint(3)), "v8: array ends with 2 properties and length 3, read 0 and 3"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := decodeV8(tc.value)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
}

// footerOnlyTable is a table file of just a footer whose index handle
// is offset and size; the metaindex handle is never read.
func footerOnlyTable(offset, size uint64) []byte {
	footer := join(uvarint(0), uvarint(0), uvarint(offset), uvarint(size))
	footer = append(footer, make([]byte, tableFooterSize-8-len(footer))...)
	return binary.LittleEndian.AppendUint64(footer, tableMagic)
}

func TestOpenSaysATableWithDamagedBlockHandlesIsUnreadable(t *testing.T) {
	for name, tc := range map[string]struct {
		offset, size uint64
		want         string
	}{
		"offset that wraps":     {math.MaxUint64, 2, "block of 2 bytes at 18446744073709551615 runs past the file of 48"},
		"size that wraps":       {10, math.MaxUint64, "block of 18446744073709551615 bytes at 10 runs past the file of 48"},
		"trailer past the file": {40, 4, "block of 4 bytes at 40 runs past the file of 48"},
		"offset past the file":  {49, 0, "block of 0 bytes at 49 runs past the file of 48"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "https_app.example_0.indexeddb.leveldb")
			writeFile(t, filepath.Join(dir, "000005.ldb"), footerOnlyTable(tc.offset, tc.size))
			_, err := Open(dir)
			var e *Error
			want := "indexeddb unreadable: table 000005.ldb: " + tc.want
			if !errors.As(err, &e) || e.Reason != ReasonUnreadable || err.Error() != want {
				t.Fatalf("error %v, want %q", err, want)
			}
		})
	}
}

func TestReadLevelDBLeavesOutATableStillBeingWritten(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "000003.log"), logFile(batch(1, write{key: []byte("k"), value: []byte("log")})))
	finished := tableFile(false, tableEntry{key: []byte("k"), sequence: 9, value: []byte("table")})
	// LevelDB writes a table's footer last, so an unfinished one has none.
	writeFile(t, filepath.Join(dir, "000009.ldb"), finished[:len(finished)-10])
	writeFile(t, filepath.Join(dir, "000010.ldb"), make([]byte, 20))
	got, err := readLevelDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(got["k"]) != "log" {
		t.Fatalf("k = %q", got["k"])
	}
}

func TestReadLevelDBGivesUpOnAFileThatKeepsVanishing(t *testing.T) {
	dir := t.TempDir()
	// A link to nothing is listed but cannot be read, on every read.
	if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, "000004.ldb")); err != nil {
		t.Fatal(err)
	}
	_, err := readLevelDB(dir)
	if err == nil || !strings.HasPrefix(err.Error(), "files kept changing over 3 reads: table 000004.ldb: ") {
		t.Fatalf("error %v", err)
	}
}

func TestGetRejectsABlobThatNamesItself(t *testing.T) {
	// The blob holds the same reference that led to it: size 5, entry 0.
	reference := join([]byte{blinkVersionTag, blinkProcessed, blinkReplaceWithBlob}, uvarint(5), uvarint(0))
	list := join([]byte{externalBlob}, uvarint(2), keyString(""), uvarint(5))
	dir := folder(t,
		write{key: recordKey(2, 1, indexRecords, "k"), value: record(reference)},
		write{key: recordKey(2, 1, indexBlobs, "k"), value: list},
	)
	writeFile(t, filepath.Join(strings.TrimSuffix(dir, ".leveldb")+".blob", "2", "00", "2"), reference)
	f, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Get("app", "things", "k")
	var e *Error
	if !errors.As(err, &e) || e.Reason != ReasonUnreadable || err.Error() != "indexeddb unreadable: app/things/k: a blob reference inside a blob or a compressed value" {
		t.Fatalf("error %v", err)
	}
}

func TestGetFindsADatabaseByItsChromiumEncodedID(t *testing.T) {
	// Each id as Chromium's EncodeInt writes it, little-endian bytes.
	for _, tc := range []struct {
		id      uint64
		encoded []byte
	}{
		{1, []byte{0x01}},
		{127, []byte{0x7f}},
		{128, []byte{0x80}},
		{255, []byte{0xff}},
		{256, []byte{0x00, 0x01}},
		{300, []byte{0x2c, 0x01}},
	} {
		dir := folderWithDatabase(t, tc.encoded, tc.id, write{key: recordKey(tc.id, 1, indexRecords, "k"), value: record(blinkValue(v8String("v")))})
		f, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		got, err := f.Get("app", "things", "k")
		if err != nil || got != "v" {
			t.Fatalf("id %d: got %v, %v", tc.id, got, err)
		}
	}
}

func TestGetSaysADatabaseIDOfNineBytesIsUnreadable(t *testing.T) {
	dir := folderWithDatabase(t, make([]byte, 9), 0)
	f, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Get("app", "things", "k")
	var e *Error
	if !errors.As(err, &e) || e.Reason != ReasonUnreadable || err.Error() != `indexeddb unreadable: database "app" id: an integer of 9 bytes` {
		t.Fatalf("error %v", err)
	}
}
