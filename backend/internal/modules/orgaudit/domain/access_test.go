package domain_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
)

const (
	accessOrg     = "7b0e2a54-0000-4000-8000-000000000001"
	accessAccount = "7b0e2a54-0000-4000-8000-000000000021"
	accessMember  = "7b0e2a54-0000-4000-8000-000000000022"
	accessTarget  = "7b0e2a54-0000-4000-8000-000000000023"
	accessGrant   = "7b0e2a54-0000-4000-8000-000000000024"
	accessRequest = "7b0e2a54-0000-4000-8000-000000000025"
)

func accessScope(t *testing.T) kernel.OrgScope {
	t.Helper()
	id, err := kernel.ParseOrgID(accessOrg)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func servedDocument() domain.SensitiveAccess {
	return domain.SensitiveAccess{
		Category: domain.DocumentContent, Stage: domain.AccessServed,
		AccountID: accessAccount, MembershipID: accessMember, TargetID: accessTarget,
		AccessID: accessGrant, RequestID: accessRequest, Binding: strings.Repeat("b", 64),
	}
}

// BR-AUTH-011: each category has its own catalog permission, never the
// record read permission, and a fixed target type and action vocabulary.
func TestAccessCategoriesAreClosedAndPinnedToTheirOwnPermission(t *testing.T) {
	want := map[domain.AccessCategory]struct {
		permission, target, issued, served string
	}{
		domain.DocumentContent: {"doc.document.view_content", "document", "document.access.issued", "document.content.accessed"},
		domain.ExportFile:      {"rpt.export.run", "export", "export.access.issued", "export.content.accessed"},
		domain.LocationLive:    {"gps.location.read_live", "trip", "", "location.live.accessed"},
		domain.LocationTrack:   {"gps.track.read", "trip", "location.track.issued", "location.track.accessed"},
	}
	categories := domain.AccessCategories()
	if len(categories) != len(want) || !slices.IsSorted(categories) {
		t.Fatalf("closed category set changed: %v", categories)
	}
	catalog := access.SystemCatalog()
	known := catalog.Permissions()
	for _, category := range categories {
		spec, exists := want[category]
		if !exists {
			t.Fatalf("unexpected category %q", category)
		}
		if category.Permission() != spec.permission || category.TargetType() != spec.target {
			t.Fatalf("%s pinned to %q/%q", category, category.Permission(), category.TargetType())
		}
		if !slices.Contains(known, access.Permission(category.Permission())) {
			t.Fatalf("%s permission is not in the authorization catalog", category)
		}
		if slices.Contains([]string{"doc.document.read", "trip.trip.read"}, category.Permission()) {
			t.Fatalf("%s reuses a record read permission", category)
		}
		served, err := category.Action(domain.AccessServed)
		if err != nil || served != spec.served {
			t.Fatalf("%s served action %q %v", category, served, err)
		}
		issued, err := category.Action(domain.AccessIssued)
		if spec.issued == "" {
			if !errors.Is(err, domain.ErrInvalid) || issued != "" {
				t.Fatalf("%s must not issue a capability", category)
			}
		} else if err != nil || issued != spec.issued {
			t.Fatalf("%s issued action %q %v", category, issued, err)
		}
		if _, err := category.Action("REVOKED"); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("unknown stage accepted")
		}
	}
	// The document actions stay the EMP-PLAT-006 names, so earlier events keep their meaning.
	for _, unknown := range []domain.AccessCategory{"", "document_content", "SUPPORT_SESSION", "LOCATION"} {
		if unknown.Permission() != "" || unknown.TargetType() != "" {
			t.Fatalf("unknown category %q resolved", unknown)
		}
		if _, err := unknown.Action(domain.AccessServed); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("unknown category %q has an action", unknown)
		}
	}
	// Read and content permissions stay separate in the role catalog: finance
	// reads trips without any location permission, a supervisor holds neither
	// document content nor exports, and an auditor reads only recorded tracks.
	if rule, err := catalog.Lookup(access.Finance, "trip.trip.read"); err != nil || rule.Coverage() == access.Denied {
		t.Fatal("fixture premise changed: finance no longer reads trips", err)
	}
	for _, item := range []struct {
		role     access.Role
		category domain.AccessCategory
		allowed  bool
	}{
		{access.FleetManager, domain.DocumentContent, true}, {access.Supervisor, domain.DocumentContent, false},
		{access.Supervisor, domain.ExportFile, false}, {access.Auditor, domain.ExportFile, true},
		{access.Auditor, domain.LocationLive, false}, {access.Auditor, domain.LocationTrack, true},
		{access.StationAdmin, domain.LocationTrack, false}, {access.Finance, domain.LocationLive, false},
		{access.Finance, domain.LocationTrack, false},
	} {
		rule, err := catalog.Lookup(item.role, access.Permission(item.category.Permission()))
		if err != nil || (rule.Coverage() != access.Denied) != item.allowed {
			t.Fatalf("%s/%s coverage %v %v", item.role, item.category, rule.Coverage(), err)
		}
	}
}

func TestAccessDefinitionsRegisterOnlyMemberReasons(t *testing.T) {
	definitions := domain.AccessDefinitions()
	if len(definitions) != 7 {
		t.Fatalf("expected seven sensitive actions, got %d", len(definitions))
	}
	for _, definition := range definitions {
		if !definition.RequireReason || !slices.Equal(definition.Reasons, []string{domain.AccessReason}) {
			t.Fatalf("%s registers a non-member reason", definition.Action)
		}
	}
	catalog, err := domain.NewCatalog(definitions)
	if err != nil {
		t.Fatal(err)
	}
	documents := domain.AccessDefinitions(domain.DocumentContent)
	if len(documents) != 2 || documents[0].Action != "document.access.issued" || documents[1].Action != "document.content.accessed" {
		t.Fatalf("document vocabulary changed: %+v", documents)
	}
	documents[0].Reasons[0] = "support.session"
	if again := domain.AccessDefinitions(domain.DocumentContent); again[0].Reasons[0] != domain.AccessReason {
		t.Fatal("definitions share mutable reason slices")
	}
	if domain.AccessDefinitions(domain.DocumentContent, "SUPPORT_SESSION") != nil {
		t.Fatal("unknown category registered vocabulary")
	}
	if domain.AccessDefinitions(domain.DocumentContent, domain.DocumentContent) != nil {
		t.Fatal("duplicate category registered twice")
	}
	scope := accessScope(t)
	for _, category := range domain.AccessCategories() {
		for _, stage := range []domain.AccessStage{domain.AccessIssued, domain.AccessServed} {
			item := servedDocument()
			item.Category, item.Stage = category, stage
			if category == domain.LocationLive {
				item.AccessID = item.RequestID
			}
			event, err := item.Event(scope)
			if category == domain.LocationLive && stage == domain.AccessIssued {
				if !errors.Is(err, domain.ErrInvalid) {
					t.Fatal("live location issued a capability record")
				}
				continue
			}
			if err != nil {
				t.Fatalf("%s/%s: %v", category, stage, err)
			}
			if err := catalog.Validate(event); err != nil {
				t.Fatalf("%s/%s event not registered: %v", category, stage, err)
			}
			event.ReasonCode = "support.session"
			if err := catalog.Validate(event); !errors.Is(err, domain.ErrInvalid) {
				t.Fatal("support/operator reason accepted without a support session contract")
			}
		}
	}
}

func TestSensitiveAccessEventHoldsReferencesAndCanonicalDigest(t *testing.T) {
	scope := accessScope(t)
	item := servedDocument()
	event, err := item.Event(scope)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal([]string{"orgaudit.access.v1", accessOrg, accessAccount, accessMember, "DOCUMENT_CONTENT", "SERVED",
		"document", accessTarget, accessGrant, accessRequest, strings.Repeat("b", 64)})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	want := domain.Event{OperationID: accessRequest, ActorID: accessAccount, Action: "document.content.accessed",
		TargetType: "document", TargetID: accessTarget, ReasonCode: domain.AccessReason, RequestHash: hex.EncodeToString(sum[:])}
	if event != want {
		t.Fatalf("event %+v, want %+v", event, want)
	}
	if again, _ := item.Event(scope); again != event {
		t.Fatal("digest is not deterministic")
	}
	// Every fact, including the membership, grant and binding that the
	// generic event does not hold as columns, changes the digest.
	for name, change := range map[string]func(*domain.SensitiveAccess){
		"member":   func(a *domain.SensitiveAccess) { a.MembershipID = accessGrant },
		"grant":    func(a *domain.SensitiveAccess) { a.AccessID = accessMember },
		"binding":  func(a *domain.SensitiveAccess) { a.Binding = strings.Repeat("c", 64) },
		"account":  func(a *domain.SensitiveAccess) { a.AccountID = accessMember },
		"category": func(a *domain.SensitiveAccess) { a.Category = domain.ExportFile },
		"stage":    func(a *domain.SensitiveAccess) { a.Stage = domain.AccessIssued },
	} {
		changed := item
		change(&changed)
		other, err := changed.Event(scope)
		if err != nil || other.RequestHash == event.RequestHash {
			t.Fatalf("%s did not change the digest: %v", name, err)
		}
	}
	otherOrg, _ := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000002")
	otherScope, _ := kernel.NewOrgScope(otherOrg)
	if other, err := item.Event(otherScope); err != nil || other.RequestHash == event.RequestHash {
		t.Fatal("organization is not bound into the digest")
	}
	if _, err := item.Event(kernel.OrgScope{}); err == nil {
		t.Fatal("record without organization scope accepted")
	}
}

// BR-AUD-004/BR-PRIV-008/BR-GPS-039: no field can carry content, names, keys,
// URLs, tokens or coordinates; only UUIDs, closed codes and a 64-hex digest.
func TestSensitiveAccessRefusesContentShapedOrInconsistentFacts(t *testing.T) {
	if err := servedDocument().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*domain.SensitiveAccess){
		"category":          func(a *domain.SensitiveAccess) { a.Category = "SUPPORT_SESSION" },
		"stage":             func(a *domain.SensitiveAccess) { a.Stage = "DOWNLOADED" },
		"coordinate target": func(a *domain.SensitiveAccess) { a.TargetID = "-23.550520,-46.633308" },
		"file name target":  func(a *domain.SensitiveAccess) { a.TargetID = "receipt-2026.pdf" },
		"storage key":       func(a *domain.SensitiveAccess) { a.TargetID = "org/" + accessOrg + "/doc/" + accessTarget },
		"url binding":       func(a *domain.SensitiveAccess) { a.Binding = "https://storage.invalid/object?X-Amz-Signature=x" },
		"token grant":       func(a *domain.SensitiveAccess) { a.AccessID = "v1.synthetic-capability-token" },
		"upper binding":     func(a *domain.SensitiveAccess) { a.Binding = strings.Repeat("B", 64) },
		"short binding":     func(a *domain.SensitiveAccess) { a.Binding = strings.Repeat("b", 63) },
		"nil account":       func(a *domain.SensitiveAccess) { a.AccountID = "00000000-0000-0000-0000-000000000000" },
		"upper member":      func(a *domain.SensitiveAccess) { a.MembershipID = strings.ToUpper(accessMember) },
		"missing request":   func(a *domain.SensitiveAccess) { a.RequestID = "" },
		"served own grant":  func(a *domain.SensitiveAccess) { a.AccessID = a.RequestID },
		"live with grant":   func(a *domain.SensitiveAccess) { a.Category = domain.LocationLive },
		"live issued": func(a *domain.SensitiveAccess) {
			a.Category, a.Stage, a.AccessID = domain.LocationLive, domain.AccessIssued, a.RequestID
		},
	} {
		item := servedDocument()
		change(&item)
		if err := item.Validate(); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("%s accepted: %v", name, err)
		}
		if event, err := item.Event(accessScope(t)); !errors.Is(err, domain.ErrInvalid) || event != (domain.Event{}) {
			t.Fatalf("%s produced an event", name)
		}
	}
	// An issuance may reuse its grant as request ID; a live read is its own grant.
	for _, item := range []domain.SensitiveAccess{
		func() domain.SensitiveAccess { a := servedDocument(); a.Stage = domain.AccessIssued; return a }(),
		func() domain.SensitiveAccess {
			a := servedDocument()
			a.Stage, a.AccessID = domain.AccessIssued, a.RequestID
			return a
		}(),
		func() domain.SensitiveAccess {
			a := servedDocument()
			a.Category, a.AccessID = domain.LocationLive, a.RequestID
			return a
		}(),
	} {
		if err := item.Validate(); err != nil {
			t.Fatalf("%+v refused: %v", item, err)
		}
	}
}
