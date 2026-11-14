package application

import (
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

func TestValidateEditorialExportPage(t *testing.T) {
	cases := []struct {
		name    string
		afterID string
		limit   int
		ok      bool
	}{
		{"first page", "", 1, true},
		{"max page", "", MaxEditorialExportLimit, true},
		{"cursor", "0a1b2c3d-0000-4000-8000-00000000abcd", 10, true},
		{"zero limit", "", 0, false},
		{"over limit", "", MaxEditorialExportLimit + 1, false},
		{"upper-case cursor", "0A1B2C3D-0000-4000-8000-00000000ABCD", 10, false},
		{"injected cursor", "' OR 1=1 --", 10, false},
		{"short cursor", "0a1b2c3d", 10, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateEditorialExportPage(tc.afterID, tc.limit)
			if tc.ok != (err == nil) {
				t.Fatalf("ValidateEditorialExportPage(%q, %d) = %v", tc.afterID, tc.limit, err)
			}
			if err != nil && !errors.Is(err, ErrInvalidEditorialExportPage) {
				t.Fatalf("error = %v, want ErrInvalidEditorialExportPage", err)
			}
		})
	}
}

func TestEditorialCategoryUsesTheCodeOwnedTaxonomy(t *testing.T) {
	for _, item := range domain.DefaultCategories() {
		name, description := EditorialCategory(item.Code)
		if name != item.Name || description != item.Description || name == "" {
			t.Errorf("%s = %q/%q", item.Code, name, description)
		}
	}
	if name, description := EditorialCategory("FUTURE"); name != "FUTURE" || description != "" {
		t.Errorf("unknown category = %q/%q", name, description)
	}
}
