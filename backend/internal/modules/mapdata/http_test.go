package mapdata

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerRejectsInvalidQueriesBeforeStorage(t *testing.T) {
	h := Handler{}
	cases := []string{
		"/v1/map/assertions?min_lat=-16&min_lon=-48&max_lat=-15&max_lon=-47",
		"/v1/map/assertions?min_lat=-16&min_lon=-48&max_lat=-15&max_lon=-47&source=OTHER",
		"/v1/map/assertions?min_lat=-16&min_lon=-48&max_lat=-15&max_lon=-47&source=ANP&product=bad-product!",
		"/v1/map/assertions?min_lat=-20&min_lon=-60&max_lat=5&max_lon=-30&source=ANP",
		"/v1/map/assertions?min_lat=-15&min_lon=-48&max_lat=-16&max_lon=-47&source=ANP",
		"/v1/map/assertions?min_lat=NaN&min_lon=-48&max_lat=-15&max_lon=-47&source=ANP",
		"/v1/map/assertions?min_lat=-16&min_lon=Inf&max_lat=-15&max_lon=-47&source=ANP",
	}
	for _, target := range cases {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		h.assertions(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d body %s", target, rec.Code, rec.Body.String())
		}
	}
}
