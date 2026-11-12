//go:build integration

package adapters_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

type legalFixtureState struct {
	f          *orgtest.Fixture
	store      *adapters.LegalStore
	scope      kernel.OrgScope
	actor      app.CompanyActor
	member     string
	onboarding app.Onboarding
	flags      *cappg.Store
}

func legalFixture(t *testing.T) legalFixtureState {
	t.Helper()
	f, _, onboarding, actor, flags := onboardingFixture(t)
	return legalFixtureFor(t, f, onboarding, actor, flags)
}
func legalFixtureFor(t *testing.T, f *orgtest.Fixture, onboarding app.Onboarding, actor app.CompanyActor, flags *cappg.Store) legalFixtureState {
	t.Helper()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE,DELETE ON org_legal_entities,org_establishments TO "+role+"; GRANT SELECT,INSERT ON org_legal_commands TO "+role); err != nil {
		t.Fatal(err)
	}
	defs := adapters.LegalAuditDefinitions()
	catalog, err := audit.NewCatalog(defs)
	if err != nil {
		t.Fatal(err)
	}
	store := &adapters.LegalStore{Pool: f.Runtime, Memberships: memberpg.NewStore(f.Runtime), Roles: accesspg.NewStore(f.Runtime), Audit: auditpg.NewStore(f.Runtime, catalog), Capabilities: flags}
	created, _, err := onboarding.Create(t.Context(), actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	scope := scopeFor(t, created.OrgID.String())
	if _, err := flags.Set(t.Context(), scope, cap.Actor{AccountID: actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	return legalFixtureState{f, store, scope, actor, created.MembershipID, onboarding, flags}
}
func legalPlan(t *testing.T, c app.LegalWrite) app.LegalPlan {
	t.Helper()
	v, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var plan app.LegalPlan
	if v.Kind == app.DeclareLegal {
		plan.EntityID = onboardingID(t)
		if v.Declaration.InputForStorage().Kind == d.Company {
			plan.EstablishmentID = onboardingID(t)
		}
	} else if v.Kind == app.AddLegalEstablishment {
		plan.EstablishmentID = onboardingID(t)
	}
	return plan
}
func legalCommand(t *testing.T, operation string, natural bool, n int) app.LegalWrite {
	t.Helper()
	input := d.LegalDeclarationInput{Kind: d.Company, LegalName: "Synthetic company declaration", Identifier: "EXEMPL01000170", Establishment: d.EstablishmentInput{Role: d.NotInformed, TradeName: "Synthetic trade", MunicipalityCode: "5103403"}}
	if natural {
		input = d.LegalDeclarationInput{Kind: d.NaturalPerson, LegalName: "Synthetic private party", Identifier: syntheticLegalCPF(n), Purpose: d.VerificationAndAgreements}
	}
	declaration, err := d.NewLegalDeclaration(operation, input)
	if err != nil {
		t.Fatal(err)
	}
	command, err := app.DeclareLegalCommand(declaration)
	if err != nil {
		t.Fatal(err)
	}
	return command
}
func (fx legalFixtureState) write(t *testing.T, c app.LegalWrite) (app.LegalResult, bool, error) {
	t.Helper()
	return fx.store.WriteLegal(t.Context(), fx.scope, fx.actor, fx.member, legalPlan(t, c), c)
}
func TestLegalStoreDeclaresReplaysMasksAndAuditsIdentifierRead(t *testing.T) {
	fx := legalFixture(t)
	command := legalCommand(t, onboardingID(t), true, 8)
	first, replay, err := fx.write(t, command)
	if err != nil || replay || first.State != "DECLARED" || first.Revision != 1 || first.EstablishmentID != "" {
		t.Fatal("natural declaration failed", err)
	}
	repeated, replay, err := fx.write(t, command)
	if err != nil || !replay || repeated != first {
		t.Fatal("legal replay changed acknowledgement", err)
	}
	value, err := fx.store.ReadLegal(t.Context(), fx.scope, fx.actor, fx.member, first.EntityID, false, "")
	if err != nil || value.Revealed || value.Entity.MaskedIdentifier(fx.scope) == syntheticLegalCPF(8) {
		t.Fatal("unsafe default view", err)
	}
	reveal, err := fx.store.ReadLegal(t.Context(), fx.scope, fx.actor, fx.member, first.EntityID, true, onboardingID(t))
	if err != nil || !reveal.Revealed || reveal.AuditOperationID == "" {
		t.Fatal("identifier read not audited", err)
	}
	snap, err := reveal.Entity.Snapshot(fx.scope)
	if err != nil || snap.Identifier != syntheticLegalCPF(8) {
		t.Fatal("fresh manager read lost identifier", err)
	}
	duplicate, wasReplay, err := fx.write(t, legalCommand(t, onboardingID(t), true, 8))
	if !errors.Is(err, app.ErrLegalDuplicate) || wasReplay || duplicate != (app.LegalResult{}) {
		t.Fatal("same-org duplicate accepted", err)
	}
	page, err := fx.store.ListLegal(t.Context(), fx.scope, fx.actor, fx.member, app.LegalFilter{Limit: 20})
	if err != nil || len(page.Entities) != 1 {
		t.Fatal("legal list", err)
	}
	other, _, err := fx.onboarding.Create(t.Context(), fx.actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	otherScope := scopeFor(t, other.OrgID.String())
	if _, err := fx.flags.Set(t.Context(), otherScope, cap.Actor{AccountID: fx.actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	otherCommand := legalCommand(t, onboardingID(t), true, 8)
	if _, _, err := fx.store.WriteLegal(t.Context(), otherScope, fx.actor, other.MembershipID, legalPlan(t, otherCommand), otherCommand); err != nil {
		t.Fatal("foreign declaration affected independent org", err)
	}
	if _, err := fx.store.FindLegalEntity(t.Context(), otherScope, first.EntityID); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("foreign legal lookup exposed entity", err)
	}
	var payload string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT COALESCE(string_agg(to_jsonb(e)::text,''),'') FROM org_audit_events e`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payload, syntheticLegalCPF(8)) || strings.Contains(payload, "Synthetic private party") {
		t.Fatal("audit contains personal content")
	}
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT COALESCE(string_agg(to_jsonb(e)::text,''),'') FROM org_legal_commands e`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payload, syntheticLegalCPF(8)) || strings.Contains(payload, "Synthetic private party") {
		t.Fatal("receipt contains personal content")
	}
}
func TestLegalStoreUsesRealForeignKeysToRemoveOrRevoke(t *testing.T) {
	for _, retained := range []bool{false, true} {
		name := "unreferenced removal"
		if retained {
			name = "deferred referenced revocation"
		}
		t.Run(name, func(t *testing.T) {
			fx := legalFixture(t)
			created, _, err := fx.write(t, legalCommand(t, onboardingID(t), false, 0))
			if err != nil {
				t.Fatal(err)
			}
			if retained {
				if _, err := fx.f.Admin.Exec(t.Context(), `CREATE TABLE synthetic_legal_references(org_id UUID NOT NULL,id UUID PRIMARY KEY,establishment_id UUID NOT NULL,FOREIGN KEY(org_id,establishment_id) REFERENCES org_establishments(org_id,id) DEFERRABLE INITIALLY DEFERRED)`); err != nil {
					t.Fatal(err)
				}
				if _, err := fx.f.Admin.Exec(t.Context(), `INSERT INTO synthetic_legal_references(org_id,id,establishment_id) VALUES($1,$2,$3)`, fx.scope.OrgID().String(), onboardingID(t), created.EstablishmentID); err != nil {
					t.Fatal(err)
				}
			}
			withdrawal, err := app.WithdrawLegalCommand(created.EntityID, onboardingID(t), 1)
			if err != nil {
				t.Fatal(err)
			}
			result, replay, err := fx.write(t, withdrawal)
			if err != nil || replay {
				t.Fatal("withdrawal failed", err)
			}
			repeated, replay, err := fx.write(t, withdrawal)
			if err != nil || !replay || repeated != result {
				t.Fatal("withdrawal retry changed result", err)
			}
			if retained {
				if result.State != "REVOKED" || result.Revision != 2 {
					t.Fatal("referenced declaration removed")
				}
				if _, err := fx.store.FindEstablishment(t.Context(), fx.scope, created.EstablishmentID); err != nil {
					t.Fatal("cascade not rolled back", err)
				}
				if _, _, err := fx.write(t, legalCommand(t, onboardingID(t), false, 0)); err != nil {
					t.Fatal("revoked history prevented explicit new declaration", err)
				}
			} else {
				if result.State != "REMOVED" {
					t.Fatal("unreferenced content retained")
				}
				if _, err := fx.store.FindLegalEntity(t.Context(), fx.scope, created.EntityID); !errors.Is(err, app.ErrNotFound) {
					t.Fatal("removed declaration remains", err)
				}
			}
		})
	}
}
func TestLegalStoreSerializesDuplicateAndReplayRaces(t *testing.T) {
	for _, same := range []bool{false, true} {
		name := "distinct operations"
		if same {
			name = "same operation"
		}
		t.Run(name, func(t *testing.T) {
			fx := legalFixture(t)
			operation := onboardingID(t)
			const n = 12
			commands := make([]app.LegalWrite, n)
			plans := make([]app.LegalPlan, n)
			for i := range n {
				if !same {
					operation = onboardingID(t)
				}
				commands[i] = legalCommand(t, operation, true, 12)
				plans[i] = legalPlan(t, commands[i])
			}
			type outcome struct {
				result app.LegalResult
				replay bool
				err    error
			}
			outcomes := make(chan outcome, n)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range n {
				wg.Go(func() {
					<-start
					result, replay, err := fx.store.WriteLegal(t.Context(), fx.scope, fx.actor, fx.member, plans[i], commands[i])
					outcomes <- outcome{result, replay, err}
				})
			}
			close(start)
			wg.Wait()
			close(outcomes)
			fresh, repeated, refused := 0, 0, 0
			var original app.LegalResult
			for got := range outcomes {
				if got.err != nil {
					if !errors.Is(got.err, app.ErrLegalDuplicate) || got.replay || got.result != (app.LegalResult{}) {
						t.Fatal("unsafe duplicate result", got.err)
					}
					refused++
					continue
				}
				if got.replay {
					repeated++
				} else {
					fresh++
				}
				if original == (app.LegalResult{}) {
					original = got.result
				} else if same && original != got.result {
					t.Fatal("racing retry changed result")
				}
			}
			if same {
				if fresh != 1 || repeated != 11 || refused != 0 {
					t.Fatal(fresh, repeated, refused)
				}
			} else if fresh != 1 || refused != 11 || repeated != 0 {
				t.Fatal(fresh, repeated, refused)
			}
			var consistent bool
			if err := fx.f.Admin.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_legal_entities)=1 AND (SELECT count(*) FROM org_legal_commands)=1 AND (SELECT count(*) FROM org_audit_events WHERE action='org.legal_entity.declared')=1`).Scan(&consistent); err != nil || !consistent {
				t.Fatal("partial duplicate composition", err)
			}
		})
	}
}
func TestLegalStoreFailedCommitNeverReturnsContentOrSuccess(t *testing.T) {
	fx := legalFixture(t)
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_legal_commit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic private failure'; END $$;
 CREATE CONSTRAINT TRIGGER synthetic_legal_commit_failure AFTER INSERT ON org_legal_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_legal_commit_failure()`); err != nil {
		t.Fatal(err)
	}
	result, replay, err := fx.write(t, legalCommand(t, onboardingID(t), false, 0))
	if !errors.Is(err, app.ErrUnavailable) || replay || result != (app.LegalResult{}) {
		t.Fatal("failed commit leaked result", err)
	}
	for _, table := range []string{"org_legal_entities", "org_establishments", "org_legal_commands"} {
		var count int
		if err := fx.f.Admin.QueryRow(t.Context(), "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil || count != 0 {
			t.Fatal("failed commit retained rows", table, err)
		}
	}
	var count int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_audit_events WHERE action='org.legal_entity.declared'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed commit retained audit", err)
	}
}

func TestLegalEstablishmentChangesUseCurrentParentAndCAS(t *testing.T) {
	fx := legalFixture(t)
	created, _, err := fx.write(t, legalCommand(t, onboardingID(t), false, 0))
	if err != nil {
		t.Fatal(err)
	}
	edit, err := app.ChangeEstablishmentCommand(created.EntityID, created.EstablishmentID, onboardingID(t), 1, d.EstablishmentMetadata{Role: d.HeadOffice, TradeName: "Updated synthetic trade", MunicipalityCode: "3550308"})
	if err != nil {
		t.Fatal(err)
	}
	result, replay, err := fx.write(t, edit)
	if err != nil || replay || result.Revision != 2 {
		t.Fatal("establishment metadata failed", err)
	}
	repeated, replay, err := fx.write(t, edit)
	if err != nil || !replay || repeated != result {
		t.Fatal("metadata retry changed acknowledgement", err)
	}
	stale, err := app.ChangeEstablishmentCommand(created.EntityID, created.EstablishmentID, onboardingID(t), 1, d.EstablishmentMetadata{Role: d.Branch})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fx.write(t, stale); !errors.Is(err, d.ErrConflict) {
		t.Fatal("stale metadata accepted", err)
	}
	changed, err := fx.store.FindEstablishment(t.Context(), fx.scope, created.EstablishmentID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := changed.Snapshot(fx.scope)
	if err != nil || snapshot.CNPJ != "EXEMPL01000170" || snapshot.Role != d.HeadOffice {
		t.Fatal("metadata rewrote identifier", err)
	}
	page, err := fx.store.ListEstablishments(t.Context(), fx.scope, fx.actor, fx.member, created.EntityID, app.LegalFilter{Limit: 20})
	if err != nil || len(page.Establishments) != 1 {
		t.Fatal("establishment list", err)
	}
	foreign, err := app.ChangeEstablishmentCommand(onboardingID(t), created.EstablishmentID, onboardingID(t), 2, d.EstablishmentMetadata{Role: d.Branch})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fx.write(t, foreign); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("foreign parent adopted establishment", err)
	}
}
func TestLegalStoreRechecksProofAndCapabilityAtTransactionEntry(t *testing.T) {
	for _, test := range []string{"revoked cookie", "revoked family", "elapsed freshness", "disabled capability"} {
		t.Run(test, func(t *testing.T) {
			fx := legalFixture(t)
			created, _, err := fx.write(t, legalCommand(t, onboardingID(t), true, 42))
			if err != nil {
				t.Fatal(err)
			}
			switch test {
			case "revoked cookie":
				_, err = fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET revoked_at=clock_timestamp() WHERE account_id=$1`, fx.actor.AccountID)
			case "revoked family":
				_, err = fx.f.Admin.Exec(t.Context(), `UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE account_id=$1`, fx.actor.AccountID)
			case "elapsed freshness":
				_, err = fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=clock_timestamp()-interval '17 minutes',created_at=clock_timestamp()-interval '16 minutes',expires_at=clock_timestamp()+interval '7 hours' WHERE account_id=$1`, fx.actor.AccountID)
			case "disabled capability":
				_, err = fx.flags.Set(t.Context(), fx.scope, cap.Actor{AccountID: fx.actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1})
			}
			if err != nil {
				t.Fatal(err)
			}
			result, replay, err := fx.write(t, legalCommand(t, onboardingID(t), true, 43))
			if err == nil || replay || result != (app.LegalResult{}) {
				t.Fatal("stale authority/capability wrote declaration")
			}
			revealed, err := fx.store.ReadLegal(t.Context(), fx.scope, fx.actor, fx.member, created.EntityID, true, onboardingID(t))
			if test == "disabled capability" {
				if err != nil || !revealed.Revealed {
					t.Fatal("capability disable removed authorized existing read", err)
				}
			} else if err == nil || revealed != (app.LegalReadResult{}) {
				t.Fatal("invalid proof exposed identifier", err)
			}
			if test == "elapsed freshness" {
				normal, err := fx.store.ReadLegal(t.Context(), fx.scope, fx.actor, fx.member, created.EntityID, false, "")
				if err != nil || normal.Revealed {
					t.Fatal("ordinary live masked read rejected", err)
				}
			}
		})
	}
}
func TestLegalRevealReturnsNoContentAfterAuditCommitFailure(t *testing.T) {
	fx := legalFixture(t)
	created, _, err := fx.write(t, legalCommand(t, onboardingID(t), true, 44))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_sensitive_read_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic audit outage'; END $$;
 CREATE CONSTRAINT TRIGGER synthetic_sensitive_read_failure AFTER INSERT ON org_audit_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN(NEW.action='org.legal_entity.identifier_read') EXECUTE FUNCTION synthetic_sensitive_read_failure()`); err != nil {
		t.Fatal(err)
	}
	revealed, err := fx.store.ReadLegal(t.Context(), fx.scope, fx.actor, fx.member, created.EntityID, true, onboardingID(t))
	if !errors.Is(err, app.ErrUnavailable) || revealed != (app.LegalReadResult{}) {
		t.Fatal("unaudited content returned", err)
	}
}

func syntheticLegalCNPJ(base string) string {
	value := base
	for len(value) < 14 {
		sum, weight := 0, 2
		for i := len(value) - 1; i >= 0; i-- {
			sum += int(value[i]-48) * weight
			weight++
			if weight == 10 {
				weight = 2
			}
		}
		digit := 0
		if remainder := sum % 11; remainder >= 2 {
			digit = 11 - remainder
		}
		value += string(byte('0' + digit))
	}
	return value
}
func TestLegalAdditionalEstablishmentIsScopedToItsActualCompanyRoot(t *testing.T) {
	fx := legalFixture(t)
	created, _, err := fx.write(t, legalCommand(t, onboardingID(t), false, 0))
	if err != nil {
		t.Fatal(err)
	}
	cnpj := syntheticLegalCNPJ("EXEMPL010002")
	input := d.EstablishmentInput{CNPJ: cnpj, Role: d.HeadOffice, TradeName: "Second synthetic establishment", MunicipalityCode: "5103403"}
	command, err := app.AddEstablishmentCommand(created.EntityID, onboardingID(t), 1, input)
	if err != nil {
		t.Fatal(err)
	}
	added, replay, err := fx.write(t, command)
	if err != nil || replay || added.EstablishmentID == created.EstablishmentID || added.Revision != 1 {
		t.Fatal("additional establishment failed", err)
	}
	repeated, replay, err := fx.write(t, command)
	if err != nil || !replay || repeated != added {
		t.Fatal("additional establishment retry changed identity", err)
	}
	value, err := fx.store.FindEstablishment(t.Context(), fx.scope, added.EstablishmentID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := value.Snapshot(fx.scope)
	if err != nil || snapshot.CNPJ != cnpj || snapshot.Role != d.HeadOffice || snapshot.EntityID != created.EntityID {
		t.Fatal("declared role/root lost", err)
	}
	duplicate, err := app.AddEstablishmentCommand(created.EntityID, onboardingID(t), 1, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fx.write(t, duplicate); !errors.Is(err, app.ErrLegalDuplicate) {
		t.Fatal("duplicate establishment accepted", err)
	}
	input.CNPJ = "EXEMPL02000114"
	different, err := app.AddEstablishmentCommand(created.EntityID, onboardingID(t), 1, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fx.write(t, different); !errors.Is(err, d.ErrInvalidCNPJ) {
		t.Fatal("different company root adopted", err)
	}
	firstPage, err := fx.store.ListEstablishments(t.Context(), fx.scope, fx.actor, fx.member, created.EntityID, app.LegalFilter{Limit: 1})
	if err != nil || len(firstPage.Establishments) != 1 || firstPage.NextID == "" {
		t.Fatal("establishment keyset first page", err)
	}
	secondPage, err := fx.store.ListEstablishments(t.Context(), fx.scope, fx.actor, fx.member, created.EntityID, app.LegalFilter{Limit: 1, AfterID: firstPage.NextID})
	if err != nil || len(secondPage.Establishments) != 1 || secondPage.NextID != "" {
		t.Fatal("establishment keyset second page", err)
	}
}

func TestLegalTargetReferenceRetainsOnlyScopedReplayIdentityAfterRemoval(t *testing.T) {
	fx := legalFixture(t)
	created, _, err := fx.write(t, legalCommand(t, onboardingID(t), false, 0))
	if err != nil {
		t.Fatal(err)
	}
	withdrawal, err := app.WithdrawLegalCommand(created.EntityID, onboardingID(t), 1)
	if err != nil {
		t.Fatal(err)
	}
	if result, _, err := fx.write(t, withdrawal); err != nil || result.State != "REMOVED" {
		t.Fatal("withdrawal failed", err)
	}
	for _, establishment := range []string{"", created.EstablishmentID} {
		found, err := fx.store.LegalTargetExists(t.Context(), fx.scope, created.EntityID, establishment)
		if err != nil || !found {
			t.Fatal("scoped acknowledgement identity lost", err)
		}
		found, err = fx.store.LegalTargetExists(t.Context(), scopeFor(t, onboardingID(t)), created.EntityID, establishment)
		if err != nil || found {
			t.Fatal("foreign acknowledgement exposed", err)
		}
		found, err = fx.store.LegalTargetExists(t.Context(), fx.scope, onboardingID(t), establishment)
		if err != nil || found {
			t.Fatal("wrong parent acknowledgement exposed", err)
		}
	}
	if _, err := fx.store.FindLegalEntity(t.Context(), fx.scope, created.EntityID); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("removed private content survived", err)
	}
	if _, err := fx.store.FindEstablishment(t.Context(), fx.scope, created.EstablishmentID); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("removed establishment survived", err)
	}
}
