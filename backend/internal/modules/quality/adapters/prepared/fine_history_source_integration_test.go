//go:build integration && qualitysource

package prepared

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Explicit deterministic acceptance against private operator-provided copies of
// the six pinned official resources. Never downloads/prints private observations.
func TestOfficialHistoricalFineRustGoBoundary(t *testing.T) {
	binary := os.Getenv("ANPFUEL_TEST_FINE_HISTORY_PREPARER_PATH")
	root := os.Getenv("ANPFUEL_TEST_FINE_HISTORY_SOURCE_DIR")
	if !filepath.IsAbs(binary) || !filepath.IsAbs(root) {
		t.Fatal("absolute trusted history CLI/private source directory required")
	}
	st, e := os.Lstat(binary)
	if e != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0111 == 0 || st.Mode().Perm()&0022 != 0 {
		t.Fatal("trusted history preparer unavailable")
	}
	for _, tc := range []struct {
		period, file, hash, precision, clock                                   string
		bytes, input, accepted, quarantine, metadata, unknown, emptyDepartment int
	}{
		{"2016-2019", "multas-aplicadas-2016a2019.csv", "15ccbf54367881946104dc6baf65c246b6189638cb306ab06662570451989f25", "DAY", "2020-02-06", 2301471, 16440, 16290, 150, 0, 0, 81},
		{"2020", "multas-aplicadas-2020.csv", "aef7fce874b29db234ab7779019c0bc9fcf13d1bdb2de7d59bbf09f51e32654f", "MONTH", "2021-02", 304268, 2026, 2001, 25, 3, 0, 0},
		{"2021", "multas-2021-junho22.xlsx", "85122d0d1ae999173bac2c772ffb4e14ec414aec3c2f8e30bf7203187936a92d", "MONTH", "2022-06", 720842, 4976, 4929, 47, 3, 0, 0},
		{"2022", "multas-aplicadas-2022.csv", "5ee3df1d9188f273fbf3fbeb7e95f2cea0bc2171fdda23bc4af3cce9478251b5", "MONTH", "2022-04", 436282, 2996, 2970, 26, 0, 0, 0},
		{"2023", "multas-aplicadas-2023.csv", "3865025eb04a82ee5d937d703f063d617843eb7a9f5cbf4c18f989caa33839bb", "MONTH", "2024-06", 445861, 3074, 3053, 21, 0, 1303, 78},
		{"2024", "multas-aplicadas-2024.csv", "0723561b182ac0cc3d031a77050049844e3b29ec098472383b0229051cb67dcd", "MONTH", "2025-03", 416653, 3004, 2994, 10, 0, 0, 0},
	} {
		t.Run(tc.period, func(t *testing.T) {
			filename := filepath.Join(root, tc.file)
			info, e := os.Lstat(filename)
			if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() != int64(tc.bytes) {
				t.Fatal("private immutable source unavailable")
			}
			raw, e := os.ReadFile(filename)
			if e != nil {
				t.Fatal("private source read unavailable")
			}
			sum := sha256.Sum256(raw)
			hash := hex.EncodeToString(sum[:])
			if hash != tc.hash {
				t.Fatal("official resource changed; reconcile a new candidate")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			out := filepath.Join(t.TempDir(), "prepared")
			cmd := exec.CommandContext(ctx, binary, filename, tc.period, out)
			cmd.Stdout = io.Discard
			cmd.Stderr = io.Discard
			if cmd.Run() != nil {
				t.Fatal("trusted history preparation refused")
			}
			manifest, e := os.ReadFile(filepath.Join(out, "manifest.json"))
			if e != nil {
				t.Fatal("manifest unavailable")
			}
			batch, e := LoadFineHistory(ctx, manifest, func(name string) (io.ReadCloser, error) { return os.Open(filepath.Join(out, name)) }, InputIdentity{SHA256: hash, Bytes: int64(len(raw))}, tc.period)
			if e != nil {
				t.Fatal("native typed historical boundary", e)
			}
			if batch.Counts.Input != tc.input || batch.Counts.Accepted != tc.accepted || batch.Counts.Duplicates != 0 || batch.Counts.Quarantined != tc.quarantine || batch.MetadataRecords != tc.metadata || len(batch.Notes) != tc.metadata || len(batch.Observations) != tc.accepted || batch.Clock.Precision != tc.precision || batch.Clock.Value != tc.clock {
				t.Fatal("whole-source historical loss/precision mismatch", batch.Counts)
			}
			unknown, emptyDepartment, unlabelled := 0, 0, 0
			for _, row := range batch.Observations {
				if !row.Valid() {
					t.Fatal("typed source fact invalid")
				}
				if row.PaidMinor == nil {
					unknown++
				}
				if row.Department == "" {
					emptyDepartment++
				}
				if row.UnlabelledField != "" {
					unlabelled++
				}
			}
			if unknown != tc.unknown || emptyDepartment != tc.emptyDepartment || (tc.period == "2024" && unlabelled != 34) {
				t.Fatal("unknown source facts lost", unknown, emptyDepartment, unlabelled)
			}
			t.Logf("%s: %d=%d+0+%d; metadata=%d, unknown_paid=%d", tc.period, tc.input, tc.accepted, tc.quarantine, tc.metadata, unknown)
		})
	}
}
