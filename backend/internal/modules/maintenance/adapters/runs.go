package adapters

// EMP-CHECK-002 run persistence (EDR-284/285, migration 000242): the intake
// store of check.run.complete, the FLEET-007 checklist block source and the
// company run read and release owner.

import (
	"context"
	"errors"
	"slices"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/maintenance"
	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RunTargetType is the audit target vocabulary of a run.
const (
	RunTargetType   = "checklist_run"
	RunReleaseAudit = "check.run.released"
)

// RunAuditDefinitions registers the release; it carries the command digest
// and no reason (BR-AUD-004).
func RunAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: RunReleaseAudit, TargetType: RunTargetType}}
}

// RunStore owns the run intake rows and the company read and release. The
// intake methods run in the borrowed intake transaction and need no pool.
type RunStore struct {
	Pool      *pgxpool.Pool
	Authority app.RunCompanyAuthority
	Audit     auditing.TransactionRunner
}

var (
	_ app.RunStore                = (*RunStore)(nil)
	_ app.RunRepository           = (*RunStore)(nil)
	_ fleetapp.ChecklistBlockPort = ChecklistBlocks{}
)

func (s *RunStore) port(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope) (*q.Queries, error) {
	db = syncapp.Borrow(db)
	if s == nil || db == nil || database.ValidateOrgDBTX(ctx, db, scope) != nil {
		return nil, app.ErrUnavailable
	}
	return q.New(db), nil
}

// RunOperationTx answers the operation already holding a run identity.
func (s *RunStore) RunOperationTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, run string) (string, error) {
	queries, err := s.port(ctx, db, scope)
	if err != nil || !checklist.CanonicalID(run) {
		return "", app.ErrUnavailable
	}
	op, err := queries.ChecklistRunOperation(ctx, q.ChecklistRunOperationParams{OrgID: scope.OrgID().String(), ID: run})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", intakeError(err)
	}
	return op, nil
}

// AuthorEmployeeTx reads the authoring membership's employee record.
func (s *RunStore) AuthorEmployeeTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, membership string) (string, error) {
	queries, err := s.port(ctx, db, scope)
	if err != nil || !checklist.CanonicalID(membership) {
		return "", app.ErrUnavailable
	}
	employee, err := queries.IncidentReporterEmployee(ctx, q.IncidentReporterEmployeeParams{OrgID: scope.OrgID().String(), MembershipID: membership})
	if err != nil {
		return "", intakeError(err)
	}
	return employee, nil
}

// AssetStateTx reads the asset's lifecycle state.
func (s *RunStore) AssetStateTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset string) (string, error) {
	queries, err := s.port(ctx, db, scope)
	if err != nil || !checklist.CanonicalID(asset) {
		return "", app.ErrUnavailable
	}
	state, err := queries.IncidentAssetState(ctx, q.IncidentAssetStateParams{OrgID: scope.OrgID().String(), ID: asset})
	if err != nil {
		return "", intakeError(err)
	}
	return state, nil
}

func runInstant(v pgtype.Timestamptz) (time.Time, bool) {
	if !v.Valid || v.InfinityModifier != pgtype.Finite {
		return time.Time{}, false
	}
	return v.Time.UTC(), true
}

// RunTemplateTx reads the named version of this organization with every
// version stamp, the archive instant, the whole policy history and the
// asset's category (B-BR-EMP-440).
func (s *RunStore) RunTemplateTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, template string, version int64, asset string) (app.RunTemplate, error) {
	queries, err := s.port(ctx, db, scope)
	if err != nil || !checklist.CanonicalID(template) || !checklist.CanonicalID(asset) || version < 1 || version > app.MaxRunCounter {
		return app.RunTemplate{}, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	head, err := queries.ChecklistTemplateHead(ctx, q.ChecklistTemplateHeadParams{OrgID: org, ID: template})
	if err != nil {
		return app.RunTemplate{}, intakeError(err)
	}
	stored, err := queries.ChecklistVersion(ctx, q.ChecklistVersionParams{OrgID: org, TemplateID: template, Version: int32(version)})
	if err != nil {
		return app.RunTemplate{}, intakeError(err)
	}
	category, err := queries.ChecklistAssetCategory(ctx, q.ChecklistAssetCategoryParams{OrgID: org, ID: asset})
	if err != nil {
		return app.RunTemplate{}, app.ErrUnavailable
	}
	context, err := checklist.ParseContext(head.Context)
	if err != nil {
		return app.RunTemplate{}, app.ErrUnavailable
	}
	facts := checklist.RunFacts{Context: context, Version: version, Target: checklist.Target{Kind: checklist.TargetKind(stored.TargetKind)}}
	if head.ArchivedAt.Valid {
		at, ok := runInstant(head.ArchivedAt)
		if !ok {
			return app.RunTemplate{}, app.ErrUnavailable
		}
		facts.ArchivedAt = &at
	}
	items, err := queries.ChecklistVersionItems(ctx, q.ChecklistVersionItemsParams{OrgID: org, TemplateID: template, Version: int32(version)})
	if err != nil {
		return app.RunTemplate{}, app.ErrUnavailable
	}
	for i, item := range items {
		if item.Position != int32(i+1) {
			return app.RunTemplate{}, app.ErrUnavailable
		}
		facts.Items = append(facts.Items, checklist.Item{Label: item.Label, Critical: item.Critical, Optional: item.Optional, PhotoOnNotOK: item.PhotoOnNotOk})
	}
	if facts.Target.Kind != checklist.TargetOrganization {
		targets, err := queries.ChecklistVersionTargets(ctx, q.ChecklistVersionTargetsParams{OrgID: org, TemplateID: template, Version: int32(version)})
		if err != nil {
			return app.RunTemplate{}, app.ErrUnavailable
		}
		facts.Target.IDs = targets
	}
	stamps, err := queries.ChecklistVersionStamps(ctx, q.ChecklistVersionStampsParams{OrgID: org, TemplateID: template})
	if err != nil {
		return app.RunTemplate{}, app.ErrUnavailable
	}
	for _, v := range stamps {
		at, ok := runInstant(v.PublishedAt)
		if !ok {
			return app.RunTemplate{}, app.ErrUnavailable
		}
		facts.Versions = append(facts.Versions, checklist.VersionStamp{Version: int64(v.Version), PublishedAt: at})
	}
	policies, err := queries.ChecklistPolicyHistory(ctx, q.ChecklistPolicyHistoryParams{OrgID: org, TemplateID: template})
	if err != nil {
		return app.RunTemplate{}, app.ErrUnavailable
	}
	for _, p := range policies {
		at, ok := runInstant(p.EffectiveAt)
		if !ok {
			return app.RunTemplate{}, app.ErrUnavailable
		}
		facts.Policies = append(facts.Policies, checklist.PolicyRevision{Revision: int64(p.Revision), Policy: checklist.Policy{Mode: checklist.Mode(p.Mode), ValiditySeconds: int64(p.ValiditySeconds)}, EffectiveAt: at})
	}
	return app.RunTemplate{Facts: facts, CategoryID: category}, nil
}

// InsertRunTx writes the accepted run and its answers (B-BR-EMP-441); the
// database re-proves the source, the asset, the allocation and the recheck.
func (s *RunStore) InsertRunTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, command app.RunCommand, result checklist.Recheck, facts app.RunFacts) error {
	queries, err := s.port(ctx, db, scope)
	if err != nil {
		return err
	}
	v, err := command.Snapshot()
	if err != nil || source.MembershipID != v.AuthorMembershipID || source.Actor.AccountID != v.OwnerAccountID || !checklist.CanonicalID(facts.EmployeeID) ||
		result.DeviceOutcome != v.DeviceOutcome || result.PolicyRevision < 1 || result.PolicyRevision > app.MaxRunCounter {
		return app.ErrUnavailable
	}
	if _, err := checklist.ParseOutcome(string(result.Evaluation.Outcome)); err != nil {
		return app.ErrUnavailable
	}
	org := scope.OrgID().String()
	at, sub := exactInstant(v.Declared.OccurredAt)
	if err := queries.InsertChecklistRun(ctx, q.InsertChecklistRunParams{OrgID: org, ID: v.RunID, OperationID: v.OperationID, PayloadDigest: v.PayloadDigest,
		AssetID: v.AssetID, UnitID: nullable(facts.UnitID), CostCentreID: nullable(facts.CostCentreID), Context: string(v.Context), TemplateID: v.TemplateID,
		TemplateVersion: int32(v.Version), PolicyRevision: int32(v.PolicyRevision), ServerPolicyRevision: int32(result.PolicyRevision),
		StartedAt: pgtype.Timestamptz{Time: v.StartedAt.UTC(), Valid: true}, OccurredAt: at, OccurredSubmicro: sub, EntryZone: v.Declared.EntryZone,
		DeviceOffsetSeconds: v.Declared.DeviceOffsetSeconds, DeviceOutcome: string(v.DeviceOutcome), RecheckOutcome: string(result.Evaluation.Outcome),
		OutcomeDiffers: result.Differs, TemplateSuperseded: result.Superseded, Acknowledged: v.Acknowledged, AuthorAccountID: source.Actor.AccountID,
		AuthorMembershipID: source.MembershipID, AuthorEmployeeID: facts.EmployeeID, EnrollmentID: source.EnrollmentID}); err != nil {
		return intakeError(err)
	}
	for i, r := range v.Responses {
		note := pgtype.Text{String: r.Note, Valid: r.Note != ""}
		if err := queries.InsertChecklistRunAnswer(ctx, q.InsertChecklistRunAnswerParams{OrgID: org, RunID: v.RunID, TemplateID: v.TemplateID,
			TemplateVersion: int32(v.Version), Position: int32(i + 1), Answer: string(r.Answer), Note: note}); err != nil {
			return intakeError(err)
		}
	}
	return nil
}

func runRecord(row q.GetChecklistRunRow, answers []q.ChecklistRunAnswersRow) (app.RunRecord, error) {
	at, ok := storedInstant(row.OccurredAt, row.OccurredSubmicro)
	started, okStarted := runInstant(row.StartedAt)
	received, okReceived := runInstant(row.ReceivedAt)
	context, errContext := checklist.ParseContext(row.Context)
	device, errDevice := checklist.ParseOutcome(row.DeviceOutcome)
	recheck, errRecheck := checklist.ParseOutcome(row.RecheckOutcome)
	outcome, errOutcome := checklist.ParseOutcome(row.Outcome)
	if !ok || !okStarted || !okReceived || errContext != nil || errDevice != nil || errRecheck != nil || errOutcome != nil || row.Revision < 1 || row.Revision > app.MaxRunRevision ||
		(row.Revision == 2) != row.ReleasedAt.Valid || len(answers) < 1 || len(answers) > checklist.MaxItems {
		return app.RunRecord{}, app.ErrUnavailable
	}
	r := app.RunRecord{ID: row.ID, OperationID: row.OperationID, PayloadDigest: row.PayloadDigest, AssetID: row.AssetID, UnitID: row.UnitID, CostCentreID: row.CostCentreID,
		TemplateID: row.TemplateID, AuthorAccountID: row.AuthorAccountID, AuthorMembershipID: row.AuthorMembershipID, AuthorEmployeeID: row.AuthorEmployeeID,
		EnrollmentID: row.EnrollmentID, Context: context, Version: int64(row.TemplateVersion), PolicyRevision: int64(row.PolicyRevision),
		ServerPolicyRevision: int64(row.ServerPolicyRevision), Revision: row.Revision, StartedAt: started, ReceivedAt: received,
		Declared:      orgtime.Declaration{OccurredAt: at, EntryZone: row.EntryZone, DeviceOffsetSeconds: row.DeviceOffsetSeconds},
		DeviceOutcome: device, RecheckOutcome: recheck, Outcome: outcome, Acknowledged: row.Acknowledged, Superseded: row.TemplateSuperseded, Differs: row.OutcomeDiffers}
	for i, a := range answers {
		if a.Position != int32(i+1) {
			return app.RunRecord{}, app.ErrUnavailable
		}
		r.Responses = append(r.Responses, checklist.Response{Answer: checklist.Answer(a.Answer), Note: a.Note.String, Photo: a.Photo})
		if a.Answer == string(checklist.AnswerNotOK) {
			r.Issues = append(r.Issues, i+1)
			if a.Critical {
				r.Critical = append(r.Critical, i+1)
			}
		}
	}
	if row.ReleasedAt.Valid {
		released, ok := runInstant(row.ReleasedAt)
		if !ok || !checklist.CanonicalID(row.ReleasedBy) {
			return app.RunRecord{}, app.ErrUnavailable
		}
		r.Release = &app.ReleaseView{ActorMembershipID: row.ReleasedBy, RecordedAt: released}
	}
	return r, nil
}

func readRun(ctx context.Context, queries *q.Queries, org, run string) (app.RunRecord, error) {
	row, err := queries.GetChecklistRun(ctx, q.GetChecklistRunParams{OrgID: org, ID: run})
	if err != nil {
		return app.RunRecord{}, intakeError(err)
	}
	answers, err := queries.ChecklistRunAnswers(ctx, q.ChecklistRunAnswersParams{OrgID: org, RunID: run})
	if err != nil {
		return app.RunRecord{}, app.ErrUnavailable
	}
	return runRecord(row, answers)
}

// ReadRunTx reads one stored run of the scope.
func (s *RunStore) ReadRunTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, run string) (app.RunRecord, error) {
	queries, err := s.port(ctx, db, scope)
	if err != nil || !checklist.CanonicalID(run) {
		return app.RunRecord{}, app.ErrUnavailable
	}
	return readRun(ctx, queries, scope.OrgID().String(), run)
}

// ---- FLEET-007 checklist block source (EDR-285) ----

// ChecklistBlocks answers the BLOCKING runs of an asset whose period covers
// the instant, on the caller's transaction (BR-CHECK-010).
type ChecklistBlocks struct{}

func (ChecklistBlocks) ChecklistBlocksDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset string, at time.Time) (fleetapp.ChecklistBlockAnswer, error) {
	db = syncapp.Borrow(db)
	if db == nil || !checklist.CanonicalID(asset) || at.IsZero() || database.ValidateOrgDBTX(ctx, db, scope) != nil {
		return fleetapp.ChecklistBlockAnswer{}, fleetapp.ErrUnavailable
	}
	rows, err := q.New(db).ChecklistBlockingRuns(ctx, q.ChecklistBlockingRunsParams{OrgID: scope.OrgID().String(), AssetID: asset, At: pgtype.Timestamptz{Time: at.UTC(), Valid: true}})
	if err != nil {
		return fleetapp.ChecklistBlockAnswer{}, fleetapp.ErrUnavailable
	}
	out := fleetapp.ChecklistBlockAnswer{Sourced: true}
	for _, row := range rows {
		from, ok := runInstant(row.OccurredAt)
		if !ok {
			return fleetapp.ChecklistBlockAnswer{}, fleetapp.ErrUnavailable
		}
		fact := fleet.ChecklistBlockFact{ID: row.ID, From: from}
		if row.ReleasedAt.Valid {
			to, ok := runInstant(row.ReleasedAt)
			if !ok {
				return fleetapp.ChecklistBlockAnswer{}, fleetapp.ErrUnavailable
			}
			fact.To = &to
		}
		out.Blocks = append(out.Blocks, fact)
	}
	return out, nil
}

// ---- company run read and release (B-BR-EMP-441/442) ----

func (s *RunStore) ready(write bool) bool {
	return s != nil && s.Pool != nil && !missing(s.Authority) && (!write || !missing(s.Audit))
}

func viewOf(r app.RunRecord) app.RunView {
	v := app.RunView{RunID: r.ID, AssetID: r.AssetID, TemplateID: r.TemplateID, Context: r.Context, Version: r.Version, PolicyRevision: r.PolicyRevision,
		ServerPolicyRevision: r.ServerPolicyRevision, Revision: r.Revision, StartedAt: r.StartedAt, CompletedAt: r.Declared.OccurredAt, ReceivedAt: r.ReceivedAt,
		DeviceOutcome: r.DeviceOutcome, RecheckOutcome: r.RecheckOutcome, Outcome: r.Outcome, Acknowledged: r.Acknowledged, Superseded: r.Superseded, Differs: r.Differs,
		Issues: slices.Clone(r.Issues), Critical: slices.Clone(r.Critical)}
	if r.Release != nil {
		at := r.Release.RecordedAt
		v.ReleasedAt = &at
	}
	return v
}

// ReadRun reads one run of the asset between two authority checks; a run of
// another asset is not found (B-BR-EMP-441).
func (s *RunStore) ReadRun(ctx context.Context, scope kernel.OrgScope, actor app.IncidentActor, member, asset, run string) (app.RunView, error) {
	if err := validIncidentCall(scope, actor, member); err != nil {
		return app.RunView{}, err
	}
	if !checklist.CanonicalID(asset) || !checklist.CanonicalID(run) {
		return app.RunView{}, app.ErrNotFound
	}
	if !s.ready(false) {
		return app.RunView{}, app.ErrUnavailable
	}
	var view app.RunView
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if _, err := s.Authority.CheckRunCompanyTx(ctx, tx, scope, actor, member, asset, app.RunRead); err != nil {
			return err
		}
		r, err := readRun(ctx, q.New(tx), scope.OrgID().String(), run)
		if err != nil {
			return err
		}
		if r.AssetID != asset {
			return app.ErrNotFound
		}
		view = viewOf(r)
		_, err = s.Authority.CheckRunCompanyTx(ctx, tx, scope, actor, member, asset, app.RunRead)
		return err
	})
	if err != nil {
		return app.RunView{}, incidentError(ctx, err)
	}
	return view, nil
}

// ListRuns pages the organization's runs in queue order (B-BR-EMP-506)
// between two queue authority checks; each item is the run read's view.
func (s *RunStore) ListRuns(ctx context.Context, scope kernel.OrgScope, actor app.IncidentActor, member string, filter app.RunQueueFilter) (app.RunQueuePage, error) {
	if err := validIncidentCall(scope, actor, member); err != nil {
		return app.RunQueuePage{}, err
	}
	if err := filter.Validate(); err != nil {
		return app.RunQueuePage{}, err
	}
	if !s.ready(false) {
		return app.RunQueuePage{}, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	params := q.ListChecklistRunQueueParams{OrgID: org, RowLimit: int32(filter.Limit + 1)}
	if filter.After != nil {
		params.AfterID, params.AfterGroup = filter.After.RunID, int32(filter.After.Group)
		params.AfterAt = pgtype.Timestamptz{Time: filter.After.CompletedAt.UTC(), Valid: true}
	}
	page := app.RunQueuePage{Items: []app.RunView{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if _, err := s.Authority.CheckRunCompanyTx(ctx, tx, scope, actor, member, org, app.RunQueue); err != nil {
			return err
		}
		queries := q.New(tx)
		ids, err := queries.ListChecklistRunQueue(ctx, params)
		if err != nil {
			return err
		}
		more := len(ids) > filter.Limit
		if more {
			ids = ids[:filter.Limit]
		}
		for _, id := range ids {
			r, err := readRun(ctx, queries, org, id)
			if err != nil {
				return err
			}
			page.Items = append(page.Items, viewOf(r))
		}
		if more {
			next := app.KeyOf(page.Items[len(page.Items)-1])
			page.Next = &next
		}
		_, err = s.Authority.CheckRunCompanyTx(ctx, tx, scope, actor, member, org, app.RunQueue)
		return err
	})
	if err != nil {
		return app.RunQueuePage{}, incidentError(ctx, err)
	}
	return page, nil
}

// ReleaseRun releases a blocking run (B-BR-EMP-442) under the organization
// root and the run row lock: authority, replay by operation, state and
// revision, separation of releaser and author, then one release row and one
// audit event, and authority again.
func (s *RunStore) ReleaseRun(ctx context.Context, scope kernel.OrgScope, actor app.IncidentActor, member string, command app.ReleaseCommand) (app.RunView, bool, error) {
	if err := validIncidentCall(scope, actor, member); err != nil {
		return app.RunView{}, false, err
	}
	in, err := command.Snapshot()
	if err != nil {
		return app.RunView{}, false, err
	}
	digest := command.Digest(scope)
	if digest == "" || !s.ready(true) {
		return app.RunView{}, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	bound := audit.Event{OperationID: in.OperationID, ActorID: actor.AccountID, Action: RunReleaseAudit, TargetType: RunTargetType, TargetID: in.RunID, RequestHash: digest}
	var view app.RunView
	replayed := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		handler, err := s.Authority.CheckRunCompanyTx(ctx, tx, scope, actor, member, in.RunID, app.RunRelease)
		if err != nil {
			return err
		}
		recheck := func() error {
			again, err := s.Authority.CheckRunCompanyTx(ctx, tx, scope, actor, member, in.RunID, app.RunRelease)
			if err != nil {
				return err
			}
			if again != handler {
				return app.ErrUnavailable
			}
			return nil
		}
		queries := q.New(tx)
		run, err := queries.ChecklistRunForUpdate(ctx, q.ChecklistRunForUpdateParams{OrgID: org, ID: in.RunID})
		if err != nil {
			return err
		}
		finish := func() error {
			r, err := readRun(ctx, queries, org, in.RunID)
			if err != nil {
				return err
			}
			if r.Revision != 2 || r.Outcome != checklist.CompletedWithIssues || r.Release == nil {
				return app.ErrUnavailable
			}
			view = viewOf(r)
			return recheck()
		}
		prior, err := queries.ChecklistRunReleaseByOperation(ctx, q.ChecklistRunReleaseByOperationParams{OrgID: org, OperationID: in.OperationID})
		if err == nil {
			if prior.RunID != in.RunID || prior.ActorAccountID != actor.AccountID || prior.RequestHash != digest {
				return app.ErrOperationReused
			}
			repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(context.Context, pgx.Tx) error { return app.ErrUnavailable })
			if err != nil {
				return err
			}
			if !repeated {
				return app.ErrUnavailable
			}
			replayed = true
			return finish()
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if run.Outcome != string(checklist.Blocking) || run.Revision != in.ExpectedRevision {
			return app.ErrConflict
		}
		author, err := identity(ctx, queries, org, run.AuthorAccountID, run.AuthorMembershipID, run.AuthorEmployeeID)
		if err != nil {
			return err
		}
		self, err := identity(ctx, queries, org, handler.AccountID, handler.MembershipID, handler.EmployeeID)
		if err != nil {
			return err
		}
		for _, key := range self {
			if slices.Contains(author, key) {
				return app.ErrSelfApproval
			}
		}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(ctx context.Context, tx pgx.Tx) error {
			return q.New(tx).InsertChecklistRunRelease(ctx, q.InsertChecklistRunReleaseParams{OrgID: org, RunID: in.RunID, Reason: in.Reason, ActorAccountID: handler.AccountID,
				ActorMembershipID: handler.MembershipID, ActorEmployeeID: handler.EmployeeID, OperationID: in.OperationID, RequestHash: digest})
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		return finish()
	})
	if err != nil {
		return app.RunView{}, false, incidentError(ctx, err)
	}
	return view, replayed, nil
}
