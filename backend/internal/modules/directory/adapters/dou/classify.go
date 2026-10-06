package dou

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
)

// SourceActs is the frozen staging source for DOU act assertions.
const SourceActs = "dou-acts"

// Class is the deterministic act classification. Ambiguous quarantines
// for review; no free-text or model interpretation ever publishes.
type Class string

const (
	ClassGrant      Class = "grant"
	ClassCorrection Class = "correction"
	ClassRevocation Class = "revocation"
	ClassUnrelated  Class = "unrelated"
	ClassAmbiguous  Class = "ambiguous"
)

var (
	grantVerbs      = []string{"autoriza", "concede", "outorga", "defere", "homologa"}
	correctionVerbs = []string{"retifica", "corrige", "altera"}
	revocationVerbs = []string{"revoga", "cancela", "cassa", "suspende"}
	fuelMarkers     = []string{"revenda", "combustiv", "posto", "gasolina", "diesel", "etanol", "gnv", "anp"}
)

func containsAnyFold(text string, words []string) bool {
	for _, w := range words {
		if strings.Contains(text, w) {
			return true
		}
	}
	return false
}

// Classify applies deterministic keyword rules over lowercased,
// accent-folded act text. Revocation wins over grant wording (a
// revoking act may quote the grant); corrections are explicit;
// grants need fuel-retail context; fuel context without a verb is
// ambiguous; anything else is unrelated.
func Classify(act Act) Class {
	text := fold(act.Text + " " + act.Title)
	hasRevoke := containsAnyFold(text, revocationVerbs)
	hasCorrect := containsAnyFold(text, correctionVerbs)
	hasGrant := containsAnyFold(text, grantVerbs)
	hasFuel := containsAnyFold(text, fuelMarkers)
	switch {
	case hasRevoke:
		return ClassRevocation
	case hasCorrect:
		return ClassCorrection
	case hasGrant && hasFuel:
		return ClassGrant
	case hasFuel:
		return ClassAmbiguous
	default:
		return ClassUnrelated
	}
}

// fold lowercases and strips common Portuguese accents for keyword
// matching. Matching stays deterministic and reviewable.
func fold(text string) string {
	lower := strings.ToLower(text)
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "ã", "a", "â", "a",
		"é", "e", "ê", "e", "í", "i", "ó", "o",
		"õ", "o", "ô", "o", "ú", "u", "ü", "u",
		"ç", "c",
	)
	return replacer.Replace(lower)
}

var cnpjPattern = regexp.MustCompile(`[0-9A-Za-z./-]{14,18}`)

// ExtractCNPJs finds candidate identifiers in free text and validates
// each with the kernel alphanumeric program. Invalid candidates drop
// silently (they are not evidence); the caller quarantines acts with
// zero or multiple distinct identifiers.
func ExtractCNPJs(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, candidate := range cnpjPattern.FindAllString(text, -1) {
		cnpj, err := registry.ValidCNPJText(candidate)
		if err != nil || seen[cnpj] {
			continue
		}
		seen[cnpj] = true
		out = append(out, cnpj)
	}
	return out
}

// StageActs turns classified acts into staged assertions plus a skip
// count. Only single-identifier grants and revocations stage;
// corrections need chain resolution (T03), multi-identifier acts need
// review, and unrelated/ambiguous acts never publish. The edition date
// becomes the effective date; it is never an inauguration date (no
// opening-date output exists anywhere in this package).
func StageActs(edition Edition, acts []Act) (assertions []registry.Assertion, skipped int) {
	editionDate, err := time.Parse("2006-01-02", edition.Date)
	if err != nil {
		return nil, len(acts)
	}
	for _, act := range acts {
		class := Classify(act)
		if class != ClassGrant && class != ClassRevocation {
			skipped++
			continue
		}
		cnpjs := ExtractCNPJs(act.Text)
		if len(cnpjs) != 1 {
			skipped++
			continue
		}
		auth := registry.AuthorizationAuthorized
		if class == ClassRevocation {
			auth = registry.AuthorizationRevoked
		}
		sum := sha256.Sum256([]byte(edition.Checksum + "|" + act.ID + "|" + cnpjs[0]))
		assertions = append(assertions, registry.Assertion{
			Source:          SourceActs,
			SourceKey:       cnpjs[0],
			Checksum:        hex.EncodeToString(sum[:]),
			DisplayName:     "CNPJ " + cnpjs[0] + " (DOU " + act.ID + ")",
			Address:         map[string]string{},
			AuthState:       string(auth),
			Eligibility:     string(registry.DecideEligibility(registry.Record{IdentityOK: true})),
			LocationQuality: "unknown",
			SourceReference: act.ID,
			EffectiveDate:   pgtype.Date{Time: editionDate, Valid: true},
		})
	}
	return assertions, skipped
}
