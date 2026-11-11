package domain_test

import (
	"bufio"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	value "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgvalue/domain"
)

func fields(t *testing.T, raw string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, pair := range strings.Split(raw, ";") {
		key, val, ok := strings.Cut(pair, "=")
		if !ok || key == "" {
			t.Fatal("invalid vector field")
		}
		if _, duplicate := out[key]; duplicate {
			t.Fatal("duplicate vector field")
		}
		out[key] = val
	}
	return out
}
func number(t *testing.T, raw string) int64 {
	t.Helper()
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
func optional(t *testing.T, raw string) *int64 {
	if raw == "?" {
		return nil
	}
	n := number(t, raw)
	return &n
}
func text(n *int64) string {
	if n == nil {
		return "?"
	}
	return strconv.FormatInt(*n, 10)
}
func execute(t *testing.T, op string, args map[string]string) (map[string]string, error) {
	t.Helper()
	switch op {
	case "PARSE_CENTAVOS", "PARSE_QUANTITY", "PARSE_PRICE":
		var n int64
		var err error
		switch op {
		case "PARSE_CENTAVOS":
			n, err = value.ParseCentavos(args["text"])
		case "PARSE_QUANTITY":
			n, err = value.ParseQuantityMilli(args["text"])
		case "PARSE_PRICE":
			n, err = value.ParseUnitPriceMilliBRL(args["text"])
		}
		return map[string]string{"value": strconv.FormatInt(n, 10)}, err
	case "RECONCILE":
		r, err := value.Reconcile(value.Record{
			PaidCentavos: optional(t, args["paid"]), QuantityMilli: optional(t, args["quantity"]),
			UnitPriceMilliBRL: optional(t, args["price"]), DiscountCentavos: optional(t, args["discount"]),
			Unit: args["unit"], SourcePriceText: args["source"],
		})
		return map[string]string{"paid": text(r.PaidCentavos), "gross": text(r.GrossCentavos),
			"expected": text(r.ExpectedCentavos), "difference": text(r.DifferenceCentavos), "derived": emptyUnknown(r.DerivedPriceMilliBRL),
			"warning": r.Warning, "unknown": r.Unknown, "unit": r.Unit, "source": r.SourcePriceText}, err
	case "ALLOCATE":
		var lines []value.Line
		if args["lines"] != "" {
			for _, raw := range strings.Split(args["lines"], ",") {
				pair := strings.Split(raw, ":")
				if len(pair) != 2 {
					t.Fatal("invalid allocation vector")
				}
				lines = append(lines, value.Line{Order: number(t, pair[0]), Weight: optional(t, pair[1])})
			}
		}
		r, err := value.Allocate(optional(t, args["source"]), lines)
		parts := "?"
		if r.Parts != nil {
			var values []string
			for _, part := range r.Parts {
				values = append(values, strconv.FormatInt(part.Order, 10)+":"+strconv.FormatInt(part.Centavos, 10))
			}
			parts = strings.Join(values, ",")
		}
		return map[string]string{"parts": parts, "unknown": r.Unknown}, err
	default:
		t.Fatal("unhandled shared operation", op)
	}
	return nil, nil
}
func emptyUnknown(s string) string {
	if s == "" {
		return "?"
	}
	return s
}
func TestSharedEnterpriseExactValueVectors(t *testing.T) {
	f, err := os.Open("../../../../../contracts/testdata/enterprise/exact-values-v1.psv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		t.Fatal("invalid shared header")
	}
	seen := map[string]bool{}
	for scanner.Scan() {
		row := strings.Split(scanner.Text(), "|")
		if len(row) != 4 || row[0] == "" || seen[row[0]] {
			t.Fatal("invalid/duplicate shared vector")
		}
		seen[row[0]] = true
		t.Run(row[0], func(t *testing.T) {
			want := fields(t, row[3])
			got, err := execute(t, row[1], fields(t, row[2]))
			if err != nil {
				if !errors.Is(err, value.ErrInvalid) {
					t.Fatal(err)
				}
				got = map[string]string{"error": "invalid-value"}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v; want %v", got, want)
			}
		})
	}
	if scanner.Err() != nil || len(seen) != 112 {
		t.Fatalf("shared rows=%d error=%v", len(seen), scanner.Err())
	}
}
