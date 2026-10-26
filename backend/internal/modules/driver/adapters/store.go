package adapters

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/driver"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OwnershipPort must check live account/vehicle ownership in this transaction.
// Its owner implementation lives in cloudsync, not in driver SQL. Nil denies.
type OwnershipPort func(context.Context, pgx.Tx, string, string) (bool, error)
type Store struct {
	pool      *pgxpool.Pool
	ownership OwnershipPort
	ledger    LedgerCostPort
}

var _ app.EarningsStore = (*Store)(nil)

func New(pool *pgxpool.Pool, ownership OwnershipPort) *Store {
	return NewWithLedger(pool, ownership, nil)
}
func NewWithLedger(pool *pgxpool.Pool, ownership OwnershipPort, ledger LedgerCostPort) *Store {
	return &Store{pool: pool, ownership: ownership, ledger: ledger}
}
func uid(s string) pgtype.UUID { var v pgtype.UUID; _ = v.Scan(s); return v }
func uuid(v pgtype.UUID) string {
	b := v.Bytes
	return hex.EncodeToString(b[0:4]) + "-" + hex.EncodeToString(b[4:6]) + "-" + hex.EncodeToString(b[6:8]) + "-" + hex.EncodeToString(b[8:10]) + "-" + hex.EncodeToString(b[10:16])
}
func stamp(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
func money(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
func optional(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}
func storageError(ctx context.Context, e error) error {
	if e == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for _, known := range []error{domain.ErrInvalid, domain.ErrConflict, app.ErrDenied, app.ErrNotFound, app.ErrErased, app.ErrUnavailable} {
		if errors.Is(e, known) {
			return known
		}
	}
	var pg *pgconn.PgError
	if errors.As(e, &pg) {
		switch pg.Code {
		case "23505":
			return domain.ErrConflict
		case "23514", "22003":
			return domain.ErrInvalid
		case "23503":
			return app.ErrDenied
		}
	}
	return app.ErrUnavailable
}
func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func (s *Store) authorize(ctx context.Context, tx pgx.Tx, owner, vehicle string) error {
	if s.ownership == nil {
		return app.ErrDenied
	}
	ok, e := s.ownership(ctx, tx, owner, vehicle)
	if e != nil {
		return storageError(ctx, e)
	}
	if !ok {
		return app.ErrDenied
	}
	return nil
}
func (s *Store) begin(ctx context.Context, owner, vehicle string) (pgx.Tx, *q.Queries, error) {
	if !domain.ValidID(owner) {
		return nil, nil, domain.ErrInvalid
	}
	if s.pool == nil || s.ownership == nil {
		return nil, nil, app.ErrDenied
	}
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if e != nil {
		return nil, nil, storageError(ctx, e)
	}
	if e = s.authorize(ctx, tx, owner, vehicle); e != nil {
		rollback(tx)
		return nil, nil, e
	}
	query := q.New(tx)
	if e = query.LockDriverOwner(ctx, owner); e != nil {
		rollback(tx)
		return nil, nil, storageError(ctx, e)
	}
	return tx, query, nil
}
func ownerEpoch(ctx context.Context, query *q.Queries, owner string, expected int64) (int64, error) {
	if expected < 0 {
		return 0, domain.ErrInvalid
	}
	if e := query.EnsureDriverOwnerState(ctx, uid(owner)); e != nil {
		return 0, storageError(ctx, e)
	}
	epoch, e := query.LockDriverEpoch(ctx, uid(owner))
	if e != nil {
		return 0, storageError(ctx, e)
	}
	if epoch != expected {
		return epoch, app.ErrErased
	}
	return epoch, nil
}
func replay(ctx context.Context, query *q.Queries, owner, operation, digest string) (app.Receipt, bool, error) {
	r, e := query.DriverCommand(ctx, q.DriverCommandParams{AccountID: uid(owner), OperationID: uid(operation)})
	if errors.Is(e, pgx.ErrNoRows) {
		return app.Receipt{}, false, nil
	}
	if e != nil {
		return app.Receipt{}, false, storageError(ctx, e)
	}
	if r.RequestSha256 != digest {
		return app.Receipt{}, false, domain.ErrConflict
	}
	return app.Receipt{ID: uuid(r.RecordID), Revision: r.Revision, Epoch: r.Epoch, Replayed: true}, true, nil
}
func (s *Store) commitReceipt(ctx context.Context, tx pgx.Tx, query *q.Queries, owner, vehicle, operation, digest string, receipt app.Receipt, save bool) (app.Receipt, error) {
	if save {
		if e := query.SaveDriverCommand(ctx, q.SaveDriverCommandParams{AccountID: uid(owner), OperationID: uid(operation), RequestSha256: digest, RecordID: uid(receipt.ID), Revision: receipt.Revision, Epoch: receipt.Epoch}); e != nil {
			return app.Receipt{}, storageError(ctx, e)
		}
	}
	if e := s.authorize(ctx, tx, owner, vehicle); e != nil {
		return app.Receipt{}, e
	}
	if e := tx.Commit(ctx); e != nil {
		return app.Receipt{}, storageError(ctx, e)
	}
	return receipt, nil
}
func (s *Store) Save(ctx context.Context, owner string, c app.SaveEarning) (app.Receipt, error) {
	if !domain.ValidID(c.OperationID) || c.ExpectedRevision < 0 || c.ExpectedRevision == math.MaxInt64 || c.ExpectedEpoch < 0 {
		return app.Receipt{}, domain.ErrInvalid
	}
	r, e := c.Record.Normalize()
	if e != nil {
		return app.Receipt{}, e
	}
	c.Record = r
	digest, e := domain.Digest(struct {
		Action  string
		Command app.SaveEarning
	}{"UPSERT", c})
	if e != nil {
		return app.Receipt{}, e
	}
	business, e := r.BusinessHash()
	if e != nil {
		return app.Receipt{}, e
	}
	tx, query, e := s.begin(ctx, owner, r.VehicleID)
	if e != nil {
		return app.Receipt{}, e
	}
	defer rollback(tx)
	epoch, e := ownerEpoch(ctx, query, owner, c.ExpectedEpoch)
	if e != nil {
		return app.Receipt{}, e
	}
	erased, e := query.DriverSourceErased(ctx, q.DriverSourceErasedParams{AccountID: uid(owner), Platform: r.Platform, SourceHash: r.SourceHash})
	if e != nil {
		return app.Receipt{}, storageError(ctx, e)
	}
	if erased {
		return app.Receipt{}, app.ErrErased
	}
	_, e = query.DriverTombstone(ctx, q.DriverTombstoneParams{AccountID: uid(owner), RecordID: uid(r.ID)})
	if e == nil {
		return app.Receipt{}, app.ErrErased
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return app.Receipt{}, storageError(ctx, e)
	}
	if receipt, ok, e := replay(ctx, query, owner, c.OperationID, digest); e != nil {
		return app.Receipt{}, e
	} else if ok {
		return s.commitReceipt(ctx, tx, query, owner, r.VehicleID, c.OperationID, digest, receipt, false)
	}
	source, e := query.FindDriverSource(ctx, q.FindDriverSourceParams{AccountID: uid(owner), Platform: r.Platform, SourceHash: r.SourceHash})
	if e == nil {
		if c.ExpectedRevision == 0 && source.ContentSha256 == business {
			return s.commitReceipt(ctx, tx, query, owner, r.VehicleID, c.OperationID, digest, app.Receipt{ID: uuid(source.ID), Revision: source.Revision, Epoch: epoch, Replayed: true}, true)
		}
		if uuid(source.ID) != r.ID {
			return app.Receipt{}, domain.ErrConflict
		}
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return app.Receipt{}, storageError(ctx, e)
	}
	old, e := query.GetDriverEarning(ctx, q.GetDriverEarningParams{AccountID: uid(owner), ID: uid(r.ID)})
	if errors.Is(e, pgx.ErrNoRows) {
		if c.ExpectedRevision != 0 {
			return app.Receipt{}, domain.ErrConflict
		}
	} else if e != nil {
		return app.Receipt{}, storageError(ctx, e)
	} else if old.Revision != c.ExpectedRevision || old.SourceHash != r.SourceHash || old.Platform != r.Platform {
		return app.Receipt{}, domain.ErrConflict
	}
	overlap, e := query.DriverCoverageConflict(ctx, q.DriverCoverageConflictParams{AccountID: uid(owner), Platform: r.Platform, ExcludingID: uid(r.ID), FromAt: stamp(r.From), ToAt: stamp(r.To)})
	if e != nil {
		return app.Receipt{}, storageError(ctx, e)
	}
	if overlap {
		return app.Receipt{}, domain.ErrConflict
	}
	row, e := query.SaveDriverEarning(ctx, q.SaveDriverEarningParams{AccountID: uid(owner), ID: uid(r.ID), VehicleID: uid(r.VehicleID), Platform: r.Platform, SourceHash: r.SourceHash, CoverageKind: r.Coverage, Currency: r.Currency, FromAt: stamp(r.From), ToAt: stamp(r.To), Timezone: r.Timezone, Basis: r.Money.Basis, GrossCentavos: money(r.Money.Gross), FeesCentavos: money(r.Money.Fees), TipsCentavos: money(r.Money.Tips), BonusCentavos: money(r.Money.Bonus), RefundCentavos: money(r.Money.Refund), NetCentavos: money(r.Money.Net), ContentSha256: business, ExpectedRevision: c.ExpectedRevision})
	if e != nil {
		return app.Receipt{}, storageError(ctx, e)
	}
	payload, e := json.Marshal(r)
	if e != nil {
		return app.Receipt{}, domain.ErrInvalid
	}
	if e = query.SaveDriverRevision(ctx, q.SaveDriverRevisionParams{AccountID: uid(owner), RecordID: row.ID, Revision: row.Revision, Payload: payload}); e != nil {
		return app.Receipt{}, storageError(ctx, e)
	}
	return s.commitReceipt(ctx, tx, query, owner, r.VehicleID, c.OperationID, digest, app.Receipt{ID: r.ID, Revision: row.Revision, Epoch: epoch}, true)
}
func (s *Store) SaveAdjustment(ctx context.Context, owner string, c app.SaveAdjustment) (app.Receipt, error) {
	a := c.Adjustment
	if !domain.ValidID(c.OperationID) || !domain.ValidSource(c.SourceHash) || !domain.ValidID(a.ID) || !domain.ValidID(a.TargetID) || a.Validate() != nil || c.ExpectedEpoch < 0 {
		return app.Receipt{}, domain.ErrInvalid
	}
	digest, e := domain.Digest(struct {
		Action  string
		Command app.SaveAdjustment
	}{"ADJUST", c})
	if e != nil {
		return app.Receipt{}, e
	}
	semantic := a
	semantic.ID = ""
	content, e := domain.Digest(struct {
		Source     string
		Adjustment domain.Adjustment
	}{c.SourceHash, semantic})
	if e != nil {
		return app.Receipt{}, e
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return app.Receipt{}, e
	}
	defer rollback(tx)
	epoch, e := ownerEpoch(ctx, query, owner, c.ExpectedEpoch)
	if e != nil {
		return app.Receipt{}, e
	}
	target, e := query.GetDriverEarning(ctx, q.GetDriverEarningParams{AccountID: uid(owner), ID: uid(a.TargetID)})
	if errors.Is(e, pgx.ErrNoRows) {
		return app.Receipt{}, app.ErrNotFound
	}
	if e != nil {
		return app.Receipt{}, storageError(ctx, e)
	}
	vehicle := uuid(target.VehicleID)
	if e = s.authorize(ctx, tx, owner, vehicle); e != nil {
		return app.Receipt{}, e
	}
	if receipt, ok, e := replay(ctx, query, owner, c.OperationID, digest); e != nil {
		return app.Receipt{}, e
	} else if ok {
		return s.commitReceipt(ctx, tx, query, owner, vehicle, c.OperationID, digest, receipt, false)
	}
	source, e := query.DriverAdjustmentSource(ctx, q.DriverAdjustmentSourceParams{AccountID: uid(owner), SourceHash: c.SourceHash})
	if e == nil {
		if source.ContentSha256 != content {
			return app.Receipt{}, domain.ErrConflict
		}
		return s.commitReceipt(ctx, tx, query, owner, vehicle, c.OperationID, digest, app.Receipt{ID: uuid(source.ID), Revision: source.TargetRevision, Epoch: epoch, Replayed: true}, true)
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return app.Receipt{}, storageError(ctx, e)
	}
	if target.Revision != a.TargetRevision {
		return app.Receipt{}, domain.ErrConflict
	}
	if e = query.SaveDriverAdjustment(ctx, q.SaveDriverAdjustmentParams{AccountID: uid(owner), ID: uid(a.ID), SourceHash: c.SourceHash, TargetID: uid(a.TargetID), TargetRevision: a.TargetRevision, Kind: a.Kind, DeltaCentavos: a.Delta, ContentSha256: content}); e != nil {
		return app.Receipt{}, storageError(ctx, e)
	}
	return s.commitReceipt(ctx, tx, query, owner, vehicle, c.OperationID, digest, app.Receipt{ID: a.ID, Revision: a.TargetRevision, Epoch: epoch}, true)
}
func record(row q.DriverEarning) domain.EarningRecord {
	return domain.EarningRecord{ID: uuid(row.ID), VehicleID: uuid(row.VehicleID), Platform: row.Platform, SourceHash: row.SourceHash, Coverage: row.CoverageKind, Currency: row.Currency, From: row.FromAt.Time.UTC(), To: row.ToAt.Time.UTC(), Timezone: row.Timezone, Money: domain.Earnings{Basis: row.Basis, Gross: optional(row.GrossCentavos), Fees: optional(row.FeesCentavos), Tips: optional(row.TipsCentavos), Bonus: optional(row.BonusCentavos), Refund: optional(row.RefundCentavos), Net: optional(row.NetCentavos)}}
}
func enrich(ctx context.Context, query *q.Queries, owner string, rows []q.DriverEarning) ([]app.StoredEarning, error) {
	ids := make([]pgtype.UUID, len(rows))
	out := make([]app.StoredEarning, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
		out[i] = app.StoredEarning{Record: record(r), Revision: r.Revision}
	}
	if len(ids) == 0 {
		return out, nil
	}
	stats, e := query.DriverAdjustmentStats(ctx, q.DriverAdjustmentStatsParams{AccountID: uid(owner), TargetIds: ids})
	if e != nil {
		return nil, storageError(ctx, e)
	}
	byID := map[string]q.DriverAdjustmentStatsRow{}
	for _, s := range stats {
		byID[uuid(s.TargetID)] = s
	}
	for i := range out {
		s := byID[out[i].Record.ID]
		out[i].AdjustmentCount = s.AdjustmentCount
		out[i].NeedsReconciliation = s.NeedsReconciliation
	}
	return out, nil
}
func (s *Store) commitOwner(ctx context.Context, tx pgx.Tx, owner string) error {
	if e := s.authorize(ctx, tx, owner, ""); e != nil {
		return e
	}
	return storageError(ctx, tx.Commit(ctx))
}

func (s *Store) Get(ctx context.Context, owner, id string) (app.StoredEarning, error) {
	if !domain.ValidID(id) {
		return app.StoredEarning{}, domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return app.StoredEarning{}, e
	}
	defer rollback(tx)
	row, e := query.GetDriverEarning(ctx, q.GetDriverEarningParams{AccountID: uid(owner), ID: uid(id)})
	if errors.Is(e, pgx.ErrNoRows) {
		return app.StoredEarning{}, app.ErrNotFound
	}
	if e != nil {
		return app.StoredEarning{}, storageError(ctx, e)
	}
	out, e := enrich(ctx, query, owner, []q.DriverEarning{row})
	if e != nil {
		return app.StoredEarning{}, e
	}
	if e = s.commitOwner(ctx, tx, owner); e != nil {
		return app.StoredEarning{}, storageError(ctx, e)
	}
	return out[0], nil
}

const firstID = "00000000-0000-0000-0000-000000000000"

func cursor(after string, limit int) (string, error) {
	if limit < 1 || limit > domain.MaxItems {
		return "", domain.ErrInvalid
	}
	if after == "" {
		return firstID, nil
	}
	if !domain.ValidID(after) {
		return "", domain.ErrInvalid
	}
	return after, nil
}
func (s *Store) List(ctx context.Context, owner, after string, limit int) (app.Page, error) {
	after, e := cursor(after, limit)
	if e != nil {
		return app.Page{}, e
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return app.Page{}, e
	}
	defer rollback(tx)
	rows, e := query.ListDriverEarnings(ctx, q.ListDriverEarningsParams{AccountID: uid(owner), ID: uid(after), Limit: int32(limit + 1)})
	if e != nil {
		return app.Page{}, storageError(ctx, e)
	}
	out := app.Page{}
	if len(rows) > limit {
		rows = rows[:limit]
		out.NextID = uuid(rows[len(rows)-1].ID)
	}
	out.Records, e = enrich(ctx, query, owner, rows)
	if e != nil {
		return out, e
	}
	if e = s.commitOwner(ctx, tx, owner); e != nil {
		return app.Page{}, storageError(ctx, e)
	}
	return out, nil
}
func (s *Store) ListAdjustments(ctx context.Context, owner, target, after string, limit int) (app.AdjustmentPage, error) {
	after, e := cursor(after, limit)
	if e != nil || !domain.ValidID(target) {
		return app.AdjustmentPage{}, domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return app.AdjustmentPage{}, e
	}
	defer rollback(tx)
	if _, e = query.GetDriverEarning(ctx, q.GetDriverEarningParams{AccountID: uid(owner), ID: uid(target)}); errors.Is(e, pgx.ErrNoRows) {
		return app.AdjustmentPage{}, app.ErrNotFound
	} else if e != nil {
		return app.AdjustmentPage{}, storageError(ctx, e)
	}
	rows, e := query.PageDriverAdjustments(ctx, q.PageDriverAdjustmentsParams{AccountID: uid(owner), TargetID: uid(target), ID: uid(after), Limit: int32(limit + 1)})
	if e != nil {
		return app.AdjustmentPage{}, storageError(ctx, e)
	}
	out := app.AdjustmentPage{Adjustments: []app.StoredAdjustment{}}
	if len(rows) > limit {
		rows = rows[:limit]
		out.NextID = uuid(rows[len(rows)-1].ID)
	}
	for _, a := range rows {
		out.Adjustments = append(out.Adjustments, app.StoredAdjustment{SourceHash: a.SourceHash, Adjustment: domain.Adjustment{ID: uuid(a.ID), TargetID: uuid(a.TargetID), Kind: a.Kind, TargetRevision: a.TargetRevision, Delta: a.DeltaCentavos}})
	}
	if e = s.commitOwner(ctx, tx, owner); e != nil {
		return app.AdjustmentPage{}, storageError(ctx, e)
	}
	return out, nil
}
func (s *Store) EraseRecord(ctx context.Context, owner, id string, revision, expectedEpoch int64) error {
	if !domain.ValidID(id) || revision < 1 || expectedEpoch < 0 {
		return domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return e
	}
	defer rollback(tx)
	if _, e = ownerEpoch(ctx, query, owner, expectedEpoch); e != nil {
		return e
	}
	tomb, e := query.DriverTombstone(ctx, q.DriverTombstoneParams{AccountID: uid(owner), RecordID: uid(id)})
	if e == nil {
		if tomb.ErasedRevision != revision {
			return domain.ErrConflict
		}
		return s.commitOwner(ctx, tx, owner)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return storageError(ctx, e)
	}
	row, e := query.GetDriverEarning(ctx, q.GetDriverEarningParams{AccountID: uid(owner), ID: uid(id)})
	if errors.Is(e, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	if e != nil {
		return storageError(ctx, e)
	}
	if row.Revision != revision {
		return domain.ErrConflict
	}
	if e = query.TombstoneDriverEarning(ctx, q.TombstoneDriverEarningParams{AccountID: uid(owner), RecordID: row.ID, Platform: row.Platform, SourceHash: row.SourceHash, ErasedRevision: row.Revision}); e != nil {
		return storageError(ctx, e)
	}
	if e = query.DeleteDriverEarning(ctx, q.DeleteDriverEarningParams{AccountID: uid(owner), ID: row.ID}); e != nil {
		return storageError(ctx, e)
	}
	return s.commitOwner(ctx, tx, owner)
}
func (s *Store) EraseOwner(ctx context.Context, owner string, expectedEpoch int64) error {
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return e
	}
	defer rollback(tx)
	if _, e = ownerEpoch(ctx, query, owner, expectedEpoch); e != nil {
		return e
	}
	if _, e = query.AdvanceDriverEpoch(ctx, uid(owner)); e != nil {
		return storageError(ctx, e)
	}
	for _, remove := range []func(context.Context, pgtype.UUID) error{query.DeleteDriverOwnerEarnings, query.DeleteDriverOwnerCommands, query.DeleteDriverOwnerTombstones} {
		if e = remove(ctx, uid(owner)); e != nil {
			return storageError(ctx, e)
		}
	}
	return s.commitOwner(ctx, tx, owner)
}
