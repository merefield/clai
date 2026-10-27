package shellintegration

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"unicode/utf16"
	"unicode/utf8"
)

type textEncoding int

const (
	utf8Text textEncoding = iota
	utf8BOM
	utf16LE
	utf16BE
)

// Windows PowerShell profiles commonly use UTF-16. Preserve their encoding
// instead of appending UTF-8 bytes to an existing UTF-16 script.
func decodeText(data []byte) (string, textEncoding, error) {
	encoding := utf8Text
	if bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		encoding, data = utf8BOM, data[3:]
	} else if bytes.HasPrefix(data, []byte{0xff, 0xfe}) {
		encoding, data = utf16LE, data[2:]
	} else if bytes.HasPrefix(data, []byte{0xfe, 0xff}) {
		encoding, data = utf16BE, data[2:]
	}
	if encoding == utf16LE || encoding == utf16BE {
		if len(data)%2 != 0 {
			return "", encoding, fmt.Errorf("shell profile has incomplete UTF-16 text")
		}
		var order binary.ByteOrder = binary.LittleEndian
		if encoding == utf16BE {
			order = binary.BigEndian
		}
		units := make([]uint16, len(data)/2)
		for index := range units {
			units[index] = order.Uint16(data[index*2:])
		}
		return string(utf16.Decode(units)), encoding, nil
	}
	if !utf8.Valid(data) {
		return "", encoding, fmt.Errorf("shell profile must use UTF-8 or UTF-16 with a byte-order mark")
	}
	return string(data), encoding, nil
}

func (encoding textEncoding) encode(content string) []byte {
	if encoding == utf16LE || encoding == utf16BE {
		var order binary.ByteOrder = binary.LittleEndian
		data := []byte{0xff, 0xfe}
		if encoding == utf16BE {
			order, data = binary.BigEndian, []byte{0xfe, 0xff}
		}
		for _, unit := range utf16.Encode([]rune(content)) {
			data = append(data, 0, 0)
			order.PutUint16(data[len(data)-2:], unit)
		}
		return data
	}
	if encoding == utf8BOM {
		return append([]byte{0xef, 0xbb, 0xbf}, []byte(content)...)
	}
	return []byte(content)
}
