package application_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
)

// EMP-DOC-014: the unsafe verdict reaches the owner only for a snapshot whose
// exact size and digest matched, before anything is stored; a read whose
// bytes are now judged unsafe serves nothing and records nothing.
func TestUnsafeVerdictFollowsIntegrityAndNeverStoresOrServes(t *testing.T) {
	service, s, scope, member := fixture(t)
	original := bytes.Clone(s.objects.data)
	s.objects.data = nil
	s.policy.object.Ready = false
	s.inspector.failure = domain.ErrUnsafe
	if err := service.Upload(t.Context(), scope, member, documentID, strings.NewReader("synthetic-extra")); !errors.Is(err, domain.ErrTooLarge) || s.inspector.calls != 0 {
		t.Fatal("integrity must be decided before content", err)
	}
	if err := service.Upload(t.Context(), scope, member, documentID, strings.NewReader("substitut")); !errors.Is(err, domain.ErrIntegrity) || s.inspector.calls != 0 {
		t.Fatal("a digest mismatch reached the inspector", err)
	}
	if err := service.Upload(t.Context(), scope, member, documentID, bytes.NewReader(original)); !errors.Is(err, domain.ErrUnsafe) || s.inspector.calls != 1 || s.objects.puts != 0 {
		t.Fatal("unsafe verified snapshot was stored or lost its verdict", err)
	}
	s.policy.object.Ready = true
	s.objects.data = original
	s.inspector.failure = nil
	access, err := service.Issue(t.Context(), scope, member, documentID)
	if err != nil {
		t.Fatal(err)
	}
	s.inspector.failure = domain.ErrUnsafe
	recorded := s.audit.calls
	if content, err := service.Open(t.Context(), scope, member, documentID, access.Token); !errors.Is(err, domain.ErrUnsafe) || content.Bytes != nil || s.audit.calls != recorded {
		t.Fatal("unsafe stored bytes served or recorded", err)
	}
}
