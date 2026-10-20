package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

func TestCompanyAssociationPlanSharesExactIdentityWithoutMergingObservations(t *testing.T) {
	records := []domain.CompanyObservationRecord{{RecordKey: "one", CNPJ: "11222333000181"}, {RecordKey: "two", CNPJ: "11222333000181"}, {RecordKey: "three", CNPJ: "04218406000104"}, {RecordKey: "four", CNPJ: "12ABC34501DE35"}}
	before := append([]domain.CompanyObservationRecord(nil), records...)
	calls := 0
	lookup := func(_ context.Context, ids []string) (CompanyIdentitySnapshot, error) {
		calls++
		if !reflect.DeepEqual(ids, []string{"04218406000104", "11222333000181", "12ABC34501DE35"}) {
			t.Fatal("lookup lost full identifiers/deduplication", ids)
		}
		return CompanyIdentitySnapshot{Evidence: []domain.IdentityEvidence{{CNPJ: "11222333000181", StationID: "station-a", Current: true, StationActive: true}, {CNPJ: "12ABC34501DE35", StationID: "station-b", Current: true, StationActive: true}}, OtherBranches: []string{"04218406000104"}}, nil
	}
	got, e := PlanCompanyAssociations(context.Background(), records, "2026-10-20", lookup)
	if e != nil || calls != 1 || len(got) != 4 || got[0].StationID != "station-a" || got[1].StationID != "station-a" || got[2].Kind != domain.ResolutionWrongBranch || got[3].StationID != "station-b" {
		t.Fatal("company plan mismatch", got, e)
	}
	for i, d := range got {
		if d.RecordKey != records[i].RecordKey || d.HistoricalBusinessWindowKnown {
			t.Fatal("source order/historical semantics lost")
		}
	}
	if !reflect.DeepEqual(records, before) {
		t.Fatal("source records mutated")
	}
}

func TestCompanyAssociationPlanningRefusesCompleteInputBeforeIO(t *testing.T) {
	valid := domain.CompanyObservationRecord{RecordKey: "one", CNPJ: "11222333000181"}
	for _, records := range [][]domain.CompanyObservationRecord{
		{valid, {RecordKey: "two", CNPJ: "11222333000180"}}, {valid, valid},
		{valid, {RecordKey: "two", CNPJ: "11.222.333/0001-81"}},
		{valid, {RecordKey: "bad\nkey", CNPJ: valid.CNPJ}},
		make([]domain.CompanyObservationRecord, MaxCompanyObservations+1),
	} {
		calls := 0
		result, e := PlanCompanyAssociations(context.Background(), records, "2026-10-20", func(context.Context, []string) (CompanyIdentitySnapshot, error) {
			calls++
			return CompanyIdentitySnapshot{}, nil
		})
		if e == nil || result != nil || calls != 0 {
			t.Fatal("invalid source performed I/O/partial plan", e, calls)
		}
	}
	if result, e := PlanCompanyAssociations(context.Background(), nil, "2026-02-30", nil); e == nil || result != nil {
		t.Fatal("invalid decision clock accepted")
	}
	if result, e := PlanCompanyAssociations(context.Background(), []domain.CompanyObservationRecord{valid}, "2026-10-20", nil); e == nil || result != nil {
		t.Fatal("missing lookup accepted")
	}
	result, e := PlanCompanyAssociations(context.Background(), nil, "2026-10-20", nil)
	if e != nil || len(result) != 0 {
		t.Fatal("empty plan invented dependency", e)
	}
}

// Numeric synthetic CNPJs use the production kernel validator; no real rows.
func companyTestCNPJ(t *testing.T, n int) string {
	t.Helper()
	base := fmt.Sprintf("%012d", n)
	for digits := 0; digits < 100; digits++ {
		value := fmt.Sprintf("%s%02d", base, digits)
		if _, e := kernel.ParseCNPJ(value); e == nil {
			return value
		}
	}
	t.Fatal("synthetic CNPJ generation failed")
	return ""
}

func TestCompanyAssociationBoundedBatchLateFailureAndCancellationYieldNoPartialPlan(t *testing.T) {
	records := make([]domain.CompanyObservationRecord, 513)
	for i := range records {
		records[i] = domain.CompanyObservationRecord{RecordKey: fmt.Sprint("record:", i), CNPJ: companyTestCNPJ(t, i+1)}
	}
	for _, mode := range []string{"success", "late failure", "late cancel", "foreign evidence", "malformed evidence", "oversized evidence", "foreign branch"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			got, e := PlanCompanyAssociations(ctx, records, "2026-10-20", func(_ context.Context, ids []string) (CompanyIdentitySnapshot, error) {
				calls++
				want := 512
				if calls == 2 {
					want = 1
				}
				if len(ids) != want {
					t.Fatal("unbounded or incorrect batch", len(ids))
				}
				rows := make([]domain.IdentityEvidence, len(ids))
				for i, id := range ids {
					rows[i] = domain.IdentityEvidence{CNPJ: id, StationID: "station-a", Current: true, StationActive: true}
				}
				out := CompanyIdentitySnapshot{Evidence: rows}
				if calls == 2 {
					switch mode {
					case "late failure":
						return out, errors.New("private row diagnostic")
					case "late cancel":
						cancel()
					case "foreign evidence":
						out.Evidence[0].CNPJ = "11222333000181"
					case "malformed evidence":
						out.Evidence[0].StationID = ""
					case "oversized evidence":
						out.Evidence = append(out.Evidence, out.Evidence[0], out.Evidence[0])
					case "foreign branch":
						out.OtherBranches = []string{"11222333000181"}
					}
				}
				return out, nil
			})
			if mode == "success" {
				if e != nil || len(got) != 513 || calls != 2 {
					t.Fatal("complete batching failed", e)
				}
			} else if e == nil || got != nil || strings.Contains(e.Error(), "private") {
				t.Fatal("failure yielded partial plan/diagnostic", e)
			}
		})
	}
}

func TestCompanyAssociationCancelledBeforeLookupAndPortCannotRewriteIdentifiers(t *testing.T) {
	records := []domain.CompanyObservationRecord{{RecordKey: "one", CNPJ: "11222333000181"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	if result, e := PlanCompanyAssociations(ctx, records, "2026-10-20", func(context.Context, []string) (CompanyIdentitySnapshot, error) {
		calls++
		return CompanyIdentitySnapshot{}, nil
	}); e == nil || result != nil || calls != 0 {
		t.Fatal("cancelled plan performed I/O")
	}
	result, e := PlanCompanyAssociations(context.Background(), records, "2026-10-20", func(_ context.Context, ids []string) (CompanyIdentitySnapshot, error) {
		original := ids[0]
		ids[0] = "04218406000104"
		return CompanyIdentitySnapshot{Evidence: []domain.IdentityEvidence{{CNPJ: original, StationID: "station-a", Current: true, StationActive: true}}}, nil
	})
	if e != nil || len(result) != 1 || result[0].StationID != "station-a" || records[0].CNPJ != "11222333000181" {
		t.Fatal("port changed owned source/lookup identity", e)
	}
}
