package adapters

import (
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
)

// Without a configured lookup or transaction nothing structural is resolved:
// the source fails closed and a grantless re-check keeps its facts unchanged.
func TestResourceStructureFailsClosedWithoutItsSources(t *testing.T) {
	id, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000301")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	facts := permissions.ResourceFacts{OrgID: id, ID: "7b0e2a54-0000-4000-8000-000000000901"}
	policy := access.Policy{Permission: "fleet.asset.read", Capability: "organizations", Effect: access.Read}
	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	var missing *ResourceStructure
	if _, err := missing.ResolveStructure(t.Context(), scope, policy, facts, now); !errors.Is(err, access.ErrUnavailable) {
		t.Fatal("nil structure source answered", err)
	}
	if _, err := (&ResourceStructure{Kind: AssetStructure}).ResolveStructure(t.Context(), scope, policy, facts, now); !errors.Is(err, access.ErrUnavailable) {
		t.Fatal("structure source without a pool answered", err)
	}
	if got, err := missing.structureTx(t.Context(), nil, scope, "7b0e2a54-0000-4000-8000-000000000601", nil, policy, facts, now); err != nil || got.ID != facts.ID || got.UnitID != "" {
		t.Fatal("absent lookup changed a re-check", err)
	}
	if _, err := (&ResourceStructure{Kind: AssetStructure}).resolveTx(t.Context(), nil, scope, policy, facts, now); !errors.Is(err, org.ErrUnavailable) {
		t.Fatal("lookup without a transaction answered", err)
	}
	if _, err := (&ResourceStructure{Kind: AssetStructure}).structureTx(t.Context(), nil, scope, "bad", nil, policy, facts, now); !errors.Is(err, org.ErrUnavailable) {
		t.Fatal("invalid member accepted by the re-check", err)
	}
}
