//go:build integration

package adapters

import (
	"context"
	"testing"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

func TestEnforcementCompanyAssociationStorageAndWithdrawal(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	q := quality.New(p)
	m := &idMaker{}
	in := enforcementInput()
	company := in.Records[0]
	rid := newEnforcementRevision(t, q, m, "enforcement", "enforcement-v1")
	if _, e := q.RecordEnforcementSourceContext(ctx, enforcementContextArgs(rid)); e != nil {
		t.Fatal(e)
	}
	raw := enforcementJSON(t, m, nil)
	if _, e := q.StageEnforcementSourceRecords(ctx, quality.StageEnforcementSourceRecordsParams{RevisionID: rid, Records: raw}); e != nil {
		t.Fatal(e)
	}
	station, _ := m.next()
	stationID, _ := mustUUID(station)
	if _, e := p.Exec(ctx, `INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic enforcement station')`, stationID); e != nil {
		t.Fatal(e)
	}
	lookup := func(ctx context.Context, cnpjs []string) (application.CompanyIdentitySnapshot, error) {
		return application.CompanyIdentitySnapshot{Evidence: []domain.IdentityEvidence{
			{CNPJ: company.SourceCNPJ, StationID: uuidString(stationID), Current: true, StationActive: true},
		}}, nil
	}
	decisions, e := PlanEnforcementCompanyAssociations(ctx, []application.EnforcementSourceRecord{company}, "2026-10-22", lookup)
	if e != nil || len(decisions) != 1 || decisions[0].Kind != domain.ResolutionBound {
		t.Fatal("enforcement linkage not planned", e)
	}
	if e := StageEnforcementCompanyAssociations(ctx, q, rid, "2026-10-22", 1, decisions); e != nil {
		t.Fatal(e)
	}
	rows, e := q.EnforcementCompanyAssociationDecisions(ctx, rid)
	if e != nil || len(rows) != 1 || rows[0].ResolutionKind != "bound" || !rows[0].StationID.Valid {
		t.Fatal("bound enforcement decision lost", e)
	}
	counts, e := q.EnforcementCompanyAssociationCounts(ctx, rid)
	if e != nil || counts.Total != 1 || counts.BoundCount != 1 {
		t.Fatal("association census diverged", e)
	}
	// Staging is once per revision (guarded by the publisher transaction);
	// a restage of the same revision conflicts instead of forking.
	if e := StageEnforcementCompanyAssociations(ctx, q, rid, "2026-10-22", 1, decisions); e == nil {
		t.Fatal("restage of the same revision forked")
	}
	if _, e := p.Exec(ctx, `DELETE FROM quality_revisions WHERE id=$1`, rid); e != nil {
		t.Fatal(e)
	}
	var remaining int64
	if e := p.QueryRow(ctx, `SELECT count(*) FROM quality_enforcement_company_associations WHERE revision_id=$1`, rid).Scan(&remaining); e != nil || remaining != 0 {
		t.Fatal("withdrawal left associations behind", e)
	}
	var stations int64
	if e := p.QueryRow(ctx, `SELECT count(*) FROM directory_stations`).Scan(&stations); e != nil || stations != 1 {
		t.Fatal("association touched the Directory", e)
	}
}
