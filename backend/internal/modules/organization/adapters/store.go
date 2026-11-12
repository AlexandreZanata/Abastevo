// Package adapters persists organization roots under forced scoped RLS.
package adapters

import (
	"context"
	"errors"
	"math"
	"slices"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/organization"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

var _ app.Repository = (*Store)(nil)
var _ app.TransactionRepository = (*Store)(nil)

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func safe(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, kernel.ErrInvalidOrgScope) {
		return domain.ErrScope
	}
	for _, known := range []error{domain.ErrScope, domain.ErrInvalid, domain.ErrConflict,
		domain.ErrClosed, domain.ErrRevision, app.ErrNotFound, app.ErrUninitialized} {
		if errors.Is(err, known) {
			return known
		}
	}
	return app.ErrUnavailable
}
func (s *Store) run(ctx context.Context, scope kernel.OrgScope, work func(pgx.Tx) error) error {
	if scope.Validate() != nil {
		return domain.ErrScope
	}
	if s == nil || s.pool == nil {
		return app.ErrUnavailable
	}
	return safe(ctx, database.WithOrgTx(ctx, s.pool, scope, work))
}
func (s *Store) Get(ctx context.Context, scope kernel.OrgScope) (domain.Organization, error) {
	var result domain.Organization
	err := s.run(ctx, scope, func(tx pgx.Tx) error {
		var err error
		result, err = s.GetTx(ctx, tx, scope)
		return err
	})
	if err != nil {
		return domain.Organization{}, err
	}
	return result, nil
}
func (s *Store) Create(ctx context.Context, scope kernel.OrgScope, value domain.Organization) (bool, error) {
	if _, err := initial(scope, value); err != nil {
		return false, err
	}
	var changed bool
	err := s.run(ctx, scope, func(tx pgx.Tx) error {
		var err error
		changed, err = s.CreateTx(ctx, tx, scope, value)
		return err
	})
	if err != nil {
		return false, err
	}
	return changed, nil
}
func (s *Store) Save(ctx context.Context, scope kernel.OrgScope, expected int64, value domain.Organization) (bool, error) {
	if _, err := advance(scope, expected, value); err != nil {
		return false, err
	}
	var changed bool
	err := s.run(ctx, scope, func(tx pgx.Tx) error {
		var err error
		changed, err = s.SaveTx(ctx, tx, scope, expected, value)
		return err
	})
	if err != nil {
		return false, err
	}
	return changed, nil
}
func initial(scope kernel.OrgScope, value domain.Organization) (domain.Snapshot, error) {
	snap, err := value.Snapshot(scope)
	if err != nil {
		return domain.Snapshot{}, err
	}
	if snap.State != domain.PendingSetup || snap.Revision != 1 {
		return domain.Snapshot{}, domain.ErrInvalid
	}
	return snap, nil
}
func advance(scope kernel.OrgScope, expected int64, value domain.Organization) (domain.Snapshot, error) {
	snap, err := value.Snapshot(scope)
	if err != nil {
		return domain.Snapshot{}, err
	}
	if expected == math.MaxInt64 {
		return domain.Snapshot{}, domain.ErrRevision
	}
	if expected < 1 || snap.Revision != expected+1 {
		return domain.Snapshot{}, domain.ErrInvalid
	}
	return snap, nil
}
func check(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope) error {
	return safe(ctx, database.ValidateOrgTx(ctx, tx, scope))
}
func (s *Store) GetTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope) (domain.Organization, error) {
	if err := check(ctx, tx, scope); err != nil {
		return domain.Organization{}, err
	}
	return read(ctx, tx, scope)
}
func read(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope) (domain.Organization, error) {
	row, err := q.New(tx).GetRoot(ctx, scope.OrgID().String())
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Organization{}, app.ErrNotFound
	}
	if err != nil {
		return domain.Organization{}, safe(ctx, err)
	}
	if !row.HasSnapshot {
		return domain.Organization{}, app.ErrUninitialized
	}
	id, err := kernel.ParseOrgID(row.OrgID)
	if err != nil {
		return domain.Organization{}, app.ErrUnavailable
	}
	kinds := make([]domain.Kind, len(row.KindTags))
	for i, k := range row.KindTags {
		kinds[i] = domain.Kind(k)
	}
	result, err := domain.Rehydrate(scope, domain.Snapshot{ID: id, Profile: domain.Profile{
		Name: row.DisplayName, TimeZone: row.TimeZone, Kinds: kinds,
	}, State: domain.State(row.State), Revision: row.Revision, SuspensionReason: domain.Reason(row.SuspensionReason)})
	if err != nil {
		return domain.Organization{}, app.ErrUnavailable
	}
	return result, nil
}
func kinds(snap domain.Snapshot) []string {
	result := make([]string, len(snap.Profile.Kinds))
	for i, k := range snap.Profile.Kinds {
		result[i] = string(k)
	}
	return result
}
func same(left, right domain.Snapshot) bool {
	return left.ID == right.ID && left.State == right.State && left.Revision == right.Revision &&
		left.SuspensionReason == right.SuspensionReason && left.Profile.Name == right.Profile.Name &&
		left.Profile.TimeZone == right.Profile.TimeZone && slices.Equal(left.Profile.Kinds, right.Profile.Kinds)
}
func (s *Store) CreateTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, value domain.Organization) (bool, error) {
	snap, err := initial(scope, value)
	if err != nil {
		return false, err
	}
	if err := check(ctx, tx, scope); err != nil {
		return false, err
	}
	n, err := q.New(tx).InsertRoot(ctx, q.InsertRootParams{OrgID: scope.OrgID().String(),
		DisplayName: snap.Profile.Name, TimeZone: snap.Profile.TimeZone, KindTags: kinds(snap)})
	if err != nil {
		return false, safe(ctx, err)
	}
	if n == 1 {
		return true, nil
	}
	prior, err := read(ctx, tx, scope)
	if errors.Is(err, app.ErrUninitialized) {
		return false, app.ErrUninitialized
	}
	if err != nil {
		return false, err
	}
	before, err := prior.Snapshot(scope)
	if err != nil {
		return false, app.ErrUnavailable
	}
	if same(before, snap) {
		return false, nil
	}
	return false, domain.ErrConflict
}
func (s *Store) SaveTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, expected int64, value domain.Organization) (bool, error) {
	snap, err := advance(scope, expected, value)
	if err != nil {
		return false, err
	}
	if err := check(ctx, tx, scope); err != nil {
		return false, err
	}
	n, err := q.New(tx).AdvanceRoot(ctx, q.AdvanceRootParams{OrgID: scope.OrgID().String(),
		DisplayName: snap.Profile.Name, TimeZone: snap.Profile.TimeZone, KindTags: kinds(snap),
		State: string(snap.State), Revision: snap.Revision, SuspensionReason: string(snap.SuspensionReason), ExpectedRevision: expected})
	if err != nil {
		return false, safe(ctx, err)
	}
	if n == 1 {
		return true, nil
	}
	prior, err := read(ctx, tx, scope)
	if err != nil {
		return false, err
	}
	before, err := prior.Snapshot(scope)
	if err != nil {
		return false, app.ErrUnavailable
	}
	if same(before, snap) {
		return false, nil
	}
	if before.State == domain.Closed {
		return false, domain.ErrClosed
	}
	return false, domain.ErrConflict
}
