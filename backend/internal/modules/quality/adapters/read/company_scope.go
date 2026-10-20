package read

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5"
)

type CompanyIdentityProbe func(context.Context, []string) ([]domain.IdentityEvidence, error)
type CompanyIdentityGuard func(context.Context, pgx.Tx, []application.CompanyBinding) error

// WithCompanyIdentity composes Directory-owned reads/locks without sharing its
// SQL or write capabilities. Request-local historical readers preserve the ports.
func (r *Reader) WithCompanyIdentity(probe CompanyIdentityProbe, guard CompanyIdentityGuard) *Reader {
	clone := *r
	clone.companyProbe = probe
	clone.companyGuard = guard
	return &clone
}

type CompanyScopeView struct {
	StationID                     string `json:"station_id"`
	Available                     bool   `json:"available"`
	Reason                        string `json:"reason"`
	PolicyVersion                 string `json:"policy_version,omitempty"`
	AssociationScope              string `json:"association_scope,omitempty"`
	DecisionDay                   string `json:"decision_day,omitempty"`
	HistoricalBusinessWindowKnown bool   `json:"historical_business_window_known"`
}

// CompanyScope owns one frozen lease/current-company set. Its private proof
// cannot be replaced with public station fields or reused under another lease.
// Fingerprint binds later HTTP cursors/ETags; source CNPJs never enter View.
type CompanyScope struct {
	view                            CompanyScopeView
	leaseID, stationID, fingerprint string
	allowed                         []string
	validated                       bool
}

func (s CompanyScope) View() CompanyScopeView { return s.view }
func (s CompanyScope) Fingerprint() string    { return s.fingerprint }
func (s *CompanyScope) seal() {
	raw, _ := json.Marshal(struct {
		Version, Revision string
		View              CompanyScopeView
		Allowed           []string
	}{"company-scope-v1", s.leaseID, s.view, s.allowed})
	sum := sha256.Sum256(raw)
	s.fingerprint = hex.EncodeToString(sum[:])
	s.validated = true
}

func (r *Reader) ResolveCompanyScope(ctx context.Context, now time.Time, lease Lease, stationID string) (CompanyScope, error) {
	if e := ctx.Err(); e != nil {
		return CompanyScope{}, e
	}
	station, e := application.ValidateStationID(stationID)
	if e != nil {
		return CompanyScope{}, e
	}
	if lease.Source != "sanctions" || lease.Stream != domain.StreamSanction {
		return CompanyScope{}, errors.New("read: wrong company source lease")
	}
	out := CompanyScope{leaseID: lease.RevisionID, stationID: station, allowed: []string{}, view: CompanyScopeView{StationID: station, Reason: "SOURCE_UNAVAILABLE"}}
	if !lease.HasPublished {
		out.seal()
		return out, nil
	}
	if r.companyProbe == nil || r.companyGuard == nil || r.pool == nil {
		return CompanyScope{}, errors.New("read: company identity ports unavailable")
	}
	revision, e := mustUUID(lease.RevisionID)
	if e != nil {
		return CompanyScope{}, e
	}
	c, e := quality.New(r.pool).CompanyAssociationContext(ctx, revision)
	if errors.Is(e, pgx.ErrNoRows) {
		out.view.Reason = "ASSOCIATION_EVIDENCE_UNAVAILABLE"
		out.seal()
		return out, nil
	}
	if e != nil {
		return CompanyScope{}, e
	}
	if c.PolicyVersion != domain.CompanyObservationPolicyVersion || c.AssociationScope != domain.CurrentCompanyOnly || c.HistoricalBusinessWindowKnown || c.ObservationCount < 0 || int(c.ObservationCount) != lease.Counts.Accepted || !domain.ValidDay(dateString(c.DecisionDay)) {
		return CompanyScope{}, errors.New("read: company association context unavailable")
	}
	out.view.Available = true
	out.view.Reason = "NO_CURRENT_COMPANY_MATCH"
	out.view.PolicyVersion = c.PolicyVersion
	out.view.AssociationScope = c.AssociationScope
	out.view.DecisionDay = dateString(c.DecisionDay)
	sid, _ := mustUUID(station)
	candidates, e := quality.New(r.pool).CompanyStationCandidates(ctx, quality.CompanyStationCandidatesParams{RevisionID: revision, StationID: sid})
	if e != nil {
		return CompanyScope{}, e
	}
	if len(candidates) > int(c.ObservationCount) {
		return CompanyScope{}, errors.New("read: company candidate census diverged")
	}
	records := make([]domain.CompanyObservationRecord, 0, len(candidates))
	for _, cnpj := range candidates {
		records = append(records, domain.CompanyObservationRecord{RecordKey: "company:" + cnpj, CNPJ: cnpj})
	}
	decisions, e := application.PlanCompanyAssociations(ctx, records, now.UTC().Format("2006-01-02"), func(ctx context.Context, ids []string) (application.CompanyIdentitySnapshot, error) {
		rows, e := r.companyProbe(ctx, ids)
		return application.CompanyIdentitySnapshot{Evidence: rows}, e
	})
	if e != nil {
		return CompanyScope{}, errors.New("read: company identity evidence unavailable")
	}
	for i, d := range decisions {
		if d.Kind == domain.ResolutionBound && d.StationID == station {
			out.allowed = append(out.allowed, records[i].CNPJ)
		}
	}
	sort.Strings(out.allowed)
	if len(out.allowed) > 0 {
		out.view.Reason = "CURRENT_COMPANY_MATCH"
	}
	out.seal()
	return out, nil
}

func (s CompanyScope) validFor(lease Lease) bool {
	return s.validated && s.stationID != "" && s.view.StationID == s.stationID && s.leaseID == lease.RevisionID && lease.Source == "sanctions" && lease.Stream == domain.StreamSanction
}
func (s CompanyScope) bindings() []application.CompanyBinding {
	out := make([]application.CompanyBinding, 0, len(s.allowed))
	for _, cnpj := range s.allowed {
		out = append(out, application.CompanyBinding{CNPJ: cnpj, StationID: s.stationID})
	}
	return out
}

func (r *Reader) CompanySanctionsAtScope(ctx context.Context, lease Lease, scope CompanyScope, status, afterNative, afterID string, limit int) ([]SanctionEvent, string, error) {
	if !scope.validFor(lease) || limit < 1 || limit > 100 || len(afterNative) > 256 {
		return nil, "", errors.New("read: invalid company scope or page")
	}
	if afterID != "" {
		if _, e := application.ValidateStationID(afterID); e != nil || afterNative == "" {
			return nil, "", errors.New("read: invalid company continuation")
		}
	} else if afterNative != "" {
		return nil, "", errors.New("read: incomplete company continuation")
	}
	if !lease.HasPublished || !scope.view.Available || len(scope.allowed) == 0 {
		return []SanctionEvent{}, "", nil
	}
	if r.companyGuard == nil {
		return nil, "", errors.New("read: company identity guard unavailable")
	}
	rid, e := mustUUID(lease.RevisionID)
	if e != nil {
		return nil, "", e
	}
	sid, e := mustUUID(scope.stationID)
	if e != nil {
		return nil, "", e
	}
	tx, e := r.pool.Begin(ctx)
	if e != nil {
		return nil, "", e
	}
	defer tx.Rollback(context.Background())
	if e := r.companyGuard(ctx, tx, scope.bindings()); e != nil {
		return nil, "", errors.New("read: company identity changed or unavailable")
	}
	rows, e := quality.New(tx).CompanySanctionPage(ctx, quality.CompanySanctionPageParams{RevisionID: rid, StationID: sid, AllowedCnpjs: append([]string(nil), scope.allowed...), Status: status, AfterNative: afterNative, AfterID: afterID, LimitPlusOne: int32(limit + 1)})
	if e != nil {
		return nil, "", e
	}
	mapped := make([]quality.SanctionPageByRevisionRow, 0, len(rows))
	for _, row := range rows {
		mapped = append(mapped, quality.SanctionPageByRevisionRow(row))
	}
	out, next, e := sanctionPageRows(mapped, lease, limit)
	if e != nil {
		return nil, "", e
	}
	if e := tx.Commit(ctx); e != nil {
		return nil, "", e
	}
	return out, next, nil
}
func (r *Reader) CompanySanctionAtScope(ctx context.Context, lease Lease, scope CompanyScope, id string) (SanctionEvent, error) {
	if !scope.validFor(lease) {
		return SanctionEvent{}, errors.New("read: invalid company detail scope")
	}
	normalized, e := application.ValidateStationID(id)
	if e != nil {
		return SanctionEvent{}, e
	}
	if !lease.HasPublished || !scope.view.Available || len(scope.allowed) == 0 {
		return SanctionEvent{}, ErrNotFound
	}
	if r.companyGuard == nil {
		return SanctionEvent{}, errors.New("read: company identity guard unavailable")
	}
	rid, e := mustUUID(lease.RevisionID)
	if e != nil {
		return SanctionEvent{}, e
	}
	sid, e := mustUUID(scope.stationID)
	if e != nil {
		return SanctionEvent{}, e
	}
	rowID, e := mustUUID(normalized)
	if e != nil {
		return SanctionEvent{}, e
	}
	tx, e := r.pool.Begin(ctx)
	if e != nil {
		return SanctionEvent{}, e
	}
	defer tx.Rollback(context.Background())
	if e := r.companyGuard(ctx, tx, scope.bindings()); e != nil {
		return SanctionEvent{}, errors.New("read: company identity changed or unavailable")
	}
	row, e := quality.New(tx).CompanySanctionByID(ctx, quality.CompanySanctionByIDParams{RevisionID: rid, StationID: sid, AllowedCnpjs: append([]string(nil), scope.allowed...), ID: rowID})
	if errors.Is(e, pgx.ErrNoRows) {
		return SanctionEvent{}, ErrNotFound
	}
	if e != nil {
		return SanctionEvent{}, e
	}
	event, e := sanctionDetailRow(quality.QualitySanctionEvent(row), lease)
	if e != nil {
		return SanctionEvent{}, e
	}
	if e := tx.Commit(ctx); e != nil {
		return SanctionEvent{}, e
	}
	return event, nil
}
