package charging

import (
	"encoding/csv"
	"os"
	"strings"
	"testing"
)

func TestAuditFrozenTariffIdentityAndUnicodeLength(t *testing.T) {
	file, err := os.Open("../../../../../contracts/testdata/ev/tariff-v1.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"id", "target"} {
		t.Run(field, func(t *testing.T) {
			tariff, scenario := decodeVector(t, rows[1])
			if field == "id" {
				tariff.ID = "not-a-uuid"
			} else {
				tariff.TargetID = "not-a-uuid"
			}
			estimate, err := Calculate(tariff, scenario)
			assertExpected(t, "ERROR:invalid-tariff", estimate, err)
		})
	}
	tariff, scenario := decodeVector(t, rows[1])
	tariff.Revision = strings.Repeat("😀", 128)
	estimate, err := Calculate(tariff, scenario)
	assertExpected(t, "KNOWN:3200", estimate, err)
}
