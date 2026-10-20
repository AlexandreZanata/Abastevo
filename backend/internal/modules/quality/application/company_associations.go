package application

import (
	"context"
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

// MaxCompanyObservations shares the native fine/verified artifact row budget.
const MaxCompanyObservations = 500000
const companyIdentityBatchSize = 512

var ErrCompanyObservationInput = errors.New("quality: invalid company observation plan")
var ErrCompanyIdentityUnavailable = errors.New("quality: company identity evidence unavailable")

// CompanyIdentitySnapshot contains only requested full identifiers and at most
// two retained canonical candidates per identifier, sufficient to retain
// ambiguity. OtherBranches identifies requested CNPJs with foreign-branch
// evidence; it can only route review, never associate a station by base.
type CompanyIdentitySnapshot struct {
	Evidence      []domain.IdentityEvidence
	OtherBranches []string
}

// CompanyIdentityLookup is a read-only module composition port. No source or
// operational Directory dates, station writes or fuzzy identifiers are exposed.
type CompanyIdentityLookup func(context.Context, []string) (CompanyIdentitySnapshot, error)

// PlanCompanyAssociations validates the whole source before I/O, performs
// bounded per-identifier lookups, and returns one decision per input record.
// Failures/cancellation discard the entire plan. Decisions remain company-only;
// publication must additionally fence live bindings in its own transaction.
func PlanCompanyAssociations(ctx context.Context, records []domain.CompanyObservationRecord, decidedAt string, lookup CompanyIdentityLookup) ([]domain.CompanyObservationDecision, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if len(records) > MaxCompanyObservations || !domain.ValidDay(decidedAt) {
		return nil, ErrCompanyObservationInput
	}
	representatives := make(map[string]domain.CompanyObservationRecord)
	seen := make(map[string]bool, len(records))
	for i, record := range records {
		if i%companyIdentityBatchSize == 0 {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
		}
		if !record.ValidShape() || seen[record.RecordKey] {
			return nil, ErrCompanyObservationInput
		}
		id, e := kernel.ParseCNPJ(record.CNPJ)
		if e != nil || id.Normalized() != record.CNPJ {
			return nil, ErrCompanyObservationInput
		}
		seen[record.RecordKey] = true
		representatives[record.CNPJ] = record
	}
	if len(records) == 0 {
		return []domain.CompanyObservationDecision{}, nil
	}
	if lookup == nil {
		return nil, ErrCompanyIdentityUnavailable
	}
	keys := make([]string, 0, len(representatives))
	for key := range representatives {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	resolved := make(map[string]domain.CompanyObservationDecision, len(keys))
	for offset := 0; offset < len(keys); offset += companyIdentityBatchSize {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		part := keys[offset:min(offset+companyIdentityBatchSize, len(keys))]
		requested := make(map[string]bool, len(part))
		for _, id := range part {
			requested[id] = true
		}
		snapshot, e := lookup(ctx, append([]string(nil), part...))
		if e != nil {
			return nil, ErrCompanyIdentityUnavailable
		}
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if len(snapshot.Evidence) > 2*len(part) || len(snapshot.OtherBranches) > len(part) {
			return nil, ErrCompanyIdentityUnavailable
		}
		index := make(map[string][]domain.IdentityEvidence, len(part))
		branches := make(map[string]bool, len(snapshot.OtherBranches))
		for _, row := range snapshot.Evidence {
			if !requested[row.CNPJ] || row.StationID == "" || len(row.StationID) > 512 || strings.TrimSpace(row.StationID) != row.StationID || !utf8.ValidString(row.StationID) || strings.IndexFunc(row.StationID, unicode.IsControl) >= 0 || len(index[row.CNPJ]) >= 2 {
				return nil, ErrCompanyIdentityUnavailable
			}
			index[row.CNPJ] = append(index[row.CNPJ], row)
		}
		for _, cnpj := range snapshot.OtherBranches {
			if !requested[cnpj] {
				return nil, ErrCompanyIdentityUnavailable
			}
			branches[cnpj] = true
		}
		for _, cnpj := range part {
			resolved[cnpj] = domain.ResolveCompanyObservation(representatives[cnpj], decidedAt, index[cnpj], branches[cnpj])
		}
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	out := make([]domain.CompanyObservationDecision, 0, len(records))
	for _, record := range records {
		d := resolved[record.CNPJ]
		d.RecordKey = record.RecordKey
		out = append(out, d)
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	return out, nil
}
