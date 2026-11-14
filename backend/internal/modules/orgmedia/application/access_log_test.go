package application_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
)

// B-BR-EMP-208: the issuance is recorded before any capability exists, so a
// refused record never mints one.
func TestAccessRecordPrecedesCapabilityMinting(t *testing.T) {
	service, s, scope, member := fixture(t)
	minted := true
	s.audit.after = func() { minted = s.tokens.issued != "" }
	if _, err := service.Issue(t.Context(), scope, member, documentID); err != nil {
		t.Fatal(err)
	}
	if minted {
		t.Fatal("capability minted before the access record")
	}
	service, s, scope, member = fixture(t)
	s.audit.failure = errors.New("SYNTHETIC-PRIVATE-AUDIT-DETAIL")
	if access, err := service.Issue(t.Context(), scope, member, documentID); !errors.Is(err, domain.ErrUnavailable) || access.Token != "" || s.tokens.issued != "" {
		t.Fatal("refused record minted a capability", err)
	}
}

// BR-AUTH-013: a member without doc.document.view_content gets FORBIDDEN,
// distinct from not-found, and nothing is recorded, issued or read.
func TestMissingContentPermissionIsForbiddenWithoutRecord(t *testing.T) {
	service, s, scope, member := fixture(t)
	s.policy.failure = domain.ErrForbidden
	if access, err := service.Issue(t.Context(), scope, member, documentID); !errors.Is(err, domain.ErrForbidden) || access.Token != "" {
		t.Fatal("member without content permission was not forbidden", err)
	}
	if s.audit.calls != 0 || s.tokens.issued != "" {
		t.Fatal("forbidden request was recorded or minted")
	}
	s.policy.failure = nil
	access, err := service.Issue(t.Context(), scope, member, documentID)
	if err != nil {
		t.Fatal(err)
	}
	// A role change takes effect at the next request (BR-AUTH-014).
	s.policy.failure = fmt.Errorf("SYNTHETIC-ROLE-DETAIL: %w", domain.ErrForbidden)
	content, err := service.Open(t.Context(), scope, member, documentID, access.Token)
	if !errors.Is(err, domain.ErrForbidden) || len(content.Bytes) != 0 || err.Error() != domain.ErrForbidden.Error() {
		t.Fatal("capability outlived the content permission", err)
	}
	if s.audit.calls != 1 || s.objects.gets != 0 {
		t.Fatal("forbidden read was recorded or reached storage")
	}
	s.policy.failure = nil
	s.policy.grantFailure = domain.ErrForbidden
	if content, err := service.Open(t.Context(), scope, member, documentID, access.Token); !errors.Is(err, domain.ErrForbidden) || len(content.Bytes) != 0 {
		t.Fatal("grant check lost the forbidden outcome", err)
	}
}
