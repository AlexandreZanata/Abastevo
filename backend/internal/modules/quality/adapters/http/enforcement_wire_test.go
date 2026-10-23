package http

import (
	"encoding/json"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// Shared with the Kotlin HTTP client: complete source wire shape, including
// legacy nullable fields which remain present in the accepted API contract.
func TestEnforcementPublicWireFixture(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../../../../contracts/testdata/quality/enforcement-source-record-valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]any
	if err = json.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	got := wireEnforcement(read.EnforcementEvent{
		ID: "eeeeeeee-6666-4666-8666-000000000002", NativeKey: "enforcement-source:abc", EvidenceRef: "synthetic.xlsx#row:2", SourceProfile: "anp-enforcement-xlsx-2019-v1", SubjectKind: "company-shaped", UF: "DF", Municipality: "BRASILIA", Segment: "Revenda de Combustíveis", DocumentDay: "2026-10-04", DocumentNumber: "712508", Procedure: "Inspection", Result: "Resultado",
	})
	wire, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err = json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, expected) {
		t.Fatal("public source wire differs from cross-client fixture")
	}
}
