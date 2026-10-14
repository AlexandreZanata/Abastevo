package http

import (
	"context"
	"encoding/json"
	"errors"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
	"github.com/go-chi/chi/v5"
	"net/http/httptest"
	"testing"
)

type noOfficialPrices struct{ fakePrices }

func (noOfficialPrices) Groups(context.Context, string, string) ([]application.PriceGroup, error) {
	return nil, nil
}
func TestCommunityOnlyStationGroupsUseOneBatchAndNeverBecomeANP(t *testing.T) {
	calls := 0
	h := Handler{Prices: noOfficialPrices{}, StationsExist: func(context.Context, string) (bool, error) { return true, nil }, CommunityGroups: func(_ context.Context, station, fuel string) ([]CommunityGroup, error) {
		calls++
		if station != priceStation || fuel != "ETHANOL" {
			t.Fatal("filter lost")
		}
		return []CommunityGroup{{Product: "ETHANOL", Unit: "L", Price: CommunityPrice{Availability: "AVAILABLE", AmountMilli: 4321, HasAmount: true, Confidence: "LOW", Freshness: "FRESH", AlgorithmVersion: "consensus-community-v2", ProjectionVersion: 1}}}, nil
	}}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/stations/"+priceStation+"/prices?fuel_product=ETHANOL", nil))
	var body struct {
		Items []struct {
			Official  any `json:"official"`
			Community struct {
				Source string `json:"source"`
				Amount int64  `json:"amount_milli_brl"`
			} `json:"community"`
		}
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body.Items) != 1 || body.Items[0].Official != nil || body.Items[0].Community.Source != "COMMUNITY" || body.Items[0].Community.Amount != 4321 || calls != 1 {
		t.Fatalf("response %d %s calls=%d", w.Code, w.Body.String(), calls)
	}
	h.CommunityGroups = func(context.Context, string, string) ([]CommunityGroup, error) {
		return nil, errors.New("synthetic outage")
	}
	r = chi.NewRouter()
	h.RegisterRoutes(r)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/stations/"+priceStation+"/prices", nil))
	if w.Code != 500 {
		t.Fatal("outage became empty success")
	}
}
