// Package protocol freezes enterprise operation identity without installing intake or authority.
package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"unicode/utf8"
)

var (
	ErrInvalid = errors.New("orgsync: invalid protocol value")
	ErrLimit   = errors.New("orgsync: protocol limit exceeded")
	ErrBinding = errors.New("orgsync: creation binding mismatch")
	ErrDigest  = errors.New("orgsync: immutable digest mismatch")
	ErrReused  = errors.New("orgsync: operation identifier reused")
)

const (
	MaxBatchBytes     = 65536
	MaxOperationBytes = 8192
	MaxOperations     = 50
	MaxTransportDepth = 64
	MaxOperationDepth = 62
)

type redacted struct{}

func (redacted) String() string                        { return "EnterpriseSyncValue(redacted)" }
func (value redacted) Format(writer fmt.State, _ rune) { _, _ = io.WriteString(writer, value.String()) }
func (value redacted) MarshalJSON() ([]byte, error)    { return json.Marshal(value.String()) }
func (value redacted) LogValue() slog.Value            { return slog.StringValue(value.String()) }

type node struct {
	kind       byte
	value      string
	list       []*node
	fields     map[string]*node
	start, end int
}
type parser struct {
	raw           string
	offset, limit int
}

func snake(raw string) bool {
	if len(raw) == 0 || raw[0] < 'a' || raw[0] > 'z' {
		return false
	}
	previousUnderscore := false
	for _, c := range []byte(raw) {
		if c == '_' {
			if previousUnderscore {
				return false
			}
			previousUnderscore = true
			continue
		}
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9') {
			return false
		}
		previousUnderscore = false
	}
	return !previousUnderscore
}
func (p *parser) whitespace() {
	for p.offset < len(p.raw) {
		switch p.raw[p.offset] {
		case ' ', '\t', '\r', '\n':
			p.offset++
		default:
			return
		}
	}
}
func (p *parser) fail() (*node, error) { return nil, ErrInvalid }
func (p *parser) value(depth int) (*node, error) {
	p.whitespace()
	if p.offset >= len(p.raw) {
		return p.fail()
	}
	start := p.offset
	mark := p.raw[p.offset]
	value := &node{start: start}
	switch mark {
	case '{', '[':
		if depth >= p.limit {
			return nil, ErrLimit
		}
		p.offset++
		if mark == '{' {
			value.kind = 'o'
			value.fields = map[string]*node{}
		} else {
			value.kind = 'a'
		}
		end := byte('}')
		if mark == '[' {
			end = ']'
		}
		p.whitespace()
		if p.offset < len(p.raw) && p.raw[p.offset] == end {
			p.offset++
			break
		}
		for {
			key := ""
			if mark == '{' {
				p.whitespace()
				if p.offset >= len(p.raw) || p.raw[p.offset] != '"' {
					return p.fail()
				}
				var err error
				key, err = p.string()
				if err != nil {
					return nil, err
				}
				if !snake(key) {
					return p.fail()
				}
				if _, duplicate := value.fields[key]; duplicate {
					return p.fail()
				}
				p.whitespace()
				if p.offset >= len(p.raw) || p.raw[p.offset] != ':' {
					return p.fail()
				}
				p.offset++
			}
			item, err := p.value(depth + 1)
			if err != nil {
				return nil, err
			}
			if mark == '{' {
				value.fields[key] = item
			} else {
				value.list = append(value.list, item)
			}
			p.whitespace()
			if p.offset >= len(p.raw) {
				return p.fail()
			}
			if p.raw[p.offset] == end {
				p.offset++
				break
			}
			if p.raw[p.offset] != ',' {
				return p.fail()
			}
			p.offset++
		}
	case '"':
		var err error
		value.kind = 's'
		value.value, err = p.string()
		if err != nil {
			return nil, err
		}
	case 'n', 't', 'f':
		literal := "null"
		value.kind = 'n'
		if mark == 't' {
			literal = "true"
			value.kind = 'b'
		}
		if mark == 'f' {
			literal = "false"
			value.kind = 'b'
		}
		if len(p.raw)-p.offset < len(literal) || p.raw[p.offset:p.offset+len(literal)] != literal {
			return p.fail()
		}
		p.offset += len(literal)
		value.value = literal
	default:
		value.kind = 'i'
		begin := p.offset
		if mark == '-' {
			p.offset++
		}
		if p.offset >= len(p.raw) || p.raw[p.offset] < '0' || p.raw[p.offset] > '9' {
			return p.fail()
		}
		if p.raw[p.offset] == '0' {
			p.offset++
		} else {
			for p.offset < len(p.raw) && p.raw[p.offset] >= '0' && p.raw[p.offset] <= '9' {
				p.offset++
			}
		}
		value.value = p.raw[begin:p.offset]
		if value.value == "-0" {
			return p.fail()
		}
	}
	value.end = p.offset
	return value, nil
}
func hexDigit(value byte) (uint16, bool) {
	switch {
	case value >= '0' && value <= '9':
		return uint16(value - '0'), true
	case value >= 'a' && value <= 'f':
		return uint16(value - 'a' + 10), true
	case value >= 'A' && value <= 'F':
		return uint16(value - 'A' + 10), true
	}
	return 0, false
}
func (p *parser) escapedRune() (uint16, error) {
	if p.offset+4 > len(p.raw) {
		return 0, ErrInvalid
	}
	var result uint16
	for range 4 {
		n, ok := hexDigit(p.raw[p.offset])
		if !ok {
			return 0, ErrInvalid
		}
		result = result*16 + n
		p.offset++
	}
	return result, nil
}
func (p *parser) string() (string, error) {
	p.offset++
	var out bytes.Buffer
	for p.offset < len(p.raw) {
		value := p.raw[p.offset]
		p.offset++
		if value == '"' {
			return out.String(), nil
		}
		if value < 0x20 {
			return "", ErrInvalid
		}
		if value != '\\' {
			out.WriteByte(value)
			continue
		}
		if p.offset >= len(p.raw) {
			return "", ErrInvalid
		}
		escape := p.raw[p.offset]
		p.offset++
		switch escape {
		case '"', '\\', '/':
			out.WriteByte(escape)
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'u':
			first, err := p.escapedRune()
			if err != nil {
				return "", err
			}
			point := rune(first)
			if first >= 0xd800 && first <= 0xdbff {
				if p.offset+2 > len(p.raw) || p.raw[p.offset:p.offset+2] != "\\u" {
					return "", ErrInvalid
				}
				p.offset += 2
				second, err := p.escapedRune()
				if err != nil || second < 0xdc00 || second > 0xdfff {
					return "", ErrInvalid
				}
				point = 0x10000 + (rune(first)-0xd800)*0x400 + (rune(second) - 0xdc00)
			} else if first >= 0xdc00 && first <= 0xdfff {
				return "", ErrInvalid
			}
			out.WriteRune(point)
		default:
			return "", ErrInvalid
		}
	}
	return "", ErrInvalid
}
func parseJSON(raw []byte, limit int) (*node, error) {
	if !utf8.Valid(raw) {
		return nil, ErrInvalid
	}
	p := parser{raw: string(raw), limit: limit}
	value, err := p.value(0)
	if err != nil {
		return nil, err
	}
	p.whitespace()
	if p.offset != len(p.raw) {
		return nil, ErrInvalid
	}
	return value, nil
}
func quote(out *bytes.Buffer, value string) {
	const hexadecimal = "0123456789abcdef"
	out.WriteByte('"')
	for _, point := range value {
		switch {
		case point == '"' || point == '\\':
			out.WriteByte('\\')
			out.WriteRune(point)
		case point < 0x20:
			out.WriteString("\\u00")
			out.WriteByte(hexadecimal[byte(point)>>4])
			out.WriteByte(hexadecimal[byte(point)&15])
		default:
			out.WriteRune(point)
		}
	}
	out.WriteByte('"')
}
func writeCanonical(out *bytes.Buffer, value *node) {
	switch value.kind {
	case 'o':
		out.WriteByte('{')
		keys := make([]string, 0, len(value.fields))
		for key := range value.fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for i, key := range keys {
			if i != 0 {
				out.WriteByte(',')
			}
			quote(out, key)
			out.WriteByte(':')
			writeCanonical(out, value.fields[key])
		}
		out.WriteByte('}')
	case 'a':
		out.WriteByte('[')
		for i, item := range value.list {
			if i != 0 {
				out.WriteByte(',')
			}
			writeCanonical(out, item)
		}
		out.WriteByte(']')
	case 's':
		quote(out, value.value)
	default:
		out.WriteString(value.value)
	}
}
func canonical(value *node) []byte {
	var out bytes.Buffer
	writeCanonical(&out, value)
	return out.Bytes()
}

// CanonicalJSON is the small enterprise exact-integer profile, not RFC8785.
func CanonicalJSON(raw []byte) ([]byte, error) {
	if len(raw) > MaxBatchBytes {
		return nil, ErrLimit
	}
	value, err := parseJSON(raw, MaxTransportDepth)
	if err != nil {
		return nil, err
	}
	return canonical(value), nil
}
