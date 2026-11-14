package http_test

import (
	"testing"

	documentweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

// The positive record-gate paths need a real Subject and are proven by the
// composed company HTTP suite; here every shape outside the gate is refused.
func TestLinkRecordsRefuseOtherPoliciesAndShapes(t *testing.T) {
	id, _ := kernel.ParseOrgID(orgID)
	scope, _ := kernel.NewOrgScope(id)
	rules := []permissions.Rule{{}}
	records := documentweb.LinkRecords{}
	for name, check := range map[string]struct {
		policy access.Policy
		facts  permissions.ResourceFacts
		rules  []permissions.Rule
	}{
		"content-policy":     {app.ContentPolicy(), permissions.ResourceFacts{OrgID: id, ID: docID}, rules},
		"owner-policy":       {access.Policy{Permission: "fleet.asset.manage", Capability: "organizations", Effect: access.OperationalWrite}, permissions.ResourceFacts{OrgID: id, ID: docID}, rules},
		"no-rules":           {app.AttachPolicy(), permissions.ResourceFacts{OrgID: id, ID: docID, AuthoredBy: memberID}, nil},
		"foreign-facts":      {app.LinkReadPolicy(), permissions.ResourceFacts{ID: docID}, rules},
		"zero-subject":       {app.AttachPolicy(), permissions.ResourceFacts{OrgID: id, ID: docID, AuthoredBy: memberID}, rules},
		"zero-subject-read":  {app.LinkReadPolicy(), permissions.ResourceFacts{OrgID: id, ID: docID, AuthoredBy: memberID}, rules},
		"attach-on-root":     {app.AttachPolicy(), permissions.ResourceFacts{OrgID: id, ID: orgID, AuthoredBy: memberID}, rules},
		"zero-scope-content": {app.LinkReadPolicy(), permissions.ResourceFacts{OrgID: id, ID: orgID}, rules},
	} {
		if records.Check(t.Context(), scope, access.Subject{}, check.policy, check.facts, check.rules) == nil {
			t.Fatal("link record gate admitted", name)
		}
	}
	if records.Check(t.Context(), kernel.OrgScope{}, access.Subject{}, app.AttachPolicy(), permissions.ResourceFacts{OrgID: id, ID: docID}, rules) == nil {
		t.Fatal("link record gate admitted an invalid scope")
	}
	// The upload record gate still refuses an attach decision on a document,
	// so the link routes cannot reuse it to create documents or vice versa.
	if (documentweb.DocumentRecords{}).Check(t.Context(), scope, access.Subject{}, app.LinkReadPolicy(), permissions.ResourceFacts{OrgID: id, ID: docID}, rules) == nil {
		t.Fatal("document record gate admitted the link read policy")
	}
}
