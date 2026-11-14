package portalgen

import (
	"reflect"
	"strings"
	"testing"
)

func heading(text string) forumBlock     { return forumBlock{Kind: blockHeading, Text: text} }
func paragraph(text string) forumBlock   { return forumBlock{Kind: blockParagraph, Text: text} }
func bullets(items ...string) forumBlock { return forumBlock{Kind: blockBullets, Items: items} }

// TestParseForumBodyMatchesAndroid mirrors app/.../ForumBodyParserTest.kt
// case by case (first three rows) and pins the edge cases of the Kotlin
// rules: blank-line split (\n\s*\n with Java's ASCII \s), Kotlin trim
// (Char.isWhitespace), lines() on \r too, the 80 UTF-16-unit heading bound
// and the .:;,!?… final-character exclusion.
func TestParseForumBodyMatchesAndroid(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []forumBlock
	}{
		{"android: heading and bullets",
			"Intro paragraph.\n\nA regra prática dos 70%\n\n- Até 0,70: etanol.\n- Acima: gasolina.\n\nFinal text.",
			[]forumBlock{paragraph("Intro paragraph."), heading("A regra prática dos 70%"), bullets("Até 0,70: etanol.", "Acima: gasolina."), paragraph("Final text.")}},
		{"android: punctuated or multi-line blocks stay paragraphs",
			"Ends with colon:\n\nLine one\nline two\n\n" + strings.Repeat("x", 90),
			[]forumBlock{paragraph("Ends with colon:"), paragraph("Line one\nline two"), paragraph(strings.Repeat("x", 90))}},
		{"android: mixed list and text block is a paragraph",
			"Steps:\n- one\nnot a bullet",
			[]forumBlock{paragraph("Steps:\n- one\nnot a bullet")}},
		{"empty body", "", nil},
		{"whitespace only", " \n\t\n   ", nil},
		{"crlf line endings", "Título\r\n\r\nTexto final.", []forumBlock{heading("Título"), paragraph("Texto final.")}},
		{"blank line with spaces splits", "Um.\n  \t \nDois.", []forumBlock{paragraph("Um."), paragraph("Dois.")}},
		{"nbsp-only line does not split", "Um\n \nDois", []forumBlock{paragraph("Um\n \nDois")}},
		{"lone carriage return is a line break", "- a\r- b", []forumBlock{bullets("a", "b")}},
		{"bullet items are trimmed", "-  a \n - b", []forumBlock{bullets("a", "b")}},
		{"dash without space is not a bullet", "-a", []forumBlock{heading("-a")}},
		{"bullets ignore blank inner lines", "- a\n   \n- b", []forumBlock{bullets("a"), bullets("b")}},
		{"ellipsis ends a sentence", "Continua…", []forumBlock{paragraph("Continua…")}},
		{"each final punctuation", "a.\n\nb:\n\nc;\n\nd,\n\ne!\n\nf?",
			[]forumBlock{paragraph("a."), paragraph("b:"), paragraph("c;"), paragraph("d,"), paragraph("e!"), paragraph("f?")}},
		{"other final characters stay headings", "Preço em R$\n\nQuanto custa (2026)\n\n\"Citação\"",
			[]forumBlock{heading("Preço em R$"), heading("Quanto custa (2026)"), heading("\"Citação\"")}},
		{"80 UTF-16 units is a heading", strings.Repeat("a", 80), []forumBlock{heading(strings.Repeat("a", 80))}},
		{"81 UTF-16 units is a paragraph", strings.Repeat("a", 81), []forumBlock{paragraph(strings.Repeat("a", 81))}},
		{"accented runes count once", strings.Repeat("é", 80), []forumBlock{heading(strings.Repeat("é", 80))}},
		{"astral runes count twice", strings.Repeat("🚗", 40), []forumBlock{heading(strings.Repeat("🚗", 40))}},
		{"astral runes over the bound", strings.Repeat("🚗", 40) + "a", []forumBlock{paragraph(strings.Repeat("🚗", 40) + "a")}},
		{"next line is not a blank-line separator", "Um\u0085\u0085Dois.", []forumBlock{paragraph("Um\u0085\u0085Dois.")}},
		{"kotlin trims information separators", "\u001fTítulo\u001c", []forumBlock{heading("Título")}},
		{"crlf left after cr cr lf", "- a\r\r\n- b", []forumBlock{bullets("a", "b")}},
		{"surrounding blank lines dropped", "\n\n\nSó um parágrafo.\n\n\n", []forumBlock{paragraph("Só um parágrafo.")}},
		{"paragraph keeps inner indentation", "Linha um\n   recuada.", []forumBlock{paragraph("Linha um\n   recuada.")}},
		{"vertical tab counts as blank-line space", "Um.\n\v\nDois.", []forumBlock{paragraph("Um."), paragraph("Dois.")}},
		{"html stays text", "<script>alert(1)</script>\n\n- <b>x</b>", []forumBlock{heading("<script>alert(1)</script>"), bullets("<b>x</b>")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseForumBody(tc.body)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseForumBody(%q)\n got %#v\nwant %#v", tc.body, got, tc.want)
			}
		})
	}
}

func TestForumParagraphLinesSplitOnNewlines(t *testing.T) {
	if got := paragraph("Linha um\nlinha dois").Lines(); !reflect.DeepEqual(got, []string{"Linha um", "linha dois"}) {
		t.Fatalf("lines = %#v", got)
	}
}

func TestForumPlainTextFlattensBlocks(t *testing.T) {
	blocks := parseForumBody("Introdução.\n\nRegra\n\n- um\n- dois\n\nFim\ncom quebra.")
	if got := forumPlainText(blocks); got != "Introdução. Regra. um dois Fim com quebra." {
		t.Fatalf("plain text = %q", got)
	}
}
