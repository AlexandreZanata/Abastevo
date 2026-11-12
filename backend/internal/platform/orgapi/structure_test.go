package orgapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func TestStructureRefusalsExposeOnlyRegisteredFieldsAndExactCounts(t *testing.T) {
	write := func(err error) (int, map[string]any) {
		t.Helper()
		w := httptest.NewRecorder()
		orgapi.WriteError(w, httptest.NewRequest(http.MethodPost, "/", nil), err)
		var result map[string]any
		if json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal("invalid safe error JSON")
		}
		return w.Code, result["error"].(map[string]any)
	}
	status, result := write(orgapi.UnitDuplicate("name"))
	if status != 422 || result["code"] != "VALIDATION_FAILED" {
		t.Fatal("unit duplicate not registered")
	}
	detail := result["details"].([]any)[0].(map[string]any)
	if detail["field"] != "name" || detail["code"] != "DUPLICATE" || len(detail) != 2 {
		t.Fatal("unsafe unit duplicate detail")
	}
	status, result = write(orgapi.StructureInUseCounts(orgapi.StructureCounts{Assets: 9223372036854775807, Employees: 2}))
	if status != 409 || result["code"] != "STRUCTURE_IN_USE" {
		t.Fatal("unit use refusal not registered")
	}
	details := result["details"].([]any)
	if len(details) != 4 || details[0].(map[string]any)["count"] != "9223372036854775807" || details[1].(map[string]any)["count"] != "2" || details[2].(map[string]any)["count"] != "0" {
		t.Fatal("usage counts truncated or unsafe")
	}
	for _, field := range []string{"", "identifier", "private name"} {
		if orgapi.UnitDuplicate(field) != orgapi.Unavailable {
			t.Fatal("unknown structure field accepted")
		}
	}
	for _, counts := range []orgapi.StructureCounts{{}, {Assets: -1}, {Employees: -1}, {Budgets: -1}, {Operations: -1}} {
		if orgapi.StructureInUseCounts(counts) != orgapi.Unavailable {
			t.Fatal("invalid usage counts accepted")
		}
	}
	if status, _ := write(orgapi.LastUnit); status != 409 {
		t.Fatal("last unit code missing")
	}
}
