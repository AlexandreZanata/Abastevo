//go:build integration

package http

import (
	"net/http"
	"testing"
)

func TestMEX19UnsupportedAndDuplicateStationFiltersRefuse(t *testing.T) {
	router, _ := freshRouter(t, true)
	for _, path := range []string{
		"/v1/quality/actions?station_id=" + httpStation,
		"/v1/quality/actions/aaaaaaaa-1111-4111-8111-000000000099?station_id=" + httpStation,
		"/v1/quality/sanctions?station_id=" + httpStation + "&station_id=" + httpStation,
		"/v1/quality/sanctions?station_id=",
		"/v1/quality/sanctions?status=a&status=b",
	} {
		if code, _, _ := get(t, router, path); code != http.StatusBadRequest {
			t.Errorf("filter returned %d instead of400: %s", code, path)
		}
	}
}
