package main

import (
	"os"
	"strings"
	"testing"
)

// Admission must never accidentally revert to the trusted metadata fixture
// store. Real SQL gate/race tests cover the constructor's runtime behavior.
func TestProductionEvidenceUsesGatedStoreForReservationsAndBytes(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if strings.Contains(s, "evidenceStore := evidenceadapters.NewStore(") {
		t.Fatal("production admission bypasses safety gate")
	}
	for _, required := range []string{"evidenceStore := evidenceadapters.NewIntakeStore(pool.Underlying(), cfg.EvidenceMaxPendingPhotos)", "uploadGateway.Claim = evidenceStore.ClaimUpload", "return evidenceStore.CheckIntake(ctx)"} {
		if !strings.Contains(s, required) {
			t.Fatal("missing production intake guard", required)
		}
	}
}
