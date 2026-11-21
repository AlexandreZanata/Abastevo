//go:build integration

package adapters_test

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	directoryread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	fuelpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/adapters"
	fuelapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"
	official "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters"
	officialread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	orgapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func TestMarketArtifactsExcludeFiveHundredApprovedPrivateFuelings(t *testing.T) {
	t.Setenv("ANPFUEL_TEST_RUNTIME_GRANTS", "script")
	x := fuelingFixture(t)
	fx, ctx := x.deviceFixtureState, t.Context()
	seedMarketSurvey(t, fx)
	without, before := buildMarketPrivacyArtifact(t, fx)
	if before.Counts.Stations != 5 || before.Counts.Prices != 5 {
		t.Fatalf("market privacy build needs actual public inputs: %+v", before.Counts)
	}

	router := chi.NewRouter()
	accounts := *fx.accounts
	accounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{
		Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{78}, 32), Pool: fx.f.Runtime, Accounts: &accounts,
	}); err != nil {
		t.Fatal(err)
	}
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	author := incidentLogin(t, x.syncFixtureState.person, router)
	linkGrants(t, fx)
	document := matrixStoredDocument(t, fx, router, author)
	catalog, err := audit.NewCatalog(fuelpg.ReviewAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	// Exercise every review through the same authority and owning store as
	// HTTP. The volume fixture is independent of the HTTP write-rate window.
	reviews := fuelpg.ReviewStore{Pool: fx.f.Runtime, Audit: auditpg.NewStore(fx.f.Runtime, catalog),
		Authority: orgpg.FuelingReviewAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags,
			Structure: &orgpg.ResourceStructure{Kind: orgpg.FuelingReviewStructure, Pool: fx.f.Runtime}}}
	reviewer := orgapp.CompanyActor{AccountID: owner.account, SessionID: owner.session}
	org := x.scope.OrgID().String()
	const privatePlace = "Posto Fictício Alfa"
	declared := x.declaredNow(t)
	for i := range 500 {
		op := x.op(t, x.syncFixtureState, declared.Add(time.Duration(i)*time.Microsecond), func(_, payload map[string]any) {
			// A distinct exact negotiated quantity and paid amount per fueling
			// avoids creating suspected duplicates in this fixture.
			payload["quantity"] = strconv.Itoa(150000 + 1000*i)
			payload["paid_total"] = strconv.Itoa(649 * (150 + i))
			payload["place"] = map[string]any{"kind": "NOT_FOUND", "name": privatePlace}
			payload["reading"], payload["missing_reading_reason"] = nil, "Synthetic private fueling without a meter"
		})
		ack, err := x.process(t, x.syncFixtureState, op)
		if err != nil || ack.Result() != protocol.Accepted {
			t.Fatalf("synthetic fueling %d did not settle: %s / %v", i+1, ack.Code(), err)
		}
		id := op.Snapshot().Entity.ID
		if i == 0 {
			path := "/v1/orgs/" + org + "/documents/" + document + "/links/create"
			matrixCall(t, fx, router, author, path, map[string]any{
				"schema_version": 1, "operation_id": onboardingID(t), "target": map[string]any{"kind": "FUELING", "id": id},
			}, http.StatusCreated, "", "", nil)
		}
		_, _, err = reviews.WriteReview(ctx, x.scope, reviewer, owner.member, id, onboardingID(t),
			fuelapp.ReviewInput{Kind: review.Open, ExpectedSequence: 1})
		if err != nil {
			t.Fatalf("could not open synthetic fueling %d: %v", i+1, err)
		}
		decision, replay, err := reviews.WriteReview(ctx, x.scope, reviewer, owner.member, id, onboardingID(t),
			fuelapp.ReviewInput{Kind: review.Approve, ExpectedSequence: 2})
		if err != nil || replay || decision.State != review.Approved {
			t.Fatalf("could not approve synthetic fueling %d: %v", i+1, err)
		}
	}

	track := trackingFixtureOn(t, tripFixtureOn(t, x.selfIntakeState))
	track.inForce(t)
	session := onboardingID(t)
	track.startTracking(t, track.syncFixtureState, session, 1, "PRECISE", 2)
	batch := track.batchOp(t, track.syncFixtureState, session, 1, []int{1, 11, 21}, nil)
	ack, err := track.process(t, track.syncFixtureState, batch)
	track.accepted(t, batch, ack, err, 1, trackingCounts{segments: 1})

	var approved, documents, segments int
	if err := fx.f.Admin.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM org_fuelings f JOIN org_fueling_review_entries r ON (r.org_id,r.fueling_id)=(f.org_id,f.id)
  WHERE f.org_id=$1 AND f.place_name=$2 AND r.sequence=3 AND r.state='APPROVED'),
 (SELECT count(*) FROM org_documents d JOIN org_document_links l ON (l.org_id,l.document_id)=(d.org_id,d.id)
  WHERE d.org_id=$1 AND d.id=$3 AND d.storage_state='STORED' AND l.target_kind='FUELING'),
 (SELECT count(*) FROM org_tracking_segments WHERE org_id=$1 AND session_id=$4)`,
		org, privatePlace, document, session).Scan(&approved, &documents, &segments); err != nil ||
		approved != 500 || documents != 1 || segments != 1 {
		t.Fatalf("private fixture did not reach its boundary: fuelings=%d documents=%d tracks=%d: %v", approved, documents, segments, err)
	}

	with, after := buildMarketPrivacyArtifact(t, fx)
	if before.ContentSHA256 != after.ContentSHA256 || len(before.Files) != len(after.Files) {
		t.Fatal("private operations changed the official artifact content digest")
	}
	for _, name := range []string{"stations.jsonl", "revisions.jsonl", "prices.jsonl", "manifest.json"} {
		first, err := os.ReadFile(filepath.Join(without, name))
		if err != nil {
			t.Fatal(err)
		}
		second, err := os.ReadFile(filepath.Join(with, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, second) || bytes.Contains(second, []byte(privatePlace)) ||
			bytes.Contains(second, []byte(document)) || bytes.Contains(second, []byte(session)) {
			t.Fatalf("market build included private content in %s", name)
		}
	}
}

func seedMarketSurvey(t *testing.T, fx deviceFixtureState) {
	t.Helper()
	ctx := t.Context()
	day := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	rows := make([]domain.PriceRow, 0, 5)
	for i := range 5 {
		id := onboardingID(t)
		if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO directory_stations(id,display_name,municipality_code,state)
 VALUES($1,'Synthetic official market station','5103403','MT')`, id); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, domain.PriceRow{StationID: id, Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: int64(5700 + i*50),
			RawText: "Synthetic official survey", CollectedOn: day, SourceRow: i + 10})
	}
	importer := official.NewImporter(fx.f.Admin)
	run, err := importer.BeginRun(ctx, domain.ImportKey{SourceURL: "https://example.invalid/synthetic-survey.ods",
		SourceChecksum: "sha256:" + strings.Repeat("79", 32), ParserVersion: "synthetic-val-36"}, day, day)
	if err != nil || run.NoOp {
		t.Fatalf("could not start survey fixture: %v", err)
	}
	var tally domain.QuarantineTally
	if _, err := importer.StageBatch(ctx, run.RevisionID, rows, &tally); err != nil {
		t.Fatal(err)
	}
	result, err := importer.FinishRun(ctx, run.RunID, run.RevisionID, &tally)
	if err != nil || !result.Published {
		t.Fatalf("could not publish synthetic official survey: %v", err)
	}
}

func buildMarketPrivacyArtifact(t *testing.T, fx deviceFixtureState) (string, enterpriseexport.Manifest) {
	t.Helper()
	ctx := t.Context()
	tx, err := fx.f.Runtime.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	root := filepath.Join(t.TempDir(), "core")
	builder := enterpriseexport.Builder{Stations: directoryread.NewExportReader(tx), Prices: officialread.NewExportReader(tx), PageSize: 2,
		Now: func() time.Time { return time.Date(2026, 1, 4, 12, 0, 0, 0, time.UTC) }}
	// Hold provenance-envelope time constant so the complete artifact set can
	// be compared byte for byte after a change to the private database rows.
	manifest, err := builder.Build(ctx, enterpriseexport.Options{FromSurvey: "2026-01-01", ToSurvey: "2026-01-07", OutputDir: root})
	if err != nil {
		t.Fatal(err)
	}
	return root, manifest
}
