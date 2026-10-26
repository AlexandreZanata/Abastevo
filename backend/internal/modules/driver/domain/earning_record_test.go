package domain

import (
	"testing"
	"time"
)

func validRecord() EarningRecord {
	return EarningRecord{ID: "11111111-1111-4111-8111-111111111111", VehicleID: "22222222-2222-4222-8222-222222222222", Platform: "UBER", SourceHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Coverage: "SUMMARY", Currency: "BRL", From: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC), Timezone: "America/Cuiaba", Money: Earnings{Basis: "INCLUSIVE_NET", Net: ptr(45000)}}
}
func TestEarningRecordCanonicalPrecisionAndOwnedValues(t *testing.T) {
	original := validRecord()
	r, e := original.Normalize()
	if e != nil {
		t.Fatal(e)
	}
	*original.Money.Net = 99000
	if *r.Money.Net != 45000 {
		t.Fatal("caller retained mutable money")
	}
	hash, e := r.BusinessHash()
	if e != nil {
		t.Fatal(e)
	}
	other := r
	other.ID = "33333333-3333-4333-8333-333333333333"
	other.From = r.From.In(time.FixedZone("offset", -4*3600))
	other.To = r.To.In(time.FixedZone("offset", -4*3600))
	h, e := other.BusinessHash()
	if e != nil || hash != h {
		t.Fatal("source digest changed for equivalent instants", e)
	}
	variants := []EarningRecord{r, r, r, r, r, r}
	variants[0].From = variants[0].From.Add(time.Nanosecond)
	variants[1].Currency = "USD"
	variants[2].Timezone = "Local"
	variants[3].SourceHash = "raw-provider-reference"
	variants[4].To = variants[4].From
	variants[5].Money = Earnings{Basis: "INCLUSIVE_NET", Net: ptr(MaxAmount + 1)}
	for _, v := range variants {
		if _, e = v.Normalize(); e == nil {
			t.Fatal("invalid record admitted")
		}
	}
	boundary := validRecord()
	boundary.From = time.Date(1, 1, 1, 0, 0, 0, 0, time.FixedZone("offset", 14*3600))
	boundary.To = boundary.From.Add(time.Hour)
	if _, e := boundary.Normalize(); e == nil {
		t.Fatal("UTC year zero slipped through local-year validation")
	}

}
