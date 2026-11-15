//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	maintpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/adapters"
	ma "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/jackc/pgx/v5"
)

// The one file-name constant of EMP-CHECK-001; the coordinator may renumber it.
const checklistMigration = "000223_organization_checklist_templates.sql"

// grantChecklistTables gives the ordinary runtime role exactly the
// EMP-CHECK-001 table rights (the README production grants).
func grantChecklistTables(t *testing.T, fx legalFixtureState) {
	t.Helper()
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON org_checklist_templates TO "+role+
		";GRANT SELECT,INSERT ON org_checklist_template_commands,org_checklist_template_versions,org_checklist_template_items,org_checklist_template_targets,org_checklist_policy_revisions TO "+role); err != nil {
		t.Fatal(err)
	}
}

// checklistStore is the asset assignment store (for assets, allocation and
// SELF facts) plus the checklist store with the real authority.
func checklistStore(t *testing.T, fx legalFixtureState) (*maintpg.ChecklistStore, *fleetpg.AssetStore) {
	t.Helper()
	assets := assignmentStore(t, fx)
	grantChecklistTables(t, fx)
	catalog, err := audit.NewCatalog(maintpg.ChecklistAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	structure := &adapters.ResourceStructure{Kind: adapters.AssetStructure, Pool: fx.f.Runtime, Allocations: assets}
	assignee := &adapters.ResourceAssignee{Pool: fx.f.Runtime, Assignments: assets}
	authority := &adapters.ChecklistAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags, Structure: structure, Assignee: assignee}
	return &maintpg.ChecklistStore{Pool: fx.f.Runtime, Authority: authority, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}, assets
}

func checklistActor(fx legalFixtureState) ma.ChecklistActor {
	return ma.ChecklistActor{AccountID: fx.actor.AccountID, SessionID: fx.actor.SessionID}
}

func checklistItems(n int) []checklist.Item {
	out := []checklist.Item{{Label: "Pneus", Critical: true, PhotoOnNotOK: true}, {Label: "Estepe", Optional: true}}
	for i := len(out); i < n; i++ {
		out = append(out, checklist.Item{Label: "Item sintético " + strings.Repeat("i", i)})
	}
	return out[:n]
}

func templateCommand(t *testing.T, v ma.TemplateCommandInput) ma.TemplateCommand {
	t.Helper()
	if v.OperationID == "" {
		v.OperationID = onboardingID(t)
	}
	c, err := ma.NewTemplateCommand(v)
	if err != nil {
		t.Fatal("invalid fixture template command", err)
	}
	return c
}

func templateRun(t *testing.T, fx legalFixtureState, s *maintpg.ChecklistStore, c ma.TemplateCommand) (ma.TemplateAck, bool, error) {
	t.Helper()
	v, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	id := ""
	if v.Kind == ma.CommandCreate {
		id = onboardingID(t)
	}
	return s.CommandTemplate(t.Context(), fx.scope, checklistActor(fx), fx.member, id, c)
}

func createTemplate(t *testing.T, fx legalFixtureState, s *maintpg.ChecklistStore, context checklist.Context, target checklist.Target) (ma.TemplateAck, error) {
	t.Helper()
	ack, _, err := templateRun(t, fx, s, templateCommand(t, ma.TemplateCommandInput{Kind: ma.CommandCreate, Context: context, Name: "Checklist sintético", Target: target, Items: checklistItems(2)}))
	return ack, err
}

func organizationTarget() checklist.Target {
	return checklist.Target{Kind: checklist.TargetOrganization}
}
func categoryTarget(ids ...string) checklist.Target {
	return checklist.Target{Kind: checklist.TargetCategories, IDs: ids}
}
func assetTarget(ids ...string) checklist.Target {
	return checklist.Target{Kind: checklist.TargetAssets, IDs: ids}
}

// checklistAsset creates an asset in a state with its own category label.
func checklistAsset(t *testing.T, fx legalFixtureState, s *fleetpg.AssetStore, unit, label string, state fleet.State) (string, string) {
	t.Helper()
	input := fleetCreateInput()
	input.UnitID = unit
	input.Category = fa.CategoryChoice{NewLabel: label}
	var existing string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT COALESCE((SELECT id::text FROM org_asset_categories WHERE org_id=$1 AND label=$2),'')`, fx.scope.OrgID().String(), label).Scan(&existing); err != nil {
		t.Fatal(err)
	}
	if existing != "" {
		input.Category = fa.CategoryChoice{ExistingID: existing}
	}
	created, _, err := fleetWrite(t, fx, s, fleetCommand(t, input))
	if err != nil {
		t.Fatal("checklist asset", err)
	}
	id, revision := created.AssetID, int64(1)
	if state != fleet.Draft {
		if _, _, err := fleetWrite(t, fx, s, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: id, ExpectedRevision: 1})); err != nil {
			t.Fatal("checklist asset activation", err)
		}
		revision++
		for _, command := range map[fleet.State][]fleet.LifecycleCommand{fleet.Inactive: {fleet.Deactivate}, fleet.Disposed: {fleet.Deactivate, fleet.Dispose}}[state] {
			result, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, lifecycleInputFor(t, fx, command, id, revision)))
			if err != nil {
				t.Fatal("checklist asset lifecycle", err)
			}
			revision = result.Revision
		}
	}
	var category string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT category_id::text FROM org_assets WHERE id=$1`, id).Scan(&category); err != nil {
		t.Fatal(err)
	}
	return id, category
}

func checklistCounts(t *testing.T, fx legalFixtureState, template string) (audits, receipts, versions, policies int) {
	t.Helper()
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND target_id=$2),(SELECT count(*) FROM org_checklist_template_commands WHERE org_id=$1 AND template_id=$2),
 (SELECT count(*) FROM org_checklist_template_versions WHERE org_id=$1 AND template_id=$2),(SELECT count(*) FROM org_checklist_policy_revisions WHERE org_id=$1 AND template_id=$2)`, fx.scope.OrgID().String(), template).Scan(&audits, &receipts, &versions, &policies); err != nil {
		t.Fatal(err)
	}
	return
}

// B-BR-EMP-330/331/333/335: create, replay, reuse, publish, policy, archive,
// stale revisions and the immutable earlier version.
func TestChecklistTemplateCommandsVersionsAndReplay(t *testing.T) {
	fx, assets, unit := lifecycleFixture(t)
	s, _ := checklistStore(t, fx)
	_, category := checklistAsset(t, fx, assets, unit, "Synthetic trucks", fleet.Active)
	create := templateCommand(t, ma.TemplateCommandInput{Kind: ma.CommandCreate, Context: checklist.Departure, Name: "Saída de caminhões", Target: categoryTarget(category), Items: checklistItems(2)})
	first, replayed, err := templateRun(t, fx, s, create)
	if err != nil || replayed || first.Revision != 1 || first.Version != 1 || first.PolicyRevision != 1 || first.State != ma.TemplateActive {
		t.Fatal("create", first, err)
	}
	again, replayed, err := templateRun(t, fx, s, create)
	if err != nil || !replayed || again != first {
		t.Fatal("create replay changed the acknowledgement", again, err)
	}
	snap, _ := create.Snapshot()
	reuse := snap
	reuse.Name = "Outro nome"
	if _, _, err := templateRun(t, fx, s, templateCommand(t, reuse)); !errors.Is(err, ma.ErrOperationReused) {
		t.Fatal("operation reuse with another intent", err)
	}
	id := first.TemplateID
	v1, err := s.ReadTemplate(t.Context(), fx.scope, checklistActor(fx), fx.member, id, 0)
	if err != nil || v1.Selected.Version != 1 || v1.Selected.Name != "Saída de caminhões" || len(v1.Selected.Items) != 2 || !v1.Selected.Items[0].Critical || v1.Policy.Policy != checklist.DefaultPolicy() || v1.Context != checklist.Departure {
		t.Fatal("read version 1", v1, err)
	}
	publish := func(revision int64, name string, items int) (ma.TemplateAck, error) {
		ack, _, err := templateRun(t, fx, s, templateCommand(t, ma.TemplateCommandInput{Kind: ma.CommandPublish, TemplateID: id, ExpectedRevision: revision, Name: name, Target: categoryTarget(category), Items: checklistItems(items)}))
		return ack, err
	}
	second, err := publish(1, "Saída revisada", 5)
	if err != nil || second.Revision != 2 || second.Version != 2 || second.PolicyRevision != 1 {
		t.Fatal("publish version 2", second, err)
	}
	if _, err := publish(1, "Stale", 3); !errors.Is(err, ma.ErrConflict) {
		t.Fatal("stale revision published", err)
	}
	// The earlier version is never rewritten (negative: prior revision rewritten after publication).
	old, err := s.ReadTemplate(t.Context(), fx.scope, checklistActor(fx), fx.member, id, 1)
	if err != nil || old.Selected.Version != 1 || old.Selected.Name != "Saída de caminhões" || len(old.Selected.Items) != 2 || old.Version != 2 {
		t.Fatal("version 1 after publication", old, err)
	}
	current, err := s.ReadTemplate(t.Context(), fx.scope, checklistActor(fx), fx.member, id, 0)
	if err != nil || current.Selected.Version != 2 || len(current.Selected.Items) != 5 || current.Selected.Name != "Saída revisada" {
		t.Fatal("current version", current, err)
	}
	if _, err := s.ReadTemplate(t.Context(), fx.scope, checklistActor(fx), fx.member, id, 3); !errors.Is(err, ma.ErrNotFound) {
		t.Fatal("future version", err)
	}
	policy, _, err := templateRun(t, fx, s, templateCommand(t, ma.TemplateCommandInput{Kind: ma.CommandPolicy, TemplateID: id, ExpectedRevision: 2, Policy: checklist.Policy{Mode: checklist.Warn, ValiditySeconds: 7200}}))
	if err != nil || policy.Revision != 3 || policy.PolicyRevision != 2 || policy.Version != 2 {
		t.Fatal("policy revision", policy, err)
	}
	withPolicy, err := s.ReadTemplate(t.Context(), fx.scope, checklistActor(fx), fx.member, id, 0)
	if err != nil || withPolicy.Policy.Revision != 2 || withPolicy.Policy.Policy.Mode != checklist.Warn || withPolicy.Policy.Policy.ValiditySeconds != 7200 || withPolicy.Selected.Version != 2 {
		t.Fatal("policy read", withPolicy, err)
	}
	var history []checklist.PolicyRevision
	rows, err := fx.f.Admin.Query(t.Context(), `SELECT revision,mode,validity_seconds,effective_at FROM org_checklist_policy_revisions WHERE org_id=$1 AND template_id=$2 ORDER BY revision`, fx.scope.OrgID().String(), id)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var r checklist.PolicyRevision
		var mode string
		if err := rows.Scan(&r.Revision, &mode, &r.Policy.ValiditySeconds, &r.EffectiveAt); err != nil {
			t.Fatal(err)
		}
		r.Policy.Mode = checklist.Mode(mode)
		history = append(history, r)
	}
	rows.Close()
	if at, ok, err := checklist.PolicyAt(history, history[1].EffectiveAt.Add(-time.Nanosecond)); err != nil || !ok || at.Revision != 1 {
		t.Fatal("stored policy history before revision 2", at, err)
	}
	if at, ok, err := checklist.PolicyAt(history, history[1].EffectiveAt); err != nil || !ok || at.Revision != 2 {
		t.Fatal("stored policy history at revision 2", at, err)
	}
	for name, bad := range map[string]checklist.Policy{"short": {Mode: checklist.Block, ValiditySeconds: 3599}, "long": {Mode: checklist.Block, ValiditySeconds: 259201}, "mode": {Mode: "ALLOW", ValiditySeconds: 7200}} {
		if _, err := ma.NewTemplateCommand(ma.TemplateCommandInput{Kind: ma.CommandPolicy, OperationID: onboardingID(t), TemplateID: id, ExpectedRevision: 3, Policy: bad}); !errors.Is(err, checklist.ErrInvalid) {
			t.Fatal("invalid answer policy accepted", name, err)
		}
	}
	archived, _, err := templateRun(t, fx, s, templateCommand(t, ma.TemplateCommandInput{Kind: ma.CommandArchive, TemplateID: id, ExpectedRevision: 3}))
	if err != nil || archived.State != ma.TemplateArchived || archived.Revision != 4 {
		t.Fatal("archive", archived, err)
	}
	if _, err := publish(4, "After archive", 2); !errors.Is(err, ma.ErrConflict) {
		t.Fatal("publish on an archived template", err)
	}
	if _, _, err := templateRun(t, fx, s, templateCommand(t, ma.TemplateCommandInput{Kind: ma.CommandArchive, TemplateID: id, ExpectedRevision: 4})); !errors.Is(err, ma.ErrConflict) {
		t.Fatal("archive twice", err)
	}
	if _, _, err := templateRun(t, fx, s, templateCommand(t, ma.TemplateCommandInput{Kind: ma.CommandArchive, TemplateID: onboardingID(t), ExpectedRevision: 1})); !errors.Is(err, ma.ErrNotFound) {
		t.Fatal("unknown template", err)
	}
	audits, receipts, versions, policies := checklistCounts(t, fx, id)
	if audits != 4 || receipts != 4 || versions != 2 || policies != 2 {
		t.Fatal("one audit event and one receipt per accepted command", audits, receipts, versions, policies)
	}
	page, err := s.ListTemplates(t.Context(), fx.scope, checklistActor(fx), fx.member, ma.TemplateFilter{Limit: 10})
	if err != nil || len(page.Templates) != 1 || page.Templates[0].State != ma.TemplateArchived || page.Templates[0].Name != "Saída revisada" || page.Templates[0].Version != 2 || page.Templates[0].ArchivedAt == nil {
		t.Fatal("template list", page, err)
	}
}

// B-BR-EMP-330/332: targets of this organization only, asset states,
// exclusivity per context, archive freeing a target, and the applicable read.
func TestChecklistTemplateTargetsAndApplicableResolution(t *testing.T) {
	fx, assets, unit := lifecycleFixture(t)
	s, _ := checklistStore(t, fx)
	truck, trucks := checklistAsset(t, fx, assets, unit, "Synthetic trucks", fleet.Active)
	otherTruck, _ := checklistAsset(t, fx, assets, unit, "Synthetic trucks", fleet.Active)
	van, vans := checklistAsset(t, fx, assets, unit, "Synthetic vans", fleet.Inactive)
	draft, _ := checklistAsset(t, fx, assets, unit, "Synthetic drafts", fleet.Draft)
	disposed, _ := checklistAsset(t, fx, assets, unit, "Synthetic trucks", fleet.Disposed)
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	var otherUnit string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, other.scope.OrgID().String()).Scan(&otherUnit); err != nil {
		t.Fatal(err)
	}
	foreignAsset, foreignCategory := checklistAsset(t, other, assets, otherUnit, "Synthetic foreign", fleet.Active)

	applicable := func(asset string, context checklist.Context) ma.ApplicableView {
		t.Helper()
		v, err := s.ReadApplicable(t.Context(), fx.scope, checklistActor(fx), fx.member, asset, context)
		if err != nil {
			t.Fatal("applicable read", err)
		}
		return v
	}
	if v := applicable(truck, checklist.Departure); v.Match != checklist.MatchNone || v.Template != nil || v.Policy != nil || v.At.IsZero() {
		t.Fatal("no template applies", v)
	}
	for label, target := range map[string]checklist.Target{"foreign category": categoryTarget(foreignCategory), "foreign asset": assetTarget(foreignAsset), "unknown category": categoryTarget(onboardingID(t)), "mixed": assetTarget(truck, foreignAsset)} {
		if _, err := createTemplate(t, fx, s, checklist.Departure, target); !errors.Is(err, ma.ErrNotFound) {
			t.Fatal("foreign or unknown target", label, err)
		}
	}
	for label, target := range map[string]checklist.Target{"draft": assetTarget(draft), "disposed": assetTarget(disposed)} {
		if _, err := createTemplate(t, fx, s, checklist.Departure, target); !errors.Is(err, checklist.ErrInvalid) {
			t.Fatal("asset not ACTIVE or INACTIVE", label, err)
		}
	}
	org, err := createTemplate(t, fx, s, checklist.Departure, organizationTarget())
	if err != nil {
		t.Fatal(err)
	}
	byCategory, err := createTemplate(t, fx, s, checklist.Departure, categoryTarget(trucks, vans))
	if err != nil {
		t.Fatal(err)
	}
	byAsset, err := createTemplate(t, fx, s, checklist.Departure, assetTarget(truck, van))
	if err != nil {
		t.Fatal("ACTIVE and INACTIVE asset targets", err)
	}
	for label, target := range map[string]checklist.Target{"organization default": organizationTarget(), "category": categoryTarget(trucks), "asset": assetTarget(van)} {
		if _, err := createTemplate(t, fx, s, checklist.Departure, target); !errors.Is(err, ma.ErrConflict) {
			t.Fatal("target taken in the same context", label, err)
		}
	}
	if _, err := createTemplate(t, fx, s, checklist.Return, categoryTarget(trucks)); err != nil {
		t.Fatal("same target in another context", err)
	}
	if v := applicable(truck, checklist.Departure); v.Match != checklist.MatchAsset || v.Template.TemplateID != byAsset.TemplateID || v.Template.Version != 1 || v.Policy.Revision != 1 || v.Policy.Policy != checklist.DefaultPolicy() || len(v.Template.Items) != 2 {
		t.Fatal("asset first", v)
	}
	if v := applicable(otherTruck, checklist.Departure); v.Match != checklist.MatchCategory || v.Template.TemplateID != byCategory.TemplateID {
		t.Fatal("category next", v)
	}
	if v := applicable(draft, checklist.Departure); v.Match != checklist.MatchOrganization || v.Template.TemplateID != org.TemplateID {
		t.Fatal("organization default", v)
	}
	if v := applicable(otherTruck, checklist.AdHoc); v.Match != checklist.MatchNone {
		t.Fatal("other context", v)
	}
	if _, err := s.ReadApplicable(t.Context(), fx.scope, checklistActor(fx), fx.member, foreignAsset, checklist.Departure); !errors.Is(err, ma.ErrNotFound) {
		t.Fatal("foreign asset applicable read", err)
	}
	// Archiving frees its targets; the next publication may take them.
	if _, _, err := templateRun(t, fx, s, templateCommand(t, ma.TemplateCommandInput{Kind: ma.CommandArchive, TemplateID: byAsset.TemplateID, ExpectedRevision: 1})); err != nil {
		t.Fatal(err)
	}
	if v := applicable(truck, checklist.Departure); v.Match != checklist.MatchCategory || v.Template.TemplateID != byCategory.TemplateID {
		t.Fatal("archived template no longer applies", v)
	}
	retaken, err := createTemplate(t, fx, s, checklist.Departure, assetTarget(van))
	if err != nil {
		t.Fatal("archived target retaken", err)
	}
	// Publishing a new version moves the target; the old one is free again.
	if _, _, err := templateRun(t, fx, s, templateCommand(t, ma.TemplateCommandInput{Kind: ma.CommandPublish, TemplateID: retaken.TemplateID, ExpectedRevision: 1, Name: "Somente o caminhão", Target: assetTarget(truck), Items: checklistItems(3)})); err != nil {
		t.Fatal("publish moves the target", err)
	}
	if v := applicable(truck, checklist.Departure); v.Match != checklist.MatchAsset || v.Template.TemplateID != retaken.TemplateID || v.Template.Version != 2 || len(v.Template.Items) != 3 {
		t.Fatal("new version pinned", v)
	}
	if v := applicable(van, checklist.Departure); v.Match != checklist.MatchCategory {
		t.Fatal("old target freed", v)
	}
	// A foreign template is not found through another organization.
	if _, err := s.ReadTemplate(t.Context(), other.scope, checklistActor(other), other.member, org.TemplateID, 0); !errors.Is(err, ma.ErrNotFound) {
		t.Fatal("foreign template read", err)
	}
	if _, _, err := s.CommandTemplate(t.Context(), other.scope, checklistActor(other), other.member, "", templateCommand(t, ma.TemplateCommandInput{Kind: ma.CommandArchive, TemplateID: org.TemplateID, ExpectedRevision: 1})); !errors.Is(err, ma.ErrNotFound) {
		t.Fatal("foreign template archive", err)
	}
	page, err := s.ListTemplates(t.Context(), other.scope, checklistActor(other), other.member, ma.TemplateFilter{Limit: 100})
	if err != nil || len(page.Templates) != 0 {
		t.Fatal("other organization lists none", page, err)
	}
	var ownCount int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_checklist_templates WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&ownCount); err != nil || ownCount != 5 {
		t.Fatal("refused commands left rows", ownCount, err)
	}
	first, err := s.ListTemplates(t.Context(), fx.scope, checklistActor(fx), fx.member, ma.TemplateFilter{Limit: 2})
	if err != nil || len(first.Templates) != 2 || first.NextID == "" {
		t.Fatal("first page", first, err)
	}
	rest, err := s.ListTemplates(t.Context(), fx.scope, checklistActor(fx), fx.member, ma.TemplateFilter{Limit: 10, AfterID: first.NextID})
	if err != nil || len(rest.Templates) != 3 || rest.NextID != "" || rest.Templates[0].TemplateID <= first.NextID {
		t.Fatal("next page", rest, err)
	}
}

// B-BR-EMP-332 under concurrency: two publications of the same target queued
// behind a held root lock; the first queued wins, the other is CONFLICT, in
// both orderings; a publish and a policy change on one revision race to one.
func TestChecklistTemplateConcurrentPublicationsTakeATargetOnce(t *testing.T) {
	fx, assets, unit := lifecycleFixture(t)
	s, _ := checklistStore(t, fx)
	_, category := checklistAsset(t, fx, assets, unit, "Synthetic raced", fleet.Active)
	org := fx.scope.OrgID().String()
	race := func(first, second func() error) (error, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		holder, err := fx.f.Runtime.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer holder.Rollback(ctx)
		if _, err := holder.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			t.Fatal(err)
		}
		if _, err := holder.Exec(ctx, `SELECT 1 FROM organizations WHERE org_id=$1 FOR UPDATE`, org); err != nil {
			t.Fatal(err)
		}
		waitBlocked := func(n int) {
			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()
			for {
				var blocked int
				if err := fx.f.Admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock' AND pg_blocking_pids(pid)<>'{}'`, fx.f.RuntimeRole).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked >= n {
					return
				}
				select {
				case <-ticker.C:
				case <-ctx.Done():
					t.Fatal("no lock wait observed")
				}
			}
		}
		results := [2]chan error{make(chan error, 1), make(chan error, 1)}
		go func() { results[0] <- first() }()
		waitBlocked(1)
		go func() { results[1] <- second() }()
		waitBlocked(2)
		if err := holder.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return <-results[0], <-results[1]
	}
	for _, context := range []checklist.Context{checklist.Departure, checklist.Return} {
		commands := [2]ma.TemplateCommand{}
		for i := range commands {
			commands[i] = templateCommand(t, ma.TemplateCommandInput{Kind: ma.CommandCreate, Context: context, Name: "Corrida sintética", Target: categoryTarget(category), Items: checklistItems(1)})
		}
		ids := [2]string{onboardingID(t), onboardingID(t)}
		run := func(i int) func() error {
			return func() error {
				_, _, err := s.CommandTemplate(t.Context(), fx.scope, checklistActor(fx), fx.member, ids[i], commands[i])
				return err
			}
		}
		order := [2]int{0, 1}
		if context == checklist.Return {
			order = [2]int{1, 0}
		}
		won, lost := race(run(order[0]), run(order[1]))
		if won != nil || !errors.Is(lost, ma.ErrConflict) {
			t.Fatal(context, "first queued publication must win", won, lost)
		}
		var winners int
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_checklist_templates WHERE org_id=$1 AND context=$2 AND id=$3`, org, string(context), ids[order[0]]).Scan(&winners); err != nil || winners != 1 {
			t.Fatal("winner stored", winners, err)
		}
		var losers int
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_checklist_templates WHERE id=$1)+(SELECT count(*) FROM org_audit_events WHERE target_id=$1)`, ids[order[1]]).Scan(&losers); err != nil || losers != 0 {
			t.Fatal("the loser left rows", losers, err)
		}
	}
	created, err := createTemplate(t, fx, s, checklist.AdHoc, organizationTarget())
	if err != nil {
		t.Fatal(err)
	}
	publish := func() error {
		_, _, err := templateRun(t, fx, s, templateCommand(t, ma.TemplateCommandInput{Kind: ma.CommandPublish, TemplateID: created.TemplateID, ExpectedRevision: 1, Name: "Publicada", Target: organizationTarget(), Items: checklistItems(2)}))
		return err
	}
	policy := func() error {
		_, _, err := templateRun(t, fx, s, templateCommand(t, ma.TemplateCommandInput{Kind: ma.CommandPolicy, TemplateID: created.TemplateID, ExpectedRevision: 1, Policy: checklist.Policy{Mode: checklist.Warn, ValiditySeconds: 3600}}))
		return err
	}
	won, lost := race(publish, policy)
	if won != nil || !errors.Is(lost, ma.ErrConflict) {
		t.Fatal("publish and policy on one revision", won, lost)
	}
	var revision, version, policyRevision int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT revision,version,policy_revision FROM org_checklist_templates WHERE id=$1`, created.TemplateID).Scan(&revision, &version, &policyRevision); err != nil || revision != 2 || version != 2 || policyRevision != 1 {
		t.Fatal("one change on one revision", revision, version, policyRevision, err)
	}
}
