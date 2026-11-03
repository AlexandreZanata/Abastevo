package registry

import (
	"fmt"
	"maps"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const addressParserVersion = "station-prep-v0.3.1+addresses-v1"
const addressPolicySHA256 = "ec242e9c5ec8f40cee1e49c025ffb4826201c84b5595ef3ee24b794fda6fa454"
const addressUFs = " AC AL AM AP BA CE DF ES GO MA MG MS MT PA PB PE PI PR RJ RN RO RR RS SC SE SP TO "

type BatchAddressInput struct {
	Source      string `json:"source"`
	Profile     string `json:"profile"`
	Reference   string `json:"reference"`
	Edition     string `json:"edition"`
	SHA256      string `json:"sha256"`
	Bytes       uint64 `json:"bytes"`
	Rows        uint64 `json:"rows"`
	License     string `json:"license"`
	Attribution string `json:"attribution"`
}
type BatchAddressSkip struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
}
type BatchAddressTransition struct {
	Before map[string]uint64 `json:"before"`
	After  map[string]uint64 `json:"after"`
}
type BatchAddressCoverage struct {
	StationCount  uint64                            `json:"station_count"`
	National      BatchAddressTransition            `json:"national"`
	ByUF          map[string]BatchAddressTransition `json:"by_uf"`
	ReviewReasons map[string]uint64                 `json:"review_reasons"`
}
type BatchAddressProof struct {
	Kind          string `json:"kind"`
	CNPJ          string `json:"cnpj"`
	Active        *bool  `json:"active,omitempty"`
	ObjectType    string `json:"object_type,omitempty"`
	ObjectID      int64  `json:"object_id,omitempty"`
	LinkReference string `json:"link_reference,omitempty"`
}

// BatchAddressProvenance is retained on the immutable assertion, not public JSON.
type BatchAddressProvenance struct {
	Profile            string            `json:"profile"`
	Reference          string            `json:"reference"`
	Edition            string            `json:"edition"`
	InputSHA256        string            `json:"input_sha256"`
	ObservedAt         string            `json:"observed_at"`
	License            string            `json:"license"`
	Attribution        string            `json:"attribution"`
	RightsReference    string            `json:"rights_reference"`
	RightsPolicySHA256 string            `json:"rights_policy_sha256"`
	Proof              BatchAddressProof `json:"proof"`
}
type BatchAddressFacts struct {
	Source     string                  `json:"source"`
	Components map[string]string       `json:"components"`
	Provenance *BatchAddressProvenance `json:"provenance"`
}

type addressSourceRule struct{ Profile, License, Attribution, RightsReference string }

// The policy hash and exact declarations are cross-checked against the frozen
// station-prep contract; a prepared manifest cannot admit a new licence/source.
var addressSources = map[string]addressSourceRule{
	"rfb": {"rfb-station-addresses-v1", "Creative Commons Attribution", "Receita Federal do Brasil — Cadastro Nacional da Pessoa Jurídica (CNPJ)", "https://dados.gov.br/dados/conjuntos-dados/cadastro-nacional-da-pessoa-juridica---cnpj"},
	"osm": {"osm-station-addresses-v1", "ODbL-1.0", "© OpenStreetMap contributors", "https://www.openstreetmap.org/copyright"},
}

func boundedAddressText(v string, max int) bool {
	return strings.TrimSpace(v) != "" && len(v) <= max && !strings.ContainsFunc(v, unicode.IsControl)
}
func readableAddressText(v string) bool {
	if !boundedAddressText(v, 2048) || !strings.ContainsFunc(v, unicode.IsLetter) {
		return false
	}
	folded := strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return unicode.ToUpper(r)
	}, norm.NFD.String(v))
	switch strings.Join(strings.Fields(folded), " ") {
	case "NAO INFORMADO", "SEM ENDERECO", "NAO POSSUI", "N/A":
		return false
	}
	return true
}
func validPreparedAddressUF(uf string) bool {
	return len(uf) == 2 && strings.Contains(addressUFs, " "+strings.ToUpper(uf)+" ")
}
func addressComponentShape(components map[string]string) bool {
	if components == nil {
		return false
	}
	for k, v := range components {
		switch k {
		case "street", "raw", "number", "complement", "district", "postal_code", "municipio", "municipio_ibge", "uf":
		default:
			return false
		}
		if len(v) > 2048 || strings.ContainsFunc(v, unicode.IsControl) {
			return false
		}
	}
	return true
}
func usablePreparedAddress(c map[string]string) bool {
	return addressComponentShape(c) && (readableAddressText(c["street"]) || readableAddressText(c["raw"]))
}
func validateAddressInputs(m BatchManifest) error {
	if m.ParserVersion != addressParserVersion {
		if m.AddressPolicySHA256 != "" || m.AddressInputs != nil || m.AddressSkips != nil || m.AddressCoverage != nil {
			return fmt.Errorf("registry: address metadata requires v4 parser")
		}
		return nil
	}
	if m.AddressPolicySHA256 != addressPolicySHA256 || len(m.AddressInputs) > len(addressSources) || len(m.AddressSkips) > 1 || m.AddressCoverage == nil {
		return fmt.Errorf("registry: invalid address policy or inputs")
	}
	seen := map[string]bool{}
	for _, p := range m.AddressInputs {
		r, ok := addressSources[p.Source]
		if !ok || seen[p.Source] || p.Profile != r.Profile || p.License != r.License || p.Attribution != r.Attribution || !boundedAddressText(p.Reference, 2048) || !boundedAddressText(p.Edition, 256) || !validSHA256(p.SHA256) || p.Bytes > 64<<20 || p.Rows > 500000 {
			return fmt.Errorf("registry: unadmitted address input pin")
		}
		seen[p.Source] = true
	}
	for _, s := range m.AddressSkips {
		if s.Source != "cnefe" || s.Reason != "source_reuse_not_admitted" {
			return fmt.Errorf("registry: invalid address source skip")
		}
	}
	return validateAddressCoverage(m)
}
func validateAddressCoverage(m BatchManifest) error {
	c := m.AddressCoverage
	const maxStations = 200000
	if c.StationCount > maxStations || len(c.ByUF) > 27 || len(c.ReviewReasons) > 32 || c.StationCount > uint64(max(0, m.Counts["registry-13col"].Accepted)) {
		return fmt.Errorf("registry: invalid address coverage denominator")
	}
	before := map[string]uint64{}
	after := map[string]uint64{}
	for uf, v := range c.ByUF {
		if !validPreparedAddressUF(uf) || uf != strings.ToUpper(uf) {
			return fmt.Errorf("registry: invalid address coverage UF")
		}
		var b, a uint64
		for k, n := range v.Before {
			if (k != "anp" && k != "municipality" && k != "unknown") || n == 0 || n > c.StationCount {
				return fmt.Errorf("registry: invalid prior address count")
			}
			before[k] += n
			b += n
		}
		for k, n := range v.After {
			if (k != "anp" && k != "municipality" && k != "unknown" && k != "rfb" && k != "osm") || n == 0 || n > c.StationCount {
				return fmt.Errorf("registry: invalid selected address count")
			}
			after[k] += n
			a += n
		}
		if b != a || b == 0 || v.Before["anp"] != v.After["anp"] {
			return fmt.Errorf("registry: address coverage UF does not reconcile")
		}
	}
	var count uint64
	for _, n := range before {
		count += n
	}
	if count != c.StationCount || !maps.Equal(before, c.National.Before) || !maps.Equal(after, c.National.After) {
		return fmt.Errorf("registry: address coverage total does not reconcile")
	}
	return nil
}
func validateAddressBinding(m BatchManifest, row BatchAssertion) error {
	if row.SchemaVersion != SchemaAssertionV4 {
		if row.AddressFacts != nil || m.ParserVersion == addressParserVersion {
			return fmt.Errorf("registry: address facts require assertion v4")
		}
		return nil
	}
	if m.ParserVersion != addressParserVersion || row.AddressFacts == nil {
		return fmt.Errorf("registry: v4 address facts require matching parser")
	}
	f := row.AddressFacts
	if f.Provenance == nil {
		return nil
	} // Source-specific proof requirement is checked before mapping.
	p := f.Provenance
	r, ok := addressSources[f.Source]
	if !ok || p.Profile != r.Profile || p.License != r.License || p.Attribution != r.Attribution || p.RightsReference != r.RightsReference || p.RightsPolicySHA256 != m.AddressPolicySHA256 {
		return fmt.Errorf("registry: address provenance rights mismatch")
	}
	for _, pin := range m.AddressInputs {
		if pin.Source == f.Source && pin.Profile == p.Profile && pin.Reference == p.Reference && pin.Edition == p.Edition && pin.SHA256 == p.InputSHA256 && pin.License == p.License && pin.Attribution == p.Attribution {
			return nil
		}
	}
	return fmt.Errorf("registry: address provenance matches no pinned input")
}
func projectPreparedAddress(row BatchAssertion, official map[string]string) (map[string]string, string, *BatchAddressProvenance, error) {
	bad := func() (map[string]string, string, *BatchAddressProvenance, error) {
		return nil, "", nil, fmt.Errorf("registry: invalid v4 address facts or proof")
	}
	if row.SchemaVersion != SchemaAssertionV4 {
		if row.AddressFacts != nil {
			return bad()
		}
		return official, "", nil, nil
	}
	f := row.AddressFacts
	if f == nil || !addressComponentShape(f.Components) || !validPreparedAddressUF(row.UF) || (usablePreparedAddress(official) != (f.Source == "anp")) {
		return bad()
	}
	p := f.Provenance
	switch f.Source {
	case "anp":
		if p != nil || !maps.Equal(f.Components, official) {
			return bad()
		}
	case "municipality":
		if p != nil || len(f.Components) != 2 || f.Components["municipio"] != row.MunicipalityNameRaw || f.Components["uf"] != row.UF || !readableAddressText(row.MunicipalityNameRaw) {
			return bad()
		}
	case "":
		if p != nil || len(f.Components) != 0 || readableAddressText(row.MunicipalityNameRaw) {
			return bad()
		}
	case "rfb", "osm":
		if p == nil || !usablePreparedAddress(f.Components) || p.Proof.Kind != f.Source || p.Proof.CNPJ != row.SourceKey {
			return bad()
		}
		date, err := time.Parse(time.DateOnly, p.ObservedAt)
		if err != nil || date.Year() == 0 || date.Format(time.DateOnly) != p.ObservedAt {
			return bad()
		}
		if f.Source == "rfb" && (p.Proof.Active == nil || !*p.Proof.Active || p.Proof.ObjectType != "" || p.Proof.ObjectID != 0 || p.Proof.LinkReference != "") {
			return bad()
		}
		if f.Source == "osm" && (p.Proof.Active != nil || p.Proof.ObjectID <= 0 || (p.Proof.ObjectType != "node" && p.Proof.ObjectType != "way" && p.Proof.ObjectType != "relation") || !boundedAddressText(p.Proof.LinkReference, 2048)) {
			return bad()
		}
	default:
		return bad()
	}
	if uf := f.Components["uf"]; uf != "" && !strings.EqualFold(uf, row.UF) {
		return bad()
	}
	if code := f.Components["municipio_ibge"]; code != "" && code != row.MunicipalityIBGE {
		return bad()
	}
	c := maps.Clone(f.Components)
	// These keys identify the directory station; no street/number blending.
	c["municipio_ibge"] = row.MunicipalityIBGE
	c["uf"] = strings.ToUpper(row.UF)
	if c["municipio"] == "" && readableAddressText(row.MunicipalityNameRaw) {
		c["municipio"] = row.MunicipalityNameRaw
	}
	return c, f.Source, p, nil
}
func decodeStoredAddressProvenance(raw []byte) (*BatchAddressProvenance, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var v BatchAddressProvenance
	if err := decodeStrict(raw, &v, "stored address provenance"); err != nil {
		return nil, err
	}
	return &v, nil
}
