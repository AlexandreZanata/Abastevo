//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDatasetDefinitionRequiresCurrentSourceReviewsInPostGIS(t *testing.T) {
	f := orgtest.New(t)
	ctx := t.Context()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "GRANT SELECT ON enterprise_data_source_reviews,enterprise_regional_datasets TO "+role); err != nil {
		t.Fatal(err)
	}
	readFromPool := func(ctx context.Context, pool *pgxpool.Pool, revision int64) (adapters.AdmittedDataset, error) {
		var dataset adapters.AdmittedDataset
		err := database.WithOrgBootstrapTx(ctx, pool, func(tx pgx.Tx) error {
			catalog, err := adapters.OpenCatalog(ctx, tx)
			if err != nil {
				return err
			}
			dataset, err = catalog.ReadAdmitted(ctx, "synthetic-regional", revision)
			return err
		})
		return dataset, err
	}
	read := func(revision int64) (adapters.AdmittedDataset, error) {
		return readFromPool(ctx, f.Runtime, revision)
	}
	if _, err := read(1); !errors.Is(err, adapters.ErrDatasetNotFound) {
		t.Fatalf("a commercial catalog was seeded without review: %v", err)
	}

	exampleHash := func(value string) string { return "sha256:" + strings.Repeat(value, 64) }
	registry, price, snapshot := exampleHash("a"), exampleHash("b"), exampleHash("c")
	const reviewSQL = `INSERT INTO enterprise_data_source_reviews
(source_code,source_digest,revision,status,decision_ref,license_ref,attribution)
VALUES($1,$2,$3,$4,'synthetic-review','synthetic-license','Synthetic official source')`
	sourceReview := func(source domain.Source, checksum string, revision int64, state domain.RightsState) {
		t.Helper()
		if _, err := f.Admin.Exec(ctx, reviewSQL, source, checksum, revision, state); err != nil {
			t.Fatal(err)
		}
	}
	putDataset := func(revision, priceApproval int64) {
		t.Helper()
		if _, err := f.Admin.Exec(ctx, `INSERT INTO enterprise_regional_datasets
(dataset_key,revision,display_name,state_code,minimum_stations,method_version,snapshot_digest,
 official_revision,registry_digest,registry_review,price_digest,price_review)
VALUES('synthetic-regional',$1,'Synthetic regional dataset','MT',5,'abastevo-metrics-v1',$2,$3,$4,1,$5,$6)`,
			revision, snapshot, f.Organizations[0].RecordID, registry, price, priceApproval); err != nil {
			t.Fatal(err)
		}
	}
	sourceReview(domain.PublicRegistry, registry, 1, domain.Approved)
	sourceReview(domain.OfficialPrices, price, 1, domain.RightsPending)
	putDataset(1, 1)
	if _, err := read(1); !errors.Is(err, domain.ErrRightsPending) {
		t.Fatalf("pending official source became eligible: %v", err)
	}
	sourceReview(domain.OfficialPrices, price, 2, domain.Approved)
	putDataset(2, 2)
	got, err := read(2)
	if err != nil || got.Definition.Revision != 2 || got.Definition.Snapshot != snapshot ||
		got.Definition.MinimumStations != 5 || got.Reviews[1].Pin.ReviewRevision != 2 {
		t.Fatalf("reviewed dataset did not preserve exact method basis: %+v %v", got, err)
	}
	if _, err := read(1); !errors.Is(err, domain.ErrRightsPending) {
		t.Fatalf("new approval activated an older dataset revision: %v", err)
	}
	if err := database.WithOrgBootstrapTx(ctx, f.Runtime, func(tx pgx.Tx) error {
		catalog, err := adapters.OpenCatalog(ctx, tx)
		if err != nil {
			return err
		}
		// A second ordinary reader needs no exclusive lock and can resolve
		// the same decision while the first retains its witness.
		otherCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if other, err := readFromPool(otherCtx, f.RuntimePool(t, 1), 2); err != nil || other != got {
			t.Fatalf("shared catalog decision changed across readers: %+v / %v", other, err)
		}
		writer, err := f.Admin.Begin(ctx)
		if err != nil {
			return err
		}
		defer writer.Rollback(ctx)
		if _, err := writer.Exec(ctx, "SET LOCAL lock_timeout='100ms'"); err != nil {
			return err
		}
		_, err = writer.Exec(ctx, reviewSQL, domain.OfficialPrices, price, 3, domain.Withdrawn)
		var blocked *pgconn.PgError
		if !errors.As(err, &blocked) || blocked.Code != "55P03" {
			t.Fatalf("writer changed an in-use source decision: %v", err)
		}
		held, err := catalog.ReadAdmitted(ctx, "synthetic-regional", 2)
		if err != nil || held != got {
			t.Fatalf("in-use source decision drifted: %+v / %v", held, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	uncommitted, err := f.Admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer uncommitted.Rollback(ctx)
	if _, err := uncommitted.Exec(ctx, reviewSQL, domain.OfficialPrices, price, 3, domain.Withdrawn); err != nil {
		t.Fatal(err)
	}
	if err := uncommitted.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if unchanged, err := read(2); err != nil || unchanged != got {
		t.Fatalf("rolled-back withdrawal replaced the current decision: %+v / %v", unchanged, err)
	}
	sourceReview(domain.OfficialPrices, price, 3, domain.Withdrawn)
	if _, err := read(2); !errors.Is(err, domain.ErrRightsPending) {
		t.Fatalf("withdrawn rights stayed eligible through stale approval: %v", err)
	}
	var blocked *pgconn.PgError
	if _, err := f.Admin.Exec(ctx, `UPDATE enterprise_data_source_reviews SET status='APPROVED'`); !errors.As(err, &blocked) || blocked.Code != "55000" {
		t.Fatalf("review history was mutable: %v", err)
	}
}
