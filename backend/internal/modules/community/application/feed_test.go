package application

import (
	"testing"
	"time"
)

func TestFeedScopeAndComparablePrices(t *testing.T) {
	f, err := ValidateFeed("MT", "5107925", "ETHANOL", "recent", 20, "")
	if err != nil || f.Unit != "L" {
		t.Fatalf("valid filter: %+v %v", f, err)
	}
	for _, x := range []struct {
		state, city, fuel, order string
		limit                    int
	}{
		{"", "5107925", "ETHANOL", "recent", 20}, {"ZZ", "5107925", "ETHANOL", "recent", 20},
		{"MT", "", "ETHANOL", "recent", 20}, {"MT", "5107925'", "ETHANOL", "recent", 20},
		{"MT", "5107925", "", "cheapest", 20}, {"MT", "5107925", "OTHER", "recent", 20},
		{"MT", "5107925", "ETHANOL", "popular", 20}, {"MT", "5107925", "ETHANOL", "recent", 0},
		{"MT", "5107925", "ETHANOL", "recent", 51},
	} {
		if _, err := ValidateFeed(x.state, x.city, x.fuel, x.order, x.limit, ""); err == nil {
			t.Fatalf("accepted invalid %+v", x)
		}
	}
	for fuel, unit := range map[string]string{"CNG": "M3", "LPG_P13": "KG_13", "DIESEL_S10": "L", "GASOLINE_ADDITIVED": "L"} {
		f, err := ValidateFeed("MT", "5107925", fuel, "cheapest", 50, "")
		if err != nil || f.Unit != unit {
			t.Fatalf("unit %s: %+v %v", fuel, f, err)
		}
	}
	for _, order := range []string{"recent", "cheapest", "best", "worst"} {
		if _, err := ValidateFeed("MT", "5107925", "ETHANOL", order, 20, ""); err != nil {
			t.Fatalf("order %s: %v", order, err)
		}
	}
}

func TestFeedRatingCursorRoundTripAndMalformed(t *testing.T) {
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	avg := 4.5
	key := FeedKeyAvg(at, "d6c74c23-63db-4c24-a2e5-408cb23bad26", 5999, &avg)
	f, err := ValidateFeed("MT", "5107925", "GASOLINE_REGULAR", "best", 20, key)
	if err != nil || !f.HasCursor || !f.HasAfterAvg || f.AfterAvg != 4.5 {
		t.Fatalf("round trip %+v %v", f, err)
	}
	tail, err := ValidateFeed("MT", "5107925", "GASOLINE_REGULAR", "worst", 20,
		FeedKeyAvg(at, "d6c74c23-63db-4c24-a2e5-408cb23bad26", 5999, nil))
	if err != nil || !tail.HasCursor || tail.HasAfterAvg {
		t.Fatalf("unrated tail %+v %v", tail, err)
	}
	for _, key := range []string{
		`{"time":"2026-10-06T12:00:00Z","id":"d6c74c23-63db-4c24-a2e5-408cb23bad26","amount":5}`,
		`{"time":"2026-10-06T12:00:00Z","id":"d6c74c23-63db-4c24-a2e5-408cb23bad26","amount":5,"avg":9}`,
		`{"time":"2026-10-06T12:00:00Z","id":"d6c74c23-63db-4c24-a2e5-408cb23bad26","amount":5,"avg":-2}`,
	} {
		if _, err := ValidateFeed("MT", "5107925", "ETHANOL", "best", 20, key); err == nil {
			t.Fatalf("accepted malformed rating cursor %s", key)
		}
	}
}

func TestFeedKeyRoundTripAndMalformed(t *testing.T) {
	at := time.Date(2026, 10, 6, 12, 0, 0, 123000, time.UTC)
	key := FeedKey(at, "d6c74c23-63db-4c24-a2e5-408cb23bad26", 5999)
	f, err := ValidateFeed("MT", "5107925", "GASOLINE_REGULAR", "recent", 20, key)
	if err != nil || !f.AfterTime.Equal(at) || f.AfterAmount != 5999 || !f.HasCursor {
		t.Fatalf("round trip %+v %v", f, err)
	}
	for _, key := range []string{"bad", "{}", `{"time":"2026-10-06T12:00:00Z","id":"invalid","amount":5}`, `{"time":"2026-10-06T12:00:00Z","id":"d6c74c23-63db-4c24-a2e5-408cb23bad26","amount":-1}`} {
		if _, err := ValidateFeed("MT", "5107925", "ETHANOL", "recent", 20, key); err == nil {
			t.Fatal("accepted malformed key")
		}
	}
}
