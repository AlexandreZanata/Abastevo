//go:build integration && qualitysource

package native

import (
	"context"
	"os"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
)

// Explicit opt-in local source evidence. No network, publication or raw row logs.
func TestOfficialArchivePreparationManual(t *testing.T) {
	runner := realRunner(t)
	for _, tc := range []struct {
		env, url, sha   string
		n               int
		bytes           int64
		member, edition string
	}{
		{"ANPFUEL_TEST_QUALITY_SEPTEMBER_ZIP", "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc_2026_09.zip", "98721074799569affbbcbd99e173f42e26cb1107fceed707af7fe4975e95c6ab", 1, 838040, "PMQC_2026_09.csv", "2026-09"},
		{"ANPFUEL_TEST_QUALITY_ANNUAL_ZIP", annualURL, "c25d8dcf0c93560085572c49327a077202c6ac33d24c87303c4bb34b60d383c2", 12, 24513334, "2025/PMQC_2025_01.csv", "2025-01"},
	} {
		t.Run(tc.edition, func(t *testing.T) {
			filename := os.Getenv(tc.env)
			if filename == "" {
				t.Fatal(tc.env + " must name the explicitly acquired private official archive")
			}
			id := prepared.InputIdentity{SHA256: tc.sha, Bytes: tc.bytes}
			entries, e := Inventory(context.Background(), filename, id, tc.url)
			if e != nil || len(entries) != tc.n {
				t.Fatal("official inventory", len(entries), e)
			}
			result, e := runner.Prepare(context.Background(), Request{Path: filename, Identity: id, URL: tc.url, Member: tc.member, Edition: tc.edition}, unbound)
			if e != nil {
				t.Fatal(e)
			}
			if result.Batch.Counts.Accepted == 0 || len(result.Periods) != 1 {
				t.Fatal("empty official preparation")
			}
			counts := result.Batch.Counts
			if counts.Quarantined != 0 {
				t.Fatal("verified source compatibility regression", counts)
			}
			if tc.edition == "2026-09" && (counts.Input != 66225 || counts.Accepted != 34158 || counts.Duplicates != 32067 || result.Batch.ManifestHash != "4cfde2093b610127112880c79c0dd82e2c79876ea7564e994441035bb82119ba") {
				t.Fatal("date-only source manifest drift", counts, result.Batch.ManifestHash)
			}
			if tc.edition == "2025-01" && (counts.Input != 96239 || counts.Accepted != 44544 || counts.Duplicates != 51695) {
				t.Fatal("historical clock accounting drift", counts)
			}
			t.Logf("editions=%d input=%d accepted=%d duplicate=%d quarantine=%d member_sha256=%s member_bytes=%d", len(entries), result.Batch.Counts.Input, result.Batch.Counts.Accepted, result.Batch.Counts.Duplicates, result.Batch.Counts.Quarantined, result.Resource.MemberSHA256, result.Resource.MemberBytes)
		})
	}
}
