package domain

import (
	"strings"
	"testing"
)

func TestDeviceLabelRequiresCanonicalBoundedPrivateMetadata(t *testing.T) {
	for _, label := range []string{"Synthetic device", strings.Repeat("x", 80), strings.Repeat("é", 40)} {
		if e := ValidateDeviceLabel(label); e != nil {
			t.Fatal("valid device label", e)
		}
	}
	for _, label := range []string{"", " ", " Synthetic device", "Synthetic device ", strings.Repeat("x", 81), strings.Repeat("é", 41), "Synthetic\nlabel", "Synthetic\tlabel", "Synthetic\u202elabel", "Synthetic\u200blabel", "Synthetic\u2066label", string([]byte{0xff})} {
		if e := ValidateDeviceLabel(label); e != ErrInvalid {
			t.Fatal("invalid device label accepted", e)
		}
	}
}
