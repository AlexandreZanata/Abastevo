package adapters_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

const (
	tokenOrg      = "7b0e2a54-0000-4000-8000-000000020511"
	tokenOtherOrg = "7b0e2a54-0000-4000-8000-000000020512"
	tokenDoc      = "7b0e2a54-0000-4000-8000-000000020513"
	tokenOtherDoc = "7b0e2a54-0000-4000-8000-000000020514"
	tokenAccount  = "7b0e2a54-0000-4000-8000-000000020515"
	tokenMember   = "7b0e2a54-0000-4000-8000-000000020516"
)

func tokenScope(t *testing.T, raw string) kernel.OrgScope {
	t.Helper()
	id, err := kernel.ParseOrgID(raw)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func TestUploadCapabilityIsShortAndBoundToOriginOrganizationDocumentAndMember(t *testing.T) {
	secret := []byte(strings.Repeat("synthetic-", 5))
	tokens, err := adapters.NewUploadTokens("https://api.example.invalid", secret)
	if err != nil {
		t.Fatal(err)
	}
	scope := tokenScope(t, tokenOrg)
	actor := app.Actor{AccountID: tokenAccount, MembershipID: tokenMember, Channel: "web"}
	now := time.Unix(1_800_000_000, 500_000_000)
	access, err := tokens.Issue(scope, actor, tokenDoc, now)
	if err != nil || access.Token == "" || !access.ExpiresAt.Equal(time.Unix(1_800_000_300, 0)) || access.ExpiresAt.Sub(now) > 5*time.Minute {
		t.Fatal("upload capability exceeds five minutes or is missing", err, access.ExpiresAt)
	}
	got, err := tokens.Verify(scope, tokenDoc, access.Token, now.Add(299*time.Second))
	if err != nil || got != actor {
		t.Fatal("upload capability lost its member binding", err)
	}
	if _, err := tokens.Verify(scope, tokenDoc, access.Token, time.Unix(1_800_000_300, 0)); err == nil {
		t.Fatal("expired upload capability accepted at its deadline")
	}
	if _, err := tokens.Verify(tokenScope(t, tokenOtherOrg), tokenDoc, access.Token, now); err == nil {
		t.Fatal("capability crossed organizations")
	}
	if _, err := tokens.Verify(scope, tokenOtherDoc, access.Token, now); err == nil {
		t.Fatal("capability crossed documents")
	}
	other, err := adapters.NewUploadTokens("https://other.example.invalid", secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Verify(scope, tokenDoc, access.Token, now); err == nil {
		t.Fatal("capability crossed origins")
	}
	rotated, err := adapters.NewUploadTokens("https://api.example.invalid", []byte(strings.Repeat("rotated-x-", 5)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rotated.Verify(scope, tokenDoc, access.Token, now); err == nil {
		t.Fatal("capability verified under another key")
	}
	raw, err := base64.RawURLEncoding.DecodeString(access.Token)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-2] ^= 1
	if _, err := tokens.Verify(scope, tokenDoc, base64.RawURLEncoding.EncodeToString(raw), now); err == nil {
		t.Fatal("tampered capability accepted")
	}
	for _, bad := range []string{"", strings.Repeat("a", 4097), "not base64 !"} {
		if _, err := tokens.Verify(scope, tokenDoc, bad, now); err == nil {
			t.Fatal("malformed capability accepted")
		}
	}
	// The content-access capability (orgmedia) and a page cursor cannot be
	// replayed as an upload capability: they use other binding domains.
	cursor := httpapi.Seal(secret, httpapi.FilterHash("orgmedia.access.v1"), `{"acc":"`+tokenAccount+`"}`, now)
	if _, err := tokens.Verify(scope, tokenDoc, cursor, now); err == nil {
		t.Fatal("foreign binding domain accepted")
	}
}

func TestUploadCapabilityRefusesLongInnerDeadlinesAndInvalidInputs(t *testing.T) {
	secret := []byte(strings.Repeat("synthetic-", 5))
	tokens, err := adapters.NewUploadTokens("https://api.example.invalid", secret)
	if err != nil {
		t.Fatal(err)
	}
	scope := tokenScope(t, tokenOrg)
	now := time.Unix(1_800_000_000, 0)
	for name, actor := range map[string]app.Actor{
		"missing-member": {AccountID: tokenAccount, Channel: "web"},
		"bad-channel":    {AccountID: tokenAccount, MembershipID: tokenMember, Channel: "api"},
	} {
		if _, err := tokens.Issue(scope, actor, tokenDoc, now); err == nil {
			t.Fatal("capability minted for invalid actor", name)
		}
	}
	if _, err := tokens.Issue(scope, app.Actor{AccountID: tokenAccount, MembershipID: tokenMember, Channel: "web"}, "not-a-uuid", now); err == nil {
		t.Fatal("capability minted for invalid document")
	}
	if _, err := tokens.Issue(scope, app.Actor{AccountID: tokenAccount, MembershipID: tokenMember, Channel: "web"}, tokenDoc, time.Time{}); err == nil {
		t.Fatal("capability minted without clock")
	}
	for _, origin := range []string{"http://api.example.invalid", "https://API.example.invalid", "https://api.example.invalid/path", "https://user@api.example.invalid", ""} {
		if _, err := adapters.NewUploadTokens(origin, secret); err == nil {
			t.Fatal("non-canonical origin accepted", origin)
		}
	}
	if _, err := adapters.NewUploadTokens("https://api.example.invalid", secret[:31]); err == nil {
		t.Fatal("short secret accepted")
	}
	// A correctly sealed envelope whose inner deadline exceeds five minutes, or
	// whose actor is incomplete, is refused even inside the envelope TTL.
	binding := adapters.UploadBinding("https://api.example.invalid", tokenOrg, tokenDoc)
	for name, payload := range map[string]map[string]any{
		"ten-minutes":   {"acc": tokenAccount, "mem": tokenMember, "ch": "web", "iat": now.Unix(), "exp": now.Unix() + 600},
		"future-issue":  {"acc": tokenAccount, "mem": tokenMember, "ch": "web", "iat": now.Unix() + 10, "exp": now.Unix() + 60},
		"no-member":     {"acc": tokenAccount, "ch": "web", "iat": now.Unix(), "exp": now.Unix() + 60},
		"unknown-field": {"acc": tokenAccount, "mem": tokenMember, "ch": "web", "iat": now.Unix(), "exp": now.Unix() + 60, "role": "OWNER"},
	} {
		raw, _ := json.Marshal(payload)
		forged := httpapi.Seal(secret, binding, string(raw), now)
		if _, err := tokens.Verify(scope, tokenDoc, forged, now); err == nil {
			t.Fatal("unsafe sealed upload payload accepted", name)
		}
	}
	var logs bytes.Buffer
	slog.New(slog.NewJSONHandler(&logs, nil)).Info("synthetic", "tokens", tokens)
	for _, out := range []string{fmt.Sprint(tokens), fmt.Sprintf("%#v", tokens), logs.String()} {
		if strings.Contains(out, "synthetic-") {
			t.Fatal("token handle leaked its key")
		}
	}
}
