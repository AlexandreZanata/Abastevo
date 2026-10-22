package charging

import (
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The same reviewed rows execute in Kotlin/JVM and Rust. These are synthetic
// contract vectors, not provider billing terms or customer results.
const (
	columnTariffCondition   = 1
	columnTariffQualifier   = 2
	columnScenarioCondition = 3
	columnScenarioQualifier = 4
	columnCurrency          = 5
	columnTaxBasis          = 6
	columnMandatoryComplete = 7
	columnRounding          = 8
	columnMinimum           = 9
	columnMaximum           = 10
	columnValidFrom         = 11
	columnValidUntil        = 12
	columnWindowStart       = 13
	columnWindowEnd         = 14
	columnEnergyWh          = 15
	columnChargingSeconds   = 16
	columnIdleSeconds       = 17
	columnSessionCount      = 18
	columnParkingSeconds    = 19
	columnComponentList     = 20
	columnExpected          = 21
)

func TestTariffScenarioVectors(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "..", "contracts", "testdata", "ev", "tariff-v1.csv")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	const columns = 22
	if len(rows) != 54 || len(rows[0]) != columns {
		t.Fatalf("unexpected fixture shape: %d rows, %d columns", len(rows), len(rows[0]))
	}
	seen := make(map[string]bool, len(rows)-1)
	for _, row := range rows[1:] {
		row := row
		if row[0] == "" || seen[row[0]] {
			t.Fatalf("duplicate/empty vector ID %q", row[0])
		}
		seen[row[0]] = true
		t.Run(row[0], func(t *testing.T) {
			tariff, scenario := decodeVector(t, row)
			estimate, err := Calculate(tariff, scenario)
			assertExpected(t, row[columnExpected], estimate, err)
		})
	}
}

func assertExpected(t *testing.T, expected string, estimate Estimate, err error) {
	t.Helper()
	switch {
	case strings.HasPrefix(expected, "ERROR:"):
		code := strings.TrimPrefix(expected, "ERROR:")
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Code != code {
			t.Fatalf("expected error %q, got %v %+v", code, err, estimate)
		}
	case strings.HasPrefix(expected, "UNKNOWN:"):
		reason := strings.TrimPrefix(expected, "UNKNOWN:")
		if err != nil || estimate.State != "UNKNOWN" || estimate.Reason != reason ||
			estimate.Value != nil || estimate.Method != "" || estimate.Unit != "" {
			t.Fatalf("expected UNKNOWN %q, got %v %+v", reason, err, estimate)
		}
	case strings.HasPrefix(expected, "KNOWN:"):
		cents := strings.TrimPrefix(expected, "KNOWN:")
		if err != nil || estimate.State != "KNOWN" || estimate.Method != "TARIFF_SCENARIO" ||
			estimate.Unit != "CENTAVOS" || estimate.Reason != "" || estimate.Value == nil ||
			estimate.Value.Numerator != cents || estimate.Value.Denominator != "1" {
			t.Fatalf("expected KNOWN %q centavos, got %v %+v", cents, err, estimate)
		}
	default:
		t.Fatalf("unknown expectation %q", expected)
	}
}

func decodeVector(t *testing.T, row []string) (Tariff, Scenario) {
	t.Helper()
	tariff := Tariff{
		Version:                     TariffVersion,
		ID:                          "11111111-1111-4111-8111-111111111111",
		Revision:                    "synthetic-tariff-v1",
		TargetKind:                  "CONNECTOR",
		TargetID:                    "22222222-2222-4222-8222-222222222222",
		Currency:                    row[columnCurrency],
		Condition:                   row[columnTariffCondition],
		QualifierID:                 nullableText(row[columnTariffQualifier]),
		Timezone:                    "America/Sao_Paulo",
		ValidFrom:                   parseInstant(t, row[columnValidFrom]),
		ValidUntil:                  optionalInstant(t, row[columnValidUntil]),
		TaxBasis:                    row[columnTaxBasis],
		MandatoryComponentsComplete: row[columnMandatoryComplete] == "1",
		Components:                  decodeComponents(t, row[columnComponentList]),
		MinimumCentavos:             optionalInt(t, row[columnMinimum]),
		MaximumCentavos:             optionalInt(t, row[columnMaximum]),
		BillingRounding:             row[columnRounding],
		Provenance:                  "DATED_COMMUNITY_REPORT",
	}
	scenario := Scenario{
		Condition:       row[columnScenarioCondition],
		QualifierID:     nullableText(row[columnScenarioQualifier]),
		WindowStart:     parseInstant(t, row[columnWindowStart]),
		WindowEnd:       parseInstant(t, row[columnWindowEnd]),
		EnergyWh:        optionalInt(t, row[columnEnergyWh]),
		ChargingSeconds: optionalInt(t, row[columnChargingSeconds]),
		IdleSeconds:     optionalInt(t, row[columnIdleSeconds]),
		SessionCount:    optionalInt(t, row[columnSessionCount]),
		ParkingSeconds:  optionalInt(t, row[columnParkingSeconds]),
	}
	return tariff, scenario
}

func nullableText(value string) *string {
	if value == "-" || value == "" {
		return nil
	}
	return &value
}

func optionalInt(t *testing.T, value string) *int64 {
	t.Helper()
	if value == "" || value == "-" {
		return nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		t.Fatalf("invalid integer %q: %v", value, err)
	}
	return &parsed
}

func optionalInstant(t *testing.T, value string) *time.Time {
	t.Helper()
	if value == "-" || value == "" {
		return nil
	}
	parsed := parseInstant(t, value)
	return &parsed
}

func parseInstant(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("invalid instant %q: %v", value, err)
	}
	return parsed
}

func decodeComponents(t *testing.T, value string) []TariffComponent {
	t.Helper()
	if value == "-" || value == "" {
		return nil
	}
	parts := strings.Split(value, "|")
	components := make([]TariffComponent, 0, len(parts))
	for _, part := range parts {
		fields := strings.Split(part, ":")
		if len(fields) != 4 {
			t.Fatalf("invalid component %q", part)
		}
		rate := strings.Split(fields[1], "/")
		if len(rate) != 2 {
			t.Fatalf("invalid rate %q", fields[1])
		}
		step, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			t.Fatalf("invalid step %q", fields[2])
		}
		grace, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			t.Fatalf("invalid grace %q", fields[3])
		}
		components = append(components, TariffComponent{
			Dimension:    fields[0],
			RateCentavos: Rational{Numerator: rate[0], Denominator: rate[1]},
			BillingStep:  step,
			GraceSeconds: grace,
		})
	}
	return components
}
