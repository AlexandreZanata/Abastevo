package orgapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func TestEmployeeRefusalsExposeOnlyRegistrationFieldOrReviewCode(t *testing.T) {
	for _, v := range []struct {
		err    error
		status int
		code   string
		detail bool
	}{{orgapi.EmployeeRegistrationDuplicate(), 422, "VALIDATION_FAILED", true}, {orgapi.DuplicateReviewRequired, 409, "DUPLICATE_REVIEW_REQUIRED", false}} {
		w := httptest.NewRecorder()
		orgapi.WriteError(w, httptest.NewRequest(http.MethodPost, "/", nil), v.err)
		var body struct {
			Error struct {
				Code    string           `json:"code"`
				Details []map[string]any `json:"details"`
			} `json:"error"`
		}
		if w.Code != v.status || json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Error.Code != v.code {
			t.Fatal("employee refusal contract")
		}
		if v.detail {
			if len(body.Error.Details) != 1 || len(body.Error.Details[0]) != 2 || body.Error.Details[0]["field"] != "registration_number" || body.Error.Details[0]["code"] != "DUPLICATE" {
				t.Fatal("unbounded employee detail")
			}
		} else if len(body.Error.Details) != 0 {
			t.Fatal("review refusal disclosed candidates")
		}
	}
}
