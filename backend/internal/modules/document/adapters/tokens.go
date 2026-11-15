package adapters

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/url"
	"strings"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// UploadTokens seals the upload capability with the existing HMAC envelope in
// its own binding domain and a strict five-minute inner deadline. The
// envelope's longer cursor TTL never extends the capability, and neither a
// content capability nor a cursor verifies here.
type UploadTokens struct {
	origin string
	secret []byte
}

var _ app.UploadTokens = (*UploadTokens)(nil)

func (UploadTokens) String() string   { return "document: upload token handle redacted" }
func (UploadTokens) GoString() string { return "document: upload token handle redacted" }
func (UploadTokens) LogValue() slog.Value {
	return slog.StringValue("document: upload token handle redacted")
}

func NewUploadTokens(origin string, secret []byte) (*UploadTokens, error) {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" ||
		parsed.Fragment != "" || parsed.ForceQuery || strings.ToLower(parsed.Host) != parsed.Host || len(secret) < 32 {
		return nil, app.ErrUnavailable
	}
	return &UploadTokens{origin: origin, secret: append([]byte(nil), secret...)}, nil
}

func uploadBinding(origin, org, document string) string {
	raw, _ := json.Marshal([]string{"document.upload.v1", origin, org, document})
	return httpapi.FilterHash(string(raw))
}

type uploadGrant struct {
	AccountID     string `json:"acc"`
	MembershipID  string `json:"mem"`
	Channel       string `json:"ch"`
	ContributorID string `json:"con,omitempty"`
	DeviceKeyID   string `json:"dev,omitempty"`
	IssuedAt      int64  `json:"iat"`
	ExpiresAt     int64  `json:"exp"`
}

func (t *UploadTokens) Issue(scope kernel.OrgScope, actor app.Actor, document string, now time.Time) (app.UploadAccess, error) {
	if t == nil || len(t.secret) < 32 || scope.Validate() != nil || actor.Validate() != nil || !validID(document) || now.IsZero() {
		return app.UploadAccess{}, app.ErrUnavailable
	}
	expiry := now.Add(app.UploadTTL).Truncate(time.Second)
	payload, _ := json.Marshal(uploadGrant{AccountID: actor.AccountID, MembershipID: actor.MembershipID, Channel: actor.Channel,
		ContributorID: actor.ContributorID, DeviceKeyID: actor.DeviceKeyID, IssuedAt: now.Unix(), ExpiresAt: expiry.Unix()})
	return app.UploadAccess{Token: httpapi.Seal(t.secret, uploadBinding(t.origin, scope.OrgID().String(), document), string(payload), now), ExpiresAt: expiry}, nil
}

func (t *UploadTokens) Verify(scope kernel.OrgScope, document, token string, now time.Time) (app.Actor, error) {
	if t == nil || len(t.secret) < 32 || scope.Validate() != nil || !validID(document) || len(token) == 0 || len(token) > 4096 || now.IsZero() {
		return app.Actor{}, app.ErrNotFound
	}
	payload, err := httpapi.Open(t.secret, token, uploadBinding(t.origin, scope.OrgID().String(), document), now)
	if err != nil {
		return app.Actor{}, app.ErrNotFound
	}
	var grant uploadGrant
	decoder := json.NewDecoder(bytes.NewReader([]byte(payload)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&grant) != nil || decoder.More() || grant.IssuedAt > now.Unix() || grant.ExpiresAt <= now.Unix() || grant.ExpiresAt <= grant.IssuedAt ||
		// The signed ordering check above makes this unsigned delta exact.
		uint64(grant.ExpiresAt)-uint64(grant.IssuedAt) > uint64(app.UploadTTL/time.Second) {
		return app.Actor{}, app.ErrNotFound
	}
	actor := app.Actor{AccountID: grant.AccountID, MembershipID: grant.MembershipID, Channel: grant.Channel, ContributorID: grant.ContributorID, DeviceKeyID: grant.DeviceKeyID}
	if actor.Validate() != nil {
		return app.Actor{}, app.ErrNotFound
	}
	return actor, nil
}
