package main

import (
	"testing"
	"time"
)

func TestValidateFactAcceptsExactSurveyRow(t *testing.T) {
	start := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	row, reason := validateFact(priceFact{
		SchemaVersion: factSchema, CNPJ: "01122233300000",
		Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: 5999,
		ObservedAt: "2026-10-15", PeriodStart: "2026-10-11", PeriodEnd: "2026-10-17",
		SourceRow: 11,
	}, start, end)
	if reason != "" {
		t.Fatalf("reason = %q", reason)
	}
	if row.AmountMilli != 5999 || row.Product != "GASOLINE_REGULAR" || row.Unit != "L" {
		t.Fatalf("row = %+v", row)
	}
	if row.CollectedOn.Format("2006-01-02") != "2026-10-15" {
		t.Fatalf("date = %s", row.CollectedOn)
	}
}

func TestValidateFactTranslatesHydratedEthanolToOfficialVocabulary(t *testing.T) {
	start := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	row, reason := validateFact(priceFact{
		SchemaVersion: factSchema, CNPJ: "01122233300000",
		Product: "ETHANOL_HYDRATED", Unit: "L", AmountMilli: 4299,
		ObservedAt: "2026-10-15", PeriodStart: "2026-10-11", PeriodEnd: "2026-10-17",
	}, start, end)
	if reason != "" || row.Product != "ETHANOL" || row.Unit != "L" {
		t.Fatalf("reason=%q row=%+v", reason, row)
	}
}

func TestValidateFactRefusesIncompatibleUnitPeriodAndRange(t *testing.T) {
	start := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	base := priceFact{
		SchemaVersion: factSchema, CNPJ: "01122233300000",
		Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: 5999,
		ObservedAt: "2026-10-15", PeriodStart: "2026-10-11", PeriodEnd: "2026-10-17",
	}
	cases := []struct {
		name   string
		mutate func(*priceFact)
		reason string
	}{
		{"unit", func(f *priceFact) { f.Unit = "M3" }, "product-unit-mismatch"},
		{"product", func(f *priceFact) { f.Product = "AVIATION" }, "official-product-unsupported"},
		{"premium", func(f *priceFact) { f.Product = "GASOLINE_PREMIUM_GRADE" }, "official-product-unsupported"},
		{"range", func(f *priceFact) { f.AmountMilli = 0 }, "price-out-of-range"},
		{"date", func(f *priceFact) { f.ObservedAt = "2026-10-18" }, "outside-period"},
		{"period", func(f *priceFact) { f.PeriodEnd = "2026-10-18" }, "period-mismatch"},
	}
	for _, testCase := range cases {
		fact := base
		testCase.mutate(&fact)
		if _, reason := validateFact(fact, start, end); reason != testCase.reason {
			t.Errorf("%s: reason = %q, want %q", testCase.name, reason, testCase.reason)
		}
	}
}
