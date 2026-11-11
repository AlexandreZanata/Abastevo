package application_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
)

type failedReader struct{}

func TestUnconfiguredReadFormatNeverDelivers(t *testing.T) {
	service, s, scope, member := fixture(t)
	s.policy.object.MIME = "application/pdf"
	access, err := service.Issue(t.Context(), scope, member, documentID)
	if err != nil {
		t.Fatal(err)
	}
	if content, err := service.Open(t.Context(), scope, member, documentID, access.Token); !errors.Is(err, domain.ErrUnsupported) || len(content.Bytes) != 0 {
		t.Fatal("unconfigured read format delivered content", err)
	}
}

func TestInspectorOutageNeverBecomesPermanentRefusal(t *testing.T) {
	service, s, scope, member := fixture(t)
	s.inspector.failure = domain.ErrUnavailable
	if err := service.Upload(t.Context(), scope, member, documentID, strings.NewReader("synthetic")); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("scanner outage became permanent content refusal", err)
	}
	if s.objects.puts != 0 {
		t.Fatal("scanner outage stored bytes")
	}
	s.inspector.failure = fmt.Errorf("SYNTHETIC-SCANNER-DETAIL: %w", domain.ErrUnsupported)
	if err := service.Upload(t.Context(), scope, member, documentID, strings.NewReader("synthetic")); !errors.Is(err, domain.ErrUnsupported) || strings.Contains(err.Error(), "SYNTHETIC") {
		t.Fatal("scanner details escaped")
	}
}

func TestCanceledAccessNeverReleasesContent(t *testing.T) {
	service, s, scope, member := fixture(t)
	access, err := service.Issue(t.Context(), scope, member, documentID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s.audit.after = cancel
	if content, err := service.Open(ctx, scope, member, documentID, access.Token); !errors.Is(err, domain.ErrUnavailable) || len(content.Bytes) != 0 {
		t.Fatal("canceled access delivered bytes", err)
	}
}

func (failedReader) Read([]byte) (int, error) { return 0, errors.New("SYNTHETIC-PRIVATE-IO-ERROR") }
func TestTransferConfigurationIdentityAndFailureBoundaries(t *testing.T) {
	_, s, scope, member := fixture(t)
	for _, unset := range []func(*application.Config){
		func(c *application.Config) { c.Authority = nil }, func(c *application.Config) { c.Objects = nil },
		func(c *application.Config) { c.Inspector = nil }, func(c *application.Config) { c.Tokens = nil },
		func(c *application.Config) { c.Audit = nil }, func(c *application.Config) { c.NewID = nil },
	} {
		config := s.config
		unset(&config)
		if _, err := application.NewService(config); !errors.Is(err, domain.ErrUnavailable) {
			t.Fatal("incomplete service accepted", err)
		}
	}
	config := s.config
	config.Now = nil
	if _, err := application.NewService(config); err != nil {
		t.Fatal("default clock unavailable", err)
	}
	config = s.config
	config.Enabled = false
	service, _ := application.NewService(config)
	if err := service.Upload(t.Context(), scope, member, documentID, strings.NewReader("synthetic")); !errors.Is(err, domain.ErrDisabled) {
		t.Fatal(err)
	}
	if _, err := service.Open(t.Context(), scope, member, documentID, "token"); !errors.Is(err, domain.ErrDisabled) {
		t.Fatal(err)
	}
	if _, err := service.Issue(t.Context(), scope, member, documentID); !errors.Is(err, domain.ErrDisabled) {
		t.Fatal(err)
	}
	service, _ = application.NewService(s.config)
	if err := service.Upload(t.Context(), scope, member, documentID, nil); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	for _, kind := range []string{"scope", "member", "id", "object", "real", "policy", "grant", "identifier", "token"} {
		t.Run(kind, func(t *testing.T) {
			service, s, scope, member := fixture(t)
			id := documentID
			switch kind {
			case "scope":
				scope = kernel.OrgScope{}
			case "member":
				member = domain.Member{}
			case "id":
				id = "../document"
			case "object":
				s.policy.object.OrgID = documentID
			case "real":
				s.policy.object.Synthetic = false
			case "policy":
				s.policy.failure = errors.New("SYNTHETIC-POLICY-DETAIL")
			case "grant":
				s.policy.grantFailure = errors.New("SYNTHETIC-GRANT-DETAIL")
			case "identifier":
				s.config.NewID = func() (string, error) { return "", io.ErrUnexpectedEOF }
				service, _ = application.NewService(s.config)
			case "token":
				s.tokens.issueFailure = errors.New("SYNTHETIC-SECRET")
			}
			access, err := service.Issue(t.Context(), scope, member, id)
			if err == nil || access.Token != "" || strings.Contains(err.Error(), "SYNTHETIC") {
				t.Fatal("unsafe grant boundary")
			}
		})
	}
}
func TestInterruptedOrChangedIntakeNeverConfirmsStorage(t *testing.T) {
	service, s, scope, member := fixture(t)
	if err := service.Upload(t.Context(), scope, member, documentID, failedReader{}); !errors.Is(err, domain.ErrIntegrity) {
		t.Fatal(err)
	}
	reader := strings.NewReader(strings.Repeat("x", 100))
	if err := service.Upload(t.Context(), scope, member, documentID, reader); !errors.Is(err, domain.ErrTooLarge) || reader.Len() != 90 {
		t.Fatal("body exceeded declaration+1 read", err)
	}
	s.inspector.edit = true
	if err := service.Upload(t.Context(), scope, member, documentID, strings.NewReader("synthetic")); !errors.Is(err, domain.ErrIntegrity) {
		t.Fatal("inspector transformed original", err)
	}
	s.inspector.edit = false
	s.inspector.after = func() { s.policy.object.Revision++ }
	if err := service.Upload(t.Context(), scope, member, documentID, strings.NewReader("synthetic")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("changed declaration reached storage", err)
	}
	if s.objects.puts != 0 {
		t.Fatal("failed intake stored bytes")
	}
	s.inspector.after = func() { s.policy.denied = true }
	if err := service.Upload(t.Context(), scope, member, documentID, strings.NewReader("synthetic")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("offboarding reached storage", err)
	}
}

type fixedAuthority struct{ object domain.Object }

func (a fixedAuthority) Resolve(context.Context, kernel.OrgScope, domain.Member, string, domain.Intent) (domain.Object, error) {
	return a.object, nil
}
func (fixedAuthority) CheckAccess(context.Context, kernel.OrgScope, domain.Member, domain.Object, string) error {
	return nil
}

type heldObjects struct {
	entered chan struct{}
	release chan struct{}
}

func (heldObjects) Put(context.Context, kernel.OrgScope, domain.Object, []byte) error { return nil }
func (o heldObjects) Get(ctx context.Context, _ kernel.OrgScope, _ domain.Object) ([]byte, error) {
	o.entered <- struct{}{}
	select {
	case <-o.release:
		return []byte("synthetic"), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type discardAudit struct{}

func (discardAudit) Record(context.Context, kernel.OrgScope, domain.Member, domain.Object, string, string, domain.Intent) error {
	return nil
}
func TestPrivateTransferConcurrencyIsBounded(t *testing.T) {
	service, s, scope, member := fixture(t)
	access, err := service.Issue(t.Context(), scope, member, documentID)
	if err != nil {
		t.Fatal(err)
	}
	held := heldObjects{entered: make(chan struct{}, 4), release: make(chan struct{})}
	defer func() {
		select {
		case <-held.release:
		default:
			close(held.release)
		}
	}()
	config := s.config
	config.Authority = fixedAuthority{object: s.policy.object}
	config.Objects = held
	config.Audit = discardAudit{}
	service, err = application.NewService(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan error, 4)
	for range 4 {
		go func() { _, err := service.Open(ctx, scope, member, documentID, access.Token); done <- err }()
	}
	for range 4 {
		select {
		case <-held.entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if _, err := service.Open(ctx, scope, member, documentID, access.Token); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("unbounded concurrent transfer accepted", err)
	}
	close(held.release)
	for range 4 {
		if err := <-done; err != nil {
			t.Fatal("accepted transfer failed", err)
		}
	}
}
