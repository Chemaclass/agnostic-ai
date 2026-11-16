package chromeidb

import (
	"encoding/binary"
	"fmt"
	"math"
)

// reader walks a byte slice and says where it ran out.
type reader struct {
	data []byte
	at   int
}

func (r *reader) done() bool { return r.at >= len(r.data) }

func (r *reader) byte() (byte, error) {
	if r.at >= len(r.data) {
		return 0, fmt.Errorf("ran out at byte %d", r.at)
	}
	b := r.data[r.at]
	r.at++
	return b, nil
}

func (r *reader) peek() (byte, error) {
	if r.at >= len(r.data) {
		return 0, fmt.Errorf("ran out at byte %d", r.at)
	}
	return r.data[r.at], nil
}

func (r *reader) bytes(n uint64) ([]byte, error) {
	if n > uint64(len(r.data)-r.at) {
		return nil, fmt.Errorf("%d bytes wanted at byte %d of %d", n, r.at, len(r.data))
	}
	out := r.data[r.at : r.at+int(n)]
	r.at += int(n)
	return out, nil
}

func (r *reader) uvarint() (uint64, error) {
	value, read := binary.Uvarint(r.data[r.at:])
	if read <= 0 {
		return 0, fmt.Errorf("bad varint at byte %d", r.at)
	}
	r.at += read
	return value, nil
}

func (r *reader) lengthPrefixed() ([]byte, error) {
	n, err := r.uvarint()
	if err != nil {
		return nil, err
	}
	return r.bytes(n)
}

func (r *reader) float64() (float64, error) {
	raw, err := r.bytes(8)
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(raw)), nil
}

// blockHandle reads a LevelDB block handle: an offset and a size.
func (r *reader) blockHandle() (offset, size uint64, err error) {
	if offset, err = r.uvarint(); err != nil {
		return 0, 0, err
	}
	if size, err = r.uvarint(); err != nil {
		return 0, 0, err
	}
	return offset, size, nil
}
