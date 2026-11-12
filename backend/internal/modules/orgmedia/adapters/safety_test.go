package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
	"github.com/jackc/pgx/v5"
)

func originalJPEG(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../evidence/adapters/media/testdata/valid-4x4.jpg")
	if err != nil {
		t.Fatal(err)
	}
	// A synthetic APP1 marker proves no sanitization/re-encoding of originals.
	marker := []byte("SYNTHETIC-PRIVATE-ORIGINAL")
	out := append([]byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(len(marker) + 2)}, marker...)
	return append(out, raw[2:]...)
}
func TestJPEGInspectionPreservesOriginalAndRefusesUnconfiguredFormats(t *testing.T) {
	inspector := adapters.JPEGInspector{}
	raw := originalJPEG(t)
	before := bytes.Clone(raw)
	if mime, err := inspector.Inspect(t.Context(), raw); err != nil || mime != "image/jpeg" {
		t.Fatal("synthetic jpeg refused", err)
	}
	if !bytes.Equal(raw, before) {
		t.Fatal("original changed")
	}
	for _, raw := range [][]byte{nil, []byte("%PDF-1.7 /JavaScript /Encrypt"), []byte(`<!DOCTYPE x [<!ENTITY secret SYSTEM "file:///private">]><x>&secret;</x>`),
		[]byte("PK\x03\x04archive"), []byte("MZexecutable"), append(bytes.Clone(raw), []byte("%PDF-polyglot")...), raw[:len(raw)-2]} {
		if _, err := inspector.Inspect(t.Context(), raw); err == nil {
			t.Fatal("unsafe/unsupported content accepted")
		}
	}
	if _, err := inspector.Inspect(t.Context(), make([]byte, 5<<20+1)); !errors.Is(err, domain.ErrTooLarge) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := inspector.Inspect(ctx, raw); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
}

type auditRunner struct {
	event   audit.Event
	replay  bool
	failure error
}

func (r *auditRunner) RecordChange(_ context.Context, _ kernel.OrgScope, event audit.Event, work func(context.Context, pgx.Tx) error) (bool, error) {
	r.event = event
	return r.replay, r.failure
}
func TestAccessAuditIsBoundedAndRepeatedIDsFailClosed(t *testing.T) {
	scope, member, object, _ := tokenFixture(t)
	runner := &auditRunner{}
	recorder := adapters.AccessAudit{Runner: runner}
	catalog, err := audit.NewCatalog(adapters.AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	for _, intent := range []domain.Intent{domain.Issue, domain.Read} {
		if err := recorder.Record(t.Context(), scope, member, object, accessID, otherID, intent); err != nil {
			t.Fatal(err)
		}
		if err := catalog.Validate(runner.event); err != nil {
			t.Fatal("unregistered metadata", err)
		}
		if runner.event.ActorID != member.AccountID || runner.event.TargetID != object.DocumentID || runner.event.OperationID != otherID {
			t.Fatal("wrong attribution")
		}
	}
	runner.replay = true
	if err := recorder.Record(t.Context(), scope, member, object, accessID, otherID, domain.Read); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("duplicate read ID reused audit", err)
	}
	runner.replay = false
	runner.failure = errors.New("PRIVATE-DB-DETAIL")
	if err := recorder.Record(t.Context(), scope, member, object, accessID, otherID, domain.Read); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("unsafe audit error")
	}
	recorder.Runner = nil
	if err := recorder.Record(t.Context(), scope, member, object, accessID, otherID, domain.Read); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
	if err := recorder.Record(t.Context(), scope, member, object, accessID, otherID, domain.Upload); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
}
func TestPrivateContentHeadersIgnoreConditionalCaching(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("ETag", "old")
	raw := originalJPEG(t)
	if err := adapters.WriteContent(w, application.Content{Bytes: raw, MIME: "image/jpeg"}); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" ||
		w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Content-Disposition") != `attachment; filename="document.jpg"` ||
		!bytes.Equal(w.Body.Bytes(), raw) {
		t.Fatal("unsafe content response")
	}
	if err := adapters.WriteContent(httptest.NewRecorder(), application.Content{Bytes: raw, MIME: "text/html"}); err == nil {
		t.Fatal("unsupported delivery accepted")
	}
}
func TestS3ConfigurationAndProviderErrorsArePrivate(t *testing.T) {
	credentials := storage.Credentials{AccessKeyID: "synthetic", SecretAccessKey: "synthetic-secret"}
	for _, endpoint := range []string{"http://example.invalid", "http://127.0.0.1:1", "https://user:pass@example.invalid", "https://example.invalid/path", "https://example.invalid?token=secret"} {
		if _, err := adapters.NewS3(adapters.StorageConfig{Endpoint: endpoint, Bucket: "private-fixture", Credentials: credentials}); err == nil {
			t.Fatal("unsafe configuration accepted")
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://127.0.0.1:1/redirect")
		w.WriteHeader(307)
		_, _ = w.Write([]byte("PRIVATE-PROVIDER-CONTENT"))
	}))
	defer server.Close()
	store, err := adapters.NewS3(adapters.StorageConfig{Endpoint: server.URL, Bucket: "private-fixture", Credentials: credentials, AllowLoopback: true, Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	scope, _, object, _ := tokenFixture(t)
	if raw, err := store.Get(t.Context(), scope, object); err == nil || raw != nil || strings.Contains(err.Error(), "PRIVATE") || strings.Contains(err.Error(), "http") {
		t.Fatal("provider details escaped")
	}
	if err := store.Put(t.Context(), scope, object, []byte("synthetic")); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
	if err := store.Put(t.Context(), scope, object, []byte("bad")); !errors.Is(err, domain.ErrIntegrity) {
		t.Fatal(err)
	}
	if _, err := store.Get(t.Context(), kernel.OrgScope{}, object); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
}
