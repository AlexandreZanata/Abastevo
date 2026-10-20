//go:build integration && qualitysource

package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
)

// Explicit whole-source acceptance in a disposable local database. Actual source
// files and rows are private inputs, never fixtures/log output. No network.
func TestOfficialHistoricalFineRustGoPostGISFullTraversal(t *testing.T) {
	binary := os.Getenv("ANPFUEL_TEST_FINE_HISTORY_PREPARER_PATH")
	root := os.Getenv("ANPFUEL_TEST_FINE_HISTORY_SOURCE_DIR")
	if !filepath.IsAbs(binary) || !filepath.IsAbs(root) {
		t.Fatal("trusted history CLI/private source directory required")
	}
	info, e := os.Lstat(binary)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 || info.Mode().Perm()&0022 != 0 {
		t.Fatal("trusted history CLI unavailable")
	}
	p := freshPool(t)
	pub := NewPublisher(p)
	m := &idMaker{}
	total, unknown, empty, unlabelled := 0, 0, 0, 0
	for _, tc := range []struct {
		period, file, hash                           string
		bytes, input, accepted, quarantine, metadata int
	}{
		{"2016-2019", "multas-aplicadas-2016a2019.csv", "15ccbf54367881946104dc6baf65c246b6189638cb306ab06662570451989f25", 2301471, 16440, 16290, 150, 0},
		{"2020", "multas-aplicadas-2020.csv", "aef7fce874b29db234ab7779019c0bc9fcf13d1bdb2de7d59bbf09f51e32654f", 304268, 2026, 2001, 25, 3},
		{"2021", "multas-2021-junho22.xlsx", "85122d0d1ae999173bac2c772ffb4e14ec414aec3c2f8e30bf7203187936a92d", 720842, 4976, 4929, 47, 3},
		{"2022", "multas-aplicadas-2022.csv", "5ee3df1d9188f273fbf3fbeb7e95f2cea0bc2171fdda23bc4af3cce9478251b5", 436282, 2996, 2970, 26, 0},
		{"2023", "multas-aplicadas-2023.csv", "3865025eb04a82ee5d937d703f063d617843eb7a9f5cbf4c18f989caa33839bb", 445861, 3074, 3053, 21, 0},
		{"2024", "multas-aplicadas-2024.csv", "0723561b182ac0cc3d031a77050049844e3b29ec098472383b0229051cb67dcd", 416653, 3004, 2994, 10, 0},
	} {
		t.Run(tc.period, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel()
			source := filepath.Join(root, tc.file)
			st, e := os.Lstat(source)
			if e != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() != int64(tc.bytes) {
				t.Fatal("private immutable source unavailable")
			}
			raw, e := os.ReadFile(source)
			if e != nil {
				t.Fatal("private source unavailable")
			}
			sum := sha256.Sum256(raw)
			hash := hex.EncodeToString(sum[:])
			if hash != tc.hash {
				t.Fatal("source drift requires a new candidate")
			}
			out := filepath.Join(t.TempDir(), "prepared")
			cmd := exec.CommandContext(ctx, binary, source, tc.period, out)
			cmd.Stdout = io.Discard
			cmd.Stderr = io.Discard
			if cmd.Run() != nil {
				t.Fatal("native history preparation refused")
			}
			manifest, e := os.ReadFile(filepath.Join(out, "manifest.json"))
			if e != nil {
				t.Fatal("prepared manifest unavailable")
			}
			batch, e := prepared.LoadFineHistory(ctx, manifest, func(name string) (io.ReadCloser, error) { return os.Open(filepath.Join(out, name)) }, prepared.InputIdentity{SHA256: hash, Bytes: int64(len(raw))}, tc.period)
			if e != nil {
				t.Fatal("verified history bridge refused")
			}
			if batch.Counts.Input != tc.input || batch.Counts.Accepted != tc.accepted || batch.Counts.Quarantined != tc.quarantine || batch.MetadataRecords != tc.metadata {
				t.Fatal("source counts drift")
			}
			in := application.HistoricalFineInput{Context: application.FineHistoryContext{InputContract: batch.InputContract, SourcePeriod: batch.SourcePeriod, SourceFile: batch.SourceFile, HeaderRecord: batch.HeaderRecord, Clock: batch.Clock, SourceRecordCount: batch.Counts.Input, MetadataRecords: batch.MetadataRecords}, Resources: []application.Resource{{URL: "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/" + batch.SourceFile, SHA256: hash, Bytes: uint64(len(raw))}}, Counts: batch.Counts, ManifestHash: batch.ManifestHash, Observations: batch.Observations, Notes: batch.Notes}
			revision, e := pub.PublishHistoricalFines(ctx, in, m.next)
			if e != nil {
				t.Fatal("real historical publication refused; inspect only redacted diagnostics")
			}
			again, e := pub.PublishHistoricalFines(ctx, in, m.next)
			if e != nil || again != revision {
				t.Fatal("real historical replay forked")
			}
			read := qualityread.NewReader(p).AtRevision(revision)
			lease, e := read.RevisionLease(ctx, time.Now(), "sanctions", "sanction-v1", revision)
			if e != nil || !lease.Stale || lease.SourcePeriod != tc.period || lease.SourceClockValue != batch.Clock.Value || lease.SourceMetadataRecords == nil || *lease.SourceMetadataRecords != tc.metadata {
				t.Fatal("real historical lease lost source facts")
			}
			expected := map[string]application.HistoricalFineObservation{}
			for _, row := range batch.Observations {
				expected[row.ObservationKey] = row
			}
			count, afterNative, afterID := 0, "", ""
			seen := map[string]bool{}
			for {
				rows, next, _, e := read.SanctionsAtLease(ctx, lease, "", afterNative, afterID, 100)
				if e != nil {
					t.Fatal("real historical page read refused")
				}
				for _, row := range rows {
					original, ok := expected[row.NativeKey]
					transit := ""
					if original.SourceTransitOn != nil {
						transit = *original.SourceTransitOn
					}
					if !ok || seen[row.ID] || row.ReferenceLabel != original.ReferenceLabel || row.ReferenceRef != original.ReferenceRef || row.ProceedingRef != original.ProceedingRef || row.SourceTransitOn != transit || row.DueOn != original.DueOn || row.Department != original.Department || row.Status != original.Status || row.AmountText != original.AmountText || row.AmountMinor == nil || *row.AmountMinor != original.AmountMinor || row.PaidText != original.PaidText || !reflect.DeepEqual(row.PaidMinor, original.PaidMinor) || row.UnlabelledField != original.UnlabelledField || row.EvidenceRef != original.EvidenceRef || row.SourcePeriod != tc.period || row.SourceClockValue != batch.Clock.Value || row.DecidedOn != "" || row.AsOf != "" || row.ReportYear != nil || row.ExtractionMonth != "" {
						t.Fatal("whole-source public field or traversal mismatch")
					}
					seen[row.ID] = true
					count++
					if row.PaidMinor == nil {
						unknown++
					}
					if row.Department == "" {
						empty++
					}
					if row.UnlabelledField != "" {
						unlabelled++
					}
				}
				if next == "" {
					break
				}
				var ok bool
				afterNative, afterID, ok = strings.Cut(next, "|")
				if !ok {
					t.Fatal("invalid internal keyset")
				}
			}
			if count != tc.accepted {
				t.Fatal("source rows lost during complete keyset traversal", count, tc.accepted)
			}
			total += count
			t.Logf("%s: all %d observations published/replayed/traversed; source records=%d, metadata=%d", tc.period, count, tc.input, tc.metadata)
		})
	}
	if total != 32237 || unknown != 1303 || empty != 159 || unlabelled != 34 {
		t.Fatal("whole-source public denominator mismatch", total, unknown, empty, unlabelled)
	}
	var revisions, rows, notes, links int
	if e = p.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM quality_historical_fine_revisions),(SELECT count(*) FROM quality_sanction_events),(SELECT count(*) FROM quality_historical_fine_notes),(SELECT count(*) FROM quality_links)`).Scan(&revisions, &rows, &notes, &links); e != nil || revisions != 6 || rows != 32237 || notes != 6 || links != 0 {
		t.Fatal("historical publication invented/lost context/events/notes/links")
	}
}

func TestOfficialNative2025FinePublicationFullTraversal(t *testing.T) {
	binary := os.Getenv("ANPFUEL_TEST_SANCTIONS_PREPARER_PATH")
	source := os.Getenv("ANPFUEL_TEST_SANCTIONS_SOURCE_PATH")
	if !filepath.IsAbs(binary) || !filepath.IsAbs(source) {
		t.Fatal("absolute trusted2025 preparer/private source required")
	}
	info, e := os.Lstat(source)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() != 450504 {
		t.Fatal("private2025 source unavailable")
	}
	raw, e := os.ReadFile(source)
	if e != nil {
		t.Fatal("private2025 source unavailable")
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	if hash != "277f6cf5c59716581c42c5068fde47827c28eb693ee643c75d7841bda191fe96" {
		t.Fatal("source candidate drift")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	url := "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/multas-aplicadas-2025.csv"
	batch, e := (native.FineRunner{Binary: binary, WorkspaceRoot: t.TempDir()}).Prepare(ctx, native.FineRequest{Path: source, URL: url, Edition: "2025", Identity: prepared.InputIdentity{SHA256: hash, Bytes: int64(len(raw))}})
	if e != nil {
		t.Fatal("actual native2025 boundary refused")
	}
	if batch.Counts.Input != 3395 || batch.Counts.Accepted != 3375 || batch.Counts.Quarantined != 20 {
		t.Fatal("native2025 source counts drift")
	}
	p := freshPool(t)
	pub := NewPublisher(p)
	m := &idMaker{}
	in := application.NativeFineInput{Edition: "2025", Clock: application.NativeFineClock{ReportYear: batch.ReportYear, ExtractionMonth: batch.ExtractionMonth}, Resources: []application.Resource{{URL: url, SHA256: hash, Bytes: uint64(len(raw))}}, Counts: batch.Counts, ManifestHash: batch.ManifestHash, Observations: batch.Observations}
	revision, e := pub.PublishNativeFines(ctx, in, m.next)
	if e != nil {
		t.Fatal("actual native2025 publication refused")
	}
	again, e := pub.PublishNativeFines(ctx, in, m.next)
	if e != nil || again != revision {
		t.Fatal("native2025 replay forked")
	}
	r := qualityread.NewReader(p)
	lease, e := r.RevisionLease(ctx, time.Now(), "sanctions", "sanction-v1", revision)
	if e != nil || lease.SourcePeriod != "2025" || lease.SourceClockValue != "2026-01" {
		t.Fatal("native2025 unified source context lost")
	}
	expected := map[string]application.FineObservation{}
	for _, row := range batch.Observations {
		expected[row.ObservationKey] = row
	}
	seen := map[string]bool{}
	afterNative, afterID := "", ""
	for {
		rows, next, _, e := r.SanctionsAtLease(ctx, lease, "", afterNative, afterID, 100)
		if e != nil {
			t.Fatal("native2025 traversal refused")
		}
		for _, row := range rows {
			original, ok := expected[row.NativeKey]
			if !ok || seen[row.ID] || row.DUFRef != original.DUFRef || row.ReferenceRef != original.DUFRef || row.ReferenceLabel != "Número do DUF" || row.Department != original.Department || row.Status != original.Status || row.DueOn != original.DueOn || row.AmountText != original.AmountText || row.PaidText != original.PaidText || row.AmountMinor == nil || *row.AmountMinor != original.AmountMinor || row.PaidMinor == nil || *row.PaidMinor != original.PaidMinor || row.ProceedingRef != original.ProceedingRef || row.EvidenceRef != original.EvidenceRef {
				t.Fatal("native2025 full source field/traversal mismatch")
			}
			seen[row.ID] = true
		}
		if next == "" {
			break
		}
		var ok bool
		afterNative, afterID, ok = strings.Cut(next, "|")
		if !ok {
			t.Fatal("native2025 keyset invalid")
		}
	}
	if len(seen) != 3375 {
		t.Fatal("native2025 observations missing", len(seen))
	}
	t.Log("2025: all3375observations passed trusted runner/publication/replay/full traversal; total3395=3375+0+20")
}
