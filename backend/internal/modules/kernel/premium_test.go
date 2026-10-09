package kernel

import "testing"

func TestPremiumGradeDoesNotInventANPSourceLabel(t *testing.T) {
	unit, err := GasolinePremiumGrade.Unit()
	if err != nil || unit != Liter {
		t.Fatalf("premium unit: %s %v", unit, err)
	}
	for _, label := range []string{"GASOLINA PREMIUM", "GASOLINA PODIUM", "DIESEL PODIUM"} {
		if _, err := ParseProduct(label); err == nil {
			t.Fatalf("invented survey label %s", label)
		}
	}
}
