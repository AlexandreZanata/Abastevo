package adapters

import (
	"context"
	"errors"
	"math"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/driver"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ app.ReconciliationStore = (*Store)(nil)

func decisionFromRows(row q.DriverReconciliationDecision, records []q.DriverReconciliationRecord) (domain.ReconciliationDecision, error) {
	out := domain.ReconciliationDecision{
		ID: uuid(row.ID), Version: row.Version, Kind: row.Kind, Platform: row.Platform,
		From: row.FromAt.Time.UTC(), To: row.ToAt.Time.UTC(),
		PrimaryID: uuid(row.PrimaryID), PrimaryRevision: row.PrimaryRevision,
	}
	if row.PayoutSourceHash.Valid {
		out.PayoutSourceHash = row.PayoutSourceHash.String
	}
	out.Counterparts = make([]domain.ReconciledRecord, len(records))
	for i, record := range records {
		out.Counterparts[i] = domain.ReconciledRecord{ID: uuid(record.RecordID), Revision: record.RecordRevision}
	}
	normalized, e := out.Normalize()
	if e != nil {
		return out, e
	}
	hash, e := domain.Digest(normalized)
	if e != nil || hash != row.ContentSha256 {
		return out, domain.ErrInvalid
	}
	return normalized, nil
}

func (s *Store) validateReconciliation(ctx context.Context, query *q.Queries, owner string, d domain.ReconciliationDecision) error {
	type reference struct {
		id       string
		revision int64
	}
	refs := []reference{{d.PrimaryID, d.PrimaryRevision}}
	for _, c := range d.Counterparts {
		refs = append(refs, reference{c.ID, c.Revision})
	}
	coverage := map[string]bool{}
	for _, ref := range refs {
		row, e := query.GetDriverEarning(ctx, q.GetDriverEarningParams{AccountID: uid(owner), ID: uid(ref.id)})
		if errors.Is(e, pgx.ErrNoRows) {
			return app.ErrNotFound
		}
		if e != nil {
			return storageError(ctx, e)
		}
		if row.Revision != ref.revision || row.Platform != d.Platform {
			return domain.ErrConflict
		}
		if row.FromAt.Time.Before(d.From) || row.ToAt.Time.After(d.To) {
			return domain.ErrConflict
		}
		coverage[row.CoverageKind] = true
	}
	if d.Kind == "SUMMARY_DETAIL" && !(coverage["DETAIL"] && coverage["SUMMARY"]) {
		return domain.ErrConflict
	}
	return nil
}

func (s *Store) SaveReconciliation(ctx context.Context, owner string, in app.SaveReconciliation) (app.ReconciliationReceipt, error) {
	if !domain.ValidID(owner) || !domain.ValidID(in.OperationID) || in.ExpectedVersion < 0 || in.ExpectedVersion == math.MaxInt64 || in.ExpectedEpoch < 0 {
		return app.ReconciliationReceipt{}, domain.ErrInvalid
	}
	decision, e := in.Decision.Normalize()
	if e != nil {
		return app.ReconciliationReceipt{}, e
	}
	if decision.Version != in.ExpectedVersion+1 {
		return app.ReconciliationReceipt{}, domain.ErrInvalid
	}
	in.Decision = decision
	digest, e := domain.Digest(struct {
		Action  string
		Command app.SaveReconciliation
	}{"SAVE_RECONCILIATION", in})
	if e != nil {
		return app.ReconciliationReceipt{}, e
	}
	content, e := domain.Digest(decision)
	if e != nil {
		return app.ReconciliationReceipt{}, e
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return app.ReconciliationReceipt{}, e
	}
	defer rollback(tx)
	if _, e = ownerEpoch(ctx, query, owner, in.ExpectedEpoch); e != nil {
		return app.ReconciliationReceipt{}, e
	}
	if command, e := query.DriverReconciliationCommand(ctx, q.DriverReconciliationCommandParams{AccountID: uid(owner), OperationID: uid(in.OperationID)}); e == nil {
		if command.RequestSha256 != digest {
			return app.ReconciliationReceipt{}, domain.ErrConflict
		}
		if e = s.requirePaid(ctx, tx, owner); e != nil {
			return app.ReconciliationReceipt{}, e
		}
		return app.ReconciliationReceipt{ID: uuid(command.DecisionID), Version: command.Version, Replayed: true}, nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return app.ReconciliationReceipt{}, storageError(ctx, e)
	}
	latest, e := query.LatestDriverReconciliationVersion(ctx, q.LatestDriverReconciliationVersionParams{AccountID: uid(owner), ID: uid(decision.ID)})
	if e != nil {
		return app.ReconciliationReceipt{}, storageError(ctx, e)
	}
	if latest != in.ExpectedVersion {
		return app.ReconciliationReceipt{}, domain.ErrConflict
	}
	if e = s.validateReconciliation(ctx, query, owner, decision); e != nil {
		return app.ReconciliationReceipt{}, e
	}
	var payout pgtype.Text
	if decision.PayoutSourceHash != "" {
		payout = pgtype.Text{String: decision.PayoutSourceHash, Valid: true}
	}
	if e = query.SaveDriverReconciliation(ctx, q.SaveDriverReconciliationParams{
		AccountID: uid(owner), ID: uid(decision.ID), Version: decision.Version, Kind: decision.Kind, Platform: decision.Platform,
		FromAt: stamp(decision.From), ToAt: stamp(decision.To), PrimaryID: uid(decision.PrimaryID), PrimaryRevision: decision.PrimaryRevision,
		PayoutSourceHash: payout, ContentSha256: content,
	}); e != nil {
		return app.ReconciliationReceipt{}, storageError(ctx, e)
	}
	for _, c := range decision.Counterparts {
		if e = query.SaveDriverReconciliationRecord(ctx, q.SaveDriverReconciliationRecordParams{
			AccountID: uid(owner), DecisionID: uid(decision.ID), Version: decision.Version, RecordID: uid(c.ID), RecordRevision: c.Revision,
		}); e != nil {
			return app.ReconciliationReceipt{}, storageError(ctx, e)
		}
	}
	if e = query.SaveDriverReconciliationCommand(ctx, q.SaveDriverReconciliationCommandParams{
		AccountID: uid(owner), OperationID: uid(in.OperationID), RequestSha256: digest, DecisionID: uid(decision.ID), Version: decision.Version,
	}); e != nil {
		return app.ReconciliationReceipt{}, storageError(ctx, e)
	}
	if e = s.requirePaid(ctx, tx, owner); e != nil {
		return app.ReconciliationReceipt{}, e
	}
	if e = s.authorize(ctx, tx, owner, ""); e != nil {
		return app.ReconciliationReceipt{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return app.ReconciliationReceipt{}, storageError(ctx, e)
	}
	return app.ReconciliationReceipt{ID: decision.ID, Version: decision.Version}, nil
}

func (s *Store) GetReconciliation(ctx context.Context, owner, id string, version int64) (domain.ReconciliationDecision, error) {
	if !domain.ValidID(owner) || !domain.ValidID(id) || version < 1 {
		return domain.ReconciliationDecision{}, domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return domain.ReconciliationDecision{}, e
	}
	defer rollback(tx)
	row, e := query.GetDriverReconciliation(ctx, q.GetDriverReconciliationParams{AccountID: uid(owner), ID: uid(id), Version: version})
	if errors.Is(e, pgx.ErrNoRows) {
		return domain.ReconciliationDecision{}, app.ErrNotFound
	}
	if e != nil {
		return domain.ReconciliationDecision{}, storageError(ctx, e)
	}
	records, e := query.DriverReconciliationRecords(ctx, q.DriverReconciliationRecordsParams{AccountID: uid(owner), DecisionID: uid(id), Version: version})
	if e != nil {
		return domain.ReconciliationDecision{}, storageError(ctx, e)
	}
	return decisionFromRows(row, records)
}

func (s *Store) ListReconciliations(ctx context.Context, owner, after string, limit int) ([]domain.ReconciliationDecision, error) {
	if !domain.ValidID(owner) || limit < 1 || limit > domain.MaxItems {
		return nil, domain.ErrInvalid
	}
	if after == "" {
		after = firstID
	} else if !domain.ValidID(after) {
		return nil, domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return nil, e
	}
	defer rollback(tx)
	rows, e := query.ListDriverReconciliations(ctx, q.ListDriverReconciliationsParams{AccountID: uid(owner), ID: uid(after), Limit: int32(limit)})
	if e != nil {
		return nil, storageError(ctx, e)
	}
	out := make([]domain.ReconciliationDecision, len(rows))
	for i, row := range rows {
		records, e := query.DriverReconciliationRecords(ctx, q.DriverReconciliationRecordsParams{AccountID: uid(owner), DecisionID: row.ID, Version: row.Version})
		if e != nil {
			return nil, storageError(ctx, e)
		}
		out[i], e = decisionFromRows(row, records)
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
