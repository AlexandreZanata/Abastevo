//go:build integration

package adapters_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestDatasetDefinitionRequiresCurrentSourceReviewsInPostGIS(t *testing.T) {
	f := orgtest.New(t)
	ctx := t.Context()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "GRANT SELECT ON enterprise_data_source_reviews,enterprise_regional_datasets TO "+role); err != nil {
		t.Fatal(err)
	}
	catalog := adapters.Catalog{DB: f.Runtime}
	if _, err := catalog.ReadAdmitted(ctx, "synthetic-regional", 1); !errors.Is(err, adapters.ErrDatasetNotFound) {
		t.Fatalf("a commercial catalog was seeded without review: %v", err)
	}

	exampleHash := func(value string) string { return "sha256:" + strings.Repeat(value, 64) }
	registry, price, snapshot := exampleHash("a"), exampleHash("b"), exampleHash("c")
	sourceReview := func(source domain.Source, checksum string, revision int64, state domain.RightsState) {
		t.Helper()
		if _, err := f.Admin.Exec(ctx, `INSERT INTO enterprise_data_source_reviews
(source_code,source_digest,revision,status,decision_ref,license_ref,attribution)
VALUES($1,$2,$3,$4,'synthetic-review','synthetic-license','Synthetic official source')`, source, checksum, revision, state); err != nil {
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
	if _, err := catalog.ReadAdmitted(ctx, "synthetic-regional", 1); !errors.Is(err, domain.ErrRightsPending) {
		t.Fatalf("pending official source became eligible: %v", err)
	}
	sourceReview(domain.OfficialPrices, price, 2, domain.Approved)
	putDataset(2, 2)
	got, err := catalog.ReadAdmitted(ctx, "synthetic-regional", 2)
	if err != nil || got.Definition.Revision != 2 || got.Definition.Snapshot != snapshot ||
		got.Definition.MinimumStations != 5 || got.Reviews[1].Pin.ReviewRevision != 2 {
		t.Fatalf("reviewed dataset did not preserve exact method basis: %+v %v", got, err)
	}
	if _, err := catalog.ReadAdmitted(ctx, "synthetic-regional", 1); !errors.Is(err, domain.ErrRightsPending) {
		t.Fatalf("new approval activated an older dataset revision: %v", err)
	}
	sourceReview(domain.OfficialPrices, price, 3, domain.Withdrawn)
	if _, err := catalog.ReadAdmitted(ctx, "synthetic-regional", 2); !errors.Is(err, domain.ErrRightsPending) {
		t.Fatalf("withdrawn rights stayed eligible through stale approval: %v", err)
	}
	var blocked *pgconn.PgError
	if _, err := f.Admin.Exec(ctx, `UPDATE enterprise_data_source_reviews SET status='APPROVED'`); !errors.As(err, &blocked) || blocked.Code != "55000" {
		t.Fatalf("review history was mutable: %v", err)
	}
}
