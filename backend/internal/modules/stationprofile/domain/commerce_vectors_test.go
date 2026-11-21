package domain_test

import (
	"encoding/csv"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/domain"
)

func commerceParseKV(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ";") {
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			out[kv[0]] = kv[1]
		}
	}
	return out
}

func commerceAt(t *testing.T, m map[string]string, k string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, m[k])
	if err != nil {
		t.Fatalf("parse %s=%q: %v", k, m[k], err)
	}
	return v
}

func commerceI64(t *testing.T, m map[string]string, k string) int64 {
	t.Helper()
	v, err := strconv.ParseInt(m[k], 10, 64)
	if err != nil {
		t.Fatalf("parse %s=%q: %v", k, m[k], err)
	}
	return v
}

func TestAnnouncedPriceGoldenVectors(t *testing.T) {
	f, err := os.Open("../../../../../contracts/testdata/enterprise/commerce/announced-price-v1.psv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = '|'
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows[1:] {
		id, op, args, exp := row[0], row[1], commerceParseKV(row[2]), commerceParseKV(row[3])
		t.Run(id, func(t *testing.T) {
			switch op {
			case "DISPLAY_STATE":
				state := domain.AnnouncedDisplayState(domain.AnnouncedPriceState(args["state"]), commerceAt(t, args, "now"), commerceAt(t, args, "valid_from"), commerceAt(t, args, "valid_until"))
				if string(state) != exp["display_state"] {
					t.Fatalf("display state %q, want %q", state, exp["display_state"])
				}
			case "ANNOUNCE":
				price := domain.AnnouncedPrice{Product: args["product"], Unit: args["unit"], Conditions: args["conditions"], ValidFrom: commerceAt(t, args, "valid_from"), ValidUntil: commerceAt(t, args, "valid_until")}
				if _, ok := args["price_milli_brl"]; ok {
					price.PriceMilliBRL = commerceI64(t, args, "price_milli_brl")
				}
				err := price.Validate()
				if exp["status"] == "REJECTED" {
					if err == nil || domain.CommerceErrorCode(err) != exp["error"] {
						t.Fatalf("expected %q, got %v", exp["error"], err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
			case "PROMOTION_STATE":
				state := domain.PromotionDisplayState(domain.PromotionState(args["state"]), commerceAt(t, args, "now"), commerceAt(t, args, "valid_from"), commerceAt(t, args, "valid_until"))
				if string(state) != exp["display_state"] {
					t.Fatalf("display state %q, want %q", state, exp["display_state"])
				}
			case "PROMOTE":
				promo := domain.Promotion{Label: args["label"], Description: args["description"], Product: args["fuel_product"], ValidFrom: commerceAt(t, args, "valid_from"), ValidUntil: commerceAt(t, args, "valid_until")}
				err := promo.Validate()
				if exp["status"] == "REJECTED" {
					if err == nil || domain.CommerceErrorCode(err) != exp["error"] {
						t.Fatalf("expected %q, got %v", exp["error"], err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
			default:
				t.Fatalf("unknown operation %q", op)
			}
		})
	}
}
