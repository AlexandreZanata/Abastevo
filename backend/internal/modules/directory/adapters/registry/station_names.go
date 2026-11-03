package registry

import (
	"encoding/json"
	"fmt"
	"strings"
)

const namedParserVersion = "station-prep-v0.3.1+names-v1"

// BatchNameInput pins a minimized, offline enrichment input. It does not
// contribute extra station rows to the registry denominator.
type BatchNameInput struct {
	Source    string `json:"source"`
	Profile   string `json:"profile"`
	Reference string `json:"reference"`
	Edition   string `json:"edition"`
	SHA256    string `json:"sha256"`
	Bytes     uint64 `json:"bytes"`
	Rows      int    `json:"rows"`
}

// BatchNameProvenance binds a chosen trade name to its exact input edition.
type BatchNameProvenance struct {
	Source     string `json:"source"`
	Profile    string `json:"profile"`
	Reference  string `json:"reference"`
	Edition    string `json:"edition"`
	SHA256     string `json:"sha256"`
	ObservedAt string `json:"observed_at"`
}

func validateNameInputs(manifest BatchManifest) error {
	if len(manifest.NameInputs) > 2 {
		return fmt.Errorf("registry: too many name inputs")
	}
	seen := map[string]bool{}
	for _, pin := range manifest.NameInputs {
		expected := map[string]string{"rfb": "rfb-station-names-v1", "anp-survey": "anp-station-price-v1"}[pin.Source]
		if (manifest.ParserVersion != namedParserVersion && manifest.ParserVersion != addressParserVersion) || expected == "" || expected != pin.Profile || seen[pin.Source] ||
			strings.TrimSpace(pin.Reference) == "" || len(pin.Reference) > 2048 || strings.TrimSpace(pin.Edition) == "" || len(pin.Edition) > 256 ||
			!validSHA256(pin.SHA256) || pin.Bytes > 64<<20 || pin.Rows < 0 || pin.Rows > 500000 {
			return fmt.Errorf("registry: invalid pinned name input")
		}
		seen[pin.Source] = true
	}
	return nil
}

func validateNameBinding(manifest BatchManifest, row BatchAssertion) error {
	if row.SchemaVersion != SchemaAssertionV3 && row.SchemaVersion != SchemaAssertionV4 {
		return nil
	} // Old schemas reject name fields during mapping.
	if ((row.SchemaVersion == SchemaAssertionV3 && manifest.ParserVersion != namedParserVersion) || (row.SchemaVersion == SchemaAssertionV4 && manifest.ParserVersion != addressParserVersion)) || row.TradeName == nil {
		return fmt.Errorf("registry: unversioned sourced names")
	}
	if strings.TrimSpace(*row.TradeName) == "" {
		if row.TradeNameProvenance != nil {
			return fmt.Errorf("registry: empty trade name with provenance")
		}
		return nil
	}
	p := row.TradeNameProvenance
	if p == nil || p.ObservedAt == "" {
		return fmt.Errorf("registry: trade name without provenance")
	}
	if _, err := parseEffectiveDate(p.ObservedAt, "trade name observation"); err != nil {
		return err
	}
	for _, pin := range manifest.NameInputs {
		if p.Source == pin.Source && p.Profile == pin.Profile && p.Reference == pin.Reference && p.Edition == pin.Edition && p.SHA256 == pin.SHA256 {
			return nil
		}
	}
	return fmt.Errorf("registry: trade name provenance matches no pinned input")
}

func decodeStoredNameProvenance(raw []byte) (*BatchNameProvenance, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var provenance BatchNameProvenance
	if err := json.Unmarshal(raw, &provenance); err != nil {
		return nil, fmt.Errorf("registry: invalid stored name provenance: %w", err)
	}
	return &provenance, nil
}
