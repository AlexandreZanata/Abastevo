//go:build integration

package adapters

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	dbprofile "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/stationprofile"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
	profile "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/adapters"
)

// localityBatch is one complete prepared edition for a single synthetic CNPJ
// whose official municipality/UF the caller chooses.
func localityBatch(t *testing.T, runID, municipality, uf string) ([]byte, registry.OpenPreparedStream) {
	t.Helper()
	sum := sha256.Sum256([]byte(runID + municipality + uf))
	hash := hex.EncodeToString(sum[:])
	a := registry.BatchAssertion{SchemaVersion: registry.SchemaAssertionV2, Source: "registry-13col", SourceKey: "04218406000104", Checksum: hash, BusinessNameNormalized: "[SYNC-TEST] LOCALITY", AddressRaw: "SYNTHETIC STREET 100", AddressNormalized: map[string]string{"street": "SYNTHETIC STREET", "number": "100"}, MunicipalityIBGE: municipality, UF: uf, AuthState: "unknown", Eligibility: "pending", LocationQuality: "unknown"}
	raw, _ := json.Marshal(a)
	raw = append(raw, '\n')
	streams := map[string][]byte{registry.PrepAssertionsFile: raw, registry.PrepCandidatesFile: nil, registry.PrepQuarantineFile: nil}
	m := registry.BatchManifest{FormatVersion: registry.FormatBatch, RunID: runID, ParserVersion: "station-prep-v0.3.0", PolicyVersion: "station-policy-v2", Inputs: []registry.BatchInput{{Key: "registry-13col", Reference: "synthetic-locality.csv", Edition: runID, SHA256: hash, Rows: 1}}, Counts: map[string]registry.BatchCounts{"registry-13col": {Input: 1, Accepted: 1}}}
	m.Completeness.EOFValidated = true
	m.Completeness.ExpectedManifest = true
	for _, n := range []string{registry.PrepAssertionsFile, registry.PrepCandidatesFile, registry.PrepQuarantineFile} {
		b := streams[n]
		s := sha256.Sum256(b)
		m.Outputs = append(m.Outputs, registry.BatchOutput{Path: n, SHA256: hex.EncodeToString(s[:]), Bytes: uint64(len(b)), Rows: bytes.Count(b, []byte("\n"))})
	}
	manifest, _ := json.Marshal(m)
	return manifest, func(n string) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(streams[n])), nil }
}

func publishLocalityEdition(ctx context.Context, t *testing.T, pool *pgxpool.Pool, store *registry.PGStore, ports registry.PreparedPublicationPorts, runID, municipality, uf string) error {
	t.Helper()
	manifest, open := localityBatch(t, runID, municipality, uf)
	if _, err := registry.LoadPreparedBatch(ctx, pool, manifest, open); err != nil {
		t.Fatal(err)
	}
	_, err := registry.PublishPreparedRegistry(ctx, store, ports, manifest)
	return err
}

func stationLocality(ctx context.Context, t *testing.T, pool *pgxpool.Pool) (string, string, string) {
	t.Helper()
	var id, municipality, uf string
	if err := pool.QueryRow(ctx, "SELECT id::text, municipality_code, state FROM directory_stations").Scan(&id, &municipality, &uf); err != nil {
		t.Fatal(err)
	}
	return id, municipality, uf
}

// B-BR-SYNC05 (2026-11-14): an official municipality/UF correction replaces a
// canonical locality that is itself previously published source evidence;
// a curated or otherwise unproven locality still fails visibly.
func TestPreparedRefreshFollowsSourceOwnedLocalityAndRefusesCurated(t *testing.T) {
	pool, store, canon := freshRegistryDB(t)
	ctx := context.Background()
	profiles := profile.Store{Q: dbprofile.New(pool)}
	ports := registry.PreparedPublicationPorts{Canonical: canon, Locality: canon.SetInitialLocality, EnsureProfile: profiles.EnsureUnclaimed}

	if err := publishLocalityEdition(ctx, t, pool, store, ports, "11111111-3333-4333-8444-555555555555", "3110301", "MG"); err != nil {
		t.Fatalf("first edition: %v", err)
	}
	id, municipality, uf := stationLocality(ctx, t, pool)
	if municipality != "3110301" || uf != "MG" {
		t.Fatalf("initial locality = %s/%s", municipality, uf)
	}

	// The official source moves the same CNPJ to another municipality.
	if err := publishLocalityEdition(ctx, t, pool, store, ports, "22222222-3333-4333-8444-555555555555", "3151800", "MG"); err != nil {
		t.Fatalf("source-owned locality correction refused: %v", err)
	}
	if _, municipality, uf = stationLocality(ctx, t, pool); municipality != "3151800" || uf != "MG" {
		t.Fatalf("corrected locality = %s/%s", municipality, uf)
	}

	// A later correction across UF follows the same source-owned rule.
	if err := publishLocalityEdition(ctx, t, pool, store, ports, "33333333-3333-4333-8444-555555555555", "3550308", "SP"); err != nil {
		t.Fatalf("source-owned UF correction refused: %v", err)
	}
	if _, municipality, uf = stationLocality(ctx, t, pool); municipality != "3550308" || uf != "SP" {
		t.Fatalf("cross-UF locality = %s/%s", municipality, uf)
	}

	// An operator-curated locality has no matching source evidence and must
	// never be overwritten by a later edition.
	if _, err := pool.Exec(ctx, "UPDATE directory_stations SET municipality_code='3509502', state='SP' WHERE id=$1::uuid", id); err != nil {
		t.Fatal(err)
	}
	err := publishLocalityEdition(ctx, t, pool, store, ports, "44444444-3333-4333-8444-555555555555", "3304557", "RJ")
	if err == nil || !strings.Contains(err.Error(), "locality projection conflict") {
		t.Fatalf("curated locality conflict not visible: %v", err)
	}
	if _, municipality, uf = stationLocality(ctx, t, pool); municipality != "3509502" || uf != "SP" {
		t.Fatalf("curated locality overwritten = %s/%s", municipality, uf)
	}

	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM directory_stations").Scan(&count); err != nil || count != 1 {
		t.Fatalf("identity duplicated: %d %v", count, err)
	}
}
