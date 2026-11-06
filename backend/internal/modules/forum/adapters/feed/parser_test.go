package feed

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "testdata", "forum-sources")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("testdata directory not found at %s: %v", path, err)
	}
	return path
}

func TestFeedParser_ParseRSS2Valid(t *testing.T) {
	dir := testdataDir(t)
	raw, err := os.ReadFile(filepath.Join(dir, "rss2_sample_valid.xml"))
	if err != nil {
		t.Fatalf("read sample rss: %v", err)
	}

	parser := NewFeedParser()
	now := time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC)
	items, err := parser.Parse(raw, now)
	if err != nil {
		t.Fatalf("parse valid rss: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// Verify item 1
	item1 := items[0]
	if item1.Title != "Preço médio do etanol hidratado registra recuo em dezessete estados" {
		t.Errorf("unexpected title: %q", item1.Title)
	}
	if item1.Link != "https://agenciabrasil.ebc.com.br/economia/noticia/2026-11/preco-medio-do-etanol-recua" {
		t.Errorf("unexpected link: %q", item1.Link)
	}
	if item1.GUID != "https://agenciabrasil.ebc.com.br/economia/noticia/2026-11/preco-medio-do-etanol-recua" {
		t.Errorf("unexpected guid: %q", item1.GUID)
	}
	if !strings.Contains(item1.Excerpt, "Levantamento semanal aponta estabilidade") {
		t.Errorf("unexpected excerpt: %q", item1.Excerpt)
	}
	expectedDate := time.Date(2026, 11, 5, 10, 30, 0, 0, time.FixedZone("", -3*3600)).UTC()
	if !item1.PublishedAt.Equal(expectedDate) {
		t.Errorf("expected date %v, got %v", expectedDate, item1.PublishedAt)
	}
	if len(item1.Categories) != 1 || item1.Categories[0] != "Economia" {
		t.Errorf("unexpected categories: %+v", item1.Categories)
	}

	// Verify item 2
	item2 := items[1]
	if item2.GUID != "ebc-noticia-biocombustiveis-20261104" {
		t.Errorf("unexpected guid for item 2: %q", item2.GUID)
	}
	if len(item2.Categories) != 1 || item2.Categories[0] != "Energia" {
		t.Errorf("unexpected categories for item 2: %+v", item2.Categories)
	}
}

func TestFeedParser_ParseAtom1Valid(t *testing.T) {
	dir := testdataDir(t)
	raw, err := os.ReadFile(filepath.Join(dir, "atom_sample_valid.xml"))
	if err != nil {
		t.Fatalf("read sample atom: %v", err)
	}

	parser := NewFeedParser()
	now := time.Date(2026, 11, 5, 13, 0, 0, 0, time.UTC)
	items, err := parser.Parse(raw, now)
	if err != nil {
		t.Fatalf("parse valid atom: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	entry := items[0]
	if entry.Title != "Obras de duplicação em trecho crítico da BR-163 são concluídas" {
		t.Errorf("unexpected title: %q", entry.Title)
	}
	if entry.Link != "https://transportes.gov.br/noticias/obras-duplicacao-br163" {
		t.Errorf("unexpected link: %q", entry.Link)
	}
	if entry.GUID != "urn:uuid:1225c695-cfb8-4ebb-aaaa-80da344efa6a" {
		t.Errorf("unexpected guid: %q", entry.GUID)
	}
	if !strings.Contains(entry.Excerpt, "Entrega de novo segmento rodoviário") {
		t.Errorf("unexpected excerpt: %q", entry.Excerpt)
	}
	expectedDate := time.Date(2026, 11, 5, 9, 15, 0, 0, time.UTC)
	if !entry.PublishedAt.Equal(expectedDate) {
		t.Errorf("expected date %v, got %v", expectedDate, entry.PublishedAt)
	}
}

func TestFeedParser_Security_RefusesXXEAndEntities(t *testing.T) {
	dir := testdataDir(t)
	raw, err := os.ReadFile(filepath.Join(dir, "malformed_entity_expansion.xml"))
	if err != nil {
		t.Fatalf("read malformed vector: %v", err)
	}

	parser := NewFeedParser()
	now := time.Now().UTC()
	_, err = parser.Parse(raw, now)
	if !errors.Is(err, domain.ErrFeedSecurityRefusal) {
		t.Fatalf("expected ErrFeedSecurityRefusal, got %v", err)
	}
}

func TestFeedParser_Security_RefusesExcessiveDepth(t *testing.T) {
	// Construct XML with nesting depth > 32
	var buf bytes.Buffer
	buf.WriteString("<rss version=\"2.0\"><channel><title>Deep</title>")
	for i := 0; i < 35; i++ {
		buf.WriteString("<nested>")
	}
	buf.WriteString("deep content")
	for i := 0; i < 35; i++ {
		buf.WriteString("</nested>")
	}
	buf.WriteString("</channel></rss>")

	parser := NewFeedParser()
	now := time.Now().UTC()
	_, err := parser.Parse(buf.Bytes(), now)
	if !errors.Is(err, domain.ErrFeedSecurityRefusal) {
		t.Fatalf("expected ErrFeedSecurityRefusal for depth > 32, got %v", err)
	}
}

func TestFeedParser_Security_RefusesOversizedBody(t *testing.T) {
	oversized := make([]byte, MaxFeedBodyBytes+10)

	parser := NewFeedParser()
	now := time.Now().UTC()
	_, err := parser.Parse(oversized, now)
	if !errors.Is(err, domain.ErrFeedOversized) {
		t.Fatalf("expected ErrFeedOversized, got %v", err)
	}
}

func TestFeedParser_Security_RefusesUnsupportedRoot(t *testing.T) {
	htmlDoc := []byte("<!DOCTYPE html><html><body><h1>Not a feed</h1></body></html>")

	parser := NewFeedParser()
	now := time.Now().UTC()
	// Has DOCTYPE -> rejected
	_, err := parser.Parse(htmlDoc, now)
	if !errors.Is(err, domain.ErrFeedSecurityRefusal) {
		t.Fatalf("expected ErrFeedSecurityRefusal, got %v", err)
	}

	noDocTypeHTML := []byte("<html><body><h1>Not a feed</h1></body></html>")
	_, err = parser.Parse(noDocTypeHTML, now)
	if !errors.Is(err, domain.ErrFeedMalformed) {
		t.Fatalf("expected ErrFeedMalformed for html, got %v", err)
	}
}

func TestFeedParser_SanitizationAndLimits(t *testing.T) {
	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Feed Teste</title>
    <link>https://example.com</link>
    <item>
      <title><![CDATA[ <b>Notícia</b> com <a href="#">HTML</a> &amp; entidades ]]></title>
      <link>https://example.com/noticia-1</link>
      <description><![CDATA[ <p>Parágrafo 1</p><br/>Texto com <strong>negrito</strong> &quot;aspas&quot;. ]]></description>
      <pubDate>Thu, 05 Nov 2026 12:00:00 -0300</pubDate>
    </item>
    <item>
      <title>Link Inseguro HTTP</title>
      <link>http://insecure.example.com/noticia</link>
      <description>Deve ser descartado por não ser HTTPS</description>
      <pubDate>Thu, 05 Nov 2026 12:00:00 -0300</pubDate>
    </item>
    <item>
      <title>Link com Credenciais</title>
      <link>https://user:pass@example.com/noticia</link>
      <description>Deve ser descartado por conter credenciais</description>
      <pubDate>Thu, 05 Nov 2026 12:00:00 -0300</pubDate>
    </item>
    <item>
      <title>Data no Futuro Longínquo</title>
      <link>https://example.com/noticia-futura</link>
      <description>Timestamp muito à frente do relógio</description>
      <pubDate>Fri, 06 Nov 2026 12:00:00 -0300</pubDate>
    </item>
    <item>
      <title>Sem Data Válida</title>
      <link>https://example.com/sem-data</link>
      <description>Data malformada</description>
      <pubDate>Data Invalida</pubDate>
    </item>
  </channel>
</rss>`

	parser := NewFeedParser()
	now := time.Date(2026, 11, 5, 16, 0, 0, 0, time.UTC)
	items, err := parser.Parse([]byte(xmlContent), now)
	if err != nil {
		t.Fatalf("parse feed: %v", err)
	}

	// Only item 1 should be valid (others fail link HTTPS, credentials, future skew, date parsing)
	if len(items) != 1 {
		t.Fatalf("expected exactly 1 sanitized valid item, got %d", len(items))
	}

	item := items[0]
	if item.Title != "Notícia com HTML & entidades" {
		t.Errorf("title not cleaned properly: %q", item.Title)
	}
	if item.Excerpt != "Parágrafo 1 Texto com negrito \"aspas\"." {
		t.Errorf("excerpt not cleaned properly: %q", item.Excerpt)
	}
	if item.GUID != "https://example.com/noticia-1" {
		t.Errorf("guid should fall back to link: %q", item.GUID)
	}
}
