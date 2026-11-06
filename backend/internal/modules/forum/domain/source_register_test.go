package domain_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

type sourceRegisterEntry struct {
	ID                  string            `json:"id"`
	Code                string            `json:"code"`
	Name                string            `json:"name"`
	Kind                domain.SourceKind `json:"kind"`
	OriginURL           string            `json:"origin_url"`
	FetchURL            *string           `json:"fetch_url"`
	FetchFormat         *string           `json:"fetch_format"`
	CollectionEnabled   bool              `json:"collection_enabled"`
	PublicationEnabled  bool              `json:"publication_enabled"`
	RightsStatus        string            `json:"rights_status"`
	OperationalStatus   string            `json:"operational_status"`
	TitleAllowed        bool              `json:"title_allowed"`
	ExcerptAllowed      bool              `json:"excerpt_allowed"`
	ImageAllowed        bool              `json:"image_allowed"`
	TermsURL            string            `json:"terms_url"`
	Attribution         string            `json:"attribution"`
	Notes               string            `json:"notes"`
}

type sourceRegisterDoc struct {
	Version                int                   `json:"version"`
	UpdatedAt              string                `json:"updated_at"`
	OperationalDisposition string                `json:"operational_disposition"`
	DispositionRationale   string                `json:"disposition_rationale"`
	Sources                []sourceRegisterEntry `json:"sources"`
}

func testdataDir(t *testing.T) string {
	t.Helper()
	// Relative path from internal/modules/forum/domain
	path := filepath.Join("..", "..", "..", "..", "testdata", "forum-sources")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("testdata directory not found at %s: %v", path, err)
	}
	return path
}

func TestSourceRegister_StructureAndRightsInvariants(t *testing.T) {
	dir := testdataDir(t)
	raw, err := os.ReadFile(filepath.Join(dir, "source_register.json"))
	if err != nil {
		t.Fatalf("read source_register.json: %v", err)
	}

	var reg sourceRegisterDoc
	if err := json.Unmarshal(raw, &reg); err != nil {
		t.Fatalf("unmarshal source_register.json: %v", err)
	}

	if reg.Version < 1 {
		t.Errorf("expected version >= 1, got %d", reg.Version)
	}
	if reg.OperationalDisposition != "NEWS_SOURCE_BLOCKED" {
		t.Errorf("expected operational_disposition NEWS_SOURCE_BLOCKED, got %q", reg.OperationalDisposition)
	}
	if len(reg.Sources) < 5 {
		t.Errorf("expected at least 5 evaluated sources, got %d", len(reg.Sources))
	}

	approvedCount := 0
	for _, s := range reg.Sources {
		// 1. Identification bounds
		if len(s.ID) != 36 {
			t.Errorf("source %s has invalid UUID length: %q", s.Code, s.ID)
		}
		if len(s.Code) < 2 || len(s.Code) > 64 {
			t.Errorf("source %s code length out of bounds [2, 64]", s.Code)
		}
		if len(s.Name) < 2 || len(s.Name) > 120 {
			t.Errorf("source %s name length out of bounds [2, 120]", s.Code)
		}

		// 2. HTTPS-only origin URL
		if !strings.HasPrefix(s.OriginURL, "https://") {
			t.Errorf("source %s origin_url must be HTTPS, got %q", s.Code, s.OriginURL)
		}

		// 3. Kind classification
		switch s.Kind {
		case domain.SourceKindAbastevo, domain.SourceKindGovernment, domain.SourceKindPublisher, domain.SourceKindDataset:
			// OK
		default:
			t.Errorf("source %s has invalid kind %q", s.Code, s.Kind)
		}

		// 4. Rights status check
		switch s.RightsStatus {
		case "APPROVED", "PENDING", "DENIED":
			// OK
		default:
			t.Errorf("source %s has invalid rights_status %q", s.Code, s.RightsStatus)
		}

		// 5. CRITICAL INVARIANT: Only APPROVED sources can have publication_enabled
		if s.RightsStatus != "APPROVED" && s.PublicationEnabled {
			t.Errorf("source %s is %s but has publication_enabled = true (VIOLATION)", s.Code, s.RightsStatus)
		}

		// 6. CRITICAL INVARIANT: NEWS_SOURCE_BLOCKED or BLOCKED sources must not have collection_enabled
		if (s.OperationalStatus == "NEWS_SOURCE_BLOCKED" || s.OperationalStatus == "BLOCKED") && s.CollectionEnabled {
			t.Errorf("source %s is %s but has collection_enabled = true (VIOLATION)", s.Code, s.OperationalStatus)
		}

		// 7. Architectural boundary: Phase 2 text-first MVP permits no third-party images
		if s.ImageAllowed {
			t.Errorf("source %s has image_allowed = true; images are prohibited in text-first Phase 2", s.Code)
		}

		if s.RightsStatus == "APPROVED" {
			approvedCount++
		}
	}

	// In the absence of signed external contracts, only 1 source (Abastevo Editorial) is APPROVED
	if approvedCount != 1 {
		t.Errorf("expected exactly 1 APPROVED source (Abastevo Editorial), got %d", approvedCount)
	}
}

func TestSourceRegister_TestVectorsConformToParseContract(t *testing.T) {
	dir := testdataDir(t)

	// 1. RSS 2.0 valid vector
	rssRaw, err := os.ReadFile(filepath.Join(dir, "rss2_sample_valid.xml"))
	if err != nil {
		t.Fatalf("read rss2_sample_valid.xml: %v", err)
	}
	if len(rssRaw) > 2*1024*1024 {
		t.Errorf("rss fixture exceeds 2 MiB bound: %d bytes", len(rssRaw))
	}
	if bytes.Contains(rssRaw, []byte("<!DOCTYPE")) {
		t.Errorf("rss fixture contains forbidden DOCTYPE")
	}

	var rssDoc struct {
		XMLName xml.Name `xml:"rss"`
		Version string   `xml:"version,attr"`
		Channel struct {
			Title string `xml:"title"`
			Items []struct {
				Title       string `xml:"title"`
				Link        string `xml:"link"`
				Description string `xml:"description"`
				PubDate     string `xml:"pubDate"`
				Guid        string `xml:"guid"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(rssRaw, &rssDoc); err != nil {
		t.Fatalf("unmarshal rss2_sample_valid.xml: %v", err)
	}
	if rssDoc.Version != "2.0" {
		t.Errorf("expected RSS version 2.0, got %q", rssDoc.Version)
	}
	if len(rssDoc.Channel.Items) < 2 {
		t.Errorf("expected at least 2 items in RSS fixture, got %d", len(rssDoc.Channel.Items))
	}

	// 2. Atom valid vector
	atomRaw, err := os.ReadFile(filepath.Join(dir, "atom_sample_valid.xml"))
	if err != nil {
		t.Fatalf("read atom_sample_valid.xml: %v", err)
	}
	if len(atomRaw) > 2*1024*1024 {
		t.Errorf("atom fixture exceeds 2 MiB bound: %d bytes", len(atomRaw))
	}
	if bytes.Contains(atomRaw, []byte("<!DOCTYPE")) {
		t.Errorf("atom fixture contains forbidden DOCTYPE")
	}

	var atomDoc struct {
		XMLName xml.Name `xml:"feed"`
		Title   string   `xml:"title"`
		Entries []struct {
			Title   string `xml:"title"`
			ID      string `xml:"id"`
			Updated string `xml:"updated"`
			Summary string `xml:"summary"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(atomRaw, &atomDoc); err != nil {
		t.Fatalf("unmarshal atom_sample_valid.xml: %v", err)
	}
	if len(atomDoc.Entries) < 1 {
		t.Errorf("expected at least 1 entry in Atom fixture, got %d", len(atomDoc.Entries))
	}

	// 3. Malformed / Hostile DOCTYPE expansion vector
	malformedRaw, err := os.ReadFile(filepath.Join(dir, "malformed_entity_expansion.xml"))
	if err != nil {
		t.Fatalf("read malformed_entity_expansion.xml: %v", err)
	}
	if !bytes.Contains(malformedRaw, []byte("<!DOCTYPE")) {
		t.Errorf("expected malformed vector to contain DOCTYPE for negative test verification")
	}
}
