package kernel_test

import (
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

func TestParseOrgID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"canonical", "7b0e2a54-0000-4000-8000-000000000001", "7b0e2a54-0000-4000-8000-000000000001"},
		{"uppercase", "7B0E2A54-0000-4000-8000-000000000001", "7b0e2a54-0000-4000-8000-000000000001"},
		{"mixed case", "7b0E2A54-0000-4000-8000-000000000001", "7b0e2a54-0000-4000-8000-000000000001"},
		{"non-v4 UUID", "7b0e2a54-0000-1000-8000-000000000001", "7b0e2a54-0000-1000-8000-000000000001"},
		{"empty", "", ""},
		{"nil UUID", "00000000-0000-0000-0000-000000000000", ""},
		{"short", "7b0e2a54-0000-4000-8000-00000000001", ""},
		{"long", "7b0e2a54-0000-4000-8000-0000000000010", ""},
		{"leading space", " 7b0e2a54-0000-4000-8000-000000000001", ""},
		{"trailing newline", "7b0e2a54-0000-4000-8000-000000000001\n", ""},
		{"undashed", "7b0e2a54000040008000000000000001", ""},
		{"braced", "{7b0e2a54-0000-4000-8000-000000000001}", ""},
		{"wrong separator", "7b0e2a54_0000-4000-8000-000000000001", ""},
		{"misplaced separator", "7b0e2a5-40000-4000-8000-000000000001", ""},
		{"invalid hex", "zb0e2a54-0000-4000-8000-000000000001", ""},
		{"extra separator", "7b0e2a54-0000-4000-8000-00000000000-", ""},
		{"two extra separators", "7b0e2a54-0000-4000-8000-0000000000--", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := kernel.ParseOrgID(tt.input)
			if tt.want == "" {
				if !errors.Is(err, kernel.ErrInvalidOrgID) || id != (kernel.OrgID{}) {
					t.Fatalf("invalid input returned id=%v err=%v", id, err)
				}
				return
			}
			if err != nil || id.String() != tt.want {
				t.Fatalf("ParseOrgID() = %v, %v; want %q", id, err, tt.want)
			}
			again, err := kernel.ParseOrgID(id.String())
			if err != nil || again != id {
				t.Fatalf("canonical identifier changed on reparse: %v, %v", again, err)
			}
		})
	}
}

func TestOrgScopeRejectsZeroValues(t *testing.T) {
	if err := (kernel.OrgScope{}).Validate(); !errors.Is(err, kernel.ErrInvalidOrgScope) {
		t.Fatalf("zero scope validation = %v", err)
	}
	scope, err := kernel.NewOrgScope(kernel.OrgID{})
	if !errors.Is(err, kernel.ErrInvalidOrgScope) || scope != (kernel.OrgScope{}) {
		t.Fatalf("zero ID returned scope=%v err=%v", scope, err)
	}
	if (kernel.OrgID{}).String() != "" || (kernel.OrgScope{}).OrgID() != (kernel.OrgID{}) {
		t.Fatal("zero value fabricated an organization")
	}
}

func TestOrgScopeKeepsItsSelectedOrganization(t *testing.T) {
	first, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	second, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000002")
	if err != nil {
		t.Fatal(err)
	}
	original, err := kernel.NewOrgScope(first)
	if err != nil {
		t.Fatal(err)
	}
	selected := original
	selected, err = kernel.NewOrgScope(second)
	if err != nil {
		t.Fatal(err)
	}
	if original.OrgID() != first || selected.OrgID() != second || original == selected {
		t.Fatal("changing request scope moved the original organization")
	}
	if original.Validate() != nil || selected.Validate() != nil {
		t.Fatal("valid scope was rejected")
	}
}
