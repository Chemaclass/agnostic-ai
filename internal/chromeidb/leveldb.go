package chromeidb

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// This file reads a LevelDB folder as Chrome writes it, for one need:
// the newest value of every key. It never writes, never takes the
// folder's lock, and keeps no key order: each entry carries a sequence
// number, and the highest one per key wins. A table that a compaction
// has just replaced may still be on disk, which is harmless: its
// entries are older than the ones that replaced them.

// LevelDB entry kinds, the low byte of an internal key's trailer and
// the tag of a write batch entry.
const (
	kindDeletion = 0
	kindValue    = 1
)

// The write-ahead log is cut into 32 KB blocks of records; a record
// that does not fit continues in the next block.
const (
	logBlockSize  = 32 << 10
	logHeaderSize = 7
)

// Log record types: a whole record, or its first, middle or last part.
const (
	logFull   = 1
	logFirst  = 2
	logMiddle = 3
	logLast   = 4
)

// tableFooterSize is the fixed footer at the end of every table file.
const tableFooterSize = 48

// Block compression types, the byte after each block.
const (
	blockPlain  = 0
	blockSnappy = 1
)

// entry is one write to a key, as a log or a table holds it.
type entry struct {
	sequence uint64
	deleted  bool
	value    []byte
}

// readLevelDB returns the newest live value of every key in the LevelDB
// folder dir.
func readLevelDB(dir string) (map[string][]byte, error) {
	newest := map[string]entry{}
	keep := func(key []byte, e entry) {
		if old, ok := newest[string(key)]; ok && old.sequence > e.sequence {
			return
		}
		newest[string(key)] = e
	}
	tables, err := filepath.Glob(filepath.Join(dir, "*.ldb"))
	if err != nil {
		return nil, err
	}
	for _, table := range tables {
		if err := readTable(table, keep); err != nil {
			return nil, fmt.Errorf("table %s: %w", filepath.Base(table), err)
		}
	}
	logs, err := filepath.Glob(filepath.Join(dir, "*.log"))
	if err != nil {
		return nil, err
	}
	for _, log := range logs {
		if err := readLog(log, keep); err != nil {
			return nil, fmt.Errorf("log %s: %w", filepath.Base(log), err)
		}
	}
	values := make(map[string][]byte, len(newest))
	for key, e := range newest {
		if !e.deleted {
			values[key] = e.value
		}
	}
	return values, nil
}

// readLog hands every write in a write-ahead log file to keep. A record
// cut short at the end is one the writer has not finished, and is left
// out.
func readLog(path string, keep func([]byte, entry)) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var pending []byte
	at := 0
	for at+logHeaderSize <= len(data) {
		left := logBlockSize - at%logBlockSize
		if left < logHeaderSize {
			// The end of a block too short for a header is padding.
			at += left
			continue
		}
		size := int(binary.LittleEndian.Uint16(data[at+4:]))
		kind := data[at+6]
		start := at + logHeaderSize
		if kind == 0 && size == 0 {
			// Preallocated zeroes: the rest of the block is unused.
			at += left
			continue
		}
		if start+size > len(data) {
			return nil
		}
		fragment := data[start : start+size]
		at = start + size
		switch kind {
		case logFull:
			if err := readBatch(fragment, keep); err != nil {
				return err
			}
		case logFirst:
			pending = append([]byte(nil), fragment...)
		case logMiddle:
			pending = append(pending, fragment...)
		case logLast:
			if err := readBatch(append(pending, fragment...), keep); err != nil {
				return err
			}
			pending = nil
		default:
			return fmt.Errorf("record type %d at byte %d", kind, at)
		}
	}
	return nil
}

// readBatch hands the writes of one write batch to keep: a sequence
// number and a count, then each put or delete in turn.
func readBatch(batch []byte, keep func([]byte, entry)) error {
	if len(batch) < 12 {
		return fmt.Errorf("write batch of %d bytes", len(batch))
	}
	sequence := binary.LittleEndian.Uint64(batch)
	count := int(binary.LittleEndian.Uint32(batch[8:]))
	r := reader{data: batch, at: 12}
	for i := range count {
		kind, err := r.byte()
		if err != nil {
			return err
		}
		key, err := r.lengthPrefixed()
		if err != nil {
			return err
		}
		switch kind {
		case kindValue:
			value, err := r.lengthPrefixed()
			if err != nil {
				return err
			}
			keep(key, entry{sequence: sequence + uint64(i), value: value})
		case kindDeletion:
			keep(key, entry{sequence: sequence + uint64(i), deleted: true})
		default:
			return fmt.Errorf("write batch entry kind %d", kind)
		}
	}
	return nil
}

// readTable hands every entry of a table file to keep. The footer
// points at the index block, whose values point at the data blocks.
func readTable(path string, keep func([]byte, entry)) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) < tableFooterSize {
		return fmt.Errorf("%d bytes is shorter than a footer", len(data))
	}
	footer := reader{data: data[len(data)-tableFooterSize:]}
	// The metaindex handle comes first; only the index is needed.
	if _, _, err := footer.blockHandle(); err != nil {
		return err
	}
	indexOffset, indexSize, err := footer.blockHandle()
	if err != nil {
		return err
	}
	index, err := tableBlock(data, indexOffset, indexSize)
	if err != nil {
		return err
	}
	return eachBlockEntry(index, func(_, handle []byte) error {
		h := reader{data: handle}
		offset, size, err := h.blockHandle()
		if err != nil {
			return err
		}
		block, err := tableBlock(data, offset, size)
		if err != nil {
			return err
		}
		return eachBlockEntry(block, func(internalKey, value []byte) error {
			if len(internalKey) < 8 {
				return fmt.Errorf("internal key of %d bytes", len(internalKey))
			}
			split := len(internalKey) - 8
			trailer := binary.LittleEndian.Uint64(internalKey[split:])
			keep(internalKey[:split], entry{sequence: trailer >> 8, deleted: trailer&0xFF == kindDeletion, value: value})
			return nil
		})
	})
}

// tableBlock returns the block at offset, expanded when compressed.
func tableBlock(data []byte, offset, size uint64) ([]byte, error) {
	end := offset + size
	if end+1 > uint64(len(data)) {
		return nil, fmt.Errorf("block at %d runs past the file", offset)
	}
	block := data[offset:end]
	switch data[end] {
	case blockPlain:
		return block, nil
	case blockSnappy:
		return decodeSnappy(block)
	default:
		return nil, fmt.Errorf("block compression %d", data[end])
	}
}

// eachBlockEntry walks a block's entries. Each key shares a prefix
// with the one before it; the restart table at the end is not needed
// for a walk from the start.
func eachBlockEntry(block []byte, visit func(key, value []byte) error) error {
	if len(block) < 4 {
		return fmt.Errorf("block of %d bytes", len(block))
	}
	restarts := int(binary.LittleEndian.Uint32(block[len(block)-4:]))
	end := len(block) - 4 - 4*restarts
	if end < 0 {
		return fmt.Errorf("block claims %d restarts", restarts)
	}
	r := reader{data: block[:end]}
	var key []byte
	for r.at < end {
		shared, err := r.uvarint()
		if err != nil {
			return err
		}
		unshared, err := r.uvarint()
		if err != nil {
			return err
		}
		valueSize, err := r.uvarint()
		if err != nil {
			return err
		}
		if shared > uint64(len(key)) {
			return fmt.Errorf("key shares %d bytes of %d", shared, len(key))
		}
		suffix, err := r.bytes(unshared)
		if err != nil {
			return err
		}
		key = append(key[:shared:shared], suffix...)
		value, err := r.bytes(valueSize)
		if err != nil {
			return err
		}
		if err := visit(key, value); err != nil {
			return err
		}
	}
	return nil
}
