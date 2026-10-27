package migrate

import (
	"testing"
	"testing/fstest"
)

func TestLoadMinSupported(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
		missing bool
		wantErr bool
	}{
		{name: "valid", content: "000132\n", want: "000132"},
		{name: "trimmed", content: " 000132 ", want: "000132"},
		{name: "missing", missing: true, wantErr: true},
		{name: "short", content: "132", wantErr: true},
		{name: "suffix", content: "000132_extra", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := fstest.MapFS{}
			if !tc.missing {
				files[schemaMinFile] = &fstest.MapFile{Data: []byte(tc.content)}
			}
			got, err := loadMinSupported(files)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheckLedgerCompatibilityWindow(t *testing.T) {
	chain := []migration{
		{version: "000001", checksum: "a"},
		{version: "000002", checksum: "b"},
		{version: "000003", checksum: "c"},
	}
	cases := []struct {
		name     string
		min      string
		recorded map[string]string
		wantErr  bool
	}{
		{
			name:     "exact chain",
			min:      "000003",
			recorded: map[string]string{"000001": "a", "000002": "b", "000003": "c"},
		},
		{
			name:     "newer additive schema",
			min:      "000003",
			recorded: map[string]string{"000001": "a", "000002": "b", "000003": "c", "000004": "d"},
		},
		{
			name:     "database older than floor",
			min:      "000003",
			recorded: map[string]string{"000001": "a", "000002": "b"},
			wantErr:  true,
		},
		{
			name:     "lower floor tolerates missing newer known migration",
			min:      "000002",
			recorded: map[string]string{"000001": "a", "000002": "b"},
		},
		{
			name:     "missing known migration",
			min:      "000001",
			recorded: map[string]string{"000001": "a", "000003": "c"},
			wantErr:  true,
		},
		{
			name:     "modified known migration",
			min:      "000001",
			recorded: map[string]string{"000001": "x", "000002": "b", "000003": "c"},
			wantErr:  true,
		},
		{
			name:     "empty ledger",
			min:      "000001",
			recorded: map[string]string{},
			wantErr:  true,
		},
		{
			name:     "unknown newer without known chain",
			min:      "000001",
			recorded: map[string]string{"000004": "d"},
			wantErr:  true,
		},
		{
			name:     "floor above embedded chain",
			min:      "000004",
			recorded: map[string]string{"000001": "a", "000002": "b", "000003": "c"},
			wantErr:  true,
		},
		{
			name:     "malformed ledger version",
			min:      "000001",
			recorded: map[string]string{"000001": "a", "000002": "b", "000003": "c", "9": "d"},
			wantErr:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkLedger(chain, tc.min, tc.recorded)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCheckLedgerEmptyChain(t *testing.T) {
	if err := checkLedger(nil, "000001", map[string]string{"000001": "a"}); err == nil {
		t.Fatal("expected error for empty chain")
	}
}
