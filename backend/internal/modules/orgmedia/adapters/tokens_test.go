package adapters_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

const orgID = "7b0e2a54-0000-4000-8000-000000000001"
const documentID = "7b0e2a54-0000-4000-8000-000000000002"
const accessID = "7b0e2a54-0000-4000-8000-000000000003"
const otherID = "7b0e2a54-0000-4000-8000-000000000004"

func tokenFixture(t *testing.T) (kernel.OrgScope, domain.Member, domain.Object, time.Time) {
	t.Helper()
	id, _ := kernel.ParseOrgID(orgID)
	scope, _ := kernel.NewOrgScope(id)
	sum := sha256.Sum256([]byte("synthetic"))
	return scope, domain.Member{AccountID: orgID, MembershipID: documentID}, domain.Object{OrgID: orgID, DocumentID: documentID, SHA256: hex.EncodeToString(sum[:]), MIME: "image/jpeg", Bytes: 9, Revision: 1, Synthetic: true, Ready: true}, time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
}

func TestCursorEnvelopeCannotExtendDocumentDeadline(t *testing.T) {
	scope, member, object, now := tokenFixture(t)
	secret := []byte(strings.Repeat("synthetic-", 5))
	box, err := adapters.NewTokens("https://api.example.invalid", secret)
	if err != nil {
		t.Fatal(err)
	}
	// Adversarial signed payloads use the documented binding and existing core.
	// The shared envelope alone has a longer TTL and must never be sufficient.
	binding, _ := json.Marshal([]any{"orgmedia.access.v1", "https://api.example.invalid", object.OrgID, member.AccountID, member.MembershipID,
		object.DocumentID, object.SHA256, object.MIME, object.Bytes, object.Revision})
	for _, payload := range []string{
		fmt.Sprintf(`{"id":%q,"iat":%d,"exp":%d}`, accessID, now.Unix(), now.Add(15*time.Minute).Unix()),
		fmt.Sprintf(`{"id":%q,"iat":%d,"exp":%d}`, accessID, int64(math.MinInt64), now.Add(5*time.Minute).Unix()),
		fmt.Sprintf(`{"id":%q,"iat":%d,"exp":%d,"extra":"ignored"}`, accessID, now.Unix(), now.Add(5*time.Minute).Unix()),
		fmt.Sprintf(`{"id":"bad","iat":%d,"exp":%d}`, now.Unix(), now.Add(5*time.Minute).Unix()),
		`null`,
	} {
		token := httpapi.Seal(secret, httpapi.FilterHash(string(binding)), payload, now)
		if _, err := box.Verify(scope, member, object, token, now); err == nil {
			t.Fatal("unsafe signed payload accepted")
		}
	}
	if _, err := box.Issue(scope, domain.Member{}, object, accessID, now); err == nil {
		t.Fatal("invalid member grant issued")
	}
	if _, err := box.Issue(scope, member, object, "bad", now); err == nil {
		t.Fatal("invalid grant ID issued")
	}
}

func TestPrivateHandlesAndConfigurationRedactLogging(t *testing.T) {
	config := adapters.StorageConfig{Endpoint: "https://SYNTHETIC-ENDPOINT.invalid", Bucket: "private-fixture",
		Credentials: storage.Credentials{AccessKeyID: "SYNTHETIC-ID", SecretAccessKey: "SYNTHETIC-CREDENTIAL"}}
	store, err := adapters.NewS3(config)
	if err != nil {
		t.Fatal("fixture configuration failed")
	}
	box, err := adapters.NewTokens("https://api.example.invalid", []byte(strings.Repeat("SYNTHETIC-CREDENTIAL", 3)))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{config, store, *store, box, *box} {
		var log bytes.Buffer
		slog.New(slog.NewJSONHandler(&log, nil)).Info("configuration", "value", value)
		if strings.Contains(log.String(), "SYNTHETIC") {
			t.Fatal("structured logging exposed handle")
		}
		for _, format := range []string{"%v", "%+v", "%#v"} {
			if strings.Contains(fmt.Sprintf(format, value), "SYNTHETIC") {
				t.Fatal("default formatting exposed configuration")
			}
		}
	}
}
func TestFiveMinuteAccessBindsOriginMemberAndOriginal(t *testing.T) {
	scope, member, object, now := tokenFixture(t)
	secret := []byte(strings.Repeat("synthetic-", 5))
	box, err := adapters.NewTokens("https://api.example.invalid", secret)
	if err != nil {
		t.Fatal(err)
	}
	access, err := box.Issue(scope, member, object, accessID, now)
	if err != nil || !access.ExpiresAt.Equal(now.Add(5*time.Minute)) {
		t.Fatal("invalid expiry", err)
	}
	secret[0] = 'x'
	if id, err := box.Verify(scope, member, object, access.Token, now); err != nil || id != accessID {
		t.Fatal("round trip failed", err)
	}
	for _, when := range []time.Time{now.Add(5 * time.Minute), now.Add(6 * time.Minute), now.Add(-time.Second)} {
		if _, err := box.Verify(scope, member, object, access.Token, when); err == nil {
			t.Fatal("expired/future token accepted")
		}
	}
	for _, change := range []func(*domain.Member, *domain.Object){
		func(m *domain.Member, o *domain.Object) { m.MembershipID = otherID },
		func(m *domain.Member, o *domain.Object) { m.AccountID = otherID },
		func(m *domain.Member, o *domain.Object) { o.DocumentID = otherID },
		func(m *domain.Member, o *domain.Object) { o.SHA256 = strings.Repeat("a", 64) },
		func(m *domain.Member, o *domain.Object) { o.Revision++ },
		func(m *domain.Member, o *domain.Object) { o.MIME = "image/png" },
		func(m *domain.Member, o *domain.Object) { o.Bytes++ },
	} {
		m, o := member, object
		change(&m, &o)
		if _, err := box.Verify(scope, m, o, access.Token, now); err == nil {
			t.Fatal("token rebound to different identity")
		}
	}
	id, _ := kernel.ParseOrgID(otherID)
	otherScope, _ := kernel.NewOrgScope(id)
	foreign := object
	foreign.OrgID = otherID
	if _, err := box.Verify(otherScope, member, foreign, access.Token, now); err == nil {
		t.Fatal("foreign org accepted")
	}
	otherBox, _ := adapters.NewTokens("https://other.example.invalid", []byte(strings.Repeat("synthetic-", 5)))
	if _, err := otherBox.Verify(scope, member, object, access.Token, now); err == nil {
		t.Fatal("foreign origin accepted")
	}
	for _, token := range []string{"", strings.Repeat("x", 2049), "tampered" + access.Token} {
		if _, err := box.Verify(scope, member, object, token, now); err == nil {
			t.Fatal("invalid token accepted")
		}
	}
	for _, origin := range []string{"http://example.invalid", "https://user:pass@example.invalid", "https://example.invalid/path", "https://example.invalid?key=secret"} {
		if _, err := adapters.NewTokens(origin, []byte(strings.Repeat("x", 32))); err == nil {
			t.Fatal("unsafe origin accepted")
		}
	}
	if _, err := adapters.NewTokens("https://api.example.invalid", []byte("short")); err == nil {
		t.Fatal("short secret accepted")
	}
	for _, value := range []any{access, application.Content{Bytes: []byte("SYNTHETIC-CONTENT"), MIME: "image/jpeg"}} {
		for _, format := range []string{"%v", "%+v", "%#v"} {
			formatted := fmt.Sprintf(format, value)
			if strings.Contains(formatted, access.Token) || strings.Contains(formatted, "SYNTHETIC-CONTENT") {
				t.Fatal("sensitive default formatting")
			}
		}
	}
}
