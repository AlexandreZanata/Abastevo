//go:build integration

package adapters_test

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters/preparation"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestDataRightsRecorderBindsTheSnapshotAndAuditsExactConcurrentReplay(t *testing.T) {
	f := orgtest.New(t)
	ctx := t.Context()
	path, manifest := buildRegionalExport(t, "01", 5)
	official := manifest.Sources.Official[0]
	review := preparation.RightsDecision{
		OperationID: onboardingID(t), Source: domain.OfficialPrices, Digest: official.SourceChecksum,
		Revision: 1, State: domain.Approved, DecisionRef: "synthetic-review",
		LicenseRef: "synthetic-license", SnapshotDigest: manifest.ContentSHA256, SnapshotDir: path,
		OfficialRevision: official.RevisionID,
	}

	if _, err := preparation.RecordRights(ctx, f.Runtime, review); err == nil {
		t.Fatal("serving role could record a rights decision")
	}
	script, err := os.ReadFile("../../../../../infra/db/data-rights-role.sql")
	if err != nil {
		t.Fatal(err)
	}
	role := "emp_rights_" + strings.ToLower(rand.Text())
	secret := rand.Text()
	name := pgx.Identifier{role}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "CREATE ROLE "+name+" LOGIN NOINHERIT PASSWORD '"+secret+"'"); err != nil {
		t.Fatal("could not create disposable review role")
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(cleanup, "DROP OWNED BY "+name+"; DROP ROLE "+name); err != nil {
			t.Error(err)
		}
	})
	provision := strings.NewReplacer(`:'data_rights_role'`, "'"+role+"'", `:"data_rights_role"`, name).Replace(string(script))
	for range 2 {
		if _, err := f.Admin.Exec(ctx, provision); err != nil {
			t.Fatal(err)
		}
	}
	pool := f.ScriptRuntimePool(t, role, secret, 4)
	if _, err := pool.Exec(ctx, `INSERT INTO enterprise_data_source_reviews
(source_code,source_digest,revision,status,decision_ref) VALUES ('S-ESP-01',$1,1,'RIGHTS_PENDING','bypass')`, review.Digest); err == nil {
		t.Fatal("review role bypassed the operation audit")
	}

	wrong := review
	wrong.Digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := preparation.RecordRights(ctx, pool, wrong); !errors.Is(err, domain.ErrSourceBinding) {
		t.Fatalf("approved a digest outside the verified snapshot: %v", err)
	}
	empty := review
	empty.LicenseRef = "  "
	if _, err := preparation.RecordRights(ctx, pool, empty); !errors.Is(err, preparation.ErrInvalidRightsDecision) {
		t.Fatalf("approved without a license reference: %v", err)
	}
	skipped := review
	skipped.Revision = 2
	if _, err := preparation.RecordRights(ctx, pool, skipped); !errors.Is(err, preparation.ErrRightsRevision) {
		t.Fatalf("skipped a review revision: %v", err)
	}

	premature := preparation.RightsDecision{OperationID: onboardingID(t), Source: review.Source, Digest: review.Digest,
		Revision: 1, State: domain.Withdrawn, DecisionRef: "synthetic-premature-withdrawal"}
	if _, err := preparation.RecordRights(ctx, pool, premature); !errors.Is(err, domain.ErrRightsPending) {
		t.Fatalf("withdrew a digest that was never approved: %v", err)
	}

	type result struct {
		replayed bool
		err      error
	}
	results := make(chan result, 3)
	var workers sync.WaitGroup
	for range 3 {
		workers.Go(func() {
			replayed, err := preparation.RecordRights(ctx, pool, review)
			results <- result{replayed, err}
		})
	}
	workers.Wait()
	close(results)
	recorded := 0
	for outcome := range results {
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if !outcome.replayed {
			recorded++
		}
	}
	if recorded != 1 {
		t.Fatalf("recorded %d concurrent decisions instead of one", recorded)
	}
	stale := review
	stale.OperationID = onboardingID(t)
	if _, err := preparation.RecordRights(ctx, pool, stale); !errors.Is(err, preparation.ErrRightsRevision) {
		t.Fatalf("second operation reused the review revision: %v", err)
	}
	altered := review
	altered.DecisionRef = "different-review"
	if _, err := preparation.RecordRights(ctx, pool, altered); !errors.Is(err, preparation.ErrRightsOperationReused) {
		t.Fatalf("changed replay intent was accepted: %v", err)
	}
	var actor, attribution, digest string
	if err := f.Admin.QueryRow(ctx, `SELECT recorded_by, attribution, snapshot_digest
FROM enterprise_data_source_reviews WHERE operation_id=$1::uuid`, review.OperationID).Scan(&actor, &attribution, &digest); err != nil ||
		actor != role || attribution != "Fonte: ANP" || digest != manifest.ContentSHA256 {
		t.Fatalf("wrong persisted review audit: %q %q %q %v", actor, attribution, digest, err)
	}

	registry := review
	registry.OperationID = onboardingID(t)
	registry.Source = domain.PublicRegistry
	registry.Digest = manifest.Files[0].SHA256
	registry.OfficialRevision = ""
	if replayed, err := preparation.RecordRights(ctx, pool, registry); err != nil || replayed {
		t.Fatalf("could not approve the verified registry projection: %t %v", replayed, err)
	}

	withdraw := preparation.RightsDecision{
		OperationID: onboardingID(t), Source: review.Source, Digest: review.Digest,
		Revision: 2, State: domain.Withdrawn, DecisionRef: "synthetic-withdrawal",
	}
	if replayed, err := preparation.RecordRights(ctx, pool, withdraw); err != nil || replayed {
		t.Fatalf("could not withdraw the exact previously reviewed digest: %t %v", replayed, err)
	}
	if replayed, err := preparation.RecordRights(ctx, pool, withdraw); err != nil || !replayed {
		t.Fatalf("withdrawal replay did not converge: %t %v", replayed, err)
	}
}
