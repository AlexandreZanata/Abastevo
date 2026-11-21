//go:build integration

package adapters_test

import (
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5/pgconn"
)

// The latest rights decision is the highest revision of a source digest, so a
// skipped or out-of-order revision would hide a later decision such as a
// withdrawal. Every hand-recorded decision must be the next revision.
func TestDataSourceReviewRevisionsAreConsecutive(t *testing.T) {
	f := orgtest.New(t)
	ctx := t.Context()
	record := func(digest string, revision int, status string) error {
		t.Helper()
		_, err := f.Admin.Exec(ctx, `INSERT INTO enterprise_data_source_reviews
(source_code,source_digest,revision,status,decision_ref,license_ref,attribution)
VALUES ('S-ESP-02','sha256:'||repeat($1,64),$2,$3,'synthetic-decision',
 CASE WHEN $3='APPROVED' THEN 'synthetic-license' ELSE '' END,CASE WHEN $3='APPROVED' THEN 'Synthetic survey' ELSE '' END)`,
			digest, revision, status)
		return err
	}
	refused := func(err error) {
		t.Helper()
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Fatalf("non-consecutive rights revision was not refused: %v", err)
		}
	}
	refused(record("a", 2, "APPROVED"))
	if err := record("a", 1, "RIGHTS_PENDING"); err != nil {
		t.Fatal(err)
	}
	if err := record("a", 2, "APPROVED"); err != nil {
		t.Fatal(err)
	}
	refused(record("a", 4, "WITHDRAWN"))
	if err := record("b", 1, "APPROVED"); err != nil {
		t.Fatal("another digest keeps its own sequence:", err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO enterprise_data_source_reviews
(source_code,source_digest,revision,status,decision_ref)
VALUES ('S-ESP-02','sha256:'||repeat('a',64),3,'WITHDRAWN','synthetic-withdrawal'),
       ('S-ESP-02','sha256:'||repeat('a',64),4,'RIGHTS_PENDING','synthetic-review')`); err != nil {
		t.Fatal("ordered multi-row decisions were refused:", err)
	}
	var latest string
	if err := f.Admin.QueryRow(ctx, `SELECT status FROM enterprise_data_source_reviews
WHERE source_code='S-ESP-02' AND source_digest='sha256:'||repeat('a',64) ORDER BY revision DESC LIMIT 1`).Scan(&latest); err != nil ||
		latest != "RIGHTS_PENDING" {
		t.Fatalf("latest decision diverged: %q %v", latest, err)
	}
}
