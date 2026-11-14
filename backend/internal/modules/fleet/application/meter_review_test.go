package application

import "testing"

func TestMeterReviewPolicyAndBoundedRealStates(t *testing.T) {
	read, write := MeterReviewPolicy(false), MeterReviewPolicy(true)
	if read.Validate() != nil || write.Validate() != nil || read.Permission != "meter.reading.correct" || read.Capability != "organizations" || read.Fresh || !write.Fresh || !write.Effect.Write() || write.Permission != read.Permission {
		t.Fatal("record owner review policy missing or widened")
	}
	for _, state := range []string{"", MeterReviewOpen, MeterReviewExpired, MeterReviewAccepted, MeterReviewRefused} {
		if (MeterReviewFilter{Limit: 100, State: state}).Validate() != nil {
			t.Fatal("real review state refused", state)
		}
	}
	for _, v := range []MeterReviewFilter{{Limit: 0}, {Limit: 101}, {Limit: 1, State: "AUTO_APPROVED"}, {Limit: 1, AfterID: "foreign"}} {
		if v.Validate() == nil {
			t.Fatal("unbounded or invented review filter accepted")
		}
	}
}
