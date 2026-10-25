//go:build integration

package read

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	parentadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func exportObservation(status, cnpj, duf, evidence string) application.FineObservation {
	row := application.FineObservation{
		SourceCNPJ: cnpj, ProceedingRef: "48000.1/2025-11", DUFRef: duf,
		Department: "SYNTHETIC", Status: status, DueOn: "2025-02-28",
		AmountText: "150000,00", AmountMinor: 15000000, PaidText: "0,00",
		PaidMinor: 0, ReportYear: 2025, ExtractionMonth: "2026-01", EvidenceRef: evidence,
	}
	row.ObservationKey = row.ObservationIdentity()
	return row
}

func exportNativeInput(status string) application.NativeFineInput {
	return application.NativeFineInput{
		Edition: "2025",
		Clock:   application.NativeFineClock{ReportYear: 2025, ExtractionMonth: "2026-01"},
		Resources: []application.Resource{{
			URL:    "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/multas-aplicadas-2025.csv",
			SHA256: strings.Repeat("a", 64), Bytes: 100,
		}},
		Counts:       domain.Counts{Input: 3, Accepted: 3},
		ManifestHash: strings.Repeat("b", 64),
		Observations: []application.FineObservation{
			exportObservation(status, "11222333000181", "SYNTHETIC/A", "synthetic.csv:2"),
			exportObservation(status, "11222333000262", "SYNTHETIC/B", "synthetic.csv:3"),
			exportObservation(status, "04218406000104", "SYNTHETIC/C", "synthetic.csv:4"),
		},
	}
}

func exportCompanyPlan(rows []application.FineObservation, stationID string) *application.CompanyAssociationInput {
	decisions := make([]domain.CompanyObservationDecision, 0, len(rows))
	for i, row := range rows {
		decision := domain.CompanyObservationDecision{
			RecordKey: row.ObservationKey, DecidedAt: "2026-10-20",
			PolicyVersion: domain.CompanyObservationPolicyVersion,
		}
		switch i {
		case 0:
			decision.Kind = domain.ResolutionBound
			decision.StationID = stationID
			decision.Scope = domain.CurrentCompanyOnly
			decision.Reason = "exact current cnpj; historical business window unknown"
		case 1:
			decision.Kind = domain.ResolutionUnmatched
			decision.Reason = domain.ReasonUnmatchedID
		default:
			decision.Kind = domain.ResolutionAmbiguous
			decision.Reason = domain.ReasonAmbiguous
		}
		decisions = append(decisions, decision)
	}
	return &application.CompanyAssociationInput{
		PolicyVersion: domain.CompanyObservationPolicyVersion,
		DecisionDay:   "2026-10-20", Decisions: decisions,
	}
}

func publishExportInput(t *testing.T, pool *pgxpool.Pool, input application.NativeFineInput) string {
	t.Helper()
	identity := directory.NewSourceIdentityReader(pool)
	publisher := parentadapters.NewPublisher(pool).
		WithCompanyGuard(func(ctx context.Context, tx pgx.Tx, bindings []application.CompanyBinding) error {
			expected := make([]directoryapp.SourceBinding, 0, len(bindings))
			for _, binding := range bindings {
				expected = append(expected, directoryapp.SourceBinding{CNPJ: binding.CNPJ, StationID: binding.StationID})
			}
			return identity.Guard(ctx, tx, expected)
		}).
		WithGuard(func(context.Context, pgx.Tx) error { return nil })
	revision, err := publisher.PublishNativeFines(context.Background(), input, nextID)
	if err != nil {
		t.Fatalf("publish export fixture: %v", err)
	}
	return revision
}

func exportFixture(t *testing.T, pool *pgxpool.Pool) (string, application.NativeFineInput) {
	t.Helper()
	ctx := context.Background()
	station, err := directory.NewRepository(pool).ResolveCNPJ(ctx, "11222333000181", "Synthetic export station", nil)
	if err != nil {
		t.Fatal(err)
	}
	input := exportNativeInput("source status")
	input.CompanyAssociations = exportCompanyPlan(input.Observations, station.ID)
	return publishExportInput(t, pool, input), input
}

func TestOfficialExportPinsCensusAndTraversesOnce(t *testing.T) {
	pool := freshPool(t)
	revision, input := exportFixture(t, pool)
	ctx := context.Background()
	reader := NewExportReader(pool)

	pin, err := reader.ExportPin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pin.RevisionID != revision || pin.Edition != "2025" || pin.SourcePeriod != "2025" || pin.LeaseID == "" || len(pin.LeaseID) != 64 {
		t.Fatalf("pin = %+v", pin)
	}
	if pin.SourceURL != input.Resources[0].URL || pin.SourceChecksum != input.Resources[0].SHA256 || pin.SourceBytes != int64(input.Resources[0].Bytes) {
		t.Fatalf("source pin = %+v", pin)
	}
	if pin.PublicFieldPolicy != application.OfficialExportPublicFieldPolicy || pin.Counts.Accepted != 3 || pin.AssociationPolicy != domain.CompanyObservationPolicyVersion || pin.AssociationScope != domain.CurrentCompanyOnly || pin.AssociationDecisionDay != "2026-10-20" {
		t.Fatalf("counts/association pin = %+v", pin)
	}
	if pin.AssociationCensus.Linked != 1 || pin.AssociationCensus.Unmatched != 1 || pin.AssociationCensus.Ambiguous != 1 || pin.AssociationCensus.Total != 3 {
		t.Fatalf("census = %+v", pin.AssociationCensus)
	}
	if pin.SourceClockPrecision != "MONTH" || pin.SourceClockValue != "2026-01" || !pin.Stale {
		t.Fatalf("clock/stale = %+v", pin)
	}

	all, next, err := reader.ExportSanctions(ctx, "", 10)
	if err != nil || len(all) != 3 || next != "" {
		t.Fatalf("complete page = %+v %q %v", all, next, err)
	}
	byCNPJ := map[string]application.OfficialRecord{}
	for _, record := range all {
		byCNPJ[record.SubjectCNPJ] = record
	}
	linked := byCNPJ["11222333000181"]
	if linked.AssociationState != string(application.AssociationLinked) || linked.StationID == "" || linked.OccurredOn != "" || linked.DecidedOn != "" || linked.DueOn != "2025-02-28" {
		t.Fatalf("linked record = %+v", linked)
	}
	unmatched := byCNPJ["11222333000262"]
	if unmatched.AssociationState != string(application.AssociationUnmatched) || unmatched.StationID != "" || unmatched.AssociationReason != domain.ReasonUnmatchedID {
		t.Fatalf("unmatched record = %+v", unmatched)
	}
	ambiguous := byCNPJ["04218406000104"]
	if ambiguous.AssociationState != string(application.AssociationAmbiguous) || ambiguous.StationID != "" || ambiguous.AssociationReason != domain.ReasonAmbiguous {
		t.Fatalf("ambiguous record = %+v", ambiguous)
	}

	seen := map[string]bool{}
	after := ""
	for page := 0; page < 3; page++ {
		rows, next, err := reader.ExportSanctions(ctx, after, 1)
		if err != nil || len(rows) != 1 {
			t.Fatalf("page %d = %+v %q %v", page, rows, next, err)
		}
		if seen[rows[0].NativeKey] {
			t.Fatal("pagination repeated a native key")
		}
		seen[rows[0].NativeKey] = true
		if next == "" {
			if page != 2 {
				t.Fatalf("premature end at page %d", page)
			}
			break
		}
		sealedRevision, _, err := application.OpenOfficialExportCursor(next)
		if err != nil || sealedRevision != revision {
			t.Fatalf("cursor = %q %v", sealedRevision, err)
		}
		after = next
	}
	if len(seen) != 3 {
		t.Fatalf("pagination lost records: %d", len(seen))
	}

	var payload bytes.Buffer
	manifest, err := application.BuildOfficialExport(ctx, reader, &payload, 2)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Rows != 3 || manifest.PayloadSHA256 == "" || manifest.RevisionID != revision || manifest.AssociationCensus.Total != 3 {
		t.Fatalf("manifest = %+v", manifest)
	}
	lines := strings.Split(strings.TrimSpace(payload.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("payload lines = %d", len(lines))
	}
	for _, line := range lines {
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"cpf", "department", "paid_text", "paid_minor", "unlabelled_field", "history_revision_id"} {
			if _, exists := row[forbidden]; exists {
				t.Fatalf("private field %q leaked: %s", forbidden, line)
			}
		}
		if row["revision_id"] != revision || row["lease_id"] != manifest.LeaseID {
			t.Fatalf("row revision/lease mismatch: %s", line)
		}
	}
	var again bytes.Buffer
	againManifest, err := application.BuildOfficialExport(ctx, reader, &again, 2)
	if err != nil || againManifest.PayloadSHA256 != manifest.PayloadSHA256 || !bytes.Equal(again.Bytes(), payload.Bytes()) {
		t.Fatal("repeat export diverged", err)
	}
}

func TestOfficialExportRefusesStaleCursorAndNonPublishedRevision(t *testing.T) {
	pool := freshPool(t)
	firstRevision, _ := exportFixture(t, pool)
	reader := NewExportReader(pool)
	ctx := context.Background()

	_, cursor, err := reader.ExportSanctions(ctx, "", 1)
	if err != nil || cursor == "" {
		t.Fatalf("first cursor = %q %v", cursor, err)
	}
	secondInput := exportNativeInput("corrected status")
	secondInput.CompanyAssociations = exportCompanyPlan(secondInput.Observations, func() string {
		station, err := directory.NewRepository(pool).ResolveCNPJ(ctx, "11222333000181", "Synthetic export station", nil)
		if err != nil {
			t.Fatal(err)
		}
		return station.ID
	}())
	secondRevision := publishExportInput(t, pool, secondInput)
	if secondRevision == firstRevision {
		t.Fatal("correction reused the revision")
	}
	pin, err := reader.ExportPin(ctx)
	if err != nil || pin.RevisionID != secondRevision {
		t.Fatalf("current pin = %+v %v", pin, err)
	}
	if _, _, err := reader.ExportSanctions(ctx, cursor, 1); !errors.Is(err, application.ErrOfficialExportCursor) {
		t.Fatal("stale revision cursor accepted", err)
	}
	if _, _, err := reader.ExportSanctions(ctx, "not-a-cursor", 1); !errors.Is(err, application.ErrOfficialExportCursor) {
		t.Fatal("malformed cursor accepted", err)
	}
	if _, _, err := reader.ExportSanctions(ctx, application.SealOfficialExportCursor(firstRevision, "observation:0000000000000000000000000000000000000000000000000000000000000000"), 1); !errors.Is(err, application.ErrOfficialExportCursor) {
		t.Fatal("superseded revision cursor accepted", err)
	}

	emptyPool := freshPool(t)
	if _, err := NewExportReader(emptyPool).ExportPin(ctx); !errors.Is(err, application.ErrOfficialExportUnavailable) {
		t.Fatal("empty export lease accepted", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE quality_revisions SET status='withdrawn' WHERE id=$1", secondRevision); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ExportPin(ctx); !errors.Is(err, application.ErrOfficialExportUnavailable) {
		t.Fatal("withdrawn current revision exported", err)
	}
}

func TestOfficialExportMapsReviewKindsToUnmatched(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	input := exportNativeInput("source status")
	input.Observations = input.Observations[:2]
	input.Counts = domain.Counts{Input: 2, Accepted: 2}
	input.CompanyAssociations = &application.CompanyAssociationInput{
		PolicyVersion: domain.CompanyObservationPolicyVersion, DecisionDay: "2026-10-20",
		Decisions: []domain.CompanyObservationDecision{
			{
				Kind: domain.ResolutionWrongBranch, RecordKey: input.Observations[0].ObservationKey,
				Reason: domain.ReasonWrongBranch, DecidedAt: "2026-10-20",
				PolicyVersion: domain.CompanyObservationPolicyVersion,
			},
			{
				Kind: domain.ResolutionSuccession, RecordKey: input.Observations[1].ObservationKey,
				Reason: domain.ReasonSuccession, DecidedAt: "2026-10-20",
				PolicyVersion: domain.CompanyObservationPolicyVersion,
			},
		},
	}
	revision := publishExportInput(t, pool, input)
	reader := NewExportReader(pool)
	pin, err := reader.ExportPin(ctx)
	if err != nil || pin.RevisionID != revision || pin.AssociationCensus.WrongBranch != 1 || pin.AssociationCensus.SuccessionReview != 1 || pin.AssociationCensus.Linked != 0 {
		t.Fatalf("review census = %+v %v", pin, err)
	}
	records, next, err := reader.ExportSanctions(ctx, "", 10)
	if err != nil || len(records) != 2 || next != "" {
		t.Fatalf("review records = %+v %q %v", records, next, err)
	}
	for _, record := range records {
		if record.AssociationState != string(application.AssociationUnmatched) || record.StationID != "" {
			t.Fatalf("review kind invented a link: %+v", record)
		}
	}
}

func TestOfficialExportFailsClosedOnClosedStorage(t *testing.T) {
	pool := freshPool(t)
	pool.Close()
	_, err := NewExportReader(pool).ExportPin(context.Background())
	if !errors.Is(err, application.ErrOfficialExportUnavailable) {
		t.Fatalf("closed storage error = %v", err)
	}
	if strings.Contains(err.Error(), "pgx") || strings.Contains(err.Error(), "pool") {
		t.Fatalf("storage internals leaked: %v", err)
	}
}
