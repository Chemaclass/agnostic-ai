package chromeidb

import (
	"bytes"
	"errors"
	"path/filepath"
	"reflect"
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
	root := t.TempDir()
	dir := filepath.Join(root, "https_app.example_0.indexeddb.leveldb")
	writes := []write{
		{key: namedDatabaseKey("other"), value: uvarint(1)},
		{key: namedDatabaseKey("app"), value: uvarint(2)},
		{key: storeNameKey(2, 1), value: be16("things")},
		{key: storeNameKey(2, 3), value: be16("others")},
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
