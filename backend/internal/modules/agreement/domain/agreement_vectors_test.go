package domain_test

import (
	"encoding/csv"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/agreement/domain"
)

func parseKV(s string) map[string]string {
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

func i64(t *testing.T, m map[string]string, k string) int64 {
	t.Helper()
	v, err := strconv.ParseInt(m[k], 10, 64)
	if err != nil {
		t.Fatalf("parse %s=%q: %v", k, m[k], err)
	}
	return v
}

func at(t *testing.T, m map[string]string, k string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, m[k])
	if err != nil {
		t.Fatalf("parse %s=%q: %v", k, m[k], err)
	}
	return v
}

func vectors(t *testing.T, name string) [][]string {
	t.Helper()
	f, err := os.Open("../../../../../contracts/testdata/enterprise/agreement/" + name)
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
	return rows[1:]
}

func termsOf(t *testing.T, raw string) []domain.Term {
	t.Helper()
	out := []domain.Term{}
	for _, part := range strings.Split(raw, ",") {
		if part == "" {
			continue
		}
		left, price, ok := strings.Cut(part, ":")
		if !ok {
			t.Fatalf("malformed term %q", part)
		}
		product, unit, ok := strings.Cut(left, "/")
		if !ok {
			t.Fatalf("malformed term key %q", left)
		}
		out = append(out, domain.Term{Product: product, Unit: unit, PriceMilliBRL: i64(t, map[string]string{"p": price}, "p")})
	}
	return out
}

func rejected(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %q, got nil", want)
	}
	if code := domain.ErrorCode(err); code != want {
		t.Fatalf("expected %q, got %q (%v)", want, code, err)
	}
}

func TestAgreementLifecycleGoldenVectors(t *testing.T) {
	for _, row := range vectors(t, "agreement-lifecycle-v1.psv") {
		id, op, args, exp := row[0], row[1], parseKV(row[2]), parseKV(row[3])
		t.Run(id, func(t *testing.T) {
			switch op {
			case "PROPOSE":
				head, err := domain.Propose(at(t, args, "valid_from"), at(t, args, "valid_until"), termsOf(t, args["terms"]))
				if exp["status"] == "REJECTED" {
					rejected(t, err, exp["error"])
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if head.State() != domain.Proposed || head.Revision() != 1 || len(head.Terms()) != len(termsOf(t, args["terms"])) {
					t.Fatalf("unexpected proposal: %+v", head)
				}
			case "ACCEPT", "DECLINE", "SUSPEND", "END":
				head := domain.AgreementOf(domain.State(args["state"]), domain.WithRevision(1))
				if _, ok := args["valid_from"]; ok {
					head = domain.AgreementOf(domain.State(args["state"]), domain.WithWindow(at(t, args, "valid_from"), at(t, args, "valid_until")), domain.WithRevision(1))
				}
				var next domain.Agreement
				var err error
				switch op {
				case "ACCEPT":
					next, err = head.Accept(at(t, args, "now"))
				case "DECLINE":
					next, err = head.Decline(at(t, args, "now"))
				case "SUSPEND":
					next, err = head.Suspend(at(t, args, "now"))
				case "END":
					next, err = head.End(at(t, args, "now"))
				}
				if exp["status"] == "REJECTED" {
					rejected(t, err, exp["error"])
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if string(next.State()) != exp["state"] || next.Revision() != i64(t, exp, "revision") {
					t.Fatalf("unexpected transition: %+v vs %+v", next, exp)
				}
			case "USABLE":
				head := domain.AgreementOf(domain.State(args["state"]), domain.WithWindow(at(t, args, "valid_from"), at(t, args, "valid_until")))
				if head.Usable(at(t, args, "now")) != (exp["usable"] == "true") {
					t.Fatalf("unexpected usability at %s", args["now"])
				}
				if derived := string(domain.DerivedState(domain.State(args["state"]), at(t, args, "now"), at(t, args, "valid_from"), at(t, args, "valid_until"))); exp["derived_state"] != "" && derived != exp["derived_state"] {
					t.Fatalf("derived state %q, want %q", derived, exp["derived_state"])
				}
			default:
				t.Fatalf("unknown operation %q", op)
			}
		})
	}
}

func TestVoucherLimitsGoldenVectors(t *testing.T) {
	for _, row := range vectors(t, "voucher-limits-v1.psv") {
		id, op, args, exp := row[0], row[1], parseKV(row[2]), parseKV(row[3])
		t.Run(id, func(t *testing.T) {
			switch op {
			case "ISSUE":
				v, err := domain.Issue(args["product"], args["unit"], i64(t, args, "max_quantity_milli"), i64(t, args, "max_centavos"), at(t, args, "valid_until"), at(t, args, "now"))
				if exp["status"] == "REJECTED" {
					rejected(t, err, exp["error"])
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if v.State() != domain.Issued || v.MaxQuantityMilli() != i64(t, args, "max_quantity_milli") || v.MaxCentavos() != i64(t, args, "max_centavos") {
					t.Fatalf("unexpected issue: %+v", v)
				}
			case "CANCEL":
				v, err := domain.VoucherOf(domain.VoucherState(args["state"])).Cancel()
				if exp["status"] == "REJECTED" {
					rejected(t, err, exp["error"])
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if string(v.State()) != exp["state"] {
					t.Fatalf("unexpected cancel: %+v", v)
				}
			case "ELIGIBLE":
				v := domain.VoucherOf(domain.VoucherState(args["state"]), domain.VoucherValidUntil(at(t, args, "valid_until")))
				eligible, state := v.EligibleAt(at(t, args, "now"))
				if eligible != (exp["eligible"] == "true") || string(state) != exp["derived_state"] {
					t.Fatalf("unexpected eligibility: %v %s", eligible, state)
				}
			case "ATTEMPT":
				v, err := domain.VoucherOf(domain.VoucherState(args["state"]), domain.VoucherAttempts(int(i64(t, args, "failed_attempts")))).Attempt()
				if exp["status"] == "REJECTED" {
					rejected(t, err, exp["error"])
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if string(v.State()) != exp["state"] || v.FailedAttempts() != int(i64(t, exp, "failed_attempts")) {
					t.Fatalf("unexpected attempt: %+v vs %+v", v, exp)
				}
			default:
				t.Fatalf("unknown operation %q", op)
			}
		})
	}
}

func TestRedemptionIdempotencyGoldenVectors(t *testing.T) {
	for _, row := range vectors(t, "redemption-idempotency-v1.psv") {
		id, op, args, exp := row[0], row[1], parseKV(row[2]), parseKV(row[3])
		t.Run(id, func(t *testing.T) {
			switch op {
			case "COMPUTE":
				total := domain.TotalCentavos(i64(t, args, "quantity_milli"), i64(t, args, "price_milli_brl"))
				if total != i64(t, exp, "total_centavos") {
					t.Fatalf("total %d, want %d", total, i64(t, exp, "total_centavos"))
				}
			case "REDEEM":
				v := domain.VoucherOf(domain.VoucherState(args["state"]),
					domain.VoucherProduct(args["voucher_product"], args["voucher_unit"]),
					domain.VoucherLimits(i64(t, args, "max_quantity_milli"), i64(t, args, "max_centavos")),
					domain.VoucherValidUntil(at(t, args, "valid_until")))
				err := v.Consume(args["product"], args["unit"], i64(t, args, "quantity_milli"), i64(t, args, "total_centavos"), at(t, args, "now"))
				if exp["status"] == "REJECTED" {
					rejected(t, err, exp["error"])
					return
				}
				if err != nil {
					t.Fatal(err)
				}
			case "REPLAY":
				if exp["replayed"] != "true" {
					t.Fatal("replay vectors expect the stored receipt")
				}
			default:
				t.Fatalf("unknown operation %q", op)
			}
		})
	}
}
