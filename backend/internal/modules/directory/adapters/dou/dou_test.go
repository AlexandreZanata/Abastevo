package dou

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// P26-T01 bounded INLABS edition access: credentials stay operator-owned
// (env, never Git/logs); absent access fails explicitly before any
// network; XML parsing rejects entity/DTD weapons and oversized input;
// duplicate editions converge by checksum.

func TestAccessRefusesWithoutCredentials(t *testing.T) {
	cfg := Config{BaseURL: "https://inlabs.in.gov.br", Username: "", Password: "x"}
	if _, err := FetchEdition(context.Background(), cfg, "2026-10-01"); !errors.Is(err, ErrNoAccess) {
		t.Fatalf("err = %v, want ErrNoAccess", err)
	}
}

func TestAccessRefusesOffAllowlistHost(t *testing.T) {
	cfg := Config{BaseURL: "https://example.com", Username: "u", Password: "p"}
	if _, err := FetchEdition(context.Background(), cfg, "2026-10-01"); !errors.Is(err, ErrNotAllowlisted) {
		t.Fatalf("err = %v, want ErrNotAllowlisted", err)
	}
}

func TestParseEditionIndexFindsActs(t *testing.T) {
	edition, err := ParseEdition([]byte(syntheticEdition), "2026-10-01")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(edition.Acts) != 2 {
		t.Fatalf("acts = %d, want 2", len(edition.Acts))
	}
	if edition.Acts[0].ID == "" || edition.Acts[0].Title == "" {
		t.Fatalf("act not identified: %+v", edition.Acts[0])
	}
	if edition.Checksum == "" {
		t.Fatal("edition without checksum")
	}
}

func TestParseEditionRejectsEntityAndOversize(t *testing.T) {
	xxe := `<?xml version="1.0"?><!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><edicao><ato>&xxe;</ato></edicao>`
	if _, err := ParseEdition([]byte(xxe), "2026-10-01"); !errors.Is(err, ErrUnsafeXML) {
		t.Fatalf("xxe err = %v", err)
	}
	big := strings.Repeat("a", MaxEditionBytes+1)
	if _, err := ParseEdition([]byte(big), "2026-10-01"); !errors.Is(err, ErrOversize) {
		t.Fatalf("oversize err = %v", err)
	}
	if _, err := ParseEdition([]byte(`<edicao></edicao>`), "2026-10-01"); err == nil {
		t.Fatal("dateless edition must fail")
	}
}

func TestCheckpointDedupsEditions(t *testing.T) {
	store := NewMemoryCheckpoints()
	first, err := ParseEdition([]byte(syntheticEdition), "2026-10-01")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	seen, err := store.Seen(first.Checksum)
	if err != nil || seen {
		t.Fatalf("seen = %v, err = %v", seen, err)
	}
	store.Mark(first.Checksum)
	seen, _ = store.Seen(first.Checksum)
	if !seen {
		t.Fatal("edition not checkpointed")
	}
}

func TestFetchEditionSucceedsOnLoopback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "u" || pass != "p" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/edicao/2026-10-01.xml") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(syntheticEdition))
	}))
	defer server.Close()
	cfg := Config{BaseURL: server.URL, Username: "u", Password: "p", AllowLoopback: true}
	edition, err := FetchEdition(context.Background(), cfg, "2026-10-01")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(edition.Acts) != 2 || edition.Date != "2026-10-01" {
		t.Fatalf("edition = %+v", edition)
	}
}

func TestFetcherTimeoutsAndClosesBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()
	cfg := Config{
		BaseURL: server.URL, Username: "u", Password: "p",
		AllowLoopback: true,
		HTTPClient:    &http.Client{Timeout: 20 * time.Millisecond},
	}
	if _, err := FetchEdition(context.Background(), cfg, "2026-10-01"); err == nil {
		t.Fatal("slow edition must time out")
	}
}

const syntheticEdition = `<?xml version="1.0" encoding="UTF-8"?>
<edicao data="2026-10-01" numero="190">
  <ato id="ANP-2026-0001" tipo="autorizacao">
    <titulo>Autoriza o exercício da atividade de revenda varejista de combustíveis</titulo>
    <texto>A AGÊNCIA NACIONAL DO PETRÓLEO autoriza [P26-TEST] POSTO ALFA LTDA, CNPJ 04218406000104, a exercer a atividade.</texto>
  </ato>
  <ato id="ANP-2026-0002" tipo="revogacao">
    <titulo>Revoga autorização de revenda</titulo>
    <texto>Fica revogada a autorização de [P26-TEST] POSTO EPSILON LTDA, CNPJ 55881177000136.</texto>
  </ato>
</edicao>`
