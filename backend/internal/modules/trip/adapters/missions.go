package adapters

import (
	"context"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/mission"
	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MissionTargetType is the audit target vocabulary of a mission.
const MissionTargetType = "trip_mission"

var missionActions = map[app.MissionCommandKind]string{
	app.MissionCreate:        "trip.mission.created",
	app.MissionAssignCommand: "trip.mission.assigned",
	app.MissionCancelCommand: "trip.mission.cancelled",
}

// MissionAuditDefinitions registers one action per company command kind
// (B-BR-EMP-358); responses are audited by their intake receipt.
func MissionAuditDefinitions() []audit.Definition {
	out := []audit.Definition{}
	for _, kind := range []app.MissionCommandKind{app.MissionCreate, app.MissionAssignCommand, app.MissionCancelCommand} {
		out = append(out, audit.Definition{Action: missionActions[kind], TargetType: MissionTargetType})
	}
	return out
}

// MissionStore owns the mission commands and reads (company) and the
// response facts of the signed intake (EMP-TRIP-008, migration 000227).
// Availability is the EMP-FLEET-007 start decision used only as a warning.
type MissionStore struct {
	Pool         *pgxpool.Pool
	Authority    app.MissionAuthority
	Audit        auditing.TransactionRunner
	Availability fleetapp.StartAvailabilityPort
}

var (
	_ app.MissionRepository    = (*MissionStore)(nil)
	_ app.MissionResponseStore = (*MissionStore)(nil)
)

func (s *MissionStore) ready(write bool) bool {
	return s != nil && s.Pool != nil && !absent(s.Authority) && (!write || !absent(s.Audit) && !absent(s.Availability))
}

// missionError keeps only registered refusals; provider details stay private.
func missionError(ctx context.Context, err error) error {
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
	if errors.As(err, &pe) && (pe.Code == "40001" || pe.Code == "40P01") {
		return app.ErrUnavailable
	}
	for _, known := range []error{access.ErrNoMembership, access.ErrMembershipSuspended, access.ErrOrgSuspended, access.ErrOrgArchived, access.ErrSetupIncomplete, access.ErrFreshAuthRequired,
		access.ErrEntitlementRequired, access.ErrUnauthenticated, permission.ErrPermissionDenied, permission.ErrResourceNotFound, app.ErrOperationReused, app.ErrNotFound, app.ErrUnavailable,
		app.ErrMissionUnauthenticated, app.ErrMissionIneligible, trip.ErrConflict, trip.ErrInvalidTransition, trip.ErrUnsupportedCommand, trip.ErrInvalid, trip.ErrScope, trip.ErrForbidden} {
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

func validMissionCall(scope kernel.OrgScope, actor app.MissionActor, member string) error {
	if scope.Validate() != nil || !canonicalID(member) {
		return trip.ErrInvalid
	}
	return actor.Validate()
}

func at(v pgtype.Timestamptz) (time.Time, error) {
	if !v.Valid || v.InfinityModifier != pgtype.Finite {
		return time.Time{}, app.ErrUnavailable
	}
	return v.Time.UTC(), nil
}

// missionRow is the shared shape of every head query.
type missionRow struct {
	ID                   string
	UnitID               string
	AssetID              string
	Title                string
	StartsAt             pgtype.Timestamptz
	EndsAt               pgtype.Timestamptz
	State                string
	Revision             int64
	AssigneeEmployeeID   string
	AssigneeMembershipID string
	Answer               string
	AnswerReason         pgtype.Text
	CreatedAt            pgtype.Timestamptz
	UpdatedAt            pgtype.Timestamptz
}

func missionRecord(r missionRow) (app.MissionRecord, error) {
	v := app.MissionView{MissionID: r.ID, UnitID: r.UnitID, AssetID: r.AssetID, Title: r.Title, State: trip.MissionState(r.State), Revision: r.Revision,
		AssigneeEmployeeID: r.AssigneeEmployeeID, Answer: trip.MissionResponse(r.Answer), AnswerReason: optionalText(r.AnswerReason)}
	var err error
	if v.StartsAt, err = at(r.StartsAt); err != nil {
		return app.MissionRecord{}, err
	}
	if v.EndsAt, err = at(r.EndsAt); err != nil {
		return app.MissionRecord{}, err
	}
	if v.CreatedAt, err = at(r.CreatedAt); err != nil {
		return app.MissionRecord{}, err
	}
	if v.UpdatedAt, err = at(r.UpdatedAt); err != nil {
		return app.MissionRecord{}, err
	}
	return app.MissionRecord{View: v, AssigneeMembershipID: r.AssigneeMembershipID}, nil
}

func (s *MissionStore) head(ctx context.Context, queries *q.Queries, org, mission string, lock bool) (app.MissionRecord, error) {
	if lock {
		r, err := queries.MissionForUpdate(ctx, q.MissionForUpdateParams{OrgID: org, ID: mission})
		if err != nil {
			return app.MissionRecord{}, err
		}
		return missionRecord(missionRow(r))
	}
	r, err := queries.GetMission(ctx, q.GetMissionParams{OrgID: org, ID: mission})
	if err != nil {
		return app.MissionRecord{}, err
	}
	return missionRecord(missionRow(r))
}

func facts(r app.MissionRecord) app.MissionFacts {
	return app.MissionFacts{MissionID: r.View.MissionID, UnitID: r.View.UnitID, AssetID: r.View.AssetID}
}

// assignee resolves an employee of the organization (else not found) whose
// live membership is ACTIVE on an ACTIVE, unmerged record (else ineligible).
func assignee(ctx context.Context, queries *q.Queries, org, employee string) (string, error) {
	row, err := queries.MissionAssigneeFacts(ctx, q.MissionAssigneeFactsParams{OrgID: org, ID: employee})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", app.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if row.State != "ACTIVE" || row.Merged || !canonicalID(row.MembershipID) {
		return "", app.ErrMissionIneligible
	}
	return row.MembershipID, nil
}

// warning is the FLEET-007 start decision of the assignee at the planned
// start, reported only when the start would not be admitted.
func (s *MissionStore) warning(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, record app.MissionRecord, member string) (*app.MissionWarning, error) {
	if record.View.AssetID == "" {
		return nil, nil
	}
	decision, err := s.Availability.StartDecisionDBTX(ctx, syncapp.Borrow(tx), scope, record.View.AssetID, member, record.View.StartsAt)
	if err != nil {
		return nil, app.ErrUnavailable
	}
	if decision.Admitted {
		return nil, nil
	}
	if decision.Code != "ASSET_BUSY" && decision.Code != "ASSET_UNAVAILABLE" || decision.Assessment.Availability == "" {
		return nil, app.ErrUnavailable
	}
	return &app.MissionWarning{Code: string(decision.Code), Availability: string(decision.Assessment.Availability)}, nil
}

func nullable(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

// create inserts the DRAFT head and its CREATE transition (B-BR-EMP-354).
func (s *MissionStore) create(ctx context.Context, queries *q.Queries, scope kernel.OrgScope, actor app.MissionActor, member, mission string, in app.MissionCommandInput) (app.MissionAck, error) {
	org := scope.OrgID().String()
	state, err := queries.MissionUnitState(ctx, q.MissionUnitStateParams{OrgID: org, ID: in.UnitID})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.MissionAck{}, app.ErrNotFound
	}
	if err != nil {
		return app.MissionAck{}, err
	}
	if state != "ACTIVE" {
		return app.MissionAck{}, app.ErrMissionIneligible
	}
	input := trip.MissionInput{Title: in.Title, Window: trip.MissionWindow{From: in.StartsAt, To: in.EndsAt}}
	if input.Reference, err = trip.NewMissionRef(scope, mission); err != nil {
		return app.MissionAck{}, app.ErrUnavailable
	}
	if input.Unit, err = trip.NewUnitRef(scope, in.UnitID); err != nil {
		return app.MissionAck{}, err
	}
	if in.AssetID != "" {
		asset, err := queries.MissionAssetFacts(ctx, q.MissionAssetFactsParams{OrgID: org, ID: in.AssetID})
		if errors.Is(err, pgx.ErrNoRows) {
			return app.MissionAck{}, app.ErrNotFound
		}
		if err != nil {
			return app.MissionAck{}, err
		}
		if asset.State != "ACTIVE" || asset.Kind == "IMPLEMENT" {
			return app.MissionAck{}, app.ErrMissionIneligible
		}
		ref, err := trip.NewMissionAssetRef(scope, in.AssetID)
		if err != nil {
			return app.MissionAck{}, err
		}
		input.Asset = &ref
	}
	if _, err := trip.NewMission(scope, input); err != nil {
		return app.MissionAck{}, err
	}
	if err := queries.InsertMission(ctx, q.InsertMissionParams{OrgID: org, ID: mission, UnitID: in.UnitID, AssetID: nullable(in.AssetID), Title: in.Title,
		StartsAt: pgtype.Timestamptz{Time: in.StartsAt, Valid: true}, EndsAt: pgtype.Timestamptz{Time: in.EndsAt, Valid: true}, CreatedBy: actor.AccountID,
		CreatedByMembershipID: member, CreateOperationID: in.OperationID}); err != nil {
		return app.MissionAck{}, err
	}
	if err := queries.InsertMissionTransition(ctx, q.InsertMissionTransitionParams{OrgID: org, MissionID: mission, Revision: 1, Kind: string(app.MissionCreate),
		ToState: string(trip.MissionDraft), Source: "COMPANY", ActorAccountID: actor.AccountID, ActorMembershipID: member, OperationID: in.OperationID}); err != nil {
		return app.MissionAck{}, err
	}
	return app.MissionAck{MissionID: mission, Revision: 1, State: trip.MissionDraft}, nil
}

// change applies ASSIGN or CANCEL to the locked head through the pure rules.
func (s *MissionStore) change(ctx context.Context, tx pgx.Tx, queries *q.Queries, scope kernel.OrgScope, actor app.MissionActor, member string, in app.MissionCommandInput) (app.MissionAck, error) {
	org := scope.OrgID().String()
	record, err := s.head(ctx, queries, org, in.MissionID, true)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.MissionAck{}, app.ErrNotFound
	}
	if err != nil {
		return app.MissionAck{}, err
	}
	current, err := record.Domain(scope)
	if err != nil {
		return app.MissionAck{}, err
	}
	transition := q.InsertMissionTransitionParams{OrgID: org, MissionID: in.MissionID, Revision: record.View.Revision + 1, Kind: string(in.Kind),
		FromState: nullable(string(record.View.State)), Source: "COMPANY", ActorAccountID: actor.AccountID, ActorMembershipID: member, OperationID: in.OperationID}
	var next trip.Mission
	var warning *app.MissionWarning
	switch in.Kind {
	case app.MissionAssignCommand:
		target, err := assignee(ctx, queries, org, in.AssigneeEmployeeID)
		if err != nil {
			return app.MissionAck{}, err
		}
		var to trip.MissionAssignee
		if to.Employee, err = trip.NewEmployeeRef(scope, in.AssigneeEmployeeID); err != nil {
			return app.MissionAck{}, err
		}
		if to.Member, err = trip.NewMembershipRef(scope, target); err != nil {
			return app.MissionAck{}, app.ErrUnavailable
		}
		if next, err = current.Assign(scope, in.ExpectedRevision, to); err != nil {
			return app.MissionAck{}, err
		}
		eligible, err := s.Authority.EligibleAssigneeTx(ctx, tx, scope, target, facts(record))
		if err != nil {
			return app.MissionAck{}, err
		}
		if !eligible {
			return app.MissionAck{}, app.ErrMissionIneligible
		}
		if warning, err = s.warning(ctx, tx, scope, record, target); err != nil {
			return app.MissionAck{}, err
		}
	case app.MissionCancelCommand:
		if next, err = current.Cancel(scope, in.ExpectedRevision, in.Reason); err != nil {
			return app.MissionAck{}, err
		}
		transition.Reason = nullable(in.Reason)
	default:
		return app.MissionAck{}, trip.ErrInvalid
	}
	v := next.Snapshot()
	employee, membership := "", ""
	if v.Assignee != nil {
		employee, membership = v.Assignee.Employee.ID(), v.Assignee.Member.ID()
	}
	transition.ToState, transition.AssigneeEmployeeID, transition.AssigneeMembershipID = string(v.State), nullable(employee), nullable(membership)
	if err := queries.InsertMissionTransition(ctx, transition); err != nil {
		return app.MissionAck{}, err
	}
	advance := q.AdvanceMissionParams{OrgID: org, ID: in.MissionID, Revision: record.View.Revision, State: string(v.State), AssigneeEmployeeID: nullable(employee), AssigneeMembershipID: nullable(membership)}
	if v.Answer != nil {
		advance.Answer, advance.AnswerReason = nullable(string(v.Answer.Kind)), text(v.Answer.Reason)
	}
	if n, err := queries.AdvanceMission(ctx, advance); err != nil || n != 1 {
		if err == nil {
			err = trip.ErrConflict
		}
		return app.MissionAck{}, err
	}
	return app.MissionAck{MissionID: in.MissionID, Revision: v.Revision, State: v.State, AssigneeEmployeeID: employee, Warning: warning}, nil
}

// CommandMission runs one command under the organization root lock:
// authority, receipt replay, exactly one audit event, the effect, the receipt
// and authority again, in one transaction (B-BR-EMP-358).
func (s *MissionStore) CommandMission(ctx context.Context, scope kernel.OrgScope, actor app.MissionActor, member, newID string, command app.MissionCommand) (app.MissionAck, bool, error) {
	if err := validMissionCall(scope, actor, member); err != nil {
		return app.MissionAck{}, false, err
	}
	in, err := command.Snapshot()
	if err != nil {
		return app.MissionAck{}, false, err
	}
	mission, target := in.MissionID, in.MissionID
	if in.Kind == app.MissionCreate {
		mission, target = newID, scope.OrgID().String()
	}
	if !canonicalID(mission) || in.Kind != app.MissionCreate && newID != "" {
		return app.MissionAck{}, false, trip.ErrInvalid
	}
	if !s.ready(true) {
		return app.MissionAck{}, false, app.ErrUnavailable
	}
	org, digest := scope.OrgID().String(), command.Digest(scope)
	var ack app.MissionAck
	replayed := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckMissionTx(ctx, tx, scope, actor, member, target, app.MissionManage); err != nil {
			return err
		}
		queries := q.New(tx)
		state, err := queries.MissionRootLock(ctx, org)
		if err != nil {
			return err
		}
		if !state.Valid || state.String != "ACTIVE" {
			return access.ErrSetupIncomplete
		}
		prior, err := queries.MissionCommandReceipt(ctx, q.MissionCommandReceiptParams{OrgID: org, OperationID: in.OperationID})
		if err == nil {
			if prior.ActorID != actor.AccountID || prior.Kind != string(in.Kind) || prior.RequestHash != digest || in.Kind != app.MissionCreate && prior.MissionID != in.MissionID {
				return app.ErrOperationReused
			}
			ack = app.MissionAck{MissionID: prior.MissionID, Revision: prior.ResultRevision, State: trip.MissionState(prior.ResultState), AssigneeEmployeeID: prior.AssigneeEmployeeID}
			if prior.WarningCode != "" {
				ack.Warning = &app.MissionWarning{Code: prior.WarningCode, Availability: prior.WarningAvailability}
			}
			replayed = true
			return s.Authority.CheckMissionTx(ctx, tx, scope, actor, member, target, app.MissionManage)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		event := audit.Event{OperationID: in.OperationID, ActorID: actor.AccountID, Action: missionActions[in.Kind], TargetType: MissionTargetType, TargetID: mission, RequestHash: digest}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			queries := q.New(tx)
			var err error
			if in.Kind == app.MissionCreate {
				ack, err = s.create(ctx, queries, scope, actor, member, mission, in)
			} else {
				ack, err = s.change(ctx, tx, queries, scope, actor, member, in)
			}
			if err != nil {
				return err
			}
			receipt := q.InsertMissionCommandParams{OrgID: org, OperationID: in.OperationID, ActorID: actor.AccountID, MissionID: mission, Kind: string(in.Kind), RequestHash: digest,
				ResultRevision: ack.Revision, ResultState: string(ack.State)}
			if ack.Warning != nil {
				receipt.WarningCode, receipt.WarningAvailability = nullable(ack.Warning.Code), nullable(ack.Warning.Availability)
			}
			return queries.InsertMissionCommand(ctx, receipt)
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		return s.Authority.CheckMissionTx(ctx, tx, scope, actor, member, target, app.MissionManage)
	})
	if err != nil {
		return app.MissionAck{}, false, missionError(ctx, err)
	}
	return ack, replayed, nil
}

// read runs one read between two authority checks of the action and target.
func (s *MissionStore) read(ctx context.Context, scope kernel.OrgScope, actor app.MissionActor, member, target string, action app.MissionAction, call func(*q.Queries) error) error {
	if err := validMissionCall(scope, actor, member); err != nil {
		return err
	}
	if !s.ready(false) {
		return app.ErrUnavailable
	}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckMissionTx(ctx, tx, scope, actor, member, target, action); err != nil {
			return err
		}
		if err := call(q.New(tx)); err != nil {
			return err
		}
		return s.Authority.CheckMissionTx(ctx, tx, scope, actor, member, target, action)
	})
	return missionError(ctx, err)
}

// ListMissions pages the organization's missions, or the member's own.
func (s *MissionStore) ListMissions(ctx context.Context, scope kernel.OrgScope, actor app.MissionActor, member string, own bool, filter app.MissionFilter) (app.MissionPage, error) {
	if err := filter.Validate(); err != nil {
		return app.MissionPage{}, err
	}
	action, target := app.MissionRead, scope.OrgID().String()
	if own {
		action, target = app.MissionOwnRead, member
	}
	var page app.MissionPage
	err := s.read(ctx, scope, actor, member, target, action, func(queries *q.Queries) error {
		org := scope.OrgID().String()
		var rows []missionRow
		if own {
			found, err := queries.ListOwnMissions(ctx, q.ListOwnMissionsParams{OrgID: org, MembershipID: member, AfterID: filter.AfterID, PageLimit: int32(filter.Limit) + 1})
			if err != nil {
				return err
			}
			for _, r := range found {
				rows = append(rows, missionRow(r))
			}
		} else {
			found, err := queries.ListMissions(ctx, q.ListMissionsParams{OrgID: org, AfterID: filter.AfterID, PageLimit: int32(filter.Limit) + 1})
			if err != nil {
				return err
			}
			for _, r := range found {
				rows = append(rows, missionRow(r))
			}
		}
		page = app.MissionPage{Missions: make([]app.MissionView, 0, min(len(rows), filter.Limit))}
		for i, r := range rows {
			if i == filter.Limit {
				page.NextID = page.Missions[len(page.Missions)-1].MissionID
				break
			}
			record, err := missionRecord(r)
			if err != nil || own && record.AssigneeMembershipID != member {
				return app.ErrUnavailable
			}
			page.Missions = append(page.Missions, record.View)
		}
		return nil
	})
	if err != nil {
		return app.MissionPage{}, err
	}
	return page, nil
}

// ReadMission reads one mission of the scope.
func (s *MissionStore) ReadMission(ctx context.Context, scope kernel.OrgScope, actor app.MissionActor, member, mission string) (app.MissionView, error) {
	var v app.MissionView
	err := s.read(ctx, scope, actor, member, mission, app.MissionRead, func(queries *q.Queries) error {
		record, err := s.head(ctx, queries, scope.OrgID().String(), mission, false)
		if err != nil {
			return err
		}
		v = record.View
		return nil
	})
	if err != nil {
		return app.MissionView{}, err
	}
	return v, nil
}

// ---- response facts on the borrowed intake transaction (EDR-229) ----

func (s *MissionStore) port(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope) (*q.Queries, error) {
	db = syncapp.Borrow(db)
	if s == nil || db == nil || database.ValidateOrgDBTX(ctx, db, scope) != nil {
		return nil, app.ErrUnavailable
	}
	return q.New(db), nil
}

// MissionTx reads the stored head; with lock it takes the organization root
// and then the mission row, the lock order of the company commands.
func (s *MissionStore) MissionTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, mission string, lock bool) (app.MissionRecord, error) {
	queries, err := s.port(ctx, db, scope)
	if err != nil || !canonicalID(mission) {
		return app.MissionRecord{}, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	if lock {
		// Root before the mission row: the order of every company command.
		if _, err := queries.MissionRootLock(ctx, org); err != nil {
			return app.MissionRecord{}, storeError(err)
		}
	}
	record, err := s.head(ctx, queries, org, mission, lock)
	if err != nil {
		return app.MissionRecord{}, storeError(err)
	}
	return record, nil
}

// RecordResponseTx appends the response transition of the source and moves
// the head to the decided mission.
func (s *MissionStore) RecordResponseTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, command app.RespondCommand, record app.MissionRecord, next trip.Mission) error {
	queries, err := s.port(ctx, db, scope)
	if err != nil {
		return err
	}
	v, err := command.Snapshot()
	n := next.Snapshot()
	if err != nil || source.MembershipID != v.AuthorMembershipID || n.Answer == nil || n.Assignee == nil || n.Answer.Member.ID() != source.MembershipID ||
		n.Assignee.Member.ID() != record.AssigneeMembershipID || n.Revision != record.View.Revision+1 || n.Reference.ID() != record.View.MissionID {
		return app.ErrUnavailable
	}
	org := scope.OrgID().String()
	if err := queries.InsertMissionTransition(ctx, q.InsertMissionTransitionParams{OrgID: org, MissionID: v.MissionID, Revision: n.Revision, Kind: string(n.Answer.Kind.Command()),
		FromState: nullable(string(record.View.State)), ToState: string(n.State), Source: "DEVICE", ActorAccountID: source.Actor.AccountID, ActorMembershipID: source.MembershipID,
		OperationID: v.OperationID, EnrollmentID: nullable(source.EnrollmentID), AssigneeEmployeeID: nullable(record.View.AssigneeEmployeeID),
		AssigneeMembershipID: nullable(record.AssigneeMembershipID), Reason: text(v.Reason)}); err != nil {
		return storeError(err)
	}
	changed, err := queries.AdvanceMission(ctx, q.AdvanceMissionParams{OrgID: org, ID: v.MissionID, Revision: record.View.Revision, State: string(n.State),
		AssigneeEmployeeID: nullable(record.View.AssigneeEmployeeID), AssigneeMembershipID: nullable(record.AssigneeMembershipID), Answer: nullable(string(n.Answer.Kind)), AnswerReason: text(n.Answer.Reason)})
	if err != nil || changed != 1 {
		return app.ErrUnavailable
	}
	return nil
}

// ResponseTx reads the stored response of one operation.
func (s *MissionStore) ResponseTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, operation string) (app.MissionResponseRecord, error) {
	queries, err := s.port(ctx, db, scope)
	if err != nil || !canonicalID(operation) {
		return app.MissionResponseRecord{}, app.ErrUnavailable
	}
	row, err := queries.MissionResponse(ctx, q.MissionResponseParams{OrgID: scope.OrgID().String(), OperationID: operation})
	if err != nil {
		return app.MissionResponseRecord{}, storeError(err)
	}
	kind := trip.ResponseAccept
	switch trip.MissionCommand(row.Kind) {
	case trip.MissionAccept:
	case trip.MissionDecline:
		kind = trip.ResponseDecline
	default:
		return app.MissionResponseRecord{}, app.ErrUnavailable
	}
	return app.MissionResponseRecord{MissionID: row.MissionID, OperationID: row.OperationID, ActorMembershipID: row.ActorMembershipID, ActorAccountID: row.ActorAccountID,
		EnrollmentID: row.EnrollmentID, Revision: row.Revision, Response: kind, Reason: optionalText(row.Reason)}, nil
}
