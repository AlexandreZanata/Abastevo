package kernel

import "testing"

func TestPublicStationLegalNamePreservesSourceWhitespaceAndMinimizesCPF(t *testing.T) {
	const raw = " [P25-TEST] Legal   LTDA "
	if got := PublicStationLegalName(raw); got != raw {
		t.Fatalf("source formatting changed: %q", got)
	}
	if got := PublicStationLegalName("[P25-TEST] Person 000.000.000-00"); got != "[P25-TEST] Person " {
		t.Fatalf("CPF leaked: %q", got)
	}
	if got := PublicStationLegalName("00000000000"); got != "" {
		t.Fatal("missing legal name invented", got)
	}
}
