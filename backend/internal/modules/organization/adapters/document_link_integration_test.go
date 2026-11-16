//go:build integration

package adapters_test

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	documentpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/adapters"
	docapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	docdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

// linkGrants adds the runtime privileges the link store and the owner
// resolvers need: the link table and the target columns they read.
func linkGrants(t *testing.T, fx deviceFixtureState) {
	t.Helper()
	documentGrants(t, fx)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_document_links TO "+role+
		";GRANT SELECT(org_id,id,state) ON org_assets TO "+role+
		";GRANT SELECT(org_id,id,author_membership_id) ON org_meter_readings TO "+role+
		";GRANT SELECT(org_id,reading_id,operation_id) ON org_meter_held_readings TO "+role+
		";GRANT SELECT(org_id,operation_id,membership_id) ON org_sync_operations TO "+role+
		";GRANT SELECT(org_id,id) ON org_employee_identities TO "+role); err != nil {
		t.Fatal(err)
	}
	// EMP-FUEL-011 (000245): the FUELING resolver reads the fueling's author.
	var fuelings bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_fuelings') IS NOT NULL`).Scan(&fuelings); err != nil {
		t.Fatal(err)
	}
	if fuelings {
		if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT(org_id,id,author_membership_id) ON org_fuelings TO "+role); err != nil {
			t.Fatal(err)
		}
	}
	// EMP-EXP-004 (000257): the EXPENSE resolver reads a direct expense's author.
	var expenses bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_expenses') IS NOT NULL`).Scan(&expenses); err != nil {
		t.Fatal(err)
	}
	if expenses {
		if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT(org_id,id,author_membership_id,origin_kind) ON org_expenses TO "+role); err != nil {
			t.Fatal(err)
		}
	}
}

// linkTargets is the production registry of linkable kinds (EDR-162).
func linkTargets() docapp.Targets {
	return docapp.Targets{docdomain.Asset: fleetpg.DocumentTargets{}, docdomain.MeterReading: fleetpg.DocumentTargets{}, docdomain.Employee: memberpg.EmployeeDocumentTargets{}}
}

type linkFixtureState struct {
	syncFixtureState
	store                        *documentpg.Store
	owner, driver, manager       docapp.Actor
	supervisor, finance          docapp.Actor
	managerEmployee              string
	activeAsset, draftAsset      string
	opening, reading, held       string
	other                        legalFixtureState
	otherOwner                   docapp.Actor
	otherEmployee, otherDocument string
}

func linkActor(t *testing.T, fx deviceFixtureState, role string) (docapp.Actor, string) {
	t.Helper()
	key, err := fx.accounts.CreateKeyAccount(t.Context(), "synlink"+onboardingID(t)[:8])
	if err != nil {
		t.Fatal(err)
	}
	login, err := fx.accounts.LoginWithKey(t.Context(), key.Key)
	if err != nil {
		t.Fatal(err)
	}
	employee, member := teamDriver(t, fx.legalFixtureState, login.Account.ID)
	matrixAssign(t, fx, matrixPerson{member: member}, role, "ORG", "ACTIVE")
	return docapp.Actor{AccountID: login.Account.ID, MembershipID: member, Channel: "web"}, employee
}

// linkFixture builds real targets: an ACTIVE and a DRAFT asset, the DRAFT
// asset's meter opening reading (authored by the organization creator), a
// stored and a held reading authored by the enrolled member, employees, and
// a second organization with its own employee and document.
func linkFixture(t *testing.T) *linkFixtureState {
	t.Helper()
	fx, meters, installed := meterEvidenceFixture(t)
	linkGrants(t, fx.deviceFixtureState)
	l := &linkFixtureState{syncFixtureState: fx, draftAsset: installed.AssetID, opening: installed.OpeningReadingID}
	assets := lifecycleStore(t, fx.legalFixtureState)
	l.activeAsset, _ = lifecycleAssetIn(t, fx.legalFixtureState, assets, allocationUnit(t, fx.legalFixtureState, "LINK"), fleet.Active)

	// A stored reading and a held reading, both authored by the enrolled member.
	recorded := meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond), 1000)
	if _, _, err := meterRecord(t, fx, meters, fx.source, recorded); err != nil {
		t.Fatal(err)
	}
	stored, err := recorded.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	l.reading = stored.ReadingID
	meters.Reviews = meterReviewStore(t, fx)
	handler, err := fleetpg.NewMeterIntakeHandler(meters.Authority, meters)
	if err != nil {
		t.Fatal(err)
	}
	var declared time.Time
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT clock_timestamp()`).Scan(&declared); err != nil {
		t.Fatal(err)
	}
	heldCommand := meterObservedCommand(t, installed, declared, 1200)
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: fx.source.MembershipID}, "FINANCE", "ORG", "ACTIVE")
	ack, err := syncpg.NewStore(meterReviewIntakeConfig(fx)).Process(t.Context(), fx.source, meterReviewOperation(t, fx, heldCommand), protocol.DefaultLimits(), handler)
	if err != nil || ack.Result() != protocol.AcceptedForReview {
		t.Fatal("held reading fixture", err)
	}
	held, err := heldCommand.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	l.held = held.ReadingID
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: fx.source.MembershipID}, "DRIVER", "SELF", "ACTIVE")
	l.driver = docapp.Actor{AccountID: fx.person.auth.Account.ID, MembershipID: fx.person.member, Channel: "web"}
	l.owner = docapp.Actor{AccountID: fx.actor.AccountID, MembershipID: fx.member, Channel: "web"}
	l.manager, l.managerEmployee = linkActor(t, fx.deviceFixtureState, "FLEET_MANAGER")
	l.supervisor, _ = linkActor(t, fx.deviceFixtureState, "SUPERVISOR")
	l.finance, _ = linkActor(t, fx.deviceFixtureState, "FINANCE")

	catalog, err := audit.NewCatalog(documentpg.AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	l.store = &documentpg.Store{Pool: fx.f.Runtime, Authority: documentpg.CurrentAuthority{Source: accesspg.NewStore(fx.f.Runtime), Now: time.Now},
		Audit: auditpg.NewStore(fx.f.Runtime, catalog), Targets: linkTargets()}

	// Another real organization of the same creator account.
	l.other = legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := l.other.onboarding.Repository.ActivateOrganization(t.Context(), l.other.scope, l.other.actor, l.other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	l.otherOwner = docapp.Actor{AccountID: fx.actor.AccountID, MembershipID: l.other.member, Channel: "web"}
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE org_id=$1 AND id=$2`, l.other.scope.OrgID().String(), l.other.member).Scan(&l.otherEmployee); err != nil {
		t.Fatal(err)
	}
	l.otherDocument = l.document(t, l.other.scope, l.otherOwner)
	return l
}

func (l *linkFixtureState) document(t *testing.T, scope kernel.OrgScope, actor docapp.Actor) string {
	t.Helper()
	command := docapp.Create{OperationID: onboardingID(t), DocumentID: onboardingID(t), Original: docapp.Declaration{Kind: docapp.KindReceipt, SHA256: strings.Repeat("ab", 32), Bytes: 1024, MediaType: docapp.MediaTypeJPEG}}
	view, _, err := l.store.Create(t.Context(), scope, actor, command)
	if err != nil {
		t.Fatal("synthetic document", err)
	}
	return view.ID
}

func (l *linkFixtureState) link(t *testing.T, actor docapp.Actor, document string, kind docdomain.TargetKind, target string) (docapp.LinkView, bool, error) {
	t.Helper()
	return l.store.Link(t.Context(), l.scope, actor, docapp.Link{OperationID: onboardingID(t), DocumentID: document, Target: docapp.TargetRef{Kind: kind, ID: target}})
}

func (l *linkFixtureState) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := l.f.Admin.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (l *linkFixtureState) links(t *testing.T) int {
	t.Helper()
	return l.count(t, `SELECT count(*) FROM org_document_links`)
}

func (l *linkFixtureState) linkEvents(t *testing.T) int {
	t.Helper()
	return l.count(t, `SELECT count(*) FROM org_audit_events WHERE action='document.link.created' AND target_type='document'`)
}

// documentRow is the whole persisted container, to prove linking changed no
// status, revision, timestamp or amount of the document.
func (l *linkFixtureState) documentRow(t *testing.T, id string) string {
	t.Helper()
	var row string
	if err := l.f.Admin.QueryRow(t.Context(), `SELECT to_jsonb(d)::text FROM org_documents d WHERE id=$1`, id).Scan(&row); err != nil {
		t.Fatal(err)
	}
	return row
}

func linkDenied(t *testing.T, err error, permission string) {
	t.Helper()
	var denied *permissions.PermissionDenial
	if !errors.As(err, &denied) || string(denied.Permission()) != permission {
		t.Fatalf("expected %s denial, got %v", permission, err)
	}
}

func TestDocumentLinksResolveRealTargetsAndCheckBothEndpoints(t *testing.T) {
	l := linkFixture(t)
	ctx := t.Context()
	driverDoc := l.document(t, l.scope, l.driver)
	before := l.documentRow(t, driverDoc)

	// BR-OFFL-015: a still-UPLOADING receipt links to the member's own reading.
	command := docapp.Link{OperationID: onboardingID(t), DocumentID: driverDoc, Target: docapp.TargetRef{Kind: docdomain.MeterReading, ID: l.reading}}
	view, replayed, err := l.store.Link(ctx, l.scope, l.driver, command)
	if err != nil || replayed || view.DocumentID != driverDoc || view.Target != command.Target || view.DocumentStorage != docdomain.Uploading ||
		view.OperationID != command.OperationID || view.CreatedBy != l.driver.MembershipID || view.CreatedAt.IsZero() {
		t.Fatal("pending receipt did not link to the member's own reading", err)
	}
	if l.links(t) != 1 || l.linkEvents(t) != 1 || l.count(t, `SELECT count(*) FROM org_document_links WHERE document_id=$1 AND target_kind='METER_READING' AND target_id=$2 AND reading_id=$2 AND held_reading_id IS NULL AND asset_id IS NULL AND employee_id IS NULL`, driverDoc, l.reading) != 1 {
		t.Fatal("link not stored once with its stored-reading reference and one event")
	}
	if l.count(t, `SELECT count(*) FROM org_audit_events WHERE operation_id=$1 AND action='document.link.created' AND target_type='document' AND target_id=$2 AND actor_id=$3`, command.OperationID, driverDoc, l.driver.AccountID) != 1 {
		t.Fatal("link event does not target the document under the creating operation")
	}
	// Identical replay returns the first link; changed facts are refused.
	again, replayed, err := l.store.Link(ctx, l.scope, l.driver, command)
	if err != nil || !replayed || again.OperationID != view.OperationID || !again.CreatedAt.Equal(view.CreatedAt) || l.links(t) != 1 || l.linkEvents(t) != 1 {
		t.Fatal("identical retry wrote again", err)
	}
	changed := command
	changed.Target.ID = l.held
	if _, _, err := l.store.Link(ctx, l.scope, l.driver, changed); !errors.Is(err, docapp.ErrOperationReused) {
		t.Fatal("same operation with another target accepted", err)
	}
	// Another operation naming the existing association returns it unchanged.
	duplicate := command
	duplicate.OperationID = onboardingID(t)
	existing, replayed, err := l.store.Link(ctx, l.scope, l.driver, duplicate)
	if err != nil || !replayed || existing.OperationID != command.OperationID || l.links(t) != 1 || l.linkEvents(t) != 1 {
		t.Fatal("duplicate link under another operation wrote a row or event", err)
	}
	// A held reading shares the reading identity and binds its own column.
	if held, replayed, err := l.link(t, l.driver, driverDoc, docdomain.MeterReading, l.held); err != nil || replayed || held.DocumentStorage != docdomain.Uploading ||
		l.count(t, `SELECT count(*) FROM org_document_links WHERE target_id=$1 AND held_reading_id=$1 AND reading_id IS NULL`, l.held) != 1 {
		t.Fatal("held reading link", err)
	}

	// The DRIVER lacks the owners' permissions for other kinds and readings.
	_, _, err = l.link(t, l.driver, driverDoc, docdomain.Asset, l.activeAsset)
	linkDenied(t, err, "fleet.asset.manage")
	_, _, err = l.link(t, l.driver, driverDoc, docdomain.Employee, l.managerEmployee)
	linkDenied(t, err, "idn.employee.manage")
	_, _, err = l.link(t, l.driver, driverDoc, docdomain.MeterReading, l.opening)
	linkDenied(t, err, "meter.reading.record")

	// FLEET_MANAGER: own receipt to an asset and to another member's reading;
	// another member's receipt is refused on the document side.
	managerDoc := l.document(t, l.scope, l.manager)
	if _, _, err := l.link(t, l.manager, managerDoc, docdomain.Asset, l.activeAsset); err != nil {
		t.Fatal("manager asset link", err)
	}
	if _, _, err := l.link(t, l.manager, managerDoc, docdomain.MeterReading, l.reading); err != nil {
		t.Fatal("manager reading link", err)
	}
	if _, _, err := l.link(t, l.manager, managerDoc, docdomain.Asset, l.draftAsset); !errors.Is(err, docapp.ErrConflict) {
		t.Fatal("discardable draft asset gained a link", err)
	}
	_, _, err = l.link(t, l.manager, managerDoc, docdomain.Employee, l.managerEmployee)
	linkDenied(t, err, "idn.employee.manage")
	_, _, err = l.link(t, l.manager, driverDoc, docdomain.Asset, l.activeAsset)
	linkDenied(t, err, "doc.document.attach")
	// SUPERVISOR holds the reading permission but not another's document;
	// FINANCE attaches any document but owns no target permission.
	_, _, err = l.link(t, l.supervisor, driverDoc, docdomain.MeterReading, l.reading)
	linkDenied(t, err, "doc.document.attach")
	_, _, err = l.link(t, l.finance, driverDoc, docdomain.Asset, l.activeAsset)
	linkDenied(t, err, "fleet.asset.manage")
	_, _, err = l.link(t, l.finance, driverDoc, docdomain.MeterReading, l.reading)
	linkDenied(t, err, "meter.reading.record")
	// OWNER links an own receipt to an employee of the register.
	ownerDoc := l.document(t, l.scope, l.owner)
	if _, _, err := l.link(t, l.owner, ownerDoc, docdomain.Employee, l.managerEmployee); err != nil {
		t.Fatal("owner employee link", err)
	}
	created := l.links(t)
	if created != 5 || l.linkEvents(t) != 5 {
		t.Fatal("unexpected link rows or events", created)
	}

	// Forged kinds, unresolvable, foreign and unregistered references.
	for name, c := range map[string]struct {
		actor    docapp.Actor
		document string
		kind     docdomain.TargetKind
		target   string
		want     error
	}{
		"asset-kind-reading-id":       {l.owner, ownerDoc, docdomain.Asset, l.reading, docapp.ErrNotFound},
		"asset-kind-employee-id":      {l.owner, ownerDoc, docdomain.Asset, l.managerEmployee, docapp.ErrNotFound},
		"employee-kind-asset-id":      {l.owner, ownerDoc, docdomain.Employee, l.activeAsset, docapp.ErrNotFound},
		"reading-kind-asset-id":       {l.owner, ownerDoc, docdomain.MeterReading, l.activeAsset, docapp.ErrNotFound},
		"missing-reading":             {l.owner, ownerDoc, docdomain.MeterReading, onboardingID(t), docapp.ErrNotFound},
		"foreign-employee":            {l.owner, ownerDoc, docdomain.Employee, l.otherEmployee, docapp.ErrNotFound},
		"foreign-document":            {l.owner, l.otherDocument, docdomain.Employee, l.managerEmployee, docapp.ErrNotFound},
		"missing-document":            {l.owner, onboardingID(t), docdomain.Employee, l.managerEmployee, docapp.ErrNotFound},
		"unregistered-fueling":        {l.owner, ownerDoc, docdomain.Fueling, onboardingID(t), docapp.ErrInvalid},
		"unregistered-expense":        {l.owner, ownerDoc, docdomain.Expense, onboardingID(t), docapp.ErrInvalid},
		"unregistered-work-order":     {l.owner, ownerDoc, docdomain.WorkOrder, onboardingID(t), docapp.ErrInvalid},
		"unregistered-trip":           {l.owner, ownerDoc, docdomain.Trip, onboardingID(t), docapp.ErrInvalid},
		"unknown-kind":                {l.owner, ownerDoc, "BOGUS", onboardingID(t), docapp.ErrInvalid},
		"lower-case-kind":             {l.owner, ownerDoc, "asset", l.activeAsset, docapp.ErrInvalid},
		"manager-forged-reading-kind": {l.manager, managerDoc, docdomain.MeterReading, l.managerEmployee, docapp.ErrNotFound},
	} {
		if _, _, err := l.link(t, c.actor, c.document, c.kind, c.target); !errors.Is(err, c.want) {
			t.Fatal("reference refusal", name, err)
		}
	}
	// A member of organization A asking inside organization B has no membership.
	if _, _, err := l.store.Link(ctx, l.other.scope, l.driver, docapp.Link{OperationID: onboardingID(t), DocumentID: l.otherDocument, Target: docapp.TargetRef{Kind: docdomain.Employee, ID: l.otherEmployee}}); !errors.Is(err, access.ErrNoMembership) && !errors.Is(err, docapp.ErrNotFound) {
		t.Fatal("foreign organization link", err)
	}
	// A document that can no longer hold an original refuses new links.
	failed := l.document(t, l.scope, l.manager)
	// EMP-DOC-014: a FAILED document passes through RECEIVED and carries
	// its closed reason (an open session cannot be abandoned yet).
	if err := unitSchemaExec(t, l.legalFixtureState, `UPDATE org_documents SET storage_state='RECEIVED',storage_revision=2 WHERE org_id=$1 AND id=$2`, l.scope.OrgID().String(), failed); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, l.legalFixtureState, `UPDATE org_documents SET storage_state='FAILED',storage_revision=3,storage_reason='INTEGRITY_MISMATCH' WHERE org_id=$1 AND id=$2`, l.scope.OrgID().String(), failed); err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.link(t, l.manager, failed, docdomain.Asset, l.activeAsset); !errors.Is(err, docapp.ErrConflict) {
		t.Fatal("failed document accepted a link", err)
	}
	if l.links(t) != created || l.linkEvents(t) != created {
		t.Fatal("refused links wrote rows or events")
	}
	// BR-DOC-003 / B-BR-EMP-242: no status, revision, timestamp or amount moved.
	if after := l.documentRow(t, driverDoc); after != before || !strings.Contains(after, `"reconciliation_state": "UNALLOCATED"`) {
		t.Fatal("linking changed the document container", before, after)
	}
	var columns string
	if err := l.f.Admin.QueryRow(ctx, `SELECT string_agg(column_name,',' ORDER BY column_name) FROM information_schema.columns WHERE table_name='org_document_links'`).Scan(&columns); err != nil ||
		columns != "asset_id,created_at,created_by,document_id,employee_id,expense_id,fueling_id,held_reading_id,incident_report_id,operation_id,org_id,reading_id,target_id,target_kind" {
		t.Fatal("link table carries an amount, content or locator column", columns, err)
	}

	// Lists: the member's own document, and a target through the owner's permission.
	page, err := l.store.DocumentLinks(ctx, l.scope, l.driver, driverDoc, docapp.DocumentLinkFilter{Limit: 1})
	if err != nil || len(page.Items) != 1 || !page.More || page.Items[0].Target.ID != l.held && page.Items[0].Target.ID != l.reading {
		t.Fatal("first document page", err)
	}
	after := page.Items[0].Target
	rest, err := l.store.DocumentLinks(ctx, l.scope, l.driver, driverDoc, docapp.DocumentLinkFilter{Limit: 1, After: &after})
	if err != nil || len(rest.Items) != 1 || rest.More || rest.Items[0].Target == after {
		t.Fatal("second document page", err)
	}
	if _, err := l.store.DocumentLinks(ctx, l.scope, l.manager, driverDoc, docapp.DocumentLinkFilter{Limit: 5}); err == nil {
		t.Fatal("another member listed a SELF document's links")
	} else {
		linkDenied(t, err, "doc.document.attach")
	}
	byReading, err := l.store.TargetLinks(ctx, l.scope, l.supervisor, docapp.TargetRef{Kind: docdomain.MeterReading, ID: l.reading}, docapp.TargetLinkFilter{Limit: 5})
	if err != nil || len(byReading.Items) != 2 || byReading.More {
		t.Fatal("supervisor reading links", err)
	}
	for _, item := range byReading.Items {
		if item.DocumentStorage != docdomain.Uploading || item.Target.ID != l.reading {
			t.Fatal("target link item", item.DocumentStorage)
		}
	}
	_, err = l.store.TargetLinks(ctx, l.scope, l.supervisor, docapp.TargetRef{Kind: docdomain.Asset, ID: l.activeAsset}, docapp.TargetLinkFilter{Limit: 5})
	linkDenied(t, err, "fleet.asset.manage")
	if _, err := l.store.TargetLinks(ctx, l.scope, l.manager, docapp.TargetRef{Kind: docdomain.Employee, ID: l.otherEmployee}, docapp.TargetLinkFilter{Limit: 5}); err == nil {
		t.Fatal("manager listed an employee without the owner permission")
	}
	if _, err := l.store.TargetLinks(ctx, l.scope, l.owner, docapp.TargetRef{Kind: docdomain.Employee, ID: l.otherEmployee}, docapp.TargetLinkFilter{Limit: 5}); !errors.Is(err, docapp.ErrNotFound) {
		t.Fatal("foreign target listed", err)
	}
	if _, err := l.store.TargetLinks(ctx, l.scope, l.owner, docapp.TargetRef{Kind: docdomain.Fueling, ID: onboardingID(t)}, docapp.TargetLinkFilter{Limit: 5}); !errors.Is(err, docapp.ErrInvalid) {
		t.Fatal("unregistered target listed", err)
	}
}

func TestDocumentLinkPortCommitsAndRollsBackWithTheConsumerTransaction(t *testing.T) {
	l := linkFixture(t)
	ctx := t.Context()
	document := l.document(t, l.scope, l.driver)
	command := docapp.Link{OperationID: onboardingID(t), DocumentID: document, Target: docapp.TargetRef{Kind: docdomain.MeterReading, ID: l.reading}}
	consumer := errors.New("synthetic consumer refusal")
	err := database.WithOrgTx(ctx, l.f.Runtime, l.scope, func(tx pgx.Tx) error {
		// A consumer (FUEL-002) writes its own record and the receipt link in
		// one transaction; its later refusal rolls both back.
		if _, replayed, err := l.store.LinkTx(ctx, tx, l.scope, l.driver, command); err != nil || replayed {
			return errors.Join(errors.New("port refused"), err)
		}
		return consumer
	})
	if !errors.Is(err, consumer) || l.links(t) != 0 || l.linkEvents(t) != 0 {
		t.Fatal("rolled-back consumer kept a link or event", err)
	}
	var view docapp.LinkView
	if err := database.WithOrgTx(ctx, l.f.Runtime, l.scope, func(tx pgx.Tx) error {
		var err error
		view, _, err = l.store.LinkTx(ctx, tx, l.scope, l.driver, command)
		return err
	}); err != nil || view.DocumentStorage != docdomain.Uploading || l.links(t) != 1 || l.linkEvents(t) != 1 {
		t.Fatal("committed consumer lost its link", err)
	}
	// The same port serves a signed-device consumer, the shape the FUEL-002
	// intake handler will pass for a receipt declared with its fueling.
	device := docapp.Actor{AccountID: l.source.Actor.AccountID, MembershipID: l.source.MembershipID, Channel: "android",
		ContributorID: l.source.Actor.ContributorID, DeviceKeyID: l.source.Actor.KeyID}
	if device.Validate() != nil {
		t.Fatal("device actor fixture")
	}
	held := command
	held.OperationID, held.Target.ID = onboardingID(t), l.held
	if err := database.WithOrgTx(ctx, l.f.Runtime, l.scope, func(tx pgx.Tx) error {
		_, replayed, err := l.store.LinkTx(ctx, tx, l.scope, device, held)
		if err == nil && replayed {
			err = errors.New("device link replayed")
		}
		return err
	}); err != nil || l.links(t) != 2 || l.linkEvents(t) != 2 {
		t.Fatal("device consumer could not link its pending receipt", err)
	}
	// The port refuses a transaction scoped to another organization and a
	// caller without a transaction, before reading anything.
	if err := database.WithOrgTx(ctx, l.f.Runtime, l.other.scope, func(tx pgx.Tx) error {
		_, _, err := l.store.LinkTx(ctx, tx, l.scope, l.driver, command)
		return err
	}); err == nil {
		t.Fatal("port accepted a transaction of another organization")
	}
	if _, _, err := l.store.LinkTx(ctx, nil, l.scope, l.driver, command); err == nil {
		t.Fatal("port accepted no transaction")
	}
	if l.links(t) != 2 || l.linkEvents(t) != 2 {
		t.Fatal("refused port calls wrote")
	}
}

func TestConcurrentDuplicateDocumentLinksWriteOneRowAndOneEvent(t *testing.T) {
	l := linkFixture(t)
	store := *l.store
	store.Pool = l.f.RuntimePool(t, 16)
	document := l.document(t, l.scope, l.driver)
	command := docapp.Link{OperationID: onboardingID(t), DocumentID: document, Target: docapp.TargetRef{Kind: docdomain.MeterReading, ID: l.reading}}
	run := func(n int, next func(int) docapp.Link) (int, []error) {
		var wg sync.WaitGroup
		var mu sync.Mutex
		firsts, errs := 0, []error{}
		start := make(chan struct{})
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				_, replayed, err := store.Link(t.Context(), l.scope, l.driver, next(i))
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					errs = append(errs, err)
				} else if !replayed {
					firsts++
				}
			}(i)
		}
		close(start)
		wg.Wait()
		return firsts, errs
	}
	// Twelve identical commands: one first write, eleven replays.
	if firsts, errs := run(12, func(int) docapp.Link { return command }); firsts != 1 || len(errs) != 0 || l.links(t) != 1 || l.linkEvents(t) != 1 {
		t.Fatal("concurrent identical links", firsts, errs)
	}
	// Eight operations naming one new association: one row and one event.
	operations := make([]string, 8)
	for i := range operations {
		operations[i] = onboardingID(t)
	}
	if firsts, errs := run(8, func(i int) docapp.Link {
		c := command
		c.OperationID, c.Target = operations[i], docapp.TargetRef{Kind: docdomain.MeterReading, ID: l.held}
		return c
	}); firsts != 1 || len(errs) != 0 || l.links(t) != 2 || l.linkEvents(t) != 2 {
		t.Fatal("concurrent duplicate associations", firsts, errs)
	}
}

func linkSQLState(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

// linkProbe inserts an optional event and one link row as the ordinary login
// in a scoped transaction that always rolls back, returning the outcome.
func (l *linkFixtureState) linkProbe(t *testing.T, scope kernel.OrgScope, event, eventAction, eventTarget string, columns string, values ...any) error {
	t.Helper()
	probe := errors.New("probe rollback")
	var outcome error
	err := database.WithOrgTx(t.Context(), l.f.Runtime, scope, func(tx pgx.Tx) error {
		if event != "" {
			if _, outcome = tx.Exec(t.Context(), `INSERT INTO org_audit_events(org_id,operation_id,actor_id,action,target_type,target_id,request_hash) VALUES($1,$2,$3,$4,'document',$5,repeat('a',64))`,
				scope.OrgID().String(), event, l.driver.AccountID, eventAction, eventTarget); outcome != nil {
				return outcome
			}
		}
		placeholders := make([]string, len(values))
		for i := range values {
			placeholders[i] = "$" + strconv.Itoa(i+1)
		}
		if _, outcome = tx.Exec(t.Context(), "INSERT INTO org_document_links("+columns+") VALUES("+strings.Join(placeholders, ",")+")", values...); outcome != nil {
			return outcome
		}
		if _, outcome = tx.Exec(t.Context(), "SET CONSTRAINTS ALL IMMEDIATE"); outcome != nil {
			return outcome
		}
		return probe
	})
	if outcome == nil && !errors.Is(err, probe) {
		return err
	}
	return outcome
}

func TestDocumentLinkSchemaGuardsScopeReferencesAndRetention(t *testing.T) {
	l := linkFixture(t)
	ctx := t.Context()
	org := l.scope.OrgID().String()
	document := l.document(t, l.scope, l.manager)
	if _, _, err := l.link(t, l.manager, document, docdomain.Asset, l.activeAsset); err != nil {
		t.Fatal(err)
	}
	var secured bool
	if err := l.f.Admin.QueryRow(ctx, `SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='org_document_links'::regclass`).Scan(&secured); err != nil || !secured {
		t.Fatal("link table lacks forced row-level security", err)
	}
	// Forced RLS: no organization setting sees and writes nothing; organization B sees nothing.
	var visible int
	if err := l.f.Runtime.QueryRow(ctx, `SELECT count(*) FROM org_document_links`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("unscoped session saw links", visible, err)
	}
	columns := "org_id,document_id,target_kind,target_id,asset_id,employee_id,reading_id,held_reading_id,operation_id,created_by"
	if _, err := l.f.Runtime.Exec(ctx, "INSERT INTO org_document_links("+columns+") VALUES($1,$2,'ASSET',$3,$3,NULL,NULL,NULL,$4,$5)", org, document, l.activeAsset, onboardingID(t), l.manager.MembershipID); linkSQLState(err) != "42501" {
		t.Fatal("unscoped link insert not refused", err)
	}
	if err := database.WithOrgTx(ctx, l.f.Runtime, l.other.scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM org_document_links`).Scan(&visible)
	}); err != nil || visible != 0 {
		t.Fatal("another organization saw links", visible, err)
	}
	failed := l.document(t, l.scope, l.manager)
	// EMP-DOC-014: a FAILED document passes through RECEIVED and carries
	// its closed reason (an open session cannot be abandoned yet).
	if err := unitSchemaExec(t, l.legalFixtureState, `UPDATE org_documents SET storage_state='RECEIVED',storage_revision=2 WHERE org_id=$1 AND id=$2`, org, failed); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, l.legalFixtureState, `UPDATE org_documents SET storage_state='FAILED',storage_revision=3,storage_reason='INTEGRITY_MISMATCH' WHERE org_id=$1 AND id=$2`, org, failed); err != nil {
		t.Fatal(err)
	}
	fresh := l.document(t, l.scope, l.manager)
	member := l.manager.MembershipID
	type row struct {
		event, action, eventTarget string
		values                     []any
		want                       string
	}
	ok := func(op string) row {
		return row{op, "document.link.created", fresh, []any{org, fresh, "ASSET", l.activeAsset, l.activeAsset, nil, nil, nil, op, member}, ""}
	}
	cases := map[string]func(string) row{
		"valid-baseline": ok,
		"binding-mismatch": func(op string) row {
			r := ok(op)
			r.values = []any{org, fresh, "ASSET", l.managerEmployee, nil, l.managerEmployee, nil, nil, op, member}
			r.want = "23514"
			return r
		},
		"unregistered-kind": func(op string) row {
			r := ok(op)
			// TRIP has no owner yet (FUELING and EXPENSE have one since EMP-FUEL-011 and EMP-EXP-004, carried only by their own columns).
			r.values = []any{org, fresh, "TRIP", l.activeAsset, l.activeAsset, nil, nil, nil, op, member}
			r.want = "23514"
			return r
		},
		"forged-asset-reference": func(op string) row {
			r := ok(op)
			r.values = []any{org, fresh, "ASSET", l.managerEmployee, l.managerEmployee, nil, nil, nil, op, member}
			r.want = "23503"
			return r
		},
		"forged-reading-reference": func(op string) row {
			r := ok(op)
			r.values = []any{org, fresh, "METER_READING", l.activeAsset, nil, nil, l.activeAsset, nil, op, member}
			r.want = "23503"
			return r
		},
		"target-differs-from-reference": func(op string) row {
			r := ok(op)
			r.values = []any{org, fresh, "ASSET", l.draftAsset, l.activeAsset, nil, nil, nil, op, member}
			r.want = "23514"
			return r
		},
		"two-reading-references": func(op string) row {
			r := ok(op)
			r.values = []any{org, fresh, "METER_READING", l.reading, nil, nil, l.reading, l.reading, op, member}
			r.want = "23514"
			return r
		},
		"no-event": func(op string) row {
			r := ok(op)
			r.event = ""
			r.want = "23503"
			return r
		},
		"other-action-event": func(op string) row {
			r := ok(op)
			r.action = "document.upload.created"
			r.want = "23503"
			return r
		},
		"other-document-event": func(op string) row {
			r := ok(op)
			r.eventTarget = document
			r.want = "23503"
			return r
		},
		"draft-asset": func(op string) row {
			r := ok(op)
			r.values = []any{org, fresh, "ASSET", l.draftAsset, l.draftAsset, nil, nil, nil, op, member}
			r.want = "23514"
			return r
		},
		"failed-document": func(op string) row {
			r := ok(op)
			r.eventTarget = failed
			r.values = []any{org, failed, "ASSET", l.activeAsset, l.activeAsset, nil, nil, nil, op, member}
			r.want = "23514"
			return r
		},
		"foreign-document": func(op string) row {
			r := ok(op)
			r.eventTarget = l.otherDocument
			r.values = []any{org, l.otherDocument, "ASSET", l.activeAsset, l.activeAsset, nil, nil, nil, op, member}
			r.want = "23503"
			return r
		},
		"foreign-employee": func(op string) row {
			r := ok(op)
			r.values = []any{org, fresh, "EMPLOYEE", l.otherEmployee, nil, l.otherEmployee, nil, nil, op, member}
			r.want = "23503"
			return r
		},
		"foreign-organization-row": func(op string) row {
			r := ok(op)
			r.values = []any{l.other.scope.OrgID().String(), fresh, "ASSET", l.activeAsset, l.activeAsset, nil, nil, nil, op, member}
			r.want = "42501"
			return r
		},
	}
	for name, build := range cases {
		r := build(onboardingID(t))
		err := l.linkProbe(t, l.scope, r.event, r.action, r.eventTarget, columns, r.values...)
		if linkSQLState(err) != r.want || r.want == "" && err != nil {
			t.Fatal("link schema", name, err)
		}
	}
	// Links are immutable and retained: the ordinary login has no UPDATE or
	// DELETE privilege, and even the owner is refused by the triggers.
	if _, err := l.f.Admin.Exec(ctx, `UPDATE org_document_links SET created_by=$1`, l.driver.MembershipID); linkSQLState(err) != "55000" {
		t.Fatal("link update not refused", err)
	}
	for _, statement := range []string{`DELETE FROM org_document_links`, `TRUNCATE org_document_links`} {
		if _, err := l.f.Admin.Exec(ctx, statement); linkSQLState(err) != "55000" {
			t.Fatal("link removal not refused", statement, err)
		}
	}
	if err := unitSchemaExec(t, l.legalFixtureState, `UPDATE org_document_links SET created_by=created_by`); linkSQLState(err) != "42501" {
		t.Fatal("ordinary login may update links", err)
	}
	if l.links(t) != 1 {
		t.Fatal("refused statements changed links")
	}
	// The asset usage source still serves an asset that carries a link.
	var usage string
	if err := database.WithOrgTx(ctx, l.f.Runtime, l.scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT org_asset_current_usage($1,$2)::text`, org, l.activeAsset).Scan(&usage)
	}); err != nil || !strings.Contains(usage, `"readings"`) {
		t.Fatal("asset usage source refused the link reference", err)
	}
}
