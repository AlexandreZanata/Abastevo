package adapters

import (
	"context"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var ErrSourceIdentityChanged = errors.New("directory: source identity changed")
var ErrSourceIdentityUnavailable = errors.New("directory: source identity unavailable")

// SourceIdentityReader exposes only read/lock capabilities, never identity
// creation. Callers compose values through ports, not Directory generated SQL.
type SourceIdentityReader struct{ pool *pgxpool.Pool }

func NewSourceIdentityReader(pool *pgxpool.Pool) *SourceIdentityReader {
	return &SourceIdentityReader{pool: pool}
}
func normalizedSourceCNPJ(raw string) bool {
	id, e := kernel.ParseCNPJ(raw)
	return e == nil && id.Normalized() == raw
}
func (r *SourceIdentityReader) Exact(ctx context.Context, cnpj string) ([]application.SourceIdentity, error) {
	if !normalizedSourceCNPJ(cnpj) {
		return nil, ErrSourceIdentityUnavailable
	}
	rows, e := directory.New(r.pool).SourceExactIdentities(ctx, cnpj)
	if e != nil {
		return nil, ErrSourceIdentityUnavailable
	}
	out := make([]application.SourceIdentity, 0, len(rows))
	for _, row := range rows {
		out = append(out, application.SourceIdentity{StationID: uuidString(row.StationID), Current: row.IsCurrent, StationActive: row.StationActive})
	}
	return out, nil
}
func (r *SourceIdentityReader) OtherBranch(ctx context.Context, cnpj string) (bool, error) {
	if !normalizedSourceCNPJ(cnpj) {
		return false, ErrSourceIdentityUnavailable
	}
	found, e := directory.New(r.pool).SourceHasOtherBranch(ctx, directory.SourceHasOtherBranchParams{Base: cnpj[:8], Cnpj: cnpj})
	if e != nil {
		return false, ErrSourceIdentityUnavailable
	}
	return found, nil
}

// Guard locks live expected bindings through the caller's publication commit,
// then rechecks retained canonical history. Owned identifier retirement/new
// binding and station status changes conflict with these row locks. It also
// runs for publication replay; stale preparation cannot bypass identity checks.
func (r *SourceIdentityReader) Guard(ctx context.Context, tx pgx.Tx, bindings []application.SourceBinding) error {
	if tx == nil {
		return ErrSourceIdentityUnavailable
	}
	expected := make(map[string]pgtype.UUID, len(bindings))
	for _, binding := range bindings {
		id, e := mustUUID(binding.StationID)
		if e != nil || !normalizedSourceCNPJ(binding.CNPJ) {
			return ErrSourceIdentityChanged
		}
		if old, exists := expected[binding.CNPJ]; exists && old != id {
			return ErrSourceIdentityChanged
		}
		expected[binding.CNPJ] = id
	}
	keys := make([]string, 0, len(expected))
	for cnpj := range expected {
		keys = append(keys, cnpj)
	}
	sort.Strings(keys)
	q := directory.New(tx)
	const chunk = 512
	for offset := 0; offset < len(keys); offset += chunk {
		part := keys[offset:min(offset+chunk, len(keys))]
		ids := make([]pgtype.UUID, len(part))
		for i, key := range part {
			ids[i] = expected[key]
		}
		locked, e := q.LockSourceBindings(ctx, directory.LockSourceBindingsParams{Cnpjs: part, StationIds: ids})
		if e != nil {
			return ErrSourceIdentityUnavailable
		}
		if len(locked) != len(part) {
			return ErrSourceIdentityChanged
		}
		for _, row := range locked {
			if row.StationID != expected[row.Cnpj] {
				return ErrSourceIdentityChanged
			}
		}
		history, e := q.SourceBoundHistory(ctx, part)
		if e != nil {
			return ErrSourceIdentityUnavailable
		}
		if len(history) != len(part) {
			return ErrSourceIdentityChanged
		}
		seen := make(map[string]bool, len(part))
		for _, row := range history {
			if seen[row.Cnpj] || row.StationID != expected[row.Cnpj] {
				return ErrSourceIdentityChanged
			}
			seen[row.Cnpj] = true
		}
	}
	return nil
}
