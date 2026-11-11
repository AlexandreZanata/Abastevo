package domain_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
)

const orgID = "7b0e2a54-0000-4000-8000-000000000001"
const documentID = "7b0e2a54-0000-4000-8000-000000000002"

func fixture(t *testing.T) (kernel.OrgScope, domain.Object, []byte) {
	t.Helper()
	id, _ := kernel.ParseOrgID(orgID)
	scope, _ := kernel.NewOrgScope(id)
	raw := []byte("synthetic")
	sum := sha256.Sum256(raw)
	return scope, domain.Object{OrgID: orgID, DocumentID: documentID, SHA256: hex.EncodeToString(sum[:]), MIME: "image/jpeg", Bytes: int64(len(raw)), Revision: 1, Synthetic: true}, raw
}
func TestPrivateObjectIdentityAndNamespace(t *testing.T) {
	scope, object, raw := fixture(t)
	if err := object.Validate(scope, documentID); err != nil {
		t.Fatal(err)
	}
	key, err := object.Key(scope)
	if err != nil || key != "org/"+orgID+"/doc/"+documentID+"/"+object.SHA256 {
		t.Fatal(key, err)
	}
	if err := object.CheckSnapshot(raw); err != nil {
		t.Fatal(err)
	}
	if err := object.CheckSnapshot([]byte("substitut")); !errors.Is(err, domain.ErrIntegrity) {
		t.Fatal(err)
	}
	if err := object.CheckSnapshot(append(raw, 'x')); !errors.Is(err, domain.ErrTooLarge) {
		t.Fatal(err)
	}
	if err := object.CheckSnapshot(raw[:len(raw)-1]); !errors.Is(err, domain.ErrIntegrity) {
		t.Fatal(err)
	}
	copy := object
	copy.Ready = !copy.Ready
	if !object.SameIdentity(copy) {
		t.Fatal("readiness altered immutable identity")
	}
	copy.Revision++
	if object.SameIdentity(copy) {
		t.Fatal("revision omitted from identity")
	}
	other := object
	other.OrgID = documentID
	if other.Validate(scope, documentID) == nil {
		t.Fatal("foreign object accepted")
	}
	for _, alter := range []func(*domain.Object){
		func(o *domain.Object) { o.DocumentID = "../secret" },
		func(o *domain.Object) { o.SHA256 = strings.Repeat("A", 64) },
		func(o *domain.Object) { o.MIME = "image/svg+xml" },
		func(o *domain.Object) { o.Bytes = 0 },
		func(o *domain.Object) { o.Bytes = 5<<20 + 1 },
		func(o *domain.Object) { o.Revision = 0 },
		func(o *domain.Object) { o.Synthetic = false },
	} {
		copy := object
		alter(&copy)
		if copy.Validate(scope, documentID) == nil {
			t.Fatal("unsafe declaration accepted")
		}
	}
	if _, err := object.Key(kernel.OrgScope{}); err == nil {
		t.Fatal("zero scope accepted")
	}
	if object.Validate(scope, orgID) == nil {
		t.Fatal("different document accepted")
	}
}
func TestFormatBoundsAndVerifiedMembers(t *testing.T) {
	for mime, want := range map[string]int64{"image/jpeg": 5 << 20, "image/png": 5 << 20, "application/pdf": 10 << 20, "application/xml": 1 << 20} {
		if got, err := domain.ByteLimit(mime); err != nil || got != want {
			t.Fatal(mime, got, err)
		}
	}
	if _, err := domain.ByteLimit("application/zip"); !errors.Is(err, domain.ErrUnsupported) {
		t.Fatal(err)
	}
	if err := (domain.Member{AccountID: orgID, MembershipID: documentID}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, member := range []domain.Member{{}, {AccountID: orgID}, {AccountID: "00000000-0000-0000-0000-000000000000", MembershipID: documentID}} {
		if member.Validate() == nil {
			t.Fatal("invalid identity accepted")
		}
	}
}
