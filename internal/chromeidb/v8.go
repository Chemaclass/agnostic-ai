package chromeidb

import (
	"fmt"
	"strconv"
	"unicode/utf16"
)

// This file decodes V8's ValueSerializer output, the format a page's
// IndexedDB values are stored in, as plain Go values: map[string]any,
// []any, string, float64, bool and nil. It covers what web apps store
// as data: objects, arrays, strings, numbers, booleans, null, undefined
// and dates. Any other kind (maps, sets, typed arrays, regular
// expressions, big integers) is an error naming its tag.

// V8 serialization tags.
const (
	v8Version         = 0xFF
	v8Padding         = 0x00
	v8Undefined       = '_'
	v8Hole            = '-'
	v8Null            = '0'
	v8True            = 'T'
	v8False           = 'F'
	v8Int32           = 'I'
	v8Uint32          = 'U'
	v8Double          = 'N'
	v8Date            = 'D'
	v8OneByteString   = '"'
	v8TwoByteString   = 'c'
	v8UTF8String      = 'S'
	v8ObjectReference = '^'
	v8BeginObject     = 'o'
	v8EndObject       = '{'
	v8BeginDense      = 'A'
	v8EndDense        = '$'
	v8BeginSparse     = 'a'
	v8EndSparse       = '@'
)

// maxV8Depth bounds nesting, so a damaged value cannot recurse forever.
const maxV8Depth = 256

// maxV8SparseItems bounds the slots all sparse arrays of one value may
// take together. A sparse array stores only its present items, so its
// length says nothing about the bytes it was read from; this budget is
// what stops a damaged length from asking for gigabytes. 1 << 22 slots
// is 64 MB of items.
const maxV8SparseItems = 1 << 22

// decodeV8 decodes one serialized value, after its version header.
func decodeV8(data []byte) (any, error) {
	d := v8Decoder{r: reader{data: data}}
	for {
		tag, err := d.r.peek()
		if err != nil {
			return nil, err
		}
		if tag != v8Version {
			break
		}
		d.r.at++
		if _, err := d.r.uvarint(); err != nil {
			return nil, err
		}
	}
	return d.value(0)
}

type v8Decoder struct {
	r reader
	// objects holds every object, array and date in the order read, for
	// a later reference to name by position.
	objects []any
	// sparseItems counts the slots of the sparse arrays read so far.
	sparseItems uint64
}

// tag reads the next tag, past any padding, as V8 itself does.
func (d *v8Decoder) tag() (byte, error) {
	tag, err := d.peekTag()
	if err != nil {
		return 0, err
	}
	d.r.at++
	return tag, nil
}

// peekTag returns the next tag without reading it, after reading any
// padding before it. Padding is skipped in a loop: a long run of it
// must not take one stack frame per byte.
func (d *v8Decoder) peekTag() (byte, error) {
	for {
		tag, err := d.r.peek()
		if err != nil || tag != v8Padding {
			return tag, err
		}
		d.r.at++
	}
}

func (d *v8Decoder) value(depth int) (any, error) {
	if depth > maxV8Depth {
		return nil, fmt.Errorf("v8: nested deeper than %d", maxV8Depth)
	}
	tag, err := d.tag()
	if err != nil {
		return nil, err
	}
	switch tag {
	case v8Undefined, v8Hole, v8Null:
		return nil, nil
	case v8True:
		return true, nil
	case v8False:
		return false, nil
	case v8Int32:
		zigzag, err := d.r.uvarint()
		if err != nil {
			return nil, err
		}
		return float64(int64(zigzag>>1) ^ -int64(zigzag&1)), nil
	case v8Uint32:
		n, err := d.r.uvarint()
		return float64(n), err
	case v8Double:
		return d.r.float64()
	case v8Date:
		millis, err := d.r.float64()
		if err != nil {
			return nil, err
		}
		d.objects = append(d.objects, millis)
		return millis, nil
	case v8OneByteString:
		raw, err := d.r.lengthPrefixed()
		if err != nil {
			return nil, err
		}
		return latin1(raw), nil
	case v8TwoByteString:
		raw, err := d.r.lengthPrefixed()
		if err != nil {
			return nil, err
		}
		return utf16LE(raw)
	case v8UTF8String:
		raw, err := d.r.lengthPrefixed()
		return string(raw), err
	case v8ObjectReference:
		id, err := d.r.uvarint()
		if err != nil {
			return nil, err
		}
		if id >= uint64(len(d.objects)) {
			return nil, fmt.Errorf("v8: reference to object %d of %d", id, len(d.objects))
		}
		return d.objects[id], nil
	case v8BeginObject:
		return d.object(depth)
	case v8BeginDense:
		return d.denseArray(depth)
	case v8BeginSparse:
		return d.sparseArray(depth)
	default:
		return nil, fmt.Errorf("v8: unsupported tag %q at byte %d", tag, d.r.at-1)
	}
}

func (d *v8Decoder) object(depth int) (any, error) {
	object := map[string]any{}
	d.objects = append(d.objects, object)
	for {
		tag, err := d.peekTag()
		if err != nil {
			return nil, err
		}
		if tag == v8EndObject {
			d.r.at++
			_, err := d.r.uvarint()
			return object, err
		}
		key, err := d.propertyKey(depth)
		if err != nil {
			return nil, err
		}
		value, err := d.value(depth + 1)
		if err != nil {
			return nil, err
		}
		object[key] = value
	}
}

// propertyKey reads a property name: a string, or a number that names
// an index.
func (d *v8Decoder) propertyKey(depth int) (string, error) {
	key, err := d.value(depth + 1)
	if err != nil {
		return "", err
	}
	switch k := key.(type) {
	case string:
		return k, nil
	case float64:
		return strconv.FormatFloat(k, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("v8: property key of type %T", key)
	}
}

func (d *v8Decoder) denseArray(depth int) (any, error) {
	length, err := d.r.uvarint()
	if err != nil {
		return nil, err
	}
	if length > uint64(len(d.r.data)) {
		return nil, fmt.Errorf("v8: array of %d items in %d bytes", length, len(d.r.data))
	}
	array := make([]any, length)
	// The array is registered before its items, which may refer to it.
	d.objects = append(d.objects, array)
	for i := range array {
		if array[i], err = d.value(depth + 1); err != nil {
			return nil, err
		}
	}
	// Named properties may follow the items; data arrays have none.
	properties, err := d.skipProperties(depth, v8EndDense)
	if err != nil {
		return nil, err
	}
	if err := d.endCounts(properties, length); err != nil {
		return nil, err
	}
	return array, nil
}

func (d *v8Decoder) sparseArray(depth int) (any, error) {
	length, err := d.r.uvarint()
	if err != nil {
		return nil, err
	}
	if length > maxV8SparseItems-d.sparseItems {
		return nil, fmt.Errorf("v8: sparse array of %d items, with %d of %d read before", length, d.sparseItems, maxV8SparseItems)
	}
	d.sparseItems += length
	array := make([]any, length)
	d.objects = append(d.objects, array)
	var properties uint64
	for {
		tag, err := d.peekTag()
		if err != nil {
			return nil, err
		}
		if tag == v8EndSparse {
			d.r.at++
			if err := d.endCounts(properties, length); err != nil {
				return nil, err
			}
			return array, nil
		}
		key, err := d.propertyKey(depth)
		if err != nil {
			return nil, err
		}
		value, err := d.value(depth + 1)
		if err != nil {
			return nil, err
		}
		properties++
		index, err := strconv.ParseUint(key, 10, 64)
		if err != nil || index >= length {
			// A named property on an array: kept out of the items.
			continue
		}
		array[index] = value
	}
}

// skipProperties reads the named properties up to end, and the end tag,
// and says how many it read.
func (d *v8Decoder) skipProperties(depth int, end byte) (uint64, error) {
	var properties uint64
	for {
		tag, err := d.peekTag()
		if err != nil {
			return 0, err
		}
		if tag == end {
			d.r.at++
			return properties, nil
		}
		if _, err := d.propertyKey(depth); err != nil {
			return 0, err
		}
		if _, err := d.value(depth + 1); err != nil {
			return 0, err
		}
		properties++
	}
}

// endCounts reads the two counts after an array's end tag, the
// properties written and the length, and checks them against what was
// read, as V8 does.
func (d *v8Decoder) endCounts(properties, length uint64) error {
	wroteProperties, err := d.r.uvarint()
	if err != nil {
		return err
	}
	wroteLength, err := d.r.uvarint()
	if err != nil {
		return err
	}
	if wroteProperties != properties || wroteLength != length {
		return fmt.Errorf("v8: array ends with %d properties and length %d, read %d and %d", wroteProperties, wroteLength, properties, length)
	}
	return nil
}

func latin1(raw []byte) string {
	runes := make([]rune, len(raw))
	for i, b := range raw {
		runes[i] = rune(b)
	}
	return string(runes)
}

func utf16LE(raw []byte) (string, error) {
	if len(raw)%2 != 0 {
		return "", fmt.Errorf("v8: two-byte string of %d bytes", len(raw))
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
	}
	return string(utf16.Decode(units)), nil
}
