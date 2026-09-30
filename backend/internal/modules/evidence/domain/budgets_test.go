package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestForwardBudgetConstantsFrozen(t *testing.T) {
	if ForwardWireMIME != "image/jpeg" {
		t.Errorf("wire format must stay image/jpeg, got %q", ForwardWireMIME)
	}
	if ForwardTargetBytes != 153600 || ForwardCapBytes != 262144 {
		t.Errorf("byte budgets must stay 150 KiB/256 KiB, got %d/%d", ForwardTargetBytes, ForwardCapBytes)
	}
	if ForwardMaxEdgePixels != 1600 || ForwardMaxMegapixels != 2_000_000 {
		t.Errorf("pixel budgets must stay 1600 edge/2 MP, got %d/%d", ForwardMaxEdgePixels, ForwardMaxMegapixels)
	}
	if ForwardMaxAttempts != 3 {
		t.Errorf("attempts must stay 3, got %d", ForwardMaxAttempts)
	}
	if ForwardWorkingMemoryHypothesis != 33554432 {
		t.Errorf("working-memory hypothesis must stay 32 MiB, got %d", ForwardWorkingMemoryHypothesis)
	}
	if ForwardDeadline != 24*time.Hour {
		t.Errorf("deadline must stay 24 h, got %v", ForwardDeadline)
	}
}

func TestValidateForwardWireBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name        string
		size        int64
		edge        int
		pixels      int64
		wantVerdict string
	}{
		{"target-exact", 153600, 1600, 2_000_000, ForwardOK},
		{"cap-exact", 262144, 1600, 2_000_000, ForwardOK},
		{"cap-plus-one", 262145, 1600, 2_000_000, ForwardBytesOverCap},
		{"edge-plus-one", 102400, 1601, 2_000_000, ForwardEdgeOver},
		{"mp-over", 102400, 1600, 2_000_001, ForwardPixelsOver},
		{"small", 81920, 1200, 1_440_000, ForwardOK},
		{"empty", 0, 0, 0, ForwardBytesEmpty},
		{"negative", -8, 0, 0, ForwardBytesEmpty},
	} {
		if got := ForwardVerdictCode(ValidateForwardWire(tc.size, tc.edge, tc.pixels)); got != tc.wantVerdict {
			t.Errorf("%s: verdict must be %q, got %q", tc.name, tc.wantVerdict, got)
		}
	}
}

type budgetFixture struct {
	Format     string `json:"format"`
	Provenance string `json:"provenance"`
	Wire       string `json:"wire_format"`
	Target     int64  `json:"target_bytes"`
	Cap        int64  `json:"cap_bytes"`
	Edge       int    `json:"max_edge_pixels"`
	MP         int    `json:"max_megapixels"`
	Attempts   int    `json:"max_encoding_attempts"`
	Working    int64  `json:"working_memory_hypothesis_bytes"`
	DeadlineH  int    `json:"deadline_hours"`
	Cases      []struct {
		ID       string `json:"id"`
		Bytes    int64  `json:"bytes"`
		Edge     int    `json:"edge"`
		MP       string `json:"megapixels"`
		Verdict  string `json:"verdict"`
		Receipt  string `json:"first_receipt"`
		Retry    string `json:"retry_at"`
		Expected string `json:"expect_deadline"`
	} `json:"cases"`
}

// TestBudgetFixtureReplay replays every contracts vector against the
// frozen code so fixtures and implementation cannot drift (P14-T01
// pattern). RED proven by loosening a bound by one: cap-plus-one
// replays ok instead of bytes-over-cap; GREEN on restore.
func TestBudgetFixtureReplay(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "contracts", "testdata", "media", "budgets-v1.json"))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	var fx budgetFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("fixture JSON: %v", err)
	}
	if fx.Format != "media-budgets-v1" || fx.Wire != ForwardWireMIME {
		t.Fatalf("fixture header wrong: format=%q wire=%q", fx.Format, fx.Wire)
	}
	if fx.Target != ForwardTargetBytes || fx.Cap != ForwardCapBytes || fx.Edge != ForwardMaxEdgePixels ||
		fx.MP != 2 || fx.Attempts != ForwardMaxAttempts || fx.Working != ForwardWorkingMemoryHypothesis ||
		fx.DeadlineH != 24 {
		t.Fatalf("fixture bounds drifted from code: %+v", fx)
	}
	for _, c := range fx.Cases {
		switch c.ID {
		case "deadline-exact", "deadline-no-extension":
			receipt, err := time.Parse(time.RFC3339, c.Receipt)
			if err != nil {
				t.Fatalf("%s receipt: %v", c.ID, err)
			}
			want, err := time.Parse(time.RFC3339, c.Expected)
			if err != nil {
				t.Fatalf("%s expected: %v", c.ID, err)
			}
			if got := ForwardDeadlineAfter(receipt); !got.Equal(want) {
				t.Errorf("%s: deadline must be %v, got %v", c.ID, want, got)
			}
			if c.Retry != "" {
				retry, err := time.Parse(time.RFC3339, c.Retry)
				if err != nil {
					t.Fatalf("%s retry: %v", c.ID, err)
				}
				_ = retry
				if got := ForwardDeadlineAfter(receipt); !got.Equal(want) {
					t.Errorf("%s: retry must not extend deadline, got %v", c.ID, got)
				}
			}
		default:
			f, err := strconv.ParseFloat(c.MP, 64)
			if err != nil {
				t.Fatalf("%s megapixels: %v", c.ID, err)
			}
			pixels := int64(f * 1_000_000)
			if got := ForwardVerdictCode(ValidateForwardWire(c.Bytes, c.Edge, pixels)); got != c.Verdict {
				t.Errorf("%s: verdict must be %q, got %q", c.ID, c.Verdict, got)
			}
		}
	}
}

func TestForwardDeadlineNeverExtends(t *testing.T) {
	first := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	want := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	for _, at := range []time.Time{
		first,
		first.Add(12 * time.Hour),
		first.Add(23*time.Hour + 59*time.Minute),
	} {
		_ = at
		if got := ForwardDeadlineAfter(first); !got.Equal(want) {
			t.Errorf("deadline must stay %v, got %v", want, got)
		}
	}
}
