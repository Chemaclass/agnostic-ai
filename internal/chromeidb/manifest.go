package chromeidb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	manifestName      = regexp.MustCompile(`^MANIFEST-[0-9]+\n$`)
	errFileSetChanged = errors.New("the committed file set changed during the read")
)

type manifestSnapshot struct {
	current, contents []byte
	path              string
	tables            map[uint64]bool
	log, previousLog  uint64
	hasLog            bool
}

func readManifest(dir string) (manifestSnapshot, error) {
	var snapshot manifestSnapshot
	currentPath := filepath.Join(dir, "CURRENT")
	current, err := readLimited(currentPath, 128)
	if err != nil {
		return snapshot, err
	}
	if !manifestName.Match(current) {
		return snapshot, fmt.Errorf("%s: invalid manifest name", currentPath)
	}
	snapshot.current = current
	snapshot.path = filepath.Join(dir, strings.TrimSuffix(string(current), "\n"))
	snapshot.contents, err = readLimited(snapshot.path, maxDecodedBytes)
	if err != nil {
		return snapshot, err
	}
	snapshot.tables = map[uint64]bool{}
	err = logRecords(snapshot.contents, true, snapshot.apply)
	if err != nil {
		return snapshot, fmt.Errorf("parse %s: %w", snapshot.path, err)
	}
	if !snapshot.hasLog {
		return snapshot, fmt.Errorf("parse %s: no log number", snapshot.path)
	}
	return snapshot, nil
}

func readLimited(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s: exceeds %d bytes", path, limit)
	}
	return data, nil
}

func (s *manifestSnapshot) apply(record []byte) error {
	r := reader{data: record}
	for !r.done() {
		tag, err := r.uvarint()
		if err != nil {
			return err
		}
		switch tag {
		case 1:
			_, err = r.lengthPrefixed()
		case 2, 3, 4, 9:
			var number uint64
			number, err = r.uvarint()
			if tag == 2 {
				s.log, s.hasLog = number, true
			}
			if tag == 9 {
				s.previousLog = number
			}
		case 5, 6, 7:
			var level, number uint64
			level, err = r.uvarint()
			if err == nil && level >= 7 {
				return fmt.Errorf("invalid level %d", level)
			}
			if err != nil {
				return err
			}
			if tag == 5 {
				_, err = r.lengthPrefixed()
				break
			}
			number, err = r.uvarint()
			if err != nil {
				return err
			}
			if tag == 6 {
				delete(s.tables, number)
				break
			}
			if _, err = r.uvarint(); err != nil {
				return err
			}
			if _, err = r.lengthPrefixed(); err != nil {
				return err
			}
			if _, err = r.lengthPrefixed(); err != nil {
				return err
			}
			s.tables[number] = true
		default:
			return fmt.Errorf("unsupported manifest tag %d", tag)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s manifestSnapshot) logs(dir string) ([]string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var logs []string
	for _, file := range files {
		name := file.Name()
		if !strings.HasSuffix(name, ".log") {
			continue
		}
		number, err := strconv.ParseUint(strings.TrimSuffix(name, ".log"), 10, 64)
		if err == nil && (number >= s.log || number == s.previousLog) {
			logs = append(logs, filepath.Join(dir, name))
		}
	}
	slices.Sort(logs)
	return logs, nil
}

func (s manifestSnapshot) unchanged(dir string, logs []string) error {
	next, err := readManifest(dir)
	if err != nil {
		return err
	}
	if !bytes.Equal(s.current, next.current) || !bytes.Equal(s.contents, next.contents) {
		return fmt.Errorf("%s: %w", s.path, errFileSetChanged)
	}
	nextLogs, err := next.logs(dir)
	if err != nil {
		return err
	}
	if !slices.Equal(logs, nextLogs) {
		return fmt.Errorf("%s: %w", s.path, errFileSetChanged)
	}
	return nil
}

func logRecords(data []byte, checksum bool, keep func([]byte) error) error {
	var pending []byte
	fragmented := false
	for at := 0; at+logHeaderSize <= len(data); {
		left := logBlockSize - at%logBlockSize
		if left < logHeaderSize {
			at += left
			continue
		}
		size := int(binary.LittleEndian.Uint16(data[at+4:]))
		kind := data[at+6]
		start := at + logHeaderSize
		if kind == 0 && size == 0 {
			at += left
			continue
		}
		if size+logHeaderSize > left {
			return fmt.Errorf("record crosses block at byte %d", at)
		}
		if start+size > len(data) {
			return nil
		}
		fragment := data[start : start+size]
		if checksum {
			crc := crc32.Checksum(data[at+6:start+size], crc32.MakeTable(crc32.Castagnoli))
			if binary.LittleEndian.Uint32(data[at:]) != (crc>>15|crc<<17)+0xa282ead8 {
				return fmt.Errorf("record checksum at byte %d", at)
			}
		}
		at = start + size
		switch kind {
		case logFull:
			if fragmented {
				return errors.New("full record inside fragmented record")
			}
			if err := keep(fragment); err != nil {
				return err
			}
		case logFirst:
			if fragmented {
				return errors.New("first fragment inside fragmented record")
			}
			pending, fragmented = append([]byte(nil), fragment...), true
		case logMiddle, logLast:
			if !fragmented {
				return errors.New("fragment without a first fragment")
			}
			pending = append(pending, fragment...)
			if kind == logLast {
				if err := keep(pending); err != nil {
					return err
				}
				pending, fragmented = nil, false
			}
		default:
			return fmt.Errorf("record type %d at byte %d", kind, at)
		}
	}
	return nil
}
