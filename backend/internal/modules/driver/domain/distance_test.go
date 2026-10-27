package domain

import "testing"

func TestDistanceSourcesAndPartitions(t *testing.T) {
	w := window()
	obs := []DistanceObservation{{"odo", "s", "ODOMETER", w, ptr(200000), true, false}, {"gps", "s", "GPS_ESTIMATE", w, ptr(190000), true, false}}
	r, e := SelectDistance(w, "ODOMETER", obs)
	if e != nil || r.Metres == nil || *r.Metres != 200000 {
		t.Fatal(r, e)
	}
	p, e := PartitionDistance(200000, ptr(200000), ptr(120000), ptr(0), true)
	if e != nil || *p.EmptyMetres != 80000 || p.PaidPercent != "60" {
		t.Fatal(p, e)
	}
	p, e = PartitionDistance(200000, ptr(200000), ptr(120000), nil, false)
	if e != nil || p.EmptyMetres != nil || p.PaidPercent != "" {
		t.Fatal(p, e)
	}
	if _, e = PartitionDistance(200000, ptr(100000), ptr(120000), ptr(100000), true); e == nil {
		t.Fatal("paid exceeds work")
	}
	for _, args := range [][2]string{{"s1", "s2"}, {"s1", "s1"}} {
		r, e = OdometerDistance(200, 100, args[0], args[1])
		if e != nil || r.Metres != nil {
			t.Fatal(r, e)
		}
	}
	r, e = SelectDistance(w, "ROUTE_PREDICTION", nil)
	if e != nil || r.Metres != nil || r.Unknown != "prediction-not-measured" {
		t.Fatal(r, e)
	}
	obs[0].HasGap = true
	r, e = SelectDistance(w, "ODOMETER", obs)
	if e != nil || r.Metres != nil {
		t.Fatal(r, e)
	}
	obs[0].HasGap = false
	obs = append(obs, DistanceObservation{"second", "s", "ODOMETER", w, ptr(100), true, false})
	r, e = SelectDistance(w, "ODOMETER", obs)
	if e != nil || r.Metres != nil {
		t.Fatal(r, e)
	}
}
