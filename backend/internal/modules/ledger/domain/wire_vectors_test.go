package domain

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

func TestFinancialWireVectors(t *testing.T) {
	f, err := os.Open("../../../../../contracts/testdata/premium/financial-wire-v1.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 23 || !reflect.DeepEqual(rows[0], []string{"id", "fuel", "unit", "quantity_milli", "total_centavos", "accepted"}) {
		t.Fatal("unexpected wire fixture schema/count")
	}
	seen := map[string]bool{}
	for _, row := range rows[1:] {
		if row[0] == "" || seen[row[0]] {
			t.Fatal("duplicate/empty vector ID")
		}
		seen[row[0]] = true
		t.Run(row[0], func(t *testing.T) {
			q, err := strconv.ParseInt(row[3], 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			c, err := strconv.ParseInt(row[4], 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			input := baseRefuel(t)
			input.Fuel = kernel.Product(row[1])
			input.Unit = kernel.Unit(row[2])
			input.QuantityMilli = q
			input.TotalCentavos = c
			// Exercise the actual JSON value type, not a float-valued intermediary.
			raw, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			var decoded Refuel
			if err = json.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
			got, err := NormalizeRefuel(decoded)
			if (err == nil) != (row[5] == "1") {
				t.Fatalf("acceptance mismatch: %v", err)
			}
			if err == nil && !reflect.DeepEqual(got, input) {
				t.Fatal("wire normalization rewrote an exact raw value")
			}
		})
	}
}
