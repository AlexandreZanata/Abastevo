package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

// Source streams. The five-column coordinate extract stays a separate
// station-prep stream owned by RST; quality preparation consumes only the
// streams below, each with its own manifest and complete-run watermark.
const (
	StreamPMQCAssay   = "pmqc-assay-v1"
	StreamEnforcement = "enforcement-v1"
	StreamSanction    = "sanction-v1"
)

// Wire fuels. Exact string parity with kernel Product is pinned by
// TestFuelParityWithKernel; add a value here only when the kernel adopts it.
type Fuel string

const (
	FuelEthanol              Fuel = "ETHANOL"
	FuelGasolineRegular      Fuel = "GASOLINE_REGULAR"
	FuelGasolineAdditived    Fuel = "GASOLINE_ADDITIVED"
	FuelGasolinePremiumGrade Fuel = "GASOLINE_PREMIUM_GRADE"
	FuelDieselS500           Fuel = "DIESEL_S500"
	FuelDieselS10            Fuel = "DIESEL_S10"
	FuelCNG                  Fuel = "CNG"
	FuelLPGP13               Fuel = "LPG_P13"
)

// AllFuels lists the closed v1 vocabulary in a stable order.
var AllFuels = []Fuel{
	FuelEthanol, FuelGasolineRegular, FuelGasolineAdditived,
	FuelGasolinePremiumGrade, FuelDieselS500, FuelDieselS10,
	FuelCNG, FuelLPGP13,
}

var (
	ErrMissingIdentifier = errors.New("quality: missing native identifier")
	ErrUnknownFuel       = errors.New("quality: unknown source fuel")
	ErrUnknownConformity = errors.New("quality: unknown conformity status")
	ErrCountsMismatch    = errors.New("quality: manifest counts do not reconcile")
	ErrEmptyResources    = errors.New("quality: manifest binds no resources")
)

// Quarantine reasons. Importers record one reason per refused record; FQ-03
// owns the review/replay policy for quarantined rows.
const (
	ReasonMissingSample  = "missing_sample_ref"
	ReasonMissingAssay   = "missing_assay_ref"
	ReasonUnmappedFuel   = "unmapped_fuel"
	ReasonUnknownStatus  = "unknown_conformity"
	ReasonInvalidCNPJ    = "invalid_cnpj"
	ReasonLinkCollision  = "link_collision"
	ReasonCountsMismatch = "counts_mismatch"
)

// SampleID is the semantic identity of one official collection event: the
// preparation stream plus the source-native sample reference. Two same-day
// rows with different native references are different samples and must both
// be retained; one sample with ten assays is one sample, not ten.
type SampleID struct {
	Stream    string
	NativeRef string
}

// Key returns the stable grouping key. Empty stream or reference is a
// missing identifier, never an empty-string group.
func (s SampleID) Key() (string, error) {
	if strings.TrimSpace(s.Stream) == "" || strings.TrimSpace(s.NativeRef) == "" {
		return "", ErrMissingIdentifier
	}
	return s.Stream + "\x1f" + strings.TrimSpace(s.NativeRef), nil
}

// Conformity is the closed source-status vocabulary. UNKNOWN is explicit:
// incomplete or unrecognized source text is disclosed and excluded from
// eligible denominators, never classified conforming.
type Conformity string

const (
	Conforming    Conformity = "CONFORMING"
	Nonconforming Conformity = "NONCONFORMING"
	Inconclusive  Conformity = "INCONCLUSIVE"
	UnknownStatus Conformity = "UNKNOWN"
)

// ParseConformity maps source assay text to the closed vocabulary.
// Accented and cased variants of the verified Portuguese labels are
// accepted; anything else returns UNKNOWN with ErrUnknownConformity so the
// caller records the reason instead of guessing.
func ParseConformity(text string) (Conformity, error) {
	switch normalizeConformity(text) {
	case "CONFORME", "SIM":
		return Conforming, nil
	case "NAO CONFORME", "NAO":
		return Nonconforming, nil
	case "INCONCLUSIVO":
		return Inconclusive, nil
	case "":
		return UnknownStatus, ErrUnknownConformity
	default:
		return UnknownStatus, ErrUnknownConformity
	}
}

var conformityFold = strings.NewReplacer(
	"À", "A", "Á", "A", "Â", "A", "Ã", "A",
	"È", "E", "É", "E", "Ê", "E",
	"Ì", "I", "Í", "I", "Î", "I",
	"Ò", "O", "Ó", "O", "Ô", "O", "Õ", "O",
	"Ù", "U", "Ú", "U", "Û", "U",
	"Ç", "C",
)

func normalizeConformity(text string) string {
	upper := strings.ToUpper(text)
	return strings.Join(strings.Fields(conformityFold.Replace(upper)), " ")
}

// Assay is one laboratory determination within a sample. ResultText keeps
// the original source text verbatim; non-numeric results stay text and are
// never parsed into a manufactured number.
type Assay struct {
	Sample     SampleID
	AssayRef   string
	ResultText string
	Unit       string
	Status     Conformity
}

// Key returns the stable per-assay identity: sample key plus assay
// reference. A missing assay reference cannot merge into its siblings.
func (a Assay) Key() (string, error) {
	sampleKey, err := a.Sample.Key()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(a.AssayRef) == "" {
		return "", ErrMissingIdentifier
	}
	return sampleKey + "\x1f" + strings.TrimSpace(a.AssayRef), nil
}

// Fingerprint binds the full assay content for exact-duplicate detection:
// same key, result, unit and status. Reimporting the same sample, repeating
// assays or republishing files cannot increase support.
func (a Assay) Fingerprint() (string, error) {
	key, err := a.Key()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		key, a.ResultText, a.Unit, string(a.Status),
	}, "\x1f")))
	return hex.EncodeToString(sum[:]), nil
}

// ValidDay reports whether s is a date-only YYYY-MM-DD value. Source and
// review clocks use date-only precision; no midnight time is invented.
func ValidDay(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for i, c := range s {
		if i == 4 || i == 7 {
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Quarantine records one refused input with its reason.
type Quarantine struct {
	Locator string
	Reason  string
}

// Grouped is the deterministic result of grouping one bounded batch.
type Grouped struct {
	// Samples maps sample keys to their distinct assays in first-seen order.
	Samples map[string][]Assay
	// Duplicates counts exact within-batch repeats (retained once, counted).
	Duplicates int
	// Quarantined holds refused records with reasons.
	Quarantined []Quarantine
}

// GroupAssays groups raw assay rows with loss accounting: every input row
// lands in exactly one bucket, so
// input == accepted + duplicates + quarantined, where accepted is the total
// distinct assay count across samples.
func GroupAssays(rows []Assay, locators []string) Grouped {
	out := Grouped{Samples: map[string][]Assay{}}
	seen := map[string]bool{}
	for i, row := range rows {
		locator := ""
		if i < len(locators) {
			locator = locators[i]
		}
		sampleKey, serr := row.Sample.Key()
		if serr != nil {
			out.Quarantined = append(out.Quarantined, Quarantine{Locator: locator, Reason: ReasonMissingSample})
			continue
		}
		fp, ferr := row.Fingerprint()
		if ferr != nil {
			// Sample is valid, so only a missing assay reference can fail here.
			out.Quarantined = append(out.Quarantined, Quarantine{Locator: locator, Reason: ReasonMissingAssay})
			continue
		}
		if seen[fp] {
			out.Duplicates++
			continue
		}
		seen[fp] = true
		out.Samples[sampleKey] = append(out.Samples[sampleKey], row)
	}
	return out
}

// Accepted counts distinct assays across all samples.
func (g Grouped) Accepted() int {
	total := 0
	for _, assays := range g.Samples {
		total += len(assays)
	}
	return total
}

// Resource binds one raw artifact of a source edition.
type Resource struct {
	URL    string
	SHA256 string
	Bytes  uint64
}

// Counts carries the frozen loss-accounting denominators. Rows, assays,
// samples and visits use different denominators; one sample with ten assays
// is one sample and ten assays, recorded separately by the importer.
type Counts struct {
	Input       int
	Accepted    int
	Duplicates  int
	Quarantined int
}

// Manifest binds all streams/hashes and counts of one source edition. The
// predecessor links the superseded revision; publication is atomic per
// manifest, never per partial stream.
type Manifest struct {
	Source      string
	Stream      string
	Edition     string
	Resources   []Resource
	Counts      Counts
	Predecessor string
}

// Reconcile refuses truncated, wrong-edition or mismatched manifests before
// publication: resources must be non-empty and counts must balance exactly.
func (m Manifest) Reconcile() error {
	if len(m.Resources) == 0 {
		return ErrEmptyResources
	}
	c := m.Counts
	if c.Input != c.Accepted+c.Duplicates+c.Quarantined {
		return ErrCountsMismatch
	}
	return nil
}

// StationLink binds a source record to a canonical station over an
// effective window. Links require exact documented keys; name, address,
// brand or proximity alone never assign a record to a station.
type StationLink struct {
	StationID string
	RecordKey string
	Automatic bool
	Reason    string
	Active    bool
}

// ReviewLinks keeps at most one active binding per record: a second active
// link for the same record key collides and both stay under review instead
// of silently replacing each other. Returns the surviving active set.
func ReviewLinks(links []StationLink) []StationLink {
	activeFor := map[string]int{}
	for _, l := range links {
		if l.Active {
			activeFor[l.RecordKey]++
		}
	}
	out := make([]StationLink, 0, len(links))
	for _, l := range links {
		if l.Active && activeFor[l.RecordKey] > 1 {
			l.Active = false
			l.Reason = ReasonLinkCollision + ": " + l.Reason
		}
		out = append(out, l)
	}
	return out
}

// Revision appends a business correction to history. The original stays
// intact; a corrected linkage withdraws the wrong station's association by
// supersession, and both projections rebuild without erasing audit history.
type Revision struct {
	Supersedes string
	Note       string
}

// ApplyCorrection appends exactly one revision. A correction with an empty
// note or a supersession that matches nothing is refused.
func ApplyCorrection(history []Revision, correction Revision, known []string) ([]Revision, error) {
	if strings.TrimSpace(correction.Note) == "" {
		return history, errors.New("quality: correction needs a reason")
	}
	found := false
	for _, k := range known {
		if k == correction.Supersedes {
			found = true
			break
		}
	}
	if !found {
		return history, errors.New("quality: correction supersedes an unknown revision")
	}
	return append(history, correction), nil
}
