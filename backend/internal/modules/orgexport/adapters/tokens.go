package adapters

import (
	"encoding/json"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// Tokens reuses the existing HMAC envelope with its own binding domain and a
// strict five-minute inner deadline. The envelope's longer cursor TTL never
// extends a download. No storage URL or credential appears in a capability.
type Tokens struct {
	origin string
	secret []byte
}

var _ app.Tokens = (*Tokens)(nil)

func (Tokens) String() string       { return "orgexport: token handle redacted" }
func (Tokens) GoString() string     { return "orgexport: token handle redacted" }
func (Tokens) LogValue() slog.Value { return slog.StringValue("orgexport: token handle redacted") }

// NewTokens requires the canonical HTTPS API origin and a 32-byte secret.
func NewTokens(origin string, secret []byte) (*Tokens, error) {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" ||
		parsed.Fragment != "" || parsed.ForceQuery || strings.ToLower(parsed.Host) != parsed.Host || len(secret) < 32 {
		return nil, domain.ErrInvalid
	}
	return &Tokens{origin: origin, secret: append([]byte(nil), secret...)}, nil
}

func (tokens *Tokens) binding(scope kernel.OrgScope, requester app.Requester, object domain.Object) (string, error) {
	if tokens == nil || len(tokens.secret) < 32 || object.Validate(scope) != nil || requester.Validate() != nil {
		return "", app.ErrNotFound
	}
	raw, _ := json.Marshal([]any{"orgexport.access.v1", tokens.origin, object.OrgID, requester.AccountID, requester.MembershipID,
		object.ExportID, object.SHA256, object.MediaType, object.Bytes, object.Rows})
	return httpapi.FilterHash(string(raw)), nil
}

type grant struct {
	ID        string `json:"id"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

func (tokens *Tokens) Issue(scope kernel.OrgScope, requester app.Requester, object domain.Object, id string, now time.Time) (app.Capability, error) {
	binding, err := tokens.binding(scope, requester, object)
	if err != nil || !domain.ValidID(id) || now.IsZero() {
		return app.Capability{}, app.ErrNotFound
	}
	expiry := now.Add(domain.CapabilityTTL).Truncate(time.Second)
	payload, _ := json.Marshal(grant{ID: id, IssuedAt: now.Unix(), ExpiresAt: expiry.Unix()})
	return app.Capability{Token: httpapi.Seal(tokens.secret, binding, string(payload), now), ExpiresAt: expiry}, nil
}

func (tokens *Tokens) Verify(scope kernel.OrgScope, requester app.Requester, object domain.Object, token string, now time.Time) (string, error) {
	binding, err := tokens.binding(scope, requester, object)
	if err != nil || token == "" || len(token) > app.MaxCapability || now.IsZero() {
		return "", app.ErrNotFound
	}
	payload, err := httpapi.Open(tokens.secret, token, binding, now)
	if err != nil {
		return "", app.ErrNotFound
	}
	var parsed grant
	ttl := int64(domain.CapabilityTTL / time.Second)
	if httpapi.DecodeJSON([]byte(payload), &parsed) != nil || !domain.ValidID(parsed.ID) || parsed.IssuedAt > now.Unix() || parsed.ExpiresAt <= now.Unix() ||
		// The preceding signed ordering check makes this unsigned delta exact.
		parsed.ExpiresAt <= parsed.IssuedAt || uint64(parsed.ExpiresAt)-uint64(parsed.IssuedAt) > uint64(ttl) {
		return "", app.ErrNotFound
	}
	return parsed.ID, nil
}
