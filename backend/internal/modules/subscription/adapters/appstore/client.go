package appstore

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

// App Store Server API hosts recommended since 2026-05-05.
const (
	productionAuthority = "https://api.storekit.apple.com"
	sandboxAuthority    = "https://api.storekit-sandbox.apple.com"
	autoRenewable       = "Auto-Renewable Subscription"
	tokenLifetime       = 20 * time.Minute
	maxResponseBytes    = 256 << 10
)

// Config identifies the app and its App Store Connect API key.
type Config struct {
	// BundleID is the app identity, com.anpfuel.app.
	BundleID string
	// Environment is "production" or "test" (App Store "Production" or "Sandbox").
	Environment string
	// IssuerID and KeyID name the In-App Purchase key in App Store Connect.
	IssuerID, KeyID string
	// PrivateKeyPEM is the downloaded .p8 key (PKCS #8, P-256).
	PrivateKeyPEM []byte
	// Products is the server catalog of sellable product IDs.
	Products []string
	// SandboxFallback lets a production server verify a purchase that the
	// production API does not know in the sandbox, as App Review and
	// TestFlight purchases are (Apple: call production first, then sandbox
	// on TransactionIdNotFound).
	SandboxFallback bool
}

// Client implements application.Verifier for App Store subscriptions.
type Client struct {
	config   Config
	key      *ecdsa.PrivateKey
	products map[string]bool
	http     *http.Client
	signed   SignedDataVerifier
	Now      func() time.Time

	// Authorities overrides the API hosts in tests: production then sandbox.
	Authorities [2]string

	mu      sync.Mutex
	bearer  string
	expires time.Time
}

// New validates the configuration and parses the private key. A nil roots
// pool uses the pinned Apple Root CA - G3.
func New(config Config, client *http.Client, roots *x509.CertPool) (*Client, error) {
	if config.BundleID == "" || len(config.BundleID) > 150 || strings.ContainsAny(config.BundleID, "/\\?# ") ||
		(config.Environment != "test" && config.Environment != "production") ||
		!printable(config.IssuerID, 64) || !printable(config.KeyID, 32) || len(config.Products) == 0 {
		return nil, domain.ErrInvalid
	}
	products := map[string]bool{}
	for _, product := range config.Products {
		if product == "" || len(product) > 150 {
			return nil, domain.ErrInvalid
		}
		products[product] = true
	}
	block, _ := pem.Decode(config.PrivateKeyPEM)
	if block == nil {
		return nil, domain.ErrInvalid
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, domain.ErrInvalid
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || key.Curve.Params().BitSize != 256 {
		return nil, domain.ErrInvalid
	}
	if roots == nil {
		if roots, err = AppleRoots(); err != nil {
			return nil, domain.ErrUnavailable
		}
	}
	local := http.Client{Timeout: domain.ProviderTimeout}
	if client != nil {
		local = *client
		if local.Timeout == 0 || local.Timeout > domain.ProviderTimeout {
			local.Timeout = domain.ProviderTimeout
		}
	}
	local.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return errors.New("appstore: redirects refused") }
	c := &Client{config: config, key: key, products: products, http: &local, Now: time.Now,
		Authorities: [2]string{productionAuthority, sandboxAuthority}}
	c.signed = SignedDataVerifier{Roots: roots, Now: func() time.Time { return c.now() }}
	return c, nil
}

func printable(value string, max int) bool {
	if value == "" || len(value) > max {
		return false
	}
	for _, c := range value {
		if c <= 32 || c > 126 {
			return false
		}
	}
	return true
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// DecodeNotification verifies and decodes an App Store Server Notification V2.
func (c *Client) DecodeNotification(signedPayload string) (Notification, error) {
	return c.signed.Notification(signedPayload)
}

// token returns a cached ES256 API token (Apple allows at most 60 minutes).
func (c *Client) token() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if c.bearer != "" && now.Add(time.Minute).Before(c.expires) {
		return c.bearer, nil
	}
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": c.config.KeyID, "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"iss": c.config.IssuerID, "iat": now.Unix(), "exp": now.Add(tokenLifetime).Unix(),
		"aud": "appstoreconnect-v1", "bid": c.config.BundleID,
	})
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(rand.Reader, c.key, digest[:])
	if err != nil {
		return "", domain.ErrUnavailable
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	c.bearer = input + "." + base64.RawURLEncoding.EncodeToString(signature)
	c.expires = now.Add(tokenLifetime)
	return c.bearer, nil
}

type statusResponse struct {
	Environment string `json:"environment"`
	BundleID    string `json:"bundleId"`
	Data        []struct {
		LastTransactions []struct {
			OriginalTransactionID string `json:"originalTransactionId"`
			Status                int    `json:"status"`
			SignedTransactionInfo string `json:"signedTransactionInfo"`
			SignedRenewalInfo     string `json:"signedRenewalInfo"`
		} `json:"lastTransactions"`
	} `json:"data"`
}

// errTransactionUnknown is the API's TransactionIdNotFound in one environment.
var errTransactionUnknown = errors.New("appstore: transaction unknown in environment")

func (c *Client) statuses(ctx context.Context, authority, original string) (statusResponse, error) {
	bearer, err := c.token()
	if err != nil {
		return statusResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authority+"/inApps/v1/subscriptions/"+url.PathEscape(original), nil)
	if err != nil {
		return statusResponse{}, domain.ErrInvalid
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return statusResponse{}, domain.ErrUnavailable
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil || len(raw) > maxResponseBytes {
		return statusResponse{}, domain.ErrUnavailable
	}
	switch {
	case res.StatusCode == http.StatusNotFound:
		return statusResponse{}, errTransactionUnknown
	case res.StatusCode == http.StatusBadRequest:
		return statusResponse{}, domain.ErrDenied
	case res.StatusCode < 200 || res.StatusCode >= 300:
		return statusResponse{}, domain.ErrUnavailable
	}
	var body statusResponse
	if json.Unmarshal(raw, &body) != nil {
		return statusResponse{}, domain.ErrUnavailable
	}
	return body, nil
}

// appleEnvironment names the App Store environment for a server environment.
func appleEnvironment(environment string) string {
	if environment == "production" {
		return "Production"
	}
	return "Sandbox"
}

// Verify implements application.Verifier. Token is the subscription's
// originalTransactionId; ExpectedAccount is the appAccountToken binding.
func (c *Client) Verify(ctx context.Context, r app.ProviderRequest) (app.ProviderPurchase, error) {
	if c == nil || r.Environment != c.config.Environment || r.PackageName != c.config.BundleID || r.ExpectedAccount == "" {
		return app.ProviderPurchase{}, domain.ErrDenied
	}
	if _, err := strconv.ParseUint(r.Token, 10, 64); err != nil || len(r.Token) > 20 {
		return app.ProviderPurchase{}, domain.ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, domain.ProviderTimeout)
	defer cancel()
	type attempt struct{ authority, environment string }
	attempts := []attempt{{c.Authorities[1], "Sandbox"}}
	if c.config.Environment == "production" {
		attempts = []attempt{{c.Authorities[0], "Production"}}
		if c.config.SandboxFallback {
			attempts = append(attempts, attempt{c.Authorities[1], "Sandbox"})
		}
	}
	for _, a := range attempts {
		body, err := c.statuses(ctx, a.authority, r.Token)
		if errors.Is(err, errTransactionUnknown) {
			continue
		}
		if err != nil {
			return app.ProviderPurchase{}, err
		}
		return c.purchase(body, a.environment, r)
	}
	return app.ProviderPurchase{}, domain.ErrNotFound
}

func (c *Client) purchase(body statusResponse, environment string, r app.ProviderRequest) (app.ProviderPurchase, error) {
	if body.Environment != environment || body.BundleID != c.config.BundleID {
		return app.ProviderPurchase{}, domain.ErrDenied
	}
	for _, group := range body.Data {
		for _, last := range group.LastTransactions {
			if last.OriginalTransactionID != r.Token {
				continue
			}
			tx, err := c.signed.Transaction(last.SignedTransactionInfo)
			if err != nil {
				return app.ProviderPurchase{}, domain.ErrDenied
			}
			if tx.OriginalTransactionID != r.Token || tx.BundleID != c.config.BundleID || tx.Environment != environment ||
				tx.Type != autoRenewable || !c.products[tx.ProductID] || tx.ExpiresDate <= 0 {
				return app.ProviderPurchase{}, domain.ErrDenied
			}
			// The binding is a UUID chosen by the server; Apple may return it
			// in either letter case.
			if subtle.ConstantTimeCompare([]byte(strings.ToLower(tx.AppAccountToken)), []byte(r.ExpectedAccount)) != 1 {
				return app.ProviderPurchase{}, domain.ErrDenied
			}
			autoRenewOn := false
			var graceEnds time.Time
			if last.SignedRenewalInfo != "" {
				renewal, err := c.signed.RenewalInfo(last.SignedRenewalInfo)
				if err != nil || renewal.OriginalTransactionID != r.Token || renewal.Environment != environment {
					return app.ProviderPurchase{}, domain.ErrDenied
				}
				autoRenewOn = renewal.AutoRenewStatus == 1
				if renewal.GracePeriodExpiresDate > 0 {
					graceEnds = time.UnixMilli(renewal.GracePeriodExpiresDate).UTC()
				}
			}
			state := domain.AppStoreState(last.Status, autoRenewOn, tx.RevocationDate > 0)
			until := domain.AppStoreValidUntil(state, time.UnixMilli(tx.ExpiresDate).UTC(), graceEnds)
			return app.ProviderPurchase{Snapshot: domain.Snapshot{
				Provider: domain.ProviderAppStore, State: state, VerifiedAt: c.now().UTC(), ValidUntil: until,
				ProductID: tx.ProductID, BasePlanID: tx.ProductID, Acknowledged: true,
			}}, nil
		}
	}
	return app.ProviderPurchase{}, domain.ErrNotFound
}

// NotificationTarget validates a decoded notification for this app and
// returns the bound originalTransactionId, or "" for a test notification.
func (c *Client) NotificationTarget(n Notification) (string, error) {
	if n.Version != "2.0" || !validUUID(n.NotificationUUID) {
		return "", domain.ErrInvalid
	}
	if n.NotificationType == "TEST" {
		return "", nil
	}
	if n.Data == nil || n.Data.BundleID != c.config.BundleID || n.Data.SignedTransactionInfo == "" {
		return "", domain.ErrInvalid
	}
	expected := appleEnvironment(c.config.Environment)
	if n.Data.Environment != expected && !(c.config.SandboxFallback && n.Data.Environment == "Sandbox") {
		return "", domain.ErrDenied
	}
	tx, err := c.signed.Transaction(n.Data.SignedTransactionInfo)
	if err != nil || tx.BundleID != c.config.BundleID || tx.Environment != n.Data.Environment {
		return "", domain.ErrInvalid
	}
	if _, err := strconv.ParseUint(tx.OriginalTransactionID, 10, 64); err != nil {
		return "", domain.ErrInvalid
	}
	return tx.OriginalTransactionID, nil
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, ch := range value {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if ch != '-' {
				return false
			}
		case ('0' <= ch && ch <= '9') || ('a' <= ch && ch <= 'f') || ('A' <= ch && ch <= 'F'):
		default:
			return false
		}
	}
	return true
}
