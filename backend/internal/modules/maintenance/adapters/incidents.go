package adapters

// EMP-CHECK-003 report persistence (EDR-238..241, migration 000230): the
// intake store of check.incident.report, the trip timeline source, the
// document link resolver of the INCIDENT kind and the company handling and
// read owner.

import (
	"context"
	"errors"
	"slices"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/maintenance"
	documentapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	documentdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/incident"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReportTargetType is the audit target vocabulary of a report.
const ReportTargetType = "incident_report"

var stepActions = map[incident.Action]string{
	incident.Acknowledge: "check.report.acknowledged",
	incident.Treat:       "check.report.treated",
	incident.Resolve:     "check.report.resolved",
	incident.Dismiss:     "check.report.dismissed",
}

// IncidentAuditDefinitions registers one action per handling step; they
// carry the command digest and no note (BR-AUD-004).
func IncidentAuditDefinitions() []audit.Definition {
	out := []audit.Definition{}
	for _, a := range incident.Actions() {
		out = append(out, audit.Definition{Action: stepActions[a], TargetType: ReportTargetType})
	}
	return out
}

// IncidentStore owns the report intake rows, the timeline source and the
// company handling and reads. The intake methods run in the borrowed intake
// transaction and need no pool.
type IncidentStore struct {
	Pool      *pgxpool.Pool
	Authority app.IncidentAuthority
	Audit     auditing.TransactionRunner
}

var (
	_ app.ReportStore            = (*IncidentStore)(nil)
	_ app.IncidentRepository     = (*IncidentStore)(nil)
	_ tripapp.TimelineReports    = (*IncidentStore)(nil)
	_ documentapp.TargetResolver = IncidentDocumentTargets{}
)

func (s *IncidentStore) port(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope) (*q.Queries, error) {
	db = syncapp.Borrow(db)
	if s == nil || db == nil || database.ValidateOrgDBTX(ctx, db, scope) != nil {
		return nil, app.ErrUnavailable
	}
	return q.New(db), nil
}

func intakeError(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return app.ErrNotFound
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	}
	return app.ErrUnavailable
}

// exactInstant splits an instant into the stored microsecond value and its
// sub-microsecond remainder (the trip intake format).
func exactInstant(at time.Time) (pgtype.Timestamptz, int16) {
	at = at.UTC()
	micro := at.Truncate(time.Microsecond)
	return pgtype.Timestamptz{Time: micro, Valid: true}, int16(at.Sub(micro))
}

func storedInstant(v pgtype.Timestamptz, sub int16) (time.Time, bool) {
	if !v.Valid || v.InfinityModifier != pgtype.Finite || sub < 0 || sub > 999 {
		return time.Time{}, false
	}
	return v.Time.UTC().Add(time.Duration(sub)), true
}

func optionalText(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	out := v.String
	return &out
}

func nullable(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }

func nullableText(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

// ReportOperationTx answers the operation already holding a report identity.
func (s *IncidentStore) ReportOperationTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, report string) (string, error) {
	queries, err := s.port(ctx, db, scope)
	if err != nil || !incident.CanonicalID(report) {
		return "", app.ErrUnavailable
	}
	op, err := queries.IncidentReportOperation(ctx, q.IncidentReportOperationParams{OrgID: scope.OrgID().String(), ID: report})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", intakeError(err)
	}
	return op, nil
}

// ReporterEmployeeTx reads the reporting membership's employee record.
func (s *IncidentStore) ReporterEmployeeTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, membership string) (string, error) {
	queries, err := s.port(ctx, db, scope)
	if err != nil || !incident.CanonicalID(membership) {
		return "", app.ErrUnavailable
	}
	employee, err := queries.IncidentReporterEmployee(ctx, q.IncidentReporterEmployeeParams{OrgID: scope.OrgID().String(), MembershipID: membership})
	if err != nil {
		return "", intakeError(err)
	}
	return employee, nil
}

// AssetStateTx reads the asset's lifecycle state.
func (s *IncidentStore) AssetStateTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset string) (string, error) {
	queries, err := s.port(ctx, db, scope)
	if err != nil || !incident.CanonicalID(asset) {
		return "", app.ErrUnavailable
	}
	state, err := queries.IncidentAssetState(ctx, q.IncidentAssetStateParams{OrgID: scope.OrgID().String(), ID: asset})
	if err != nil {
		return "", intakeError(err)
	}
	return state, nil
}

// InsertReportTx writes the accepted report (B-BR-EMP-369); the database
// re-proves the source, the asset, the allocation and the trip.
func (s *IncidentStore) InsertReportTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, command app.ReportCommand, facts app.ReportFacts) error {
	queries, err := s.port(ctx, db, scope)
	if err != nil {
		return err
	}
	v, err := command.Snapshot()
	if err != nil || source.MembershipID != v.AuthorMembershipID || source.Actor.AccountID != v.OwnerAccountID || !incident.CanonicalID(facts.EmployeeID) {
		return app.ErrUnavailable
	}
	at, sub := exactInstant(v.Declared.OccurredAt)
	r := v.Report
	if err := queries.InsertIncidentReport(ctx, q.InsertIncidentReportParams{OrgID: scope.OrgID().String(), ID: v.ReportID, OperationID: v.OperationID, PayloadDigest: v.PayloadDigest,
		ReportType: string(r.Type), IncidentKind: nullable(string(r.Kind)), AssetID: v.AssetID, UnitID: nullable(facts.UnitID), CostCentreID: nullable(facts.CostCentreID),
		TripID: nullable(v.TripID), Description: r.Description, Location: nullableText(r.Location), BlocksUse: r.BlocksUse, ReporterAccountID: source.Actor.AccountID,
		ReporterMembershipID: source.MembershipID, ReporterEmployeeID: facts.EmployeeID, EnrollmentID: source.EnrollmentID, OccurredAt: at, OccurredSubmicro: sub,
		EntryZone: v.Declared.EntryZone, DeviceOffsetSeconds: v.Declared.DeviceOffsetSeconds}); err != nil {
		return intakeError(err)
	}
	return nil
}

func reportRecord(row q.GetIncidentReportRow) (app.ReportRecord, error) {
	at, ok := storedInstant(row.OccurredAt, row.OccurredSubmicro)
	if !ok || !row.ReceivedAt.Valid || !row.UpdatedAt.Valid {
		return app.ReportRecord{}, app.ErrUnavailable
	}
	report, err := incident.NewReport(incident.Draft{Type: incident.ReportType(row.ReportType), Kind: incident.Kind(row.IncidentKind), Description: row.Description,
		Location: optionalText(row.Location), BlocksUse: row.BlocksUse})
	if err != nil {
		return app.ReportRecord{}, app.ErrUnavailable
	}
	state, err := incident.ParseState(row.State)
	if err != nil || row.Revision < 1 || row.Revision > incident.MaxRevision {
		return app.ReportRecord{}, app.ErrUnavailable
	}
	report.State, report.Revision = state, row.Revision
	return app.ReportRecord{ID: row.ID, OperationID: row.OperationID, PayloadDigest: row.PayloadDigest, AssetID: row.AssetID, UnitID: row.UnitID, CostCentreID: row.CostCentreID,
		TripID: row.TripID, ReporterAccountID: row.ReporterAccountID, ReporterMembershipID: row.ReporterMembershipID, ReporterEmployeeID: row.ReporterEmployeeID,
		EnrollmentID: row.EnrollmentID, Report: report, Declared: orgtime.Declaration{OccurredAt: at, EntryZone: row.EntryZone, DeviceOffsetSeconds: row.DeviceOffsetSeconds},
		ReceivedAt: row.ReceivedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}, nil
}

// ReadReportTx reads one stored report of the scope.
func (s *IncidentStore) ReadReportTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, report string) (app.ReportRecord, error) {
	queries, err := s.port(ctx, db, scope)
	if err != nil || !incident.CanonicalID(report) {
		return app.ReportRecord{}, app.ErrUnavailable
	}
	row, err := queries.GetIncidentReport(ctx, q.GetIncidentReportParams{OrgID: scope.OrgID().String(), ID: report})
	if err != nil {
		return app.ReportRecord{}, intakeError(err)
	}
	return reportRecord(row)
}

// TripReportsTx supplies the accepted reports of one trip as INCIDENT and
// DAMAGE timeline facts, without text (EDR-240).
func (s *IncidentStore) TripReportsTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, tripID string) ([]tripapp.TimelineReport, error) {
	queries, err := s.port(ctx, db, scope)
	if err != nil || !incident.CanonicalID(tripID) {
		return nil, app.ErrUnavailable
	}
	rows, err := queries.IncidentTripReports(ctx, q.IncidentTripReportsParams{OrgID: scope.OrgID().String(), TripID: tripID})
	if err != nil {
		return nil, intakeError(err)
	}
	out := make([]tripapp.TimelineReport, 0, len(rows))
	for _, row := range rows {
		at, ok := storedInstant(row.OccurredAt, row.OccurredSubmicro)
		kind := map[string]trip.LogbookType{string(incident.Incident): trip.LogbookIncident, string(incident.Damage): trip.LogbookDamage}[row.ReportType]
		if !ok || kind == "" || !row.ReceivedAt.Valid {
			return nil, app.ErrUnavailable
		}
		out = append(out, tripapp.TimelineReport{ID: row.ID, Type: kind, AuthorMembershipID: row.ReporterMembershipID,
			Declared: orgtime.Declaration{OccurredAt: at, EntryZone: row.EntryZone, DeviceOffsetSeconds: row.DeviceOffsetSeconds}, ReceivedAt: row.ReceivedAt.Time.UTC()})
	}
	return out, nil
}

// ---- document link target (EDR-239) ----

// IncidentDocumentTargets resolves the INCIDENT link kind: the reporter's
// own report under check.incident.report (S cell: only the reporter).
type IncidentDocumentTargets struct{}

func (IncidentDocumentTargets) DocumentTargetPermission(kind documentdomain.TargetKind) (permission.Permission, error) {
	if kind != documentdomain.Incident {
		return "", documentapp.ErrInvalid
	}
	return app.ReportPermission, nil
}

func (IncidentDocumentTargets) ResolveDocumentTargetTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, kind documentdomain.TargetKind, id string) (documentapp.ResolvedTarget, error) {
	if tx == nil || kind != documentdomain.Incident || database.ValidateOrgTx(ctx, tx, scope) != nil {
		return documentapp.ResolvedTarget{}, documentapp.ErrUnavailable
	}
	if !incident.CanonicalID(id) {
		return documentapp.ResolvedTarget{}, documentapp.ErrNotFound
	}
	reporter, err := q.New(tx).IncidentReportReporter(ctx, q.IncidentReportReporterParams{OrgID: scope.OrgID().String(), ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return documentapp.ResolvedTarget{}, documentapp.ErrNotFound
	}
	if err != nil {
		return documentapp.ResolvedTarget{}, documentapp.ErrUnavailable
	}
	return documentapp.ResolvedTarget{Binding: documentapp.BindIncident, AuthoredBy: reporter}, nil
}

// ---- company handling and reads (B-BR-EMP-373/374) ----

func (s *IncidentStore) ready(write bool) bool {
	return s != nil && s.Pool != nil && !missing(s.Authority) && (!write || !missing(s.Audit))
}

// incidentError keeps only registered refusals; provider details stay private.
func incidentError(ctx context.Context, err error) error {
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
	if errors.As(err, &pe) {
		return app.ErrUnavailable
	}
	for _, known := range []error{access.ErrNoMembership, access.ErrMembershipSuspended, access.ErrOrgSuspended, access.ErrOrgArchived, access.ErrSetupIncomplete, access.ErrFreshAuthRequired,
		access.ErrEntitlementRequired, access.ErrUnauthenticated, permission.ErrPermissionDenied, permission.ErrResourceNotFound, app.ErrOperationReused, app.ErrNotFound, app.ErrUnavailable,
		app.ErrUnauthenticated, app.ErrConflict, app.ErrSelfApproval, incident.ErrInvalid} {
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

func validIncidentCall(scope kernel.OrgScope, actor app.IncidentActor, member string) error {
	if scope.Validate() != nil || !incident.CanonicalID(member) {
		return incident.ErrInvalid
	}
	return actor.Validate()
}

func summaryOf(row q.GetIncidentReportRow) (app.ReportSummary, error) {
	r, err := reportRecord(row)
	if err != nil {
		return app.ReportSummary{}, err
	}
	return app.ReportSummary{ReportID: r.ID, AssetID: r.AssetID, TripID: r.TripID, ReporterMembershipID: r.ReporterMembershipID, Type: r.Report.Type, Kind: r.Report.Kind,
		BlocksUse: r.Report.BlocksUse, State: r.Report.State, Revision: r.Report.Revision, OccurredAt: r.Declared.OccurredAt, ReceivedAt: r.ReceivedAt, UpdatedAt: r.UpdatedAt}, nil
}

// identity is the set of keys of one person: account, membership, employee
// record and the canonical record of its merge chain (EDR-129).
func identity(ctx context.Context, queries *q.Queries, org, account, membership, employee string) ([]string, error) {
	canonical, err := queries.IncidentCanonicalEmployee(ctx, q.IncidentCanonicalEmployeeParams{OrgID: org, EmployeeID: employee})
	if err != nil || !incident.CanonicalID(canonical) {
		return nil, app.ErrUnavailable
	}
	return []string{account, membership, employee, canonical}, nil
}

func stepOf(action, from, to string, revision int64) (incident.Step, error) {
	a, err := incident.ParseAction(action)
	if err != nil {
		return incident.Step{}, app.ErrUnavailable
	}
	f, errFrom := incident.ParseState(from)
	t, errTo := incident.ParseState(to)
	if errFrom != nil || errTo != nil || a.Target() != t || !incident.Allowed(f, a) {
		return incident.Step{}, app.ErrUnavailable
	}
	return incident.Step{Action: a, From: f, To: t, Revision: revision}, nil
}

func domainRefusal(err error) error {
	switch {
	case errors.Is(err, incident.ErrConflict), errors.Is(err, incident.ErrTransition):
		return app.ErrConflict
	case errors.Is(err, incident.ErrSelfApproval):
		return app.ErrSelfApproval
	case errors.Is(err, incident.ErrInvalid):
		return incident.ErrInvalid
	}
	return app.ErrUnavailable
}

// DecideReport applies one handling step (B-BR-EMP-373) under the
// organization root and the report row lock: authority, replay by
// operation, the duplicate's asset, the pure decision over the stored
// report, then one history row and one audit event, and authority again.
func (s *IncidentStore) DecideReport(ctx context.Context, scope kernel.OrgScope, actor app.IncidentActor, member string, command app.DecisionCommand) (app.DecisionResult, bool, error) {
	if err := validIncidentCall(scope, actor, member); err != nil {
		return app.DecisionResult{}, false, err
	}
	in, err := command.Snapshot()
	if err != nil {
		return app.DecisionResult{}, false, err
	}
	digest := command.Digest(scope)
	if digest == "" || !s.ready(true) {
		return app.DecisionResult{}, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	bound := audit.Event{OperationID: in.OperationID, ActorID: actor.AccountID, Action: stepActions[in.Action], TargetType: ReportTargetType, TargetID: in.ReportID, RequestHash: digest}
	var result app.DecisionResult
	replayed := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		handler, err := s.Authority.CheckIncidentTx(ctx, tx, scope, actor, member, in.ReportID, app.IncidentDecide)
		if err != nil {
			return err
		}
		recheck := func() error {
			again, err := s.Authority.CheckIncidentTx(ctx, tx, scope, actor, member, in.ReportID, app.IncidentDecide)
			if err != nil {
				return err
			}
			if again != handler {
				return app.ErrUnavailable
			}
			return nil
		}
		queries := q.New(tx)
		report, err := queries.IncidentReportForUpdate(ctx, q.IncidentReportForUpdateParams{OrgID: org, ID: in.ReportID})
		if err != nil {
			return err
		}
		prior, err := queries.IncidentTransitionByOperation(ctx, q.IncidentTransitionByOperationParams{OrgID: org, OperationID: in.OperationID})
		if err == nil {
			if prior.ReportID != in.ReportID || prior.ActorAccountID != actor.AccountID || prior.RequestHash != digest {
				return app.ErrOperationReused
			}
			repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(context.Context, pgx.Tx) error { return app.ErrUnavailable })
			if err != nil {
				return err
			}
			if !repeated || !prior.RecordedAt.Valid {
				return app.ErrUnavailable
			}
			step, err := stepOf(prior.Action, prior.FromState, prior.ToState, prior.Revision)
			if err != nil {
				return err
			}
			result, replayed = app.DecisionResult{ReportID: in.ReportID, Step: step, RecordedAt: prior.RecordedAt.Time.UTC()}, true
			return recheck()
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if in.DuplicateOf != "" {
			asset, err := queries.IncidentReportAsset(ctx, q.IncidentReportAssetParams{OrgID: org, ID: in.DuplicateOf})
			if errors.Is(err, pgx.ErrNoRows) {
				return app.ErrNotFound
			}
			if err != nil {
				return err
			}
			if asset != report.AssetID {
				return incident.ErrInvalid
			}
		}
		reporter, err := identity(ctx, queries, org, report.ReporterAccountID, report.ReporterMembershipID, report.ReporterEmployeeID)
		if err != nil {
			return err
		}
		self, err := identity(ctx, queries, org, handler.AccountID, handler.MembershipID, handler.EmployeeID)
		if err != nil {
			return err
		}
		step, err := incident.Decide(incident.Current{State: incident.State(report.State), Revision: report.Revision, BlocksUse: report.BlocksUse, ReportID: in.ReportID, Reporter: reporter},
			incident.Command{Action: in.Action, ExpectedRevision: in.ExpectedRevision, Note: in.Note, DuplicateOf: in.DuplicateOf, Handler: self})
		if err != nil {
			return domainRefusal(err)
		}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(ctx context.Context, tx pgx.Tx) error {
			return q.New(tx).InsertIncidentTransition(ctx, q.InsertIncidentTransitionParams{OrgID: org, ReportID: in.ReportID, Revision: step.Revision, Action: string(step.Action),
				FromState: string(step.From), ToState: string(step.To), Note: nullableText(in.Note), DuplicateOf: nullable(in.DuplicateOf), ActorAccountID: handler.AccountID,
				ActorMembershipID: handler.MembershipID, ActorEmployeeID: handler.EmployeeID, OperationID: in.OperationID, RequestHash: digest})
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		stored, err := queries.IncidentTransitionByOperation(ctx, q.IncidentTransitionByOperationParams{OrgID: org, OperationID: in.OperationID})
		if err != nil || !stored.RecordedAt.Valid || stored.Revision != step.Revision {
			return app.ErrUnavailable
		}
		result = app.DecisionResult{ReportID: in.ReportID, Step: step, RecordedAt: stored.RecordedAt.Time.UTC()}
		return recheck()
	})
	if err != nil {
		return app.DecisionResult{}, false, incidentError(ctx, err)
	}
	return result, replayed, nil
}

// readTx runs one read between two authority checks of the action and target.
func (s *IncidentStore) readTx(ctx context.Context, scope kernel.OrgScope, actor app.IncidentActor, member, target string, action app.IncidentAction, read func(*q.Queries) error) error {
	if err := validIncidentCall(scope, actor, member); err != nil {
		return err
	}
	if !s.ready(false) {
		return app.ErrUnavailable
	}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if _, err := s.Authority.CheckIncidentTx(ctx, tx, scope, actor, member, target, action); err != nil {
			return err
		}
		if err := read(q.New(tx)); err != nil {
			return err
		}
		_, err := s.Authority.CheckIncidentTx(ctx, tx, scope, actor, member, target, action)
		return err
	})
	return incidentError(ctx, err)
}

// ReadReport reads one report with its history and its linked documents'
// own storage states; never content (B-BR-EMP-372/374).
func (s *IncidentStore) ReadReport(ctx context.Context, scope kernel.OrgScope, actor app.IncidentActor, member, reportID string) (app.ReportView, error) {
	if !incident.CanonicalID(reportID) {
		return app.ReportView{}, app.ErrNotFound
	}
	org := scope.OrgID().String()
	var view app.ReportView
	err := s.readTx(ctx, scope, actor, member, reportID, app.IncidentRead, func(queries *q.Queries) error {
		row, err := queries.GetIncidentReport(ctx, q.GetIncidentReportParams{OrgID: org, ID: reportID})
		if err != nil {
			return err
		}
		summary, err := summaryOf(row)
		if err != nil {
			return err
		}
		view = app.ReportView{Summary: summary, Description: row.Description, Location: optionalText(row.Location), UnitID: row.UnitID, CostCentre: row.CostCentreID}
		steps, err := queries.IncidentReportTransitions(ctx, q.IncidentReportTransitionsParams{OrgID: org, ReportID: reportID})
		if err != nil {
			return err
		}
		for _, h := range steps {
			step, err := stepOf(h.Action, h.FromState, h.ToState, h.Revision)
			if err != nil || !h.RecordedAt.Valid {
				return app.ErrUnavailable
			}
			view.History = append(view.History, app.TransitionView{Revision: step.Revision, Action: step.Action, From: step.From, To: step.To, Note: optionalText(h.Note),
				DuplicateOf: h.DuplicateOf, ActorMembershipID: h.ActorMembershipID, RecordedAt: h.RecordedAt.Time.UTC()})
		}
		links, err := queries.IncidentReportAttachments(ctx, q.IncidentReportAttachmentsParams{OrgID: org, ReportID: reportID})
		if err != nil {
			return err
		}
		for _, l := range links {
			if _, err := documentdomain.ParseState(documentdomain.Storage, l.StorageState); err != nil || !l.CreatedAt.Valid {
				return app.ErrUnavailable
			}
			view.Attachments = append(view.Attachments, app.AttachmentView{DocumentID: l.DocumentID, Storage: l.StorageState, LinkedAt: l.CreatedAt.Time.UTC()})
		}
		return nil
	})
	if err != nil {
		return app.ReportView{}, err
	}
	return view, nil
}

func pageIDs(ids []string, limit int) ([]string, string) {
	if len(ids) > limit {
		ids = ids[:limit]
		return ids, ids[len(ids)-1]
	}
	return ids, ""
}

// ListReports pages the organization's reports by identifier.
func (s *IncidentStore) ListReports(ctx context.Context, scope kernel.OrgScope, actor app.IncidentActor, member string, filter app.ReportFilter) (app.ReportPage, error) {
	if err := filter.Validate(); err != nil {
		return app.ReportPage{}, err
	}
	org := scope.OrgID().String()
	page := app.ReportPage{Items: []app.ReportSummary{}}
	err := s.readTx(ctx, scope, actor, member, org, app.IncidentQueue, func(queries *q.Queries) error {
		ids, err := queries.ListIncidentReports(ctx, q.ListIncidentReportsParams{OrgID: org, AfterID: filter.AfterID, RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		ids, page.NextID = pageIDs(ids, filter.Limit)
		for _, id := range ids {
			row, err := queries.GetIncidentReport(ctx, q.GetIncidentReportParams{OrgID: org, ID: id})
			if err != nil {
				return err
			}
			summary, err := summaryOf(row)
			if err != nil {
				return err
			}
			page.Items = append(page.Items, summary)
		}
		return nil
	})
	if err != nil {
		return app.ReportPage{}, err
	}
	return page, nil
}

// ListOwnReports pages the member's own reports with their text.
func (s *IncidentStore) ListOwnReports(ctx context.Context, scope kernel.OrgScope, actor app.IncidentActor, member string, filter app.ReportFilter) (app.OwnReportPage, error) {
	if err := filter.Validate(); err != nil {
		return app.OwnReportPage{}, err
	}
	org := scope.OrgID().String()
	page := app.OwnReportPage{Items: []app.OwnReport{}}
	err := s.readTx(ctx, scope, actor, member, member, app.IncidentOwn, func(queries *q.Queries) error {
		ids, err := queries.ListOwnIncidentReports(ctx, q.ListOwnIncidentReportsParams{OrgID: org, MembershipID: member, AfterID: filter.AfterID, RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		ids, page.NextID = pageIDs(ids, filter.Limit)
		for _, id := range ids {
			row, err := queries.GetIncidentReport(ctx, q.GetIncidentReportParams{OrgID: org, ID: id})
			if err != nil {
				return err
			}
			summary, err := summaryOf(row)
			if err != nil || summary.ReporterMembershipID != member {
				return app.ErrUnavailable
			}
			page.Items = append(page.Items, app.OwnReport{Summary: summary, Description: row.Description, Location: optionalText(row.Location)})
		}
		return nil
	})
	if err != nil {
		return app.OwnReportPage{}, err
	}
	return page, nil
}

// sameReport proves the stored report is exactly this operation's fact.
func sameReport(source syncapp.Source, v app.ReportInput, r app.ReportRecord) bool {
	same := func(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
	return r.ID == v.ReportID && r.OperationID == v.OperationID && r.PayloadDigest == v.PayloadDigest && r.AssetID == v.AssetID && r.TripID == v.TripID &&
		r.ReporterMembershipID == source.MembershipID && r.ReporterAccountID == source.Actor.AccountID && r.EnrollmentID == source.EnrollmentID &&
		r.Report.Type == v.Report.Type && r.Report.Kind == v.Report.Kind && r.Report.Description == v.Report.Description && same(r.Report.Location, v.Report.Location) &&
		r.Report.BlocksUse == v.Report.BlocksUse && r.Declared.OccurredAt.Equal(v.Declared.OccurredAt) && r.Declared.EntryZone == v.Declared.EntryZone &&
		r.Declared.DeviceOffsetSeconds == v.Declared.DeviceOffsetSeconds && !r.ReceivedAt.IsZero() && slices.Contains([]int64{1, 2, 3, 4}, r.Report.Revision)
}
