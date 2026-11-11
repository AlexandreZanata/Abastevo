package adapters

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// Tokens reuses the existing HMAC envelope, with a separate domain and a strict
// five-minute inner deadline. The envelope's 15-minute cursor TTL never extends
// document access. No storage URL or credential appears in the token.
type Tokens struct {
	origin string
	secret []byte
}

var _ application.Tokens = (*Tokens)(nil)

func NewTokens(origin string, secret []byte) (*Tokens, error) {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.ForceQuery ||
		strings.ToLower(parsed.Host) != parsed.Host || len(secret) < 32 {
		return nil, domain.ErrInvalid
	}
	return &Tokens{origin: origin, secret: append([]byte(nil), secret...)}, nil
}
func (tokens *Tokens) binding(scope kernel.OrgScope, member domain.Member, object domain.Object) (string, error) {
	if tokens == nil || len(tokens.secret) < 32 || object.Validate(scope, object.DocumentID) != nil || member.Validate() != nil {
		return "", domain.ErrNotFound
	}
	binding, _ := json.Marshal([]any{"orgmedia.access.v1", tokens.origin, object.OrgID, member.AccountID, member.MembershipID,
		object.DocumentID, object.SHA256, object.MIME, object.Bytes, object.Revision})
	return httpapi.FilterHash(string(binding)), nil
}

type grant struct {
	ID        string `json:"id"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

func (tokens *Tokens) Issue(scope kernel.OrgScope, member domain.Member, object domain.Object, id string, now time.Time) (application.Access, error) {
	binding, err := tokens.binding(scope, member, object)
	if err != nil || !domain.ValidID(id) || now.IsZero() {
		return application.Access{}, domain.ErrNotFound
	}
	expiry := now.Add(time.Duration(domain.AccessTTL) * time.Second).Truncate(time.Second)
	payload, _ := json.Marshal(grant{ID: id, IssuedAt: now.Unix(), ExpiresAt: expiry.Unix()})
	return application.Access{Token: httpapi.Seal(tokens.secret, binding, string(payload), now), ExpiresAt: expiry}, nil
}
func (tokens *Tokens) Verify(scope kernel.OrgScope, member domain.Member, object domain.Object, token string, now time.Time) (string, error) {
	binding, err := tokens.binding(scope, member, object)
	if err != nil || len(token) == 0 || len(token) > 2048 || now.IsZero() {
		return "", domain.ErrNotFound
	}
	payload, err := httpapi.Open(tokens.secret, token, binding, now)
	if err != nil {
		return "", domain.ErrNotFound
	}
	var parsed grant
	if httpapi.DecodeJSON([]byte(payload), &parsed) != nil || !domain.ValidID(parsed.ID) ||
		parsed.IssuedAt > now.Unix() || parsed.ExpiresAt <= now.Unix() ||
		// The preceding signed ordering check makes this unsigned delta exact,
		// including timestamps whose signed subtraction would overflow.
		parsed.ExpiresAt <= parsed.IssuedAt || uint64(parsed.ExpiresAt)-uint64(parsed.IssuedAt) > uint64(domain.AccessTTL) {
		return "", domain.ErrNotFound
	}
	return parsed.ID, nil
}
