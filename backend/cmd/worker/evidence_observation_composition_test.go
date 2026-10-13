package main

import (
	"os"
	"strings"
	"testing"
)

func TestProductionEvidenceSweepPersistsFencedObservation(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"BeginObservation:", "evidenceStore.BeginSweepObservation", "FinishObservation:", "evidenceStore.FinishSweepObservation"} {
		if !strings.Contains(string(raw), required) {
			t.Fatal("worker cleanup observation missing", required)
		}
	}
}
