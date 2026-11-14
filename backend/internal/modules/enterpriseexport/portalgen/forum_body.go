package portalgen

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf16"
)

// forumHeadingMaxUnits is Android's HEADING_MAX_CHARS, a Kotlin String.length
// bound: UTF-16 code units, not runes.
const forumHeadingMaxUnits = 80

type forumBlockKind string

const (
	blockHeading   forumBlockKind = "heading"
	blockParagraph forumBlockKind = "paragraph"
	blockBullets   forumBlockKind = "bullets"
)

// forumBlock is one block of a plain-text editorial body (B-BR-SEO-08).
type forumBlock struct {
	Kind  forumBlockKind
	Text  string
	Items []string
}

// Lines splits a paragraph at its preserved line breaks for <br> rendering.
func (b forumBlock) Lines() []string {
	return strings.Split(b.Text, "\n")
}

// forumBlankLine is Kotlin's Regex("\n\\s*\n"); Java's \s is ASCII
// [ \t\n\x0B\f\r], which Go's \s lacks \x0B from.
var forumBlankLine = regexp.MustCompile("\n[ \t\n\x0B\f\r]*\n")

// parseForumBody mirrors parseForumBody in
// app/src/main/kotlin/com/anpfuel/app/ui/forum/ForumSocial.kt: split on blank
// lines; a block whose non-blank lines all start with "- " is a bullet list;
// a single-line block of at most 80 UTF-16 units not ending in .:;,!?… is a
// subheading; anything else is a paragraph. The text itself is never changed
// and is always rendered escaped.
func parseForumBody(body string) []forumBlock {
	var out []forumBlock
	for _, raw := range forumBlankLine.Split(strings.ReplaceAll(body, "\r\n", "\n"), -1) {
		block := kotlinTrim(raw)
		if block == "" {
			continue
		}
		var lines []string
		for _, line := range kotlinLines(block) {
			if line = kotlinTrim(line); line != "" {
				lines = append(lines, line)
			}
		}
		switch {
		case allBullets(lines):
			items := make([]string, 0, len(lines))
			for _, line := range lines {
				items = append(items, kotlinTrim(strings.TrimPrefix(line, "- ")))
			}
			out = append(out, forumBlock{Kind: blockBullets, Items: items})
		case len(lines) == 1 && utf16Len(block) <= forumHeadingMaxUnits && !strings.ContainsRune(".:;,!?…", lastRune(block)):
			out = append(out, forumBlock{Kind: blockHeading, Text: block})
		default:
			out = append(out, forumBlock{Kind: blockParagraph, Text: block})
		}
	}
	return out
}

func allBullets(lines []string) bool {
	for _, line := range lines {
		if !strings.HasPrefix(line, "- ") {
			return false
		}
	}
	return true
}

// kotlinWhitespace is Kotlin/JVM Char.isWhitespace: Character.isWhitespace
// or Character.isSpaceChar (Zs/Zl/Zp, the ASCII controls \t\n\v\f\r and the
// information separators U+001C–U+001F; not U+0085).
func kotlinWhitespace(r rune) bool {
	switch {
	case r == '\t', r == '\n', r == '\v', r == '\f', r == '\r', r >= 0x1C && r <= 0x1F:
		return true
	}
	return unicode.In(r, unicode.Zs, unicode.Zl, unicode.Zp)
}

func kotlinTrim(value string) string {
	return strings.TrimFunc(value, kotlinWhitespace)
}

// kotlinLines is CharSequence.lines(): split on \r\n, \n or \r.
func kotlinLines(value string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\n':
			lines = append(lines, value[start:i])
			start = i + 1
		case '\r':
			lines = append(lines, value[start:i])
			if i+1 < len(value) && value[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
	}
	return append(lines, value[start:])
}

func utf16Len(value string) int {
	n := 0
	for _, r := range value {
		n += utf16.RuneLen(r)
	}
	return n
}

func lastRune(value string) rune {
	runes := []rune(value)
	if len(runes) == 0 {
		return 0
	}
	return runes[len(runes)-1]
}

// forumPlainText flattens blocks to one whitespace-collapsed line for
// descriptions and the structured-data text excerpt.
func forumPlainText(blocks []forumBlock) string {
	parts := []string{}
	for _, block := range blocks {
		if block.Kind == blockBullets {
			parts = append(parts, block.Items...)
			continue
		}
		parts = append(parts, block.Text)
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}
