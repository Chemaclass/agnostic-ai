package chromeidb

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func manifestRecord(data []byte) []byte {
	out := logFile(data)
	for at := 0; at+logHeaderSize <= len(out); {
		left := logBlockSize - at%logBlockSize
		if left < logHeaderSize {
			at += left
			continue
		}
		size := int(binary.LittleEndian.Uint16(out[at+4:]))
		end := at + logHeaderSize + size
		crc := crc32.Checksum(out[at+6:end], crc32.MakeTable(crc32.Castagnoli))
		binary.LittleEndian.PutUint32(out[at:], (crc>>15|crc<<17)+0xa282ead8)
		at = end
	}
	return out
}

func manifestBytes(b []byte) []byte { return append(uvarint(uint64(len(b))), b...) }
func manifestInternal(k []byte, seq uint64) []byte {
	return binary.LittleEndian.AppendUint64(append([]byte(nil), k...), seq<<8|1)
}
func TestReadLevelDB_DoesNotRestoreACompactedDeletion(t *testing.T) {
	dir := t.TempDir()
	key := recordKey(1, 1, indexRecords, "react-query-cache")
	old := tableFile(false, tableEntry{key: key, sequence: 3, value: record(blinkValue(v8String("deleted private chat")))})
	dbkey, storekey := namedDatabaseKey("keyval-store"), storeNameKey(1, 1)
	current := tableFile(false, tableEntry{key: dbkey, sequence: 1, value: idbInt(1)}, tableEntry{key: storekey, sequence: 2, value: be16("keyval")})
	writeFile(t, filepath.Join(dir, "000005.ldb"), old)
	writeFile(t, filepath.Join(dir, "000007.ldb"), current)
	// An iterator pins table 5 after compaction has dropped its deletion marker.
	initial := join(uvarint(1), manifestBytes([]byte("idb_cmp1")), uvarint(2), uvarint(8), uvarint(3), uvarint(9), uvarint(4), uvarint(4), uvarint(7), uvarint(1), uvarint(5), uvarint(uint64(len(old))), manifestBytes(manifestInternal(key, 3)), manifestBytes(manifestInternal(key, 3)))
	compact := join(uvarint(6), uvarint(1), uvarint(5), uvarint(7), uvarint(2), uvarint(7), uvarint(uint64(len(current))), manifestBytes(manifestInternal(dbkey, 1)), manifestBytes(manifestInternal(storekey, 2)))
	writeFile(t, filepath.Join(dir, "CURRENT"), []byte("MANIFEST-000006\n"))
	writeFile(t, filepath.Join(dir, "MANIFEST-000006"), append(manifestRecord(initial), manifestRecord(compact)...))
	writeFile(t, filepath.Join(dir, "000008.log"), nil)
	folder, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := folder.Get("keyval-store", "keyval", "react-query-cache")
	var reason *Error
	if !errors.As(err, &reason) || reason.Reason != ReasonMissing {
		t.Errorf("compacted-away record resurrected: got=%v err=%v", got, err)
	}
	if err := os.Remove(filepath.Join(dir, "000005.ldb")); err != nil {
		t.Fatal(err)
	}
	folder, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = folder.Get("keyval-store", "keyval", "react-query-cache")
	if !errors.As(err, &reason) || reason.Reason != ReasonMissing {
		t.Errorf("control without obsolete file: %v", err)
	}
}

func writeManifest(t *testing.T, dir string, log, previous uint64, tables ...uint64) {
	t.Helper()
	edit := join(uvarint(2), uvarint(log), uvarint(9), uvarint(previous))
	for _, table := range tables {
		edit = join(edit, uvarint(7), uvarint(0), uvarint(table), uvarint(0), manifestBytes(nil), manifestBytes(nil))
	}
	writeFile(t, filepath.Join(dir, "CURRENT"), []byte("MANIFEST-000002\n"))
	writeFile(t, filepath.Join(dir, "MANIFEST-000002"), manifestRecord(edit))
}

func TestReadLevelDB_UsesCurrentPreviousAndNewerLogs(t *testing.T) {
	dir := t.TempDir()
	for number, key := range map[uint64]string{2: "obsolete", 3: "previous", 5: "current", 8: "rotated"} {
		writeFile(t, filepath.Join(dir, fmt.Sprintf("%06d.log", number)), logFile(batch(number, write{key: []byte(key), value: []byte(key)})))
	}
	writeManifest(t, dir, 5, 3)
	got, err := readLevelDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := got["obsolete"]; exists {
		t.Error("an obsolete log restored a value")
	}
	for _, key := range []string{"previous", "current", "rotated"} {
		if string(got[key]) != key {
			t.Errorf("%s log is missing", key)
		}
	}
}

func TestReadLevelDB_RejectsAnUncommittedFolder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "000003.log"), logFile(batch(1, write{key: []byte("private"), value: []byte("stale")})))
	if _, err := readLevelDB(dir); err == nil {
		t.Error("read files without a committed manifest")
	}
}

func TestReadLevelDB_RejectsAnUnfinishedCommittedTable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "000005.ldb"), make([]byte, 20))
	writeManifest(t, dir, 0, 0, 5)
	if _, err := readLevelDB(dir); !errors.Is(err, errUnfinishedTable) {
		t.Errorf("error %v, want unfinished committed table", err)
	}
}

func TestReadLevelDB_RejectsACorruptManifest(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, 0, 0)
	path := filepath.Join(dir, "MANIFEST-000002")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 1
	writeFile(t, path, data)
	if _, err := readLevelDB(dir); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Errorf("error %v, want checksum failure", err)
	}
}

func TestReadManifest_RejectsANameOutsideTheFolder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CURRENT"), []byte("../MANIFEST-000002\n"))
	if _, err := readManifest(dir); err == nil || !strings.Contains(err.Error(), "invalid manifest name") {
		t.Errorf("error %v", err)
	}
}

func TestReadManifest_IgnoresAnIncompleteFinalEdit(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, 3, 0)
	path := filepath.Join(dir, "MANIFEST-000002")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edit := manifestRecord(join(uvarint(2), uvarint(7)))
	writeFile(t, path, append(data, edit[:len(edit)-1]...))
	snapshot, err := readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.log != 3 {
		t.Errorf("incomplete edit changed log to %d", snapshot.log)
	}
}

func TestReadManifest_DetectsAnAppendedEdit(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, 3, 0)
	snapshot, err := readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	edit := manifestRecord(join(uvarint(2), uvarint(7)))
	writeFile(t, snapshot.path, append(snapshot.contents, edit...))
	if err := snapshot.unchanged(dir, nil); !errors.Is(err, errFileSetChanged) {
		t.Errorf("error %v, want file-set change", err)
	}
}

func TestReadManifest_ReassemblesAnEditAcrossBlocks(t *testing.T) {
	dir := t.TempDir()
	edit := join(uvarint(1), manifestBytes([]byte(strings.Repeat("x", 3*logBlockSize))), uvarint(2), uvarint(3))
	writeFile(t, filepath.Join(dir, "CURRENT"), []byte("MANIFEST-000002\n"))
	writeFile(t, filepath.Join(dir, "MANIFEST-000002"), manifestRecord(edit))
	snapshot, err := readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.log != 3 {
		t.Errorf("log number %d, want 3", snapshot.log)
	}
}
