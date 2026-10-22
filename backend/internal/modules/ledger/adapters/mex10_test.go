package adapters

import (
	"math"
	"testing"
)

func TestMEX10MoneyOverflowRefusesWithoutWrappedValue(t *testing.T) {
	for _, ab := range [][2]int64{{-1, 1}, {1, -1}, {math.MaxInt64, 1}} {
		if v, e := checkedReportSum(ab[0], ab[1]); e == nil || v != 0 {
			t.Fatalf("sum %v produced %d %v", ab, v, e)
		}
	}
	if v, e := checkedReportSum(math.MaxInt64-1, 1); e != nil || v != math.MaxInt64 {
		t.Fatal(v, e)
	}
}
func TestMEX10StrictPayloadTrailingJunk(t *testing.T) {
	raw := []byte(`{"id":"11111111-1111-4111-8111-111111111111","vehicle_id":"22222222-2222-4222-8222-222222222222","occurred_at":"2026-03-01T00:00:00Z","timezone":"UTC","amount_centavos":1000,"category":"tolls"}`)
	if _, _, _, _, e := parsePayload("expense", raw); e != nil {
		t.Fatal(e)
	}
	for _, tail := range []string{"!", "{}", "null"} {
		if _, _, _, _, e := parsePayload("expense", append(append([]byte{}, raw...), []byte(tail)...)); e == nil {
			t.Fatal("trailing data admitted")
		}
	}
}
