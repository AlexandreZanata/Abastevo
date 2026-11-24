package domain

import "testing"

func TestEmailShapeRefusesHeaderAndEnvelopeInjectionBeforeIO(t *testing.T) {
	for _, address := range []string{
		"member@example.invalid\r", "member@example.invalid\n", "\tmember@example.invalid",
		"me\x00mber@example.invalid", "member@example.invalid\u0085", "m@b@example.invalid",
		"Member<member@example.invalid>", "member@example.invalid,other@example.invalid",
	} {
		if ValidEmail(address) {
			t.Errorf("accepted an unsafe address %q", address)
		}
	}
	for _, address := range []string{" member@example.invalid ", "me.mber+tag@example.invalid"} {
		if !ValidEmail(address) {
			t.Errorf("refused an address %q", address)
		}
	}
}
