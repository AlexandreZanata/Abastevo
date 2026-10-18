package jobs

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5/pgtype"
)

func linkUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	decoded, err := hex.DecodeString(strings.ReplaceAll(raw, "-", ""))
	if err != nil || len(decoded) != 16 {
		t.Fatal("uuid fixture")
	}
	var id pgtype.UUID
	copy(id.Bytes[:], decoded)
	id.Valid = true
	return id
}

func linkRow(t *testing.T, recordKey, cnpj, station string, active bool) quality.ListRevisionLinkRowsRow {
	t.Helper()
	var stationID pgtype.UUID
	if station != "" {
		stationID = linkUUID(t, station)
	}
	return quality.ListRevisionLinkRowsRow{
		RecordKey: recordKey, SourceCnpj: cnpj, StationID: stationID, Active: active,
		ObservedOn: pgtype.Date{Time: time.Date(2016, 1, 4, 0, 0, 0, 0, time.UTC), Valid: true},
	}
}

// TestDivergenceFollowsPublicationPolicy freezes the comparison contract:
// only a bound record carries an active association, every other inherited
// state must remain unbound, and the fingerprint identifies the divergence
// so unchanged divergence converges on one enqueued refresh.
func TestDivergenceFollowsPublicationPolicy(t *testing.T) {
	const stationA = "10000000-0000-0000-0000-000000000001"
	const stationB = "10000000-0000-0000-0000-000000000002"
	const cnpj = "11222333000181"
	const missing = "11222333000262"
	evidence := func(rows ...domain.IdentityEvidence) map[string][]domain.IdentityEvidence {
		out := map[string][]domain.IdentityEvidence{}
		for _, row := range rows {
			out[row.CNPJ] = append(out[row.CNPJ], row)
		}
		return out
	}
	boundA := evidence(domain.IdentityEvidence{CNPJ: cnpj, StationID: stationA, Current: true, StationActive: true})
	movedB := evidence(domain.IdentityEvidence{CNPJ: cnpj, StationID: stationB, Current: true, StationActive: true})
	retired := evidence(domain.IdentityEvidence{CNPJ: cnpj, StationID: stationA, StationActive: true})
	inactive := evidence(domain.IdentityEvidence{CNPJ: cnpj, StationID: stationA, Current: true})
	unknown := evidence()

	cases := []struct {
		name     string
		rows     []quality.ListRevisionLinkRowsRow
		evidence map[string][]domain.IdentityEvidence
		want     bool
	}{
		{"stable bound association", []quality.ListRevisionLinkRowsRow{linkRow(t, "k1", cnpj, stationA, true)}, boundA, false},
		{"stable unbound record", []quality.ListRevisionLinkRowsRow{linkRow(t, "k1", cnpj, "", false)}, unknown, false},
		{"identity moved station", []quality.ListRevisionLinkRowsRow{linkRow(t, "k1", cnpj, stationA, true)}, movedB, true},
		{"identity retired", []quality.ListRevisionLinkRowsRow{linkRow(t, "k1", cnpj, stationA, true)}, retired, true},
		{"station deactivated", []quality.ListRevisionLinkRowsRow{linkRow(t, "k1", cnpj, stationA, true)}, inactive, true},
		{"unknown identity", []quality.ListRevisionLinkRowsRow{linkRow(t, "k1", cnpj, stationA, true)}, unknown, true},
		{"unbound reactivation is not republishable", []quality.ListRevisionLinkRowsRow{linkRow(t, "k1", cnpj, "", false)}, boundA, false},
		{"unrelated record untouched", []quality.ListRevisionLinkRowsRow{linkRow(t, "k1", cnpj, stationA, true)}, evidence(domain.IdentityEvidence{CNPJ: missing, StationID: stationB, Current: true, StationActive: true}), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, divergent := divergence(tc.rows, tc.evidence, "2026-10-17")
			if divergent != tc.want {
				t.Fatalf("divergent=%v want %v", divergent, tc.want)
			}
		})
	}

	// The same divergence yields the same fingerprint (idempotent sweeps);
	// a different stored association changes it (new dedupe identity).
	stable, _ := divergence(cases[2].rows, movedB, "2026-10-17")
	again, _ := divergence([]quality.ListRevisionLinkRowsRow{linkRow(t, "k1", cnpj, stationA, true)}, movedB, "2026-10-18")
	if stable == "" || stable != again {
		t.Fatalf("fingerprint oscillated: %q vs %q", stable, again)
	}
	changed, _ := divergence([]quality.ListRevisionLinkRowsRow{linkRow(t, "k1", cnpj, stationB, true)}, movedB, "2026-10-17")
	if changed == stable {
		t.Fatal("different association produced the same fingerprint")
	}
}
