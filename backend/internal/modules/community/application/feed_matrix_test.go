package application

import "testing"

// CS04: the supported set is 8 products x recent/cheapest/best/worst.
// Every combination must validate; anything outside refuses.
func TestFeedFilterSupportsFullProductOrderMatrix(t *testing.T) {
	products := []string{
		"ETHANOL", "GASOLINE_REGULAR", "GASOLINE_ADDITIVED",
		"GASOLINE_PREMIUM_GRADE", "DIESEL_S500", "DIESEL_S10",
		"CNG", "LPG_P13",
	}
	orders := []string{"recent", "cheapest", "best", "worst"}
	for _, product := range products {
		for _, order := range orders {
			f, err := ValidateFeed("MT", "5107925", product, order, 20, "")
			if err != nil {
				t.Errorf("product %s order %s refused: %v", product, order, err)
				continue
			}
			if f.Product != product || f.Order != order {
				t.Errorf("filter mismatch: %+v", f)
			}
		}
	}
}
