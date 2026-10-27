package domain

import (
	"bufio"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func fields(s string) map[string]string {
	m := map[string]string{}
	for _, p := range strings.Split(s, ";") {
		k, v, _ := strings.Cut(p, "=")
		m[k] = v
	}
	return m
}
func number(s string) int64 {
	v, e := strconv.ParseInt(s, 10, 64)
	if e != nil {
		panic(e)
	}
	return v
}
func optional(s string) *int64 {
	if s == "null" {
		return nil
	}
	return ptr(number(s))
}
func nullable(s string) string {
	if s == "" {
		return "null"
	}
	return s
}
func ratioMap(r Ratios) map[string]string {
	return map[string]string{"per_km": nullable(r.PerKm), "per_hour": nullable(r.PerHour), "unknown": r.Unknown}
}
func TestActualDriverManualVectors(t *testing.T) {
	f, err := os.Open("../../../../../contracts/testdata/driver-suite/driver-manual-reference-v1.psv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Scan()
	count := 0
	for sc.Scan() {
		p := strings.Split(sc.Text(), "|")
		a, want := fields(p[2]), fields(p[3])
		if p[1] == "UNION" {
			continue
		}
		count++
		t.Run(p[0], func(t *testing.T) {
			got := map[string]string{}
			var err error
			switch p[1] {
			case "DETAILED", "NET":
				e := Earnings{}
				if p[1] == "NET" {
					e = Earnings{Basis: "INCLUSIVE_NET", Net: optional(a["inclusive"])}
				}
				if p[1] == "DETAILED" {
					e = Earnings{Basis: "DETAILED", Gross: optional(a["gross"]), Fees: optional(a["fees"]), Tips: optional(a["tips"]), Bonus: optional(a["bonus"]), Refund: optional(a["refund"])}
				}
				n, eerr := e.Earned()
				err = eerr
				got["earned"] = fmt.Sprint(n)
				if p[1] == "NET" {
					got["gross"] = "null"
					got["fees"] = "null"
					got["unknown"] = "gross-and-fee-components-unknown"
				}
			case "PAYOUT":
				v, e := Views([]int64{number(a["earned"])}, []CashMovement{{"payout", "PAYOUT", "BANK", number(a["payout"])}})
				err = e
				got["earned"] = fmt.Sprint(v.Earned)
				got["received"] = fmt.Sprint(v.Received["BANK"])
			case "RATIOS":
				r, e := ExactRatios(number(a["amount"]), optional(a["distance"]), optional(a["seconds"]))
				err = e
				got = ratioMap(r)
			case "RESULT", "MODEL":
				costs := []Cost{}
				estimates := []Cost{}
				earned := a["earned"]
				complete := a["complete"] == "1"
				key := "costs"
				if p[1] == "MODEL" {
					earned = a["recorded"]
					complete = true
					key = "estimates"
				}
				if a[key] != "null" {
					for i, n := range strings.Split(a[key], ",") {
						c := Cost{fmt.Sprint(i), number(n), window()}
						if p[1] == "MODEL" {
							estimates = append(estimates, c)
						} else {
							costs = append(costs, c)
						}
					}
				}
				r, e := Result(number(earned), window(), window(), costs, complete, estimates, optional(a["distance"]), optional(a["seconds"]))
				err = e
				if p[1] == "MODEL" {
					got = ratioMap(r.ModeledRatios)
					got["recorded"] = fmt.Sprint(*r.Recorded)
					got["modeled"] = fmt.Sprint(*r.Modeled)
				} else if r.Recorded == nil {
					got = map[string]string{"result": "null", "per_km": "null", "per_hour": "null", "unknown": r.Unknown}
				} else {
					got = ratioMap(r.RecordedRatios)
					got["result"] = fmt.Sprint(*r.Recorded)
				}
			default:
				t.Fatal("unknown operation", p[1])
			}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("got=%v want=%v err=%v", got, want, err)
			}
		})
	}
	if sc.Err() != nil || count != 12 {
		t.Fatalf("count=%d err=%v", count, sc.Err())
	}
}
