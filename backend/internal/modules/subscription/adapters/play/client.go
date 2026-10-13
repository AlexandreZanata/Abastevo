// Package play implements restricted server-side Developer API operations.
// No credential, catalog, fake provider or live activation is wired by this package.
package play

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const authority = "https://androidpublisher.googleapis.com"

type Config struct {
	PackageName, Environment string
	Plans                    map[string][]string
}
type AccessToken func(context.Context) (string, error)
type Client struct {
	config Config
	http   *http.Client
	token  AccessToken
	Now    func() time.Time
}

func New(config Config, client *http.Client, token AccessToken) (*Client, error) {
	if config.PackageName == "" || len(config.PackageName) > 150 || strings.ContainsAny(config.PackageName, "/\\?# ") || token == nil || (config.Environment != "test" && config.Environment != "production") || len(config.Plans) == 0 {
		return nil, domain.ErrInvalid
	}
	plans := map[string][]string{}
	for product, bases := range config.Plans {
		if product == "" || len(product) > 150 || len(bases) == 0 {
			return nil, domain.ErrInvalid
		}
		for _, base := range bases {
			if base == "" || len(base) > 150 {
				return nil, domain.ErrInvalid
			}
		}
		plans[product] = append([]string{}, bases...)
	}
	config.Plans = plans
	local := http.Client{Timeout: domain.ProviderTimeout}
	if client != nil {
		local = *client
		if local.Timeout == 0 || local.Timeout > domain.ProviderTimeout {
			local.Timeout = domain.ProviderTimeout
		}
	}
	local.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return errors.New("play: redirects refused") }
	return &Client{config: config, http: &local, token: token, Now: time.Now}, nil
}

type purchase struct {
	State   string    `json:"subscriptionState"`
	Ack     string    `json:"acknowledgementState"`
	Linked  string    `json:"linkedPurchaseToken"`
	Test    *struct{} `json:"testPurchase"`
	Account struct {
		Obfuscated string `json:"obfuscatedExternalAccountId"`
	} `json:"externalAccountIdentifiers"`
	Items []struct {
		Product string `json:"productId"`
		Expiry  string `json:"expiryTime"`
		Offer   struct {
			BasePlan string `json:"basePlanId"`
		} `json:"offerDetails"`
	} `json:"lineItems"`
}

func (c *Client) request(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	if c == nil || c.http == nil || c.token == nil {
		return nil, domain.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, domain.ProviderTimeout)
	defer cancel()
	bearer, err := c.token(ctx)
	if err != nil || !domain.ValidToken(bearer) {
		return nil, domain.ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, method, authority+path, body)
	if err != nil {
		return nil, domain.ErrInvalid
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode == 404 {
		return nil, domain.ErrNotFound
	}
	if res.StatusCode == 400 {
		return nil, domain.ErrDenied
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, domain.ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 65537))
	if err != nil || len(raw) > 65536 {
		return nil, domain.ErrUnavailable
	}
	return raw, nil
}
func (c *Client) Verify(ctx context.Context, r app.ProviderRequest) (app.ProviderPurchase, error) {
	if c == nil || r.Environment != c.config.Environment || r.PackageName != c.config.PackageName || r.ExpectedAccount == "" || !domain.ValidToken(r.Token) {
		return app.ProviderPurchase{}, domain.ErrDenied
	}
	path := "/androidpublisher/v3/applications/" + url.PathEscape(c.config.PackageName) + "/purchases/subscriptionsv2/tokens/" + url.PathEscape(r.Token)
	raw, err := c.request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return app.ProviderPurchase{}, err
	}
	var p purchase
	if err = json.Unmarshal(raw, &p); err != nil {
		return app.ProviderPurchase{}, domain.ErrUnavailable
	}
	if subtle.ConstantTimeCompare([]byte(p.Account.Obfuscated), []byte(r.ExpectedAccount)) != 1 || (p.Test != nil) != (c.config.Environment == "test") || len(p.Items) != 1 {
		return app.ProviderPurchase{}, domain.ErrDenied
	}
	item := p.Items[0]
	allowed := false
	for _, base := range c.config.Plans[item.Product] {
		if item.Offer.BasePlan == base {
			allowed = true
		}
	}
	if !allowed {
		return app.ProviderPurchase{}, domain.ErrDenied
	}
	states := map[string]domain.State{
		"SUBSCRIPTION_STATE_PENDING": domain.Pending, "SUBSCRIPTION_STATE_ACTIVE": domain.Active,
		"SUBSCRIPTION_STATE_CANCELED": domain.Canceled, "SUBSCRIPTION_STATE_IN_GRACE_PERIOD": domain.Grace,
		"SUBSCRIPTION_STATE_ON_HOLD": domain.Hold, "SUBSCRIPTION_STATE_PAUSED": domain.Paused,
		"SUBSCRIPTION_STATE_EXPIRED": domain.Expired, "SUBSCRIPTION_STATE_PENDING_PURCHASE_CANCELED": domain.Expired,
	}
	state, ok := states[p.State]
	if !ok {
		state = domain.Unknown
	}
	var until time.Time
	if item.Expiry != "" {
		until, err = time.Parse(time.RFC3339Nano, item.Expiry)
		if err != nil {
			return app.ProviderPurchase{}, domain.ErrDenied
		}
	}
	if p.Ack != "ACKNOWLEDGEMENT_STATE_PENDING" && p.Ack != "ACKNOWLEDGEMENT_STATE_ACKNOWLEDGED" {
		return app.ProviderPurchase{}, domain.ErrDenied
	}
	if p.Linked != "" && (!domain.ValidToken(p.Linked) || p.Linked == r.Token) {
		return app.ProviderPurchase{}, domain.ErrDenied
	}
	return app.ProviderPurchase{Snapshot: domain.Snapshot{State: state, VerifiedAt: c.Now().UTC(), ValidUntil: until, ProductID: item.Product, BasePlanID: item.Offer.BasePlan, Acknowledged: p.Ack == "ACKNOWLEDGEMENT_STATE_ACKNOWLEDGED"}, LinkedToken: p.Linked}, nil
}
