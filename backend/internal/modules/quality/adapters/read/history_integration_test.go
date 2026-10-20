//go:build integration

package read

import (
	"context"
	"errors"
	parentadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"strings"
	"testing"
	"time"
)

func TestHistoricalRevisionSelectionRefusesCrossStream(t *testing.T) {
	p := freshPool(t)
	seedStation(t, p)
	r := NewReader(p)
	ctx := context.Background()
	now := time.Now()
	id := seedAssays(t, p, "a")
	lease, e := r.RevisionLease(ctx, now, "pmqc", "pmqc-assay-v1", id)
	if e != nil || !lease.HasPublished || lease.RevisionID != id {
		t.Fatal(lease, e)
	}
	for _, tc := range []struct{ source, stream, id string }{{"enforcement", "enforcement-v1", id}, {"pmqc", "pmqc-assay-v1", "11111111-1111-4111-8111-111111111111"}} {
		if _, e = r.RevisionLease(ctx, now, tc.source, tc.stream, tc.id); !errors.Is(e, ErrNotFound) {
			t.Fatal("unknown/cross-stream revision accepted", e)
		}
	}
}

func TestHistoricalCatalogAndCorrectionFactsStayReachable(t *testing.T) {
	p := freshPool(t)
	seedStation(t, p)
	ctx := context.Background()
	r := NewReader(p)
	now := time.Now()
	august := seedAssays(t, p, "a", "2026-08")
	oldSeptember := seedAssays(t, p, "b")
	newSeptember := seedAssays(t, p, "c")
	want := map[string]string{august: "published", oldSeptember: "superseded", newSeptember: "published"}
	seen := map[string]bool{}
	after := ""
	for pages := 0; pages < 4; pages++ {
		rows, next, e := r.Revisions(ctx, "pmqc", "pmqc-assay-v1", after, 1)
		if e != nil {
			t.Fatal(e)
		}
		for _, row := range rows {
			if seen[row.ID] || want[row.ID] != row.Status || row.IsCurrentStream != (row.ID == newSeptember) {
				t.Fatal("catalog scope/status", row)
			}
			seen[row.ID] = true
		}
		if next == "" {
			break
		}
		after = next
	}
	if len(seen) != 3 {
		t.Fatal("historical editions omitted", seen)
	}
	for id, status := range want {
		l, e := r.RevisionLease(ctx, now, "pmqc", "pmqc-assay-v1", id)
		if e != nil || !l.HasPublished || l.Status != status || l.Counts.Accepted != 3 {
			t.Fatal(l, e)
		}
		if id == newSeptember && l.SupersedesRevisionID != oldSeptember {
			t.Fatal("correction ancestry lost", l)
		}
		selected := r.AtRevision(id)
		var count int
		date, rowID := "", ""
		for pages := 0; pages < 4; pages++ {
			rows, next, lease, e := selected.Assays(ctx, now, "pmqc", "pmqc-assay-v1", readStation, "", date, rowID, 1)
			if e != nil || lease.RevisionID != id {
				t.Fatal("mixed revision", lease, e)
			}
			count += len(rows)
			if next == "" {
				break
			}
			parts := strings.Split(next, "|")
			date, rowID = parts[0], parts[1]
		}
		if count != 3 {
			t.Fatal("incomplete historical traversal", id, count)
		}
		sample, e := selected.Sample(ctx, now, "pmqc", "pmqc-assay-v1", "S-1")
		if e != nil || len(sample.Assays) != 2 || sample.Lease.RevisionID != id {
			t.Fatal(sample, e)
		}
		resources, e := r.RevisionResources(ctx, l)
		if e != nil || len(resources) != 1 || resources[0].SHA256 != strings.Repeat("a", 64) {
			t.Fatal(resources, e)
		}
	}
	// Withdrawal and missing provenance remain visible metadata with no invented facts.
	if _, e := p.Exec(ctx, "UPDATE quality_revisions SET status='withdrawn' WHERE id=$1", august); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Exec(ctx, "UPDATE quality_revision_provenance SET provenance_state='LEGACY_UNVERIFIED',input=NULL,accepted=NULL,duplicates=NULL,quarantined=NULL,manifest_hash=NULL,publication_key=NULL WHERE revision_id=$1", oldSeptember); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{august, oldSeptember} {
		l, e := r.RevisionLease(ctx, now, "pmqc", "pmqc-assay-v1", id)
		if e != nil || l.HasPublished || !l.Stale || l.Counts.Input != 0 {
			t.Fatal("unavailable history exposed", l, e)
		}
		rows, _, _, e := r.AtRevision(id).Assays(ctx, now, "pmqc", "pmqc-assay-v1", readStation, "", "", "", 10)
		if e != nil || len(rows) != 0 {
			t.Fatal("unavailable facts", rows, e)
		}
	}
}

func TestHistoricalEventRevisionsAndDetails(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	r := NewReader(p)
	now := time.Now()
	oldAction, oldSanction := seedEvents(t, p)
	var actionID, sanctionID string
	if e := p.QueryRow(ctx, "SELECT id::text FROM quality_enforcement_events WHERE revision_id=$1", oldAction).Scan(&actionID); e != nil {
		t.Fatal(e)
	}
	if e := p.QueryRow(ctx, "SELECT id::text FROM quality_sanction_events WHERE revision_id=$1", oldSanction).Scan(&sanctionID); e != nil {
		t.Fatal(e)
	}
	pub := parentadapters.NewPublisher(p)
	_, e := pub.PublishEvents(ctx, application.EventsInput{Source: "enforcement", Stream: "enforcement-v1", Edition: "2026-09", Resources: []application.Resource{{URL: "https://www.gov.br/anp/acoes", SHA256: strings.Repeat("1", 64), Bytes: 10}}, Counts: domain.Counts{Input: 1, Accepted: 1}, ManifestHash: strings.Repeat("2", 64), Enforcement: []application.EnforcementInput{{NativeKey: "AUTO-1", Scope: "municipal", Status: "retificado", SubjectCNPJ: "11222333000181", OccurredOn: "2026-08-02", AsOf: "2026-09-30", EvidenceRef: "synthetic-correction"}}}, nextID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = pub.PublishEvents(ctx, application.EventsInput{Source: "sanctions", Stream: "sanction-v1", Edition: "2025", Resources: []application.Resource{{URL: "https://www.gov.br/anp/multas", SHA256: strings.Repeat("3", 64), Bytes: 11}}, Counts: domain.Counts{Input: 1, Accepted: 1}, ManifestHash: strings.Repeat("4", 64), Sanctions: []application.SanctionInput{{NativeKey: "MULTA-1", ProceedingRef: "48000.1/2025-11", PartyCNPJ: "04218406000104", Status: "anulada", AsOf: "2025-12-31", EvidenceRef: "synthetic-correction"}}}, nextID)
	if e != nil {
		t.Fatal(e)
	}
	old := r.AtRevision(oldAction)
	rows, _, l, e := old.Enforcement(ctx, now, "enforcement", "enforcement-v1", "", "", "", "", 1)
	if e != nil || len(rows) != 1 || rows[0].Status != "autuado" || l.Status != "superseded" {
		t.Fatal(rows, l, e)
	}
	one, l, e := old.EnforcementOne(ctx, now, "enforcement", "enforcement-v1", actionID)
	if e != nil || one.ID != actionID || l.RevisionID != oldAction {
		t.Fatal(one, l, e)
	}
	old = r.AtRevision(oldSanction)
	sanctions, _, l, e := old.Sanctions(ctx, now, "sanctions", "sanction-v1", "", "", "", 1)
	if e != nil || len(sanctions) != 1 || sanctions[0].Status != "aplicada" || sanctions[0].AmountMinor == nil || *sanctions[0].AmountMinor != 150000 || l.Status != "superseded" {
		t.Fatal(sanctions, l, e)
	}
	snapshot, l, e := old.SanctionOne(ctx, now, "sanctions", "sanction-v1", sanctionID)
	if e != nil || snapshot.ID != sanctionID || l.RevisionID != oldSanction {
		t.Fatal(snapshot, l, e)
	}
}
