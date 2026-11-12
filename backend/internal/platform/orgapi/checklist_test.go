package orgapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func TestIncompleteSetupOnlyCarriesCopiedChecklistKeys(t *testing.T) {
	fields := []string{"name", "terms"}
	failure := orgapi.IncompleteSetup(fields)
	fields[0] = "synthetic-private-detail"
	w := httptest.NewRecorder()
	orgapi.WriteError(w, httptest.NewRequest(http.MethodPost, "/", nil), failure)
	if w.Code != 409 || !strings.Contains(w.Body.String(), `"field":"name"`) || strings.Contains(w.Body.String(), "synthetic-private-detail") {
		t.Fatal("mutable/unbounded checklist error", w.Body.String())
	}
	for _, invalid := range [][]string{nil, {"name", "name"}, {"synthetic-private-detail"}, {"name", "time_zone", "active_unit", "active_owner", "terms", "name"}} {
		if orgapi.IncompleteSetup(invalid) != orgapi.Unavailable {
			t.Fatal("unregistered checklist details accepted")
		}
	}
}
