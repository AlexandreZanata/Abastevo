package adapters

import "bytes"

// Active-content rules (BR-PRIV-020, EMP-DOC-014, EDR-169). A receipt photo
// needs no markup, script, entity declaration or embedded document, archive
// or executable. Markup tokens match ASCII case-insensitively. Every
// signature is looked for in the metadata segments before the first scan,
// where markup polyglots live; over the entropy-coded stream only markup
// tokens of at least six bytes are looked for, because shorter ones occur in
// honest compressed pixels by chance.
var (
	streamMarkup = [][]byte{[]byte("<script"), []byte("<iframe"), []byte("<object"), []byte("<embed"), []byte("<applet"),
		[]byte("<!doctype"), []byte("<!entity"), []byte("javascript:"), []byte("vbscript:")}
	metadataMarkup = [][]byte{[]byte("<html"), []byte("<body"), []byte("<svg"), []byte("<?php"), []byte("<meta"), []byte("<form"),
		[]byte("<link"), []byte("<style"), []byte("<base")}
	containerMagic = [][]byte{[]byte("%PDF-"), []byte("PK\x03\x04"), []byte("\x7fELF"), []byte("MZ\x90\x00"), []byte("Rar!\x1a\x07"),
		[]byte("7z\xbc\xaf\x27\x1c")}
)

// activeContent reports a refused signature inside a structurally valid JPEG.
// It reads the original only; the folded copy never leaves this function.
func activeContent(raw []byte) bool {
	folded := make([]byte, len(raw))
	for i, c := range raw {
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		folded[i] = c
	}
	end := metadataEnd(raw)
	for _, token := range streamMarkup {
		if bytes.Contains(folded, token) {
			return true
		}
	}
	for _, token := range metadataMarkup {
		if bytes.Contains(folded[:end], token) {
			return true
		}
	}
	for _, magic := range containerMagic {
		if bytes.Contains(raw[:end], magic) {
			return true
		}
	}
	return false
}

// metadataEnd returns the offset of the first start-of-scan marker: every
// earlier byte is SOI or a length-delimited marker segment. The caller has
// validated the structure already; an unexpected layout conservatively
// treats the whole image as metadata.
func metadataEnd(raw []byte) int {
	i := 2
	for i < len(raw) {
		if raw[i] != 0xff {
			return len(raw)
		}
		for i < len(raw) && raw[i] == 0xff {
			i++
		}
		if i >= len(raw) {
			return len(raw)
		}
		switch marker := raw[i]; {
		case marker == 0xda:
			return i - 1
		case marker == 0xd8 || marker == 0x01 || (marker >= 0xd0 && marker <= 0xd7):
			i++
			continue
		case marker == 0xd9:
			return len(raw)
		}
		if i+2 >= len(raw) {
			return len(raw)
		}
		length := int(raw[i+1])<<8 | int(raw[i+2])
		if length < 2 {
			return len(raw)
		}
		i += 1 + length
	}
	return len(raw)
}
