//go:build integration

package adapters

import (
	"context"
	"errors"
	"strings"
	"testing"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	"github.com/jackc/pgx/v5"
)

func TestPublisherSourceGuardAtomicReplayAndRetirement(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	repo := directory.NewRepository(p)
	station, e := repo.ResolveCNPJ(ctx, "11222333000181", "Synthetic guarded publication", nil)
	if e != nil {
		t.Fatal(e)
	}
	identity := directory.NewSourceIdentityReader(p)
	bindings := []directoryapp.SourceBinding{{CNPJ: "11222333000181", StationID: station.ID}}
	calls := 0
	publisher := NewPublisher(p).WithGuard(func(ctx context.Context, tx pgx.Tx) error { calls++; return identity.Guard(ctx, tx, bindings) })
	in := sampleInput("b")
	in.Links[0].StationID = station.ID
	m := &idMaker{}
	id, e := publisher.Publish(ctx, in, m.next)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := publisher.Publish(ctx, in, m.next)
	if e != nil || replay != id || calls != 2 {
		t.Fatal("publication replay skipped live source guard", calls, e)
	}
	if e = repo.RetireIdentifier(ctx, "CNPJ", bindings[0].CNPJ); e != nil {
		t.Fatal(e)
	}
	if _, e = publisher.Publish(ctx, in, m.next); !errors.Is(e, directory.ErrSourceIdentityChanged) {
		t.Fatal("stale replay bypassed source guard", e)
	}
	in.Edition = "2026-10"
	in.ManifestHash = strings.Repeat("c", 64)
	if _, e = publisher.Publish(ctx, in, m.next); !errors.Is(e, directory.ErrSourceIdentityChanged) {
		t.Fatal("stale source published new edition", e)
	}
	q := quality.New(p)
	if _, e = q.GetEditionByKey(ctx, quality.GetEditionByKeyParams{Source: in.Source, Stream: in.Stream, Edition: in.Edition}); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("failed guard left partial edition", e)
	}
	current, e := q.GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{Source: in.Source, Stream: in.Stream})
	if e != nil || uuidString(current) != id {
		t.Fatal("failed guard changed current", e)
	}
	var revisions, identifiers, stations int
	if e = p.QueryRow(ctx, "SELECT (SELECT count(*) FROM quality_revisions),(SELECT count(*) FROM directory_identifiers),(SELECT count(*) FROM directory_stations)").Scan(&revisions, &identifiers, &stations); e != nil || revisions != 1 || identifiers != 1 || stations != 1 {
		t.Fatal("guard created identity or partial publication", revisions, identifiers, stations, e)
	}
}
