package document

import (
	"encoding/binary"
	"errors"
	"math"
	"strconv"
	"strings"
)

// unmarshalPython reads a value in the format of Python's marshal module, as
// Grist keeps what SQLite has no type for: None, booleans, numbers, strings,
// lists and tuples as []any, and dictionaries as their pairs in order.
func unmarshalPython(data []byte) (any, error) {
	r := &pythonReader{data: data}
	v, err := r.value(0)
	if err != nil {
		return nil, err
	}
	if _, end := v.(pythonEnd); end || r.at != len(r.data) {
		return nil, errPython
	}
	return v, nil
}

var errPython = errors.New("not a marshalled value")

// pythonEnd closes a dictionary.
type pythonEnd struct{}

type pythonReader struct {
	data     []byte
	at       int
	interned []string
}

func (r *pythonReader) take(n int) ([]byte, error) {
	if n < 0 || n > len(r.data)-r.at {
		return nil, errPython
	}
	b := r.data[r.at : r.at+n]
	r.at += n
	return b, nil
}

func (r *pythonReader) count(wide bool) (int, error) {
	if !wide {
		b, err := r.take(1)
		if err != nil {
			return 0, err
		}
		return int(b[0]), nil
	}
	b, err := r.take(4)
	if err != nil {
		return 0, err
	}
	return int(int32(binary.LittleEndian.Uint32(b))), nil
}

func (r *pythonReader) text(wide bool) (string, error) {
	n, err := r.count(wide)
	if err != nil {
		return "", err
	}
	b, err := r.take(n)
	return strings.ToValidUTF8(string(b), "�"), err
}

func (r *pythonReader) value(depth int) (any, error) {
	if depth > 32 {
		return nil, errPython
	}
	b, err := r.take(1)
	if err != nil {
		return nil, err
	}
	// The high bit marks a value later ones may refer to; Grist's are whole.
	switch kind := b[0] &^ 0x80; kind {
	case '0':
		return pythonEnd{}, nil
	case 'N':
		return nil, nil
	case 'T':
		return true, nil
	case 'F':
		return false, nil
	case 'i':
		n, err := r.count(true)
		return int64(n), err
	case 'I':
		b, err := r.take(8)
		if err != nil {
			return nil, err
		}
		return int64(binary.LittleEndian.Uint64(b)), nil
	case 'l':
		// A long integer, in digits of fifteen bits, the least first.
		n, err := r.count(true)
		if err != nil {
			return nil, err
		}
		negative := n < 0
		if negative {
			n = -n
		}
		b, err := r.take(2 * n)
		if err != nil {
			return nil, err
		}
		f := 0.0
		for i := n - 1; i >= 0; i-- {
			f = f*32768 + float64(binary.LittleEndian.Uint16(b[2*i:]))
		}
		if negative {
			f = -f
		}
		if math.Abs(f) < 1<<53 {
			return int64(f), nil
		}
		return f, nil
	case 'g':
		b, err := r.take(8)
		if err != nil {
			return nil, err
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(b)), nil
	case 'f':
		s, err := r.text(false)
		if err != nil {
			return nil, err
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, errPython
		}
		return f, nil
	case 'u', 's', 'a', 'A':
		return r.text(true)
	case 'z', 'Z':
		return r.text(false)
	case 't':
		s, err := r.text(true)
		r.interned = append(r.interned, s)
		return s, err
	case 'R':
		i, err := r.count(true)
		if err != nil || i < 0 || i >= len(r.interned) {
			return nil, errPython
		}
		return r.interned[i], nil
	case '[', '(', ')':
		n, err := r.count(kind != ')')
		if err != nil || n < 0 || n > len(r.data)-r.at {
			return nil, errPython
		}
		list := make([]any, n)
		for i := range list {
			if list[i], err = r.value(depth + 1); err != nil {
				return nil, err
			}
			if _, end := list[i].(pythonEnd); end {
				return nil, errPython
			}
		}
		return list, nil
	case '{':
		pairs := [][2]any{}
		for {
			key, err := r.value(depth + 1)
			if err != nil {
				return nil, err
			}
			if _, end := key.(pythonEnd); end {
				return pairs, nil
			}
			item, err := r.value(depth + 1)
			if err != nil {
				return nil, err
			}
			if _, end := item.(pythonEnd); end {
				return nil, errPython
			}
			pairs = append(pairs, [2]any{key, item})
		}
	}
	return nil, errPython
}
