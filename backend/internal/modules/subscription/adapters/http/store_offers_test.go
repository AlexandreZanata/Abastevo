package http

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
)

type offerProbe struct {
	apiProbe
	offers []app.Offer
}

func (o *offerProbe) Offers(p app.Principal) []app.Offer {
	if p.AccountID != owner {
		return nil
	}
	return o.offers
}

func TestStatusListsStoreOffersForTheOwner(t *testing.T) {
	probe := &offerProbe{offers: []app.Offer{
		{Provider: "play", Products: []app.Product{{ProductID: "abastevo_premium", PlanIDs: []string{"monthly"}}}, AccountBinding: strings.Repeat("a", 64), ManagementURL: "https://play.google.com/store/account/subscriptions?package=com.anpfuel.app"},
		{Provider: "app_store", Products: []app.Product{{ProductID: "com.anpfuel.app.premium.monthly", PlanIDs: []string{}}}, AccountBinding: "6f1c2b8e-6d2a-8c3e-9a41-2b7d5e0c1f34", ManagementURL: "https://apps.apple.com/account/subscriptions"},
	}}
	h := Handler{Service: probe, Sessions: handler(&probe.apiProbe).Sessions}
	w := request(h, "/v1/premium/status", sessionBody())
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	var response struct {
		Offers []app.Offer `json:"store_offers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || len(response.Offers) != 2 ||
		response.Offers[1].Provider != "app_store" || response.Offers[1].Products[0].ProductID != "com.anpfuel.app.premium.monthly" {
		t.Fatalf("offers %+v %v", response.Offers, err)
	}
}

func TestStatusWithoutStoresListsNoOffers(t *testing.T) {
	w := request(handler(&apiProbe{}), "/v1/premium/status", sessionBody())
	if !strings.Contains(w.Body.String(), `"store_offers":[]`) {
		t.Fatalf("unconfigured stores must list an empty array: %s", w.Body.String())
	}
}

func TestPurchaseCarriesProviderAndRejectsUnknownStore(t *testing.T) {
	signed := func(*http.Request, string) (string, string, error) { return foreignOwner, operation, nil }
	base := strings.TrimSuffix(sessionBody(), "}") + `,"operation_id":"` + operation + `","purchase_token":"2000000123456789"`
	s := &apiProbe{}
	h := handler(s)
	h.SignedOwner = signed
	if w := request(h, "/v1/premium/purchases/verify", base+`,"provider":"app_store"}`); w.Code != 200 || s.command.Provider != "app_store" {
		t.Fatalf("app store command: %d %+v", w.Code, s.command)
	}
	if w := request(h, "/v1/premium/purchases/verify", base+`}`); w.Code != 200 || s.command.Provider != "" {
		t.Fatalf("legacy command must default to Play: %d %+v", w.Code, s.command)
	}
	before := s.calls
	if w := request(h, "/v1/premium/purchases/verify", base+`,"provider":"web"}`); w.Code != 400 || s.calls != before {
		t.Fatalf("web provider reached admission: %d", w.Code)
	}
}
