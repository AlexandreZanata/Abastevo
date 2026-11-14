package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/membership"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type deviceCapabilities interface {
	GuardWrite(context.Context, pgx.Tx, kernel.OrgScope, string) error
}

type DeviceStore struct {
	Pool         *pgxpool.Pool
	Authority    app.DeviceAuthority
	Capabilities deviceCapabilities
	Audit        auditing.TransactionRunner
	Limit        int64
	NewID        func() (string, error)
}

var _ app.DeviceRepository = (*DeviceStore)(nil)

func DeviceAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: "idn.device.enrolled", TargetType: "device", Reasons: []string{"enrolled"}, RequireReason: true}, {Action: "idn.device.revoked", TargetType: "device", Reasons: []string{"revoked"}, RequireReason: true}}
}
func (s *DeviceStore) ready() bool {
	return s != nil && s.Pool != nil && s.Authority != nil && s.Capabilities != nil && s.Audit != nil && s.limit() >= 1 && s.limit() <= 5
}
func (s *DeviceStore) limit() int64 {
	if s.Limit == 0 {
		return 3
	}
	return s.Limit
}
func deviceError(ctx context.Context, e error) error {
	if e == nil {
		return nil
	}
	for _, v := range []error{app.ErrDeviceRevoked, app.ErrDeviceLimit, app.ErrDeviceConflict, app.ErrEmployeeOperationReused, app.ErrEmployeeNotFound, app.ErrEmployeeUnauthenticated, app.ErrManagedRestricted, app.ErrRegisterUnavailable, d.ErrConflict, d.ErrInvalid, d.ErrScope, access.ErrOrgSuspended, access.ErrOrgArchived, access.ErrSetupIncomplete, access.ErrEntitlementRequired, access.ErrNoMembership} {
		if errors.Is(e, v) {
			return v
		}
	}
	var pe *pgconn.PgError
	if errors.As(e, &pe) {
		switch pe.Code {
		case "P9401":
			return app.ErrDeviceRevoked
		case "P9402":
			return app.ErrDeviceLimit
		case "P9005", "P9403", "23505":
			return app.ErrDeviceConflict
		}
	}
	if errors.Is(e, pgx.ErrNoRows) {
		return app.ErrEmployeeNotFound
	}
	return employeeRegisterError(ctx, e)
}
func deviceStamp(v pgtype.Timestamptz) (time.Time, error) {
	if !v.Valid || v.InfinityModifier != pgtype.Finite {
		return time.Time{}, app.ErrRegisterUnavailable
	}
	return v.Time.UTC(), nil
}
func deviceRecord(scope kernel.OrgScope, r q.GetDeviceRow) (app.DeviceRecord, error) {
	if scope.Validate() != nil || r.OrgID != scope.OrgID().String() || r.EnrollmentLimit < 1 || r.EnrollmentLimit > 5 {
		return app.DeviceRecord{}, app.ErrRegisterUnavailable
	}
	enrollment, e := d.RehydrateDevice(scope, d.DeviceSnapshot{OrgID: scope.OrgID(), ID: r.ID, MembershipID: r.MembershipID, DeviceID: r.KeyID, Fingerprint: r.Fingerprint, State: d.DeviceState(r.State), Revision: r.Revision, Revocation: d.RevocationClass(r.RevocationClass)})
	if e != nil {
		return app.DeviceRecord{}, app.ErrRegisterUnavailable
	}
	at, e := deviceStamp(r.EnrolledAt)
	if e != nil {
		return app.DeviceRecord{}, e
	}
	result := app.DeviceRecord{Enrollment: enrollment, Label: r.Label, EnrolledAt: at, SourceType: r.SourceType, SourceID: r.SourceID, SourceRevision: r.SourceRevision}
	if r.LastSyncAt.Valid {
		at, e := deviceStamp(r.LastSyncAt)
		if e != nil {
			return app.DeviceRecord{}, e
		}
		result.LastSyncAt = &at
	}
	if r.RevokedAt.Valid {
		at, e := deviceStamp(r.RevokedAt)
		if e != nil {
			return app.DeviceRecord{}, e
		}
		result.RevokedAt = &at
	}
	if result.Validate(scope) != nil {
		return app.DeviceRecord{}, app.ErrRegisterUnavailable
	}
	return result, nil
}

// The receipt has no personal family, account, contributor or key inventory.
// Explicit mapping avoids accidentally serializing domain-private objects.
type deviceReceipt struct {
	ID              string            `json:"id"`
	MembershipID    string            `json:"membership_id"`
	Label           string            `json:"label"`
	State           d.DeviceState     `json:"state"`
	Revision        int64             `json:"revision"`
	RevocationClass d.RevocationClass `json:"revocation_class"`
	EnrolledAt      time.Time         `json:"enrolled_at"`
	LastSyncAt      *time.Time        `json:"last_sync_at"`
	RevokedAt       *time.Time        `json:"revoked_at"`
	SourceType      string            `json:"source_type"`
	SourceID        string            `json:"source_id"`
	SourceRevision  int64             `json:"source_revision"`
}

func deviceReceiptBytes(scope kernel.OrgScope, v app.DeviceRecord) ([]byte, error) {
	if v.Validate(scope) != nil {
		return nil, app.ErrRegisterUnavailable
	}
	s, e := v.Enrollment.Snapshot(scope)
	if e != nil {
		return nil, e
	}
	return json.Marshal(deviceReceipt{ID: s.ID, MembershipID: s.MembershipID, Label: v.Label, State: s.State, Revision: s.Revision, RevocationClass: s.Revocation, EnrolledAt: v.EnrolledAt, LastSyncAt: v.LastSyncAt, RevokedAt: v.RevokedAt, SourceType: v.SourceType, SourceID: v.SourceID, SourceRevision: v.SourceRevision})
}
func deviceReceiptRecord(scope kernel.OrgScope, current app.DeviceRecord, raw []byte) (app.DeviceRecord, error) {
	var v deviceReceipt
	if httpapi.DecodeJSON(raw, &v) != nil {
		return app.DeviceRecord{}, app.ErrRegisterUnavailable
	}
	prior, e := current.Enrollment.Snapshot(scope)
	if e != nil {
		return app.DeviceRecord{}, e
	}
	if prior.ID != v.ID || prior.MembershipID != v.MembershipID || current.Label != v.Label || !current.EnrolledAt.Equal(v.EnrolledAt) {
		return app.DeviceRecord{}, app.ErrRegisterUnavailable
	}
	prior.State, prior.Revision, prior.Revocation = v.State, v.Revision, v.RevocationClass
	enrollment, e := d.RehydrateDevice(scope, prior)
	if e != nil {
		return app.DeviceRecord{}, e
	}
	out := app.DeviceRecord{Enrollment: enrollment, Label: v.Label, EnrolledAt: v.EnrolledAt, LastSyncAt: v.LastSyncAt, RevokedAt: v.RevokedAt, SourceType: v.SourceType, SourceID: v.SourceID, SourceRevision: v.SourceRevision}
	if out.Validate(scope) != nil {
		return app.DeviceRecord{}, app.ErrRegisterUnavailable
	}
	return out, nil
}
func (s *DeviceStore) signed(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, a app.DeviceActor, enroll, required bool) (d.Membership, error) {
	var guard func() error
	if enroll {
		guard = func() error { return s.Capabilities.GuardWrite(ctx, tx, scope, "organizations") }
	}
	return s.signedFor(ctx, tx, scope, a, enroll, required, false, guard)
}

// The capability closure preserves existing enrollment behavior and permits a
// source-owned operational write to lock capability before the root without
// imposing enrollment's fresh15-minute rule or exposing transaction lifecycle.
func (s *DeviceStore) signedFor(ctx context.Context, tx database.OrgDBTX, scope kernel.OrgScope, a app.DeviceActor, enroll, required, operationalWrite bool, capabilityGuard func() error) (d.Membership, error) {
	if scope.Validate() != nil || a.Validate() != nil {
		return d.Membership{}, app.ErrEmployeeUnauthenticated
	}
	queries := q.New(tx)
	owner, e := queries.DeviceAccount(ctx, a.AccountID)
	if e != nil {
		return d.Membership{}, e
	}
	if owner.Status != "active" || owner.DeletionPending {
		return d.Membership{}, app.ErrEmployeeUnauthenticated
	}
	if owner.ManagedOrg != "" {
		return d.Membership{}, app.ErrManagedRestricted
	}
	proof, e := queries.DeviceProof(ctx, q.DeviceProofParams{AccountID: a.AccountID, FamilyID: a.FamilyID, KeyID: a.KeyID, ContributorID: a.ContributorID, Fingerprint: a.Fingerprint})
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return d.Membership{}, app.ErrEmployeeUnauthenticated
		}
		return d.Membership{}, e
	}
	now, e := deviceStamp(proof.Now)
	if e != nil {
		return d.Membership{}, e
	}
	expiry, e := deviceStamp(proof.AccessExpires)
	if e != nil {
		return d.Membership{}, e
	}
	// Identity v1 accepts created up to five minutes ahead and at most five
	// minutes of proof lifetime. Preserve that verified envelope; the source
	// still rejects expiry after every wait using the actual database clock.
	if !(account.SHA256Hasher{}).Equal(proof.AccessHash, proof.AccessSalt, a.AccessToken) || !a.ProofExpires.After(now) || a.ProofExpires.After(now.Add(10*time.Minute)) {
		return d.Membership{}, app.ErrEmployeeUnauthenticated
	}
	authenticated := expiry.Add(-time.Duration(account.SessionAccessTTLSeconds) * time.Second)
	if enroll && (authenticated.After(now) || !authenticated.After(now.Add(-15*time.Minute))) {
		return d.Membership{}, app.ErrEmployeeUnauthenticated
	}
	// Establish the private account binding before inspecting organization facts.
	_, e = NewStore(s.Pool).ActiveByAccountDBTX(ctx, tx, scope, a.AccountID)
	if e != nil {
		if errors.Is(e, app.ErrNotFound) {
			return d.Membership{}, app.ErrEmployeeNotFound
		}
		return d.Membership{}, e
	}
	if capabilityGuard != nil {
		if e := capabilityGuard(); e != nil {
			return d.Membership{}, e
		}
	}
	var state string
	var ready, synthetic bool
	e = tx.QueryRow(ctx, `SELECT COALESCE(state,''),has_snapshot,EXISTS(SELECT 1 FROM org_onboarding_commands c WHERE c.org_id=organizations.org_id AND c.kind='CREATED' AND c.terms_kind='SYNTHETIC' AND c.terms_version=$2 AND c.terms_digest=$3) FROM organizations WHERE org_id=$1 FOR UPDATE`, scope.OrgID().String(), org.SyntheticTermsVersion, org.SyntheticTermsDigest).Scan(&state, &ready, &synthetic)
	if e != nil {
		return d.Membership{}, e
	}
	if !ready || !synthetic {
		return d.Membership{}, app.ErrRegisterUnavailable
	}
	switch org.State(state) {
	case org.Active:
	case org.PendingSetup:
		if operationalWrite {
			return d.Membership{}, access.ErrSetupIncomplete
		}
	case org.Suspended:
		return d.Membership{}, access.ErrOrgSuspended
	case org.Archived:
		return d.Membership{}, access.ErrOrgArchived
	default:
		return d.Membership{}, app.ErrEmployeeNotFound
	}
	member, e := NewStore(s.Pool).ActiveByAccountDBTX(ctx, tx, scope, a.AccountID)
	if e != nil {
		if errors.Is(e, app.ErrNotFound) {
			return d.Membership{}, app.ErrEmployeeNotFound
		}
		return d.Membership{}, e
	}
	m, e := member.Snapshot(scope)
	if e != nil {
		return d.Membership{}, e
	}
	employee, e := NewStore(s.Pool).GetEmployeeDBTX(ctx, tx, scope, m.EmployeeID)
	if e != nil {
		return d.Membership{}, e
	}
	employeeState, e := employee.Snapshot(scope)
	if e != nil {
		return d.Membership{}, e
	}
	if m.State != d.MembershipActive || m.Model != d.ExistingAccount || employeeState.State != d.EmployeeActive || employeeState.MergedInto != "" {
		return d.Membership{}, app.ErrEmployeeNotFound
	}
	// Recheck clock-bound proof after capability/root waits under retained locks.
	proof, e = queries.DeviceProof(ctx, q.DeviceProofParams{AccountID: a.AccountID, FamilyID: a.FamilyID, KeyID: a.KeyID, ContributorID: a.ContributorID, Fingerprint: a.Fingerprint})
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return d.Membership{}, app.ErrEmployeeUnauthenticated
		}
		return d.Membership{}, e
	}
	now, e = deviceStamp(proof.Now)
	if e != nil {
		return d.Membership{}, e
	}
	if !a.ProofExpires.After(now) || enroll && !authenticated.After(now.Add(-15*time.Minute)) {
		return d.Membership{}, app.ErrEmployeeUnauthenticated
	}
	row, e := queries.DeviceByKey(ctx, q.DeviceByKeyParams{OrgID: scope.OrgID().String(), KeyID: a.KeyID, MembershipID: m.ID})
	if e == nil {
		if row.State == string(d.DeviceRevoked) {
			return d.Membership{}, app.ErrDeviceRevoked
		}
		if required && (row.MembershipID != m.ID || row.State != string(d.DeviceEnrolled)) {
			return d.Membership{}, app.ErrDeviceRevoked
		}
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return d.Membership{}, e
	} else if required {
		return d.Membership{}, app.ErrEmployeeNotFound
	}
	return member, nil
}
func (s *DeviceStore) PrepareDeviceEnrollment(ctx context.Context, scope kernel.OrgScope, a app.DeviceActor) (string, error) {
	if !s.ready() {
		return "", app.ErrRegisterUnavailable
	}
	var member string
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		m, e := s.signed(ctx, tx, scope, a, true, false)
		if e != nil {
			return e
		}
		v, e := m.Snapshot(scope)
		if e != nil {
			return e
		}
		member = v.ID
		return nil
	})
	if e != nil {
		return "", deviceError(ctx, e)
	}
	return member, nil
}
func (s *DeviceStore) EnrollDevice(ctx context.Context, scope kernel.OrgScope, a app.DeviceActor, c app.DeviceCommand) (app.DeviceResult, error) {
	if c.Validate(true) != nil {
		return app.DeviceResult{}, d.ErrInvalid
	}
	if !s.ready() || s.NewID == nil {
		return app.DeviceResult{}, app.ErrRegisterUnavailable
	}
	var out app.DeviceResult
	ready := false
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		member, e := s.signed(ctx, tx, scope, a, true, false)
		if e != nil {
			return e
		}
		m, e := member.Snapshot(scope)
		if e != nil {
			return e
		}
		queries := q.New(tx)
		hash := c.Digest(scope, true)
		prior, e := queries.GetDeviceCommand(ctx, q.GetDeviceCommandParams{OrgID: scope.OrgID().String(), OperationID: c.OperationID})
		if e == nil {
			if prior.Kind != "ENROLL" || prior.ActorID != a.AccountID || prior.ActorKeyID != a.KeyID || prior.RequestHash != hash || prior.Reason != "" {
				return app.ErrEmployeeOperationReused
			}
			row, e := queries.GetDevice(ctx, q.GetDeviceParams{OrgID: scope.OrgID().String(), ID: prior.DeviceID})
			if e != nil {
				return e
			}
			current, e := deviceRecord(scope, row)
			if e != nil {
				return e
			}
			v, e := deviceReceiptRecord(scope, current, prior.Result)
			if e != nil {
				return e
			}
			if _, e = s.signed(ctx, tx, scope, a, true, true); e != nil {
				return e
			}
			out = app.DeviceResult{Device: v, OperationID: c.OperationID, Replayed: true}
			ready = true
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		count, e := queries.CountEnrolled(ctx, q.CountEnrolledParams{OrgID: scope.OrgID().String(), MembershipID: m.ID})
		if e != nil {
			return e
		}
		id, e := s.NewID()
		if e != nil {
			return e
		}
		if _, e = d.EnrollDevice(scope, id, a.KeyID, a.Fingerprint, member, d.EnrollmentConditions{ProofConfirmed: true, EnrolledDevices: count, DeviceLimit: s.limit()}); e != nil {
			if errors.Is(e, d.ErrConditions) {
				return app.ErrDeviceLimit
			}
			return e
		}
		replay, e := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: c.OperationID, ActorID: a.AccountID, Action: "idn.device.enrolled", TargetType: "device", TargetID: id, ReasonCode: "enrolled", RequestHash: hash}, func(ctx context.Context, tx pgx.Tx) error {
			if e := queries.InsertDevice(ctx, q.InsertDeviceParams{OrgID: scope.OrgID().String(), ID: id, MembershipID: m.ID, KeyID: a.KeyID, ContributorID: a.ContributorID, Fingerprint: a.Fingerprint, Label: c.Label, EnrollmentLimit: int32(s.limit()), OperationID: c.OperationID}); e != nil {
				return e
			}
			row, e := queries.GetDevice(ctx, q.GetDeviceParams{OrgID: scope.OrgID().String(), ID: id})
			if e != nil {
				return e
			}
			v, e := deviceRecord(scope, row)
			if e != nil {
				return e
			}
			raw, e := deviceReceiptBytes(scope, v)
			if e != nil {
				return e
			}
			if _, e := queries.InsertDeviceCommand(ctx, q.InsertDeviceCommandParams{OrgID: scope.OrgID().String(), OperationID: c.OperationID, ActorID: a.AccountID, ActorKeyID: a.KeyID, DeviceID: id, Kind: "ENROLL", RequestHash: hash, Reason: "", Result: raw}); e != nil {
				return e
			}
			out = app.DeviceResult{Device: v, OperationID: c.OperationID}
			return nil
		})
		if e != nil {
			return e
		}
		if replay {
			return app.ErrEmployeeOperationReused
		}
		if _, e = s.signed(ctx, tx, scope, a, true, true); e != nil {
			return e
		}
		ready = true
		return nil
	})
	if e != nil {
		if ready && ctx.Err() == nil {
			return app.DeviceResult{}, app.ErrRegisterUnavailable
		}
		return app.DeviceResult{}, deviceError(ctx, e)
	}
	return out, nil
}
func (s *DeviceStore) CheckDevice(ctx context.Context, scope kernel.OrgScope, a app.DeviceActor) (app.DeviceRecord, error) {
	if !s.ready() {
		return app.DeviceRecord{}, app.ErrRegisterUnavailable
	}
	var out app.DeviceRecord
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		member, e := s.signed(ctx, tx, scope, a, false, true)
		if e != nil {
			return e
		}
		membership, e := member.Snapshot(scope)
		if e != nil {
			return e
		}
		row, e := q.New(tx).DeviceByKey(ctx, q.DeviceByKeyParams{OrgID: scope.OrgID().String(), KeyID: a.KeyID, MembershipID: membership.ID})
		if e != nil {
			return e
		}
		out, e = deviceRecord(scope, q.GetDeviceRow(row))
		if e != nil {
			return e
		}
		_, e = s.signed(ctx, tx, scope, a, false, true)
		return e
	})
	if e != nil {
		return app.DeviceRecord{}, deviceError(ctx, e)
	}
	return out, nil
}
func (s *DeviceStore) web(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, a app.EmployeeActor, caller, target string, self, write bool) error {
	if employeeCall(scope, a, target) != nil || !self && !validUUID(caller) {
		return d.ErrInvalid
	}
	if self {
		return s.Authority.CheckOwnDeviceTx(ctx, tx, scope, a, target, write)
	}
	return s.Authority.CheckManagerDeviceTx(ctx, tx, scope, a, caller, write)
}
func (s *DeviceStore) targetMember(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, target string) error {
	m, e := NewStore(s.Pool).GetTx(ctx, tx, scope, target)
	if e != nil {
		if errors.Is(e, app.ErrNotFound) {
			return app.ErrEmployeeNotFound
		}
		return e
	}
	v, e := m.Snapshot(scope)
	if e != nil {
		return e
	}
	if v.Model != d.ExistingAccount {
		return app.ErrManagedRestricted
	}
	return nil
}
func (s *DeviceStore) ListDevices(ctx context.Context, scope kernel.OrgScope, a app.EmployeeActor, caller, target string, self bool, after string, limit int) (app.DevicePage, error) {
	if limit < 1 || limit > 100 || after != "" && !validUUID(after) {
		return app.DevicePage{}, d.ErrInvalid
	}
	if !s.ready() {
		return app.DevicePage{}, app.ErrRegisterUnavailable
	}
	out := app.DevicePage{Devices: []app.DeviceRecord{}}
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.web(ctx, tx, scope, a, caller, target, self, false); e != nil {
			return e
		}
		if e := s.targetMember(ctx, tx, scope, target); e != nil {
			return e
		}
		rows, e := q.New(tx).ListDeviceRows(ctx, q.ListDeviceRowsParams{OrgID: scope.OrgID().String(), MembershipID: target, AfterID: after, RowLimit: int32(limit + 1)})
		if e != nil {
			return e
		}
		for i, row := range rows {
			if i == limit {
				v, _ := out.Devices[i-1].Enrollment.Snapshot(scope)
				out.NextID = v.ID
				break
			}
			v, e := deviceRecord(scope, q.GetDeviceRow(row))
			if e != nil {
				return e
			}
			out.Devices = append(out.Devices, v)
		}
		return s.web(ctx, tx, scope, a, caller, target, self, false)
	})
	if e != nil {
		return app.DevicePage{}, deviceError(ctx, e)
	}
	return out, nil
}
func (s *DeviceStore) RevokeDevice(ctx context.Context, scope kernel.OrgScope, a app.EmployeeActor, caller, target string, self bool, c app.DeviceCommand) (app.DeviceResult, error) {
	if c.Validate(false) != nil {
		return app.DeviceResult{}, d.ErrInvalid
	}
	if !s.ready() {
		return app.DeviceResult{}, app.ErrRegisterUnavailable
	}
	var out app.DeviceResult
	ready := false
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.web(ctx, tx, scope, a, caller, target, self, true); e != nil {
			return e
		}
		if e := s.targetMember(ctx, tx, scope, target); e != nil {
			return e
		}
		queries := q.New(tx)
		row, e := queries.GetDevice(ctx, q.GetDeviceParams{OrgID: scope.OrgID().String(), ID: c.DeviceID})
		if e != nil {
			return e
		}
		if row.MembershipID != target {
			return app.ErrEmployeeNotFound
		}
		current, e := deviceRecord(scope, row)
		if e != nil {
			return e
		}
		hash := c.Digest(scope, false)
		prior, e := queries.GetDeviceCommand(ctx, q.GetDeviceCommandParams{OrgID: scope.OrgID().String(), OperationID: c.OperationID})
		if e == nil {
			if prior.ActorID != a.AccountID || prior.Kind != "REVOKE" || prior.DeviceID != c.DeviceID || prior.RequestHash != hash || prior.Reason != c.Reason {
				return app.ErrEmployeeOperationReused
			}
			v, e := deviceReceiptRecord(scope, current, prior.Result)
			if e != nil {
				return e
			}
			if e := s.web(ctx, tx, scope, a, caller, target, self, true); e != nil {
				return e
			}
			out = app.DeviceResult{Device: v, OperationID: c.OperationID, Reason: prior.Reason, Replayed: true}
			ready = true
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if _, e := current.Enrollment.Transition(scope, c.ExpectedRevision, d.DeviceRevoked, c.Class); e != nil {
			if errors.Is(e, d.ErrRevoked) {
				return app.ErrDeviceRevoked
			}
			return e
		}
		replay, e := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: c.OperationID, ActorID: a.AccountID, Action: "idn.device.revoked", TargetType: "device", TargetID: c.DeviceID, ReasonCode: "revoked", RequestHash: hash}, func(ctx context.Context, tx pgx.Tx) error {
			n, e := queries.RevokeDevice(ctx, q.RevokeDeviceParams{OrgID: scope.OrgID().String(), ID: c.DeviceID, ExpectedRevision: c.ExpectedRevision, RevocationClass: string(c.Class), OperationID: c.OperationID})
			if e != nil {
				return e
			}
			if n != 1 {
				return d.ErrConflict
			}
			row, e := queries.GetDevice(ctx, q.GetDeviceParams{OrgID: scope.OrgID().String(), ID: c.DeviceID})
			if e != nil {
				return e
			}
			v, e := deviceRecord(scope, row)
			if e != nil {
				return e
			}
			raw, e := deviceReceiptBytes(scope, v)
			if e != nil {
				return e
			}
			if _, e := queries.InsertDeviceCommand(ctx, q.InsertDeviceCommandParams{OrgID: scope.OrgID().String(), OperationID: c.OperationID, ActorID: a.AccountID, ActorKeyID: "", DeviceID: c.DeviceID, Kind: "REVOKE", RequestHash: hash, Reason: c.Reason, Result: raw}); e != nil {
				return e
			}
			out = app.DeviceResult{Device: v, OperationID: c.OperationID, Reason: c.Reason}
			return nil
		})
		if e != nil {
			return e
		}
		if replay {
			return app.ErrEmployeeOperationReused
		}
		if e := s.web(ctx, tx, scope, a, caller, target, self, true); e != nil {
			return e
		}
		ready = true
		return nil
	})
	if e != nil {
		if ready && ctx.Err() == nil {
			return app.DeviceResult{}, app.ErrRegisterUnavailable
		}
		return app.DeviceResult{}, deviceError(ctx, e)
	}
	return out, nil
}
