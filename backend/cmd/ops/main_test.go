package main

import (
	"strings"
	"testing"
)

func TestQualityInspectionRequiresRestrictedOperator(t *testing.T) {
	for _, args := range [][]string{{"quality", "queue"}, {"quality", "inspect", "--target", "11111111-1111-4111-8111-111111111111"}, {"quality", "group", "--station", "11111111-1111-4111-8111-111111111111", "--fuel", "ETHANOL", "--day", "2026-10-13"}} {
		if e := run(args, func(string) string { return "" }); e == nil || !strings.Contains(e.Error(), "operator identity required") {
			t.Fatal("anonymous inspection", e)
		}
	}
}
