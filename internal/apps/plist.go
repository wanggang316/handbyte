package apps

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"os"
	"strings"
	"unicode/utf16"
)

// readInfo returns the string values of the top-level dictionary of a
// property list file, in XML or in binary form. Other value types are left
// out: an Info.plist needs only its strings here.
func readInfo(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(data, []byte("bplist00")) {
		return readBinaryPlist(data)
	}
	return readXMLPlist(data)
}

func readXMLPlist(data []byte) (map[string]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	out := map[string]string{}
	depth := 0 // depth of <dict> elements
	key := ""
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "dict" || t.Name.Local == "array" {
				depth++
			}
			text.Reset()
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			switch t.Name.Local {
			case "dict", "array":
				depth--
				key = ""
			case "key":
				if depth == 1 {
					key = text.String()
				}
			case "string":
				if depth == 1 && key != "" {
					out[key] = text.String()
				}
				key = ""
			default:
				if depth == 1 {
					key = ""
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no strings in property list")
	}
	return out, nil
}

// readBinaryPlist reads the top-level dictionary of a "bplist00" file.
func readBinaryPlist(data []byte) (map[string]string, error) {
	if len(data) < 40 {
		return nil, errors.New("binary property list too short")
	}
	trailer := data[len(data)-32:]
	offsetSize := int(trailer[6])
	refSize := int(trailer[7])
	numObjects := binary.BigEndian.Uint64(trailer[8:16])
	topObject := binary.BigEndian.Uint64(trailer[16:24])
	tableOffset := binary.BigEndian.Uint64(trailer[24:32])
	if offsetSize < 1 || offsetSize > 8 || refSize < 1 || refSize > 8 || numObjects > math.MaxInt32 ||
		tableOffset+numObjects*uint64(offsetSize) > uint64(len(data)) {
		return nil, errors.New("binary property list trailer is not valid")
	}
	readUint := func(b []byte) uint64 {
		var v uint64
		for _, c := range b {
			v = v<<8 | uint64(c)
		}
		return v
	}
	offset := func(ref uint64) (int, error) {
		if ref >= numObjects {
			return 0, errors.New("object reference out of range")
		}
		start := tableOffset + ref*uint64(offsetSize)
		o := readUint(data[start : start+uint64(offsetSize)])
		if o >= uint64(len(data)) {
			return 0, errors.New("object offset out of range")
		}
		return int(o), nil
	}
	// length reads the length of an object whose marker low nibble is 0xF.
	length := func(pos int) (int, int, error) {
		n := int(data[pos] & 0x0F)
		pos++
		if n != 0x0F {
			return n, pos, nil
		}
		if pos >= len(data) || data[pos]&0xF0 != 0x10 {
			return 0, 0, errors.New("bad length marker")
		}
		size := 1 << (data[pos] & 0x0F)
		if pos+1+size > len(data) {
			return 0, 0, errors.New("length out of range")
		}
		v := readUint(data[pos+1 : pos+1+size])
		if v > uint64(len(data)) {
			return 0, 0, errors.New("length out of range")
		}
		return int(v), pos + 1 + size, nil
	}
	str := func(ref uint64) (string, bool) {
		pos, err := offset(ref)
		if err != nil {
			return "", false
		}
		kind := data[pos] >> 4
		if kind != 0x5 && kind != 0x6 {
			return "", false
		}
		n, start, err := length(pos)
		if err != nil {
			return "", false
		}
		if kind == 0x5 {
			if start+n > len(data) {
				return "", false
			}
			return string(data[start : start+n]), true
		}
		if start+2*n > len(data) {
			return "", false
		}
		units := make([]uint16, n)
		for i := range units {
			units[i] = binary.BigEndian.Uint16(data[start+2*i:])
		}
		return string(utf16.Decode(units)), true
	}
	pos, err := offset(topObject)
	if err != nil {
		return nil, err
	}
	if data[pos]>>4 != 0xD {
		return nil, errors.New("top object is not a dictionary")
	}
	n, start, err := length(pos)
	if err != nil {
		return nil, err
	}
	if start+2*n*refSize > len(data) {
		return nil, errors.New("dictionary out of range")
	}
	out := map[string]string{}
	for i := 0; i < n; i++ {
		k := readUint(data[start+i*refSize : start+(i+1)*refSize])
		v := readUint(data[start+(n+i)*refSize : start+(n+i+1)*refSize])
		key, ok := str(k)
		if !ok {
			continue
		}
		if value, ok := str(v); ok {
			out[key] = value
		}
	}
	return out, nil
}
