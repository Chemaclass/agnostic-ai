package chromeidb

import (
	"encoding/binary"
	"fmt"
)

// maxDecodedBytes bounds what one Snappy stream may claim to decode to,
// so a damaged length cannot ask for gigabytes.
const maxDecodedBytes = 64 << 20

// Snappy element types, the low two bits of each tag byte.
const (
	snappyLiteral = 0
	snappyCopy1   = 1
	snappyCopy2   = 2
	snappyCopy4   = 3
)

// decodeSnappy expands one Snappy block: a varint length, then literals
// and back-references. LevelDB compresses table blocks with it, and
// Chrome compresses large IndexedDB values with it.
func decodeSnappy(src []byte) ([]byte, error) {
	length, read := binary.Uvarint(src)
	if read <= 0 {
		return nil, fmt.Errorf("snappy: no length")
	}
	if length > maxDecodedBytes {
		return nil, fmt.Errorf("snappy: %d bytes is more than %d", length, maxDecodedBytes)
	}
	out := make([]byte, 0, length)
	at := read
	for at < len(src) {
		tag := src[at]
		at++
		var size, offset int
		switch tag & 3 {
		case snappyLiteral:
			size = int(tag>>2) + 1
			if size > 60 {
				// Lengths over 60 follow the tag in 1 to 4 little-endian bytes.
				extra := size - 60
				if at+extra > len(src) {
					return nil, fmt.Errorf("snappy: literal length cut short")
				}
				size = 0
				for i := range extra {
					size |= int(src[at+i]) << (8 * i)
				}
				size++
				at += extra
			}
			if at+size > len(src) {
				return nil, fmt.Errorf("snappy: literal cut short")
			}
			out = append(out, src[at:at+size]...)
			at += size
			continue
		case snappyCopy1:
			if at+1 > len(src) {
				return nil, fmt.Errorf("snappy: copy cut short")
			}
			size = int(tag>>2&7) + 4
			offset = int(tag>>5)<<8 | int(src[at])
			at++
		case snappyCopy2:
			if at+2 > len(src) {
				return nil, fmt.Errorf("snappy: copy cut short")
			}
			size = int(tag>>2) + 1
			offset = int(binary.LittleEndian.Uint16(src[at:]))
			at += 2
		case snappyCopy4:
			if at+4 > len(src) {
				return nil, fmt.Errorf("snappy: copy cut short")
			}
			size = int(tag>>2) + 1
			offset = int(binary.LittleEndian.Uint32(src[at:]))
			at += 4
		}
		if offset <= 0 || offset > len(out) {
			return nil, fmt.Errorf("snappy: copy from %d bytes back, with %d written", offset, len(out))
		}
		// A copy may overlap what it writes, so it goes byte by byte.
		from := len(out) - offset
		for i := range size {
			out = append(out, out[from+i])
		}
	}
	if uint64(len(out)) != length {
		return nil, fmt.Errorf("snappy: decoded %d bytes, expected %d", len(out), length)
	}
	return out, nil
}
