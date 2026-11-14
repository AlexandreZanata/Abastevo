package domain_test

import (
	"errors"
	"testing"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
)

func TestOwnedNineProductCatalog(t *testing.T) {
	for _, tc := range []struct {
		code     fuel.ProductCode
		unit     fleet.Unit
		category fuel.Category
	}{
		{fuel.Ethanol, fleet.Liter, fuel.LiquidFuel}, {fuel.GasolineRegular, fleet.Liter, fuel.LiquidFuel}, {fuel.GasolineAdditived, fleet.Liter, fuel.LiquidFuel}, {fuel.GasolinePremiumGrade, fleet.Liter, fuel.LiquidFuel}, {fuel.DieselS10, fleet.Liter, fuel.LiquidFuel}, {fuel.DieselS500, fleet.Liter, fuel.LiquidFuel}, {fuel.CNG, fleet.CubicMetre, fuel.GasFuel}, {fuel.Electricity, fleet.KilowattHour, fuel.Electric}, {fuel.ARLA32, fleet.Liter, fuel.Additive},
	} {
		t.Run(string(tc.code), func(t *testing.T) {
			descriptor, err := fuel.Lookup(tc.code)
			if err != nil || descriptor.Code() != tc.code || descriptor.Unit() != tc.unit || descriptor.Category() != tc.category {
				t.Fatal("owned descriptor incorrect", err)
			}
			fleetDescriptor, ok := (fuel.Catalog{}).Product(string(tc.code))
			if !ok || fleetDescriptor.Code != string(tc.code) || fleetDescriptor.Unit != tc.unit {
				t.Fatal("fleet port does not use owned descriptor")
			}
		})
	}
	if len(fuel.Products()) != 9 {
		t.Fatal("catalog does not contain exactly nine products")
	}
}
func TestPublicCylinderAndNormalizedAliasesAreNotEnterpriseProducts(t *testing.T) {
	for _, code := range []fuel.ProductCode{"", "LPG_P13", "diesel_s10", " DIESEL_S10", "DIESEL_S10 ", "GNV", "ARLA32", "OTHER"} {
		if _, err := fuel.Lookup(code); !errors.Is(err, fuel.ErrInvalid) {
			t.Fatal("unknown product accepted")
		}
		if _, ok := (fuel.Catalog{}).Product(string(code)); ok {
			t.Fatal("unknown fleet product accepted")
		}
	}
}
