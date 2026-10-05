// Package registry implements the P25-T01 frozen ANP registry source
// policy: header-name detection, kernel CNPJ text rules, status mapping
// and sufficient-evidence eligibility. CSV/API staging, reconciliation
// jobs and migrations arrive in P25-T02…T05; this package freezes the
// executable semantics they must obey. Quarantine never deletes history
// and a single bad row never poisons a batch.
package registry

import (
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// Authorization is the frozen B-BR-D04 authorization wire state.
type Authorization string

const (
	AuthorizationUnknown    Authorization = "unknown"
	AuthorizationAuthorized Authorization = "authorized"
	AuthorizationSuspended  Authorization = "suspended"
	AuthorizationRevoked    Authorization = "revoked"
)

// Eligibility is the frozen B-BR-D04 publication eligibility.
type Eligibility string

const (
	EligibilityIneligible Eligibility = "ineligible"
	EligibilityPending    Eligibility = "pending"
	EligibilityEligible   Eligibility = "eligible"
	EligibilityWithdrawn  Eligibility = "withdrawn"
)

// requiredHeaders are the logical CSV names P25-T02 must resolve by
// header-name match (case-insensitive, accents already normalized by
// the staging reader). Column order and extra layout never matter.
var requiredHeaders = []string{
	"CNPJ", "RAZAO_SOCIAL", "COD_IBGE", "UF", "SITUACAO",
}

// ValidateHeader reports required names absent from the header row and
// columns no frozen contract knows. Matching is case-insensitive on
// trimmed names; detection is by name, never position.
func ValidateHeader(names []string) (missing []string, unknown []string) {
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		seen[strings.ToUpper(strings.TrimSpace(name))] = true
	}
	for _, want := range requiredHeaders {
		if !seen[want] {
			missing = append(missing, want)
		}
	}
	known := make(map[string]bool, len(requiredHeaders)+16)
	for _, want := range requiredHeaders {
		known[want] = true
	}
	for _, extra := range []string{
		"NOME_FANTASIA", "LOGRADOURO", "NUMERO", "COMPLEMENTO", "BAIRRO",
		"CEP", "MUNICIPIO", "ATO_AUTORIZACAO", "DATA_PUBLICACAO", "PRODUTOS",
	} {
		known[extra] = true
	}
	for _, name := range names {
		upper := strings.ToUpper(strings.TrimSpace(name))
		if upper == "" || known[upper] {
			continue
		}
		unknown = append(unknown, strings.TrimSpace(name))
	}
	return missing, unknown
}

// QuarantineRun reports whether the run must quarantine for operator
// review: any missing required name or any unknown column. Quarantine
// preserves the previous accepted catalog; it never publishes partial
// state and never deletes history.
func QuarantineRun(missing, unknown []string) bool {
	return len(missing) > 0 || len(unknown) > 0
}

// ValidCNPJText validates one registry identifier as text with the
// kernel alphanumeric program: 14 characters, letters uppercased,
// leading zeroes preserved (the value is never numeric), both check
// digits verified. Malformed identifiers quarantine the row.
func ValidCNPJText(raw string) (string, error) {
	cnpj, err := kernel.ParseCNPJ(raw)
	if err != nil {
		return "", err
	}
	return cnpj.Normalized(), nil
}

// MapStatus freezes the registry SITUACAO wire mapping. Unmapped and
// blank values stay AuthorizationUnknown (honest, never an inferred
// grant or revocation); revocation requires explicit sourced evidence
// in P25-T04, never this mapping alone.
func MapStatus(raw string) Authorization {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "ATIVA", "AUTORIZADA", "DEFERIDA":
		return AuthorizationAuthorized
	case "SUSPENSA":
		return AuthorizationSuspended
	case "CANCELADA", "CASSADA", "REVOGADA":
		return AuthorizationRevoked
	default:
		return AuthorizationUnknown
	}
}

// Record is the minimal sufficient-evidence input for eligibility.
type Record struct {
	IdentityOK bool
	AddressOK  bool
	Authorized bool
	Withdrawn  bool
}

// DecideEligibility freezes the B-BR-D04 publication rule: eligible only with
// exact-valid identity, structured address and authorization, and never
// once withdrawn. Unknown stays pending; review-approved unverified
// entries follow the separate P27 policy and stay pending here.
func DecideEligibility(rec Record) Eligibility {
	if rec.Withdrawn {
		return EligibilityWithdrawn
	}
	if !rec.IdentityOK || !rec.AddressOK {
		return EligibilityPending
	}
	if !rec.Authorized {
		return EligibilityPending
	}
	return EligibilityEligible
}
