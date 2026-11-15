// Package adapters persists checklist templates through ordinary scoped
// transactions (EMP-CHECK-001, migration 000223).
package adapters

import (
	"context"
	"errors"
	"reflect"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/maintenance"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TemplateTargetType is the audit target vocabulary of a template.
const TemplateTargetType = "checklist_template"

var auditActions = map[app.CommandKind]string{
	app.CommandCreate:  "check.template.created",
	app.CommandPublish: "check.template.published",
	app.CommandPolicy:  "check.template.policy_changed",
	app.CommandArchive: "check.template.archived",
}

// ChecklistAuditDefinitions registers one action per command kind.
func ChecklistAuditDefinitions() []audit.Definition {
	out := []audit.Definition{}
	for _, kind := range []app.CommandKind{app.CommandCreate, app.CommandPublish, app.CommandPolicy, app.CommandArchive} {
		out = append(out, audit.Definition{Action: auditActions[kind], TargetType: TemplateTargetType})
	}
	return out
}

// ChecklistStore owns the template commands and reads.
type ChecklistStore struct {
	Pool      *pgxpool.Pool
	Authority app.ChecklistAuthority
	Audit     auditing.TransactionRunner
}

var _ app.ChecklistRepository = (*ChecklistStore)(nil)

func missing(port any) bool {
	if port == nil {
		return true
	}
	v := reflect.ValueOf(port)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan:
		return v.IsNil()
	}
	return false
}

func (s *ChecklistStore) ready(write bool) bool {
	return s != nil && s.Pool != nil && !missing(s.Authority) && (!write || !missing(s.Audit))
}

// storeError keeps only registered refusals; provider details stay private.
func storeError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, audit.ErrReused) {
		return app.ErrOperationReused
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == "org_checklist_templates_target_taken" {
		return app.ErrConflict
	}
	for _, known := range []error{access.ErrNoMembership, access.ErrMembershipSuspended, access.ErrOrgSuspended, access.ErrOrgArchived, access.ErrSetupIncomplete, access.ErrFreshAuthRequired, access.ErrEntitlementRequired, access.ErrUnauthenticated, permission.ErrPermissionDenied, permission.ErrResourceNotFound,
		app.ErrOperationReused, app.ErrNotFound, app.ErrUnavailable, app.ErrUnauthenticated, app.ErrConflict, checklist.ErrInvalid} {
		if errors.Is(err, known) {
			return err
		}
	}
	if code, ok := orgapi.Registered(err); ok {
		return code
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return app.ErrUnavailable
}

func validCall(scope kernel.OrgScope, actor app.ChecklistActor, member string) error {
	if scope.Validate() != nil || !checklist.CanonicalID(member) {
		return checklist.ErrInvalid
	}
	return actor.Validate()
}

func instant(v pgtype.Timestamptz) (time.Time, error) {
	if !v.Valid || v.InfinityModifier != pgtype.Finite {
		return time.Time{}, app.ErrUnavailable
	}
	return v.Time.UTC(), nil
}

func optionalInstant(v pgtype.Timestamptz) (*time.Time, error) {
	if !v.Valid {
		return nil, nil
	}
	at, err := instant(v)
	if err != nil {
		return nil, err
	}
	return &at, nil
}

// checkTargets proves every target identifier belongs to this organization
// (else not found) and that asset targets accept a checklist.
func checkTargets(ctx context.Context, queries *q.Queries, org string, target checklist.Target) error {
	switch target.Kind {
	case checklist.TargetCategories:
		known, err := queries.ChecklistKnownCategories(ctx, q.ChecklistKnownCategoriesParams{OrgID: org, Ids: target.IDs})
		if err != nil {
			return err
		}
		if known != int64(len(target.IDs)) {
			return app.ErrNotFound
		}
	case checklist.TargetAssets:
		rows, err := queries.ChecklistTargetAssets(ctx, q.ChecklistTargetAssetsParams{OrgID: org, Ids: target.IDs})
		if err != nil {
			return err
		}
		if len(rows) != len(target.IDs) {
			return app.ErrNotFound
		}
		for _, r := range rows {
			if r.State != "ACTIVE" && r.State != "INACTIVE" {
				return checklist.ErrInvalid
			}
		}
	}
	return nil
}

// publish writes one complete version after its target checks; the head
// already names it (the 000223 sequence guard).
func publish(ctx context.Context, queries *q.Queries, org, template string, version int32, moment checklist.Context, d checklist.Definition, operation, actor string) error {
	ids := d.Target.IDs
	if ids == nil {
		ids = []string{}
	}
	taken, err := queries.ChecklistTargetTaken(ctx, q.ChecklistTargetTakenParams{OrgID: org, TemplateID: template, Context: string(moment), TargetKind: string(d.Target.Kind), Ids: ids})
	if err != nil {
		return err
	}
	if taken {
		return app.ErrConflict
	}
	if err := queries.InsertChecklistVersion(ctx, q.InsertChecklistVersionParams{OrgID: org, TemplateID: template, Version: version, Name: d.Name, TargetKind: string(d.Target.Kind), TargetCount: int32(len(ids)), ItemCount: int32(len(d.Items)), OperationID: operation, PublishedBy: actor}); err != nil {
		return err
	}
	items := q.InsertChecklistItemsParams{OrgID: org, TemplateID: template, Version: version}
	for _, item := range d.Items {
		items.Labels = append(items.Labels, item.Label)
		items.Criticals = append(items.Criticals, item.Critical)
		items.Optionals = append(items.Optionals, item.Optional)
		items.Photos = append(items.Photos, item.PhotoOnNotOK)
	}
	if err := queries.InsertChecklistItems(ctx, items); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	return queries.InsertChecklistTargets(ctx, q.InsertChecklistTargetsParams{OrgID: org, TemplateID: template, Version: version, Categories: d.Target.Kind == checklist.TargetCategories, Ids: ids})
}

// CommandTemplate runs one command under the organization root lock:
// authority, receipt replay, exactly one audit event, the effect, the receipt
// and authority again, in one transaction.
func (s *ChecklistStore) CommandTemplate(ctx context.Context, scope kernel.OrgScope, actor app.ChecklistActor, member, newID string, command app.TemplateCommand) (app.TemplateAck, bool, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.TemplateAck{}, false, err
	}
	in, err := command.Snapshot()
	if err != nil {
		return app.TemplateAck{}, false, err
	}
	template := in.TemplateID
	if in.Kind == app.CommandCreate {
		template = newID
	}
	if !checklist.CanonicalID(template) || in.Kind != app.CommandCreate && newID != "" {
		return app.TemplateAck{}, false, checklist.ErrInvalid
	}
	if !s.ready(true) {
		return app.TemplateAck{}, false, app.ErrUnavailable
	}
	org, digest := scope.OrgID().String(), command.Digest(scope)
	var ack app.TemplateAck
	replayed := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckChecklistTx(ctx, tx, scope, actor, member, org, app.TemplateManage); err != nil {
			return err
		}
		queries := q.New(tx)
		state, err := queries.ChecklistRootLock(ctx, org)
		if err != nil {
			return err
		}
		if !state.Valid || state.String != "ACTIVE" {
			return access.ErrSetupIncomplete
		}
		prior, err := queries.ChecklistCommandReceipt(ctx, q.ChecklistCommandReceiptParams{OrgID: org, OperationID: in.OperationID})
		if err == nil {
			if prior.ActorID != actor.AccountID || prior.Kind != string(in.Kind) || prior.RequestHash != digest || in.Kind != app.CommandCreate && prior.TemplateID != in.TemplateID {
				return app.ErrOperationReused
			}
			ack = app.TemplateAck{TemplateID: prior.TemplateID, Revision: prior.ResultRevision, Version: int64(prior.ResultVersion), PolicyRevision: int64(prior.ResultPolicyRevision), State: app.TemplateState(prior.ResultState)}
			replayed = true
			return s.Authority.CheckChecklistTx(ctx, tx, scope, actor, member, org, app.TemplateManage)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		event := audit.Event{OperationID: in.OperationID, ActorID: actor.AccountID, Action: auditActions[in.Kind], TargetType: TemplateTargetType, TargetID: template, RequestHash: digest}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			queries := q.New(tx)
			if in.Kind == app.CommandCreate {
				d, err := command.Definition(in.Context)
				if err != nil {
					return err
				}
				if err := checkTargets(ctx, queries, org, d.Target); err != nil {
					return err
				}
				if err := queries.InsertChecklistTemplate(ctx, q.InsertChecklistTemplateParams{OrgID: org, ID: template, Context: string(in.Context), OperationID: in.OperationID, CreatedBy: actor.AccountID}); err != nil {
					return err
				}
				if err := publish(ctx, queries, org, template, 1, in.Context, d, in.OperationID, actor.AccountID); err != nil {
					return err
				}
				if err := queries.InsertChecklistPolicy(ctx, q.InsertChecklistPolicyParams{OrgID: org, TemplateID: template, Revision: 1, Mode: string(in.Policy.Mode), ValiditySeconds: int32(in.Policy.ValiditySeconds), OperationID: in.OperationID, SetBy: actor.AccountID}); err != nil {
					return err
				}
				ack = app.TemplateAck{TemplateID: template, Revision: 1, Version: 1, PolicyRevision: 1, State: app.TemplateActive}
			} else {
				head, err := queries.ChecklistTemplateForUpdate(ctx, q.ChecklistTemplateForUpdateParams{OrgID: org, ID: template})
				if errors.Is(err, pgx.ErrNoRows) {
					return app.ErrNotFound
				}
				if err != nil {
					return err
				}
				if head.State != string(app.TemplateActive) || head.Revision != in.ExpectedRevision {
					return app.ErrConflict
				}
				ack = app.TemplateAck{TemplateID: template, Revision: head.Revision + 1, Version: int64(head.Version), PolicyRevision: int64(head.PolicyRevision), State: app.TemplateActive}
				var changed int64
				switch in.Kind {
				case app.CommandPublish:
					moment := checklist.Context(head.Context)
					d, err := command.Definition(moment)
					if err != nil {
						return err
					}
					if err := checkTargets(ctx, queries, org, d.Target); err != nil {
						return err
					}
					if changed, err = queries.AdvanceChecklistVersion(ctx, q.AdvanceChecklistVersionParams{OrgID: org, ID: template, Revision: head.Revision}); err != nil {
						return err
					}
					if changed != 1 {
						return app.ErrConflict
					}
					if err := publish(ctx, queries, org, template, head.Version+1, moment, d, in.OperationID, actor.AccountID); err != nil {
						return err
					}
					ack.Version++
				case app.CommandPolicy:
					if changed, err = queries.AdvanceChecklistPolicy(ctx, q.AdvanceChecklistPolicyParams{OrgID: org, ID: template, Revision: head.Revision}); err != nil {
						return err
					}
					if changed != 1 {
						return app.ErrConflict
					}
					if err := queries.InsertChecklistPolicy(ctx, q.InsertChecklistPolicyParams{OrgID: org, TemplateID: template, Revision: head.PolicyRevision + 1, Mode: string(in.Policy.Mode), ValiditySeconds: int32(in.Policy.ValiditySeconds), OperationID: in.OperationID, SetBy: actor.AccountID}); err != nil {
						return err
					}
					ack.PolicyRevision++
				case app.CommandArchive:
					if changed, err = queries.ArchiveChecklistTemplate(ctx, q.ArchiveChecklistTemplateParams{ArchiveOperationID: in.OperationID, ArchivedBy: actor.AccountID, OrgID: org, ID: template, Revision: head.Revision}); err != nil {
						return err
					}
					if changed != 1 {
						return app.ErrConflict
					}
					ack.State = app.TemplateArchived
				default:
					return checklist.ErrInvalid
				}
			}
			return queries.InsertChecklistCommand(ctx, q.InsertChecklistCommandParams{OrgID: org, OperationID: in.OperationID, ActorID: actor.AccountID, TemplateID: template, Kind: string(in.Kind), RequestHash: digest,
				ResultRevision: ack.Revision, ResultVersion: int32(ack.Version), ResultPolicyRevision: int32(ack.PolicyRevision), ResultState: string(ack.State)})
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		return s.Authority.CheckChecklistTx(ctx, tx, scope, actor, member, org, app.TemplateManage)
	})
	if err != nil {
		return app.TemplateAck{}, false, storeError(ctx, err)
	}
	return ack, replayed, nil
}

// read runs one read between two authority checks of the action and target.
func (s *ChecklistStore) read(ctx context.Context, scope kernel.OrgScope, actor app.ChecklistActor, member, target string, action app.ChecklistAction, fn func(*q.Queries) error) error {
	if err := validCall(scope, actor, member); err != nil {
		return err
	}
	if !s.ready(false) {
		return app.ErrUnavailable
	}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckChecklistTx(ctx, tx, scope, actor, member, target, action); err != nil {
			return err
		}
		if err := fn(q.New(tx)); err != nil {
			return err
		}
		return s.Authority.CheckChecklistTx(ctx, tx, scope, actor, member, target, action)
	})
	return storeError(ctx, err)
}

func (s *ChecklistStore) ListTemplates(ctx context.Context, scope kernel.OrgScope, actor app.ChecklistActor, member string, filter app.TemplateFilter) (app.TemplatePage, error) {
	if err := filter.Validate(); err != nil {
		return app.TemplatePage{}, err
	}
	page := app.TemplatePage{Templates: []app.TemplateSummary{}}
	err := s.read(ctx, scope, actor, member, scope.OrgID().String(), app.TemplateRead, func(queries *q.Queries) error {
		rows, err := queries.ListChecklistTemplates(ctx, q.ListChecklistTemplatesParams{OrgID: scope.OrgID().String(), AfterID: filter.AfterID, RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		more := len(rows) > filter.Limit
		if more {
			rows = rows[:filter.Limit]
		}
		for _, r := range rows {
			created, err := instant(r.CreatedAt)
			if err != nil {
				return err
			}
			archived, err := optionalInstant(r.ArchivedAt)
			if err != nil {
				return err
			}
			page.Templates = append(page.Templates, app.TemplateSummary{TemplateID: r.ID, Context: checklist.Context(r.Context), State: app.TemplateState(r.State), Revision: r.Revision, Version: int64(r.Version), PolicyRevision: int64(r.PolicyRevision),
				Name: r.Name, TargetKind: checklist.TargetKind(r.TargetKind), CreatedAt: created, ArchivedAt: archived})
		}
		if more && len(rows) > 0 {
			page.NextID = rows[len(rows)-1].ID
		}
		return nil
	})
	if err != nil {
		return app.TemplatePage{}, err
	}
	return page, nil
}

func readVersion(ctx context.Context, queries *q.Queries, org, template string, version int32) (app.VersionView, error) {
	row, err := queries.ChecklistVersion(ctx, q.ChecklistVersionParams{OrgID: org, TemplateID: template, Version: version})
	if err != nil {
		return app.VersionView{}, err
	}
	published, err := instant(row.PublishedAt)
	if err != nil {
		return app.VersionView{}, err
	}
	items, err := queries.ChecklistVersionItems(ctx, q.ChecklistVersionItemsParams{OrgID: org, TemplateID: template, Version: version})
	if err != nil {
		return app.VersionView{}, err
	}
	targets, err := queries.ChecklistVersionTargets(ctx, q.ChecklistVersionTargetsParams{OrgID: org, TemplateID: template, Version: version})
	if err != nil {
		return app.VersionView{}, err
	}
	if len(items) != int(row.ItemCount) || len(targets) != int(row.TargetCount) {
		return app.VersionView{}, app.ErrUnavailable
	}
	v := app.VersionView{Version: int64(version), Name: row.Name, Target: checklist.Target{Kind: checklist.TargetKind(row.TargetKind)}, PublishedAt: published}
	if len(targets) > 0 {
		v.Target.IDs = targets
	}
	for i, item := range items {
		if item.Position != int32(i+1) {
			return app.VersionView{}, app.ErrUnavailable
		}
		v.Items = append(v.Items, checklist.Item{Label: item.Label, Critical: item.Critical, Optional: item.Optional, PhotoOnNotOK: item.PhotoOnNotOk})
	}
	return v, nil
}

func readPolicy(ctx context.Context, queries *q.Queries, org, template string, revision int32) (app.PolicyView, error) {
	row, err := queries.ChecklistPolicyRevision(ctx, q.ChecklistPolicyRevisionParams{OrgID: org, TemplateID: template, Revision: revision})
	if err != nil {
		return app.PolicyView{}, err
	}
	effective, err := instant(row.EffectiveAt)
	if err != nil {
		return app.PolicyView{}, err
	}
	return app.PolicyView{Revision: int64(revision), Policy: checklist.Policy{Mode: checklist.Mode(row.Mode), ValiditySeconds: int64(row.ValiditySeconds)}, EffectiveAt: effective}, nil
}

func (s *ChecklistStore) ReadTemplate(ctx context.Context, scope kernel.OrgScope, actor app.ChecklistActor, member, template string, version int64) (app.TemplateView, error) {
	if !checklist.CanonicalID(template) {
		return app.TemplateView{}, app.ErrNotFound
	}
	if version < 0 || version > 1<<31-1 {
		return app.TemplateView{}, checklist.ErrInvalid
	}
	org := scope.OrgID().String()
	var view app.TemplateView
	err := s.read(ctx, scope, actor, member, org, app.TemplateRead, func(queries *q.Queries) error {
		head, err := queries.ChecklistTemplateHead(ctx, q.ChecklistTemplateHeadParams{OrgID: org, ID: template})
		if err != nil {
			return err
		}
		created, err := instant(head.CreatedAt)
		if err != nil {
			return err
		}
		archived, err := optionalInstant(head.ArchivedAt)
		if err != nil {
			return err
		}
		view = app.TemplateView{TemplateID: template, Context: checklist.Context(head.Context), State: app.TemplateState(head.State), Revision: head.Revision, Version: int64(head.Version), PolicyRevision: int64(head.PolicyRevision), CreatedAt: created, ArchivedAt: archived}
		selected := head.Version
		if version != 0 {
			if version > int64(head.Version) {
				return app.ErrNotFound
			}
			selected = int32(version)
		}
		if view.Selected, err = readVersion(ctx, queries, org, template, selected); err != nil {
			return err
		}
		view.Policy, err = readPolicy(ctx, queries, org, template, head.PolicyRevision)
		return err
	})
	if err != nil {
		return app.TemplateView{}, err
	}
	return view, nil
}

// ReadApplicable resolves the asset's applicable template with the pure
// BR-CHECK-002 rule over the matching ACTIVE templates (B-BR-EMP-332).
func (s *ChecklistStore) ReadApplicable(ctx context.Context, scope kernel.OrgScope, actor app.ChecklistActor, member, asset string, moment checklist.Context) (app.ApplicableView, error) {
	if !checklist.CanonicalID(asset) {
		return app.ApplicableView{}, app.ErrNotFound
	}
	if _, err := checklist.ParseContext(string(moment)); err != nil {
		return app.ApplicableView{}, err
	}
	org := scope.OrgID().String()
	view := app.ApplicableView{AssetID: asset, Context: moment, Match: checklist.MatchNone}
	err := s.read(ctx, scope, actor, member, asset, app.ApplicableRead, func(queries *q.Queries) error {
		category, err := queries.ChecklistAssetCategory(ctx, q.ChecklistAssetCategoryParams{OrgID: org, ID: asset})
		if err != nil {
			return err
		}
		rows, err := queries.ChecklistApplicableCandidates(ctx, q.ChecklistApplicableCandidatesParams{AssetID: asset, CategoryID: category, OrgID: org, Context: string(moment)})
		if err != nil {
			return err
		}
		var candidates []checklist.Candidate
		for _, r := range rows {
			target := checklist.Target{Kind: checklist.TargetKind(r.TargetKind)}
			if r.TargetKind != string(checklist.TargetOrganization) {
				target.IDs = []string{r.TargetID}
			}
			if n := len(candidates); n > 0 && candidates[n-1].TemplateID == r.TemplateID {
				candidates[n-1].Target.IDs = append(candidates[n-1].Target.IDs, target.IDs...)
				continue
			}
			candidates = append(candidates, checklist.Candidate{TemplateID: r.TemplateID, Context: moment, Target: target})
		}
		resolved, err := checklist.Resolve(candidates, moment, asset, category)
		if err != nil {
			return app.ErrUnavailable
		}
		view.Match = resolved.Match
		if resolved.Match != checklist.MatchNone {
			head, err := queries.ChecklistTemplateHead(ctx, q.ChecklistTemplateHeadParams{OrgID: org, ID: resolved.TemplateID})
			if err != nil {
				return err
			}
			version, err := readVersion(ctx, queries, org, resolved.TemplateID, head.Version)
			if err != nil {
				return err
			}
			policy, err := readPolicy(ctx, queries, org, resolved.TemplateID, head.PolicyRevision)
			if err != nil {
				return err
			}
			view.Template = &app.ApplicableTemplate{TemplateID: resolved.TemplateID, Name: version.Name, Version: version.Version, PublishedAt: version.PublishedAt, Items: version.Items}
			view.Policy = &policy
		}
		at, err := queries.ChecklistClock(ctx)
		if err != nil {
			return err
		}
		view.At, err = instant(at)
		return err
	})
	if err != nil {
		return app.ApplicableView{}, err
	}
	return view, nil
}
