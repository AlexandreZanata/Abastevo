package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestReadFuelCardStatementUsesExactMappedFacts(t *testing.T) {
	const head = "transaction_id,fueling_id,occurred_at,currency,paid_centavos\n"
	const line = "342e5b51-276b-466d-b6ef-ef968858636d,,2026-11-15T15:47:00.000001023Z,BRL,4567\n"
	valid := head + line
	lines, err := ReadFuelCardStatement(valid, int64(len(valid)), 1)
	if err != nil || len(lines) != 1 || lines[0].PaidCentavos != 4567 || lines[0].OccurredAt.Nanosecond() != 1023 {
		t.Fatalf("read %v, %v", lines, err)
	}
	for _, raw := range []string{
		strings.Replace(valid, ",BRL,", ",USD,", 1),
		strings.Replace(valid, ",4567", ",45.67", 1),
		strings.Replace(valid, ",4567", ",-4567", 1),
		strings.Replace(valid, ",4567", ",1000000000001", 1),
		strings.Replace(valid, "2026-11-15T15:47:00.000001023Z", "2026-11-15", 1),
		strings.Replace(valid, ",BRL,", ",4111111111111111,BRL,", 1),
	} {
		if _, err := ReadFuelCardStatement(raw, 4096, 10); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid row returned %v", err)
		}
	}
	if _, err := ReadFuelCardStatement(valid+line, 4096, 10); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate identity returned %v", err)
	}
	if _, err := ReadFuelCardStatement(valid, int64(len(valid)-1), 10); !errors.Is(err, ErrInvalid) {
		t.Fatalf("payload budget returned %v", err)
	}
}
