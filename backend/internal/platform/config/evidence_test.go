package config

import (
	"strings"
	"testing"
)

func TestEvidenceIntakeBudgetRequiresExplicitBound(t *testing.T) {
	for _, raw := range []string{"", "0", "4", "100"} {
		cfg, err := load(envFunc(map[string]string{keyEvidenceMaxPending: raw}))
		if err != nil {
			t.Fatal(raw, err)
		}
		if (raw == "" || raw == "0") && cfg.EvidenceMaxPendingPhotos != 0 {
			t.Fatal("default intake opened")
		}
		if raw == "4" && cfg.EvidenceMaxPendingPhotos != 4 {
			t.Fatal("explicit budget lost")
		}
	}
	for _, raw := range []string{"-1", "101", "untrusted-value", "2147483648"} {
		_, err := load(envFunc(map[string]string{keyEvidenceMaxPending: raw}))
		if err == nil || !strings.Contains(err.Error(), keyEvidenceMaxPending) || strings.Contains(err.Error(), raw) {
			t.Fatal("invalid budget or raw value leaked", err)
		}
	}
}
