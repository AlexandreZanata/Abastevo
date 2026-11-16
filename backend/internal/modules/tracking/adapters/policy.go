package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PolicyAuditDefinitions registers the policy change vocabulary.
func PolicyAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: app.PolicyAuditAction, TargetType: app.PolicyTargetType}}
}

// PolicyStore changes and reads the organization tracking policy through
// ordinary scoped transactions (B-BR-EMP-467..469, migrations 000240 and
// 000247): authority, receipt replay, one audit event, the version and its
// receipt, then authority again.
type PolicyStore struct {
	Pool      *pgxpool.Pool
	Authority app.PolicyAuthority
	Audit     auditapp.TransactionRunner
}

var _ app.PolicyRepository = (*PolicyStore)(nil)

// errNoticeNotInForce rolls the change back when the notice instant is after
// the database's effective instant (B-BR-EMP-468).
var errNoticeNotInForce = errors.New("tracking: notice not in force")

func canonicalID(raw string) bool {
	v, err := kernel.ParseOrgID(raw)
	return err == nil && v.String() == raw
}

func (s *PolicyStore) ready(write bool) bool {
	return s != nil && s.Pool != nil && !absent(s.Authority) && (!write || !absent(s.Audit))
}

// policyError keeps only registered refusals; provider details stay private.
func policyError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	switch {
	case errors.Is(err, errNoticeNotInForce):
		return &app.PolicyField{Field: "notice_in_force_at", Code: app.CodeNoticeNotInForce}
	case errors.Is(err, audit.ErrReused):
		return app.ErrPolicyOperationReuse
	}
	for _, known := range []error{access.ErrNoMembership, access.ErrMembershipSuspended, access.ErrOrgSuspended, access.ErrOrgArchived, access.ErrSetupIncomplete, access.ErrFreshAuthRequired,
		access.ErrEntitlementRequired, access.ErrUnauthenticated, access.ErrNotFound, permission.ErrPermissionDenied, permission.ErrResourceNotFound,
		app.ErrPolicyOperationReuse, app.ErrPolicyConflict, app.ErrPolicyUnauthorized, app.ErrNotFound, app.ErrUnavailable} {
		if errors.Is(err, known) {
			return err
		}
	}
	if code, ok := orgapi.Registered(err); ok {
		return code
	}
	return app.ErrUnavailable
}

func optionalUTC(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	v := at.UTC()
	return &v
}

// version reads one stored policy version.
func version(ctx context.Context, tx pgx.Tx, org string, revision int64) (app.PolicyAck, error) {
	var ack app.PolicyAck
	var policy string
	var notice *string
	err := tx.QueryRow(ctx, `SELECT revision,policy,notice_version,notice_in_force_at,effective_at FROM org_tracking_policies WHERE org_id=$1 AND revision=$2`, org, revision).
		Scan(&ack.Revision, &policy, &notice, &ack.NoticeInForce, &ack.EffectiveAt)
	if err != nil {
		return app.PolicyAck{}, err
	}
	parsed, err := domain.ParsePolicy(policy)
	if err != nil {
		return app.PolicyAck{}, app.ErrUnavailable
	}
	ack.Policy, ack.EffectiveAt, ack.NoticeInForce = parsed, ack.EffectiveAt.UTC(), optionalUTC(ack.NoticeInForce)
	if notice != nil {
		ack.NoticeVersion = *notice
	}
	return ack, nil
}

// ChangePolicy writes the next policy version under the organization lock.
func (s *PolicyStore) ChangePolicy(ctx context.Context, scope kernel.OrgScope, actor app.PolicyActor, member string, command app.PolicyCommand) (app.PolicyAck, bool, error) {
	if scope.Validate() != nil || !canonicalID(member) || actor.Validate() != nil {
		return app.PolicyAck{}, false, app.ErrPolicyUnauthorized
	}
	in, err := command.Snapshot()
	if err != nil {
		return app.PolicyAck{}, false, err
	}
	if !s.ready(true) {
		return app.PolicyAck{}, false, app.ErrUnavailable
	}
	org, digest := scope.OrgID().String(), command.Digest(scope)
	var ack app.PolicyAck
	replayed := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckPolicyTx(ctx, tx, scope, actor, member, app.PolicyManage); err != nil {
			return err
		}
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM organizations WHERE org_id=$1 FOR UPDATE`, org).Scan(&state); err != nil {
			return err
		}
		if state != "ACTIVE" {
			return access.ErrSetupIncomplete
		}
		var priorActor, priorHash string
		var priorRevision int64
		err := tx.QueryRow(ctx, `SELECT actor_account_id::text,request_hash,result_revision FROM org_tracking_policy_commands WHERE org_id=$1 AND operation_id=$2`, org, in.OperationID).
			Scan(&priorActor, &priorHash, &priorRevision)
		if err == nil {
			if priorActor != actor.AccountID || priorHash != digest {
				return app.ErrPolicyOperationReuse
			}
			if ack, err = version(ctx, tx, org, priorRevision); err != nil {
				return err
			}
			replayed = true
			return s.Authority.CheckPolicyTx(ctx, tx, scope, actor, member, app.PolicyManage)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		event := audit.Event{OperationID: in.OperationID, ActorID: actor.AccountID, Action: app.PolicyAuditAction, TargetType: app.PolicyTargetType, TargetID: org, RequestHash: digest}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			var current int64
			if err := tx.QueryRow(ctx, `SELECT COALESCE(max(revision),0) FROM org_tracking_policies WHERE org_id=$1`, org).Scan(&current); err != nil {
				return err
			}
			if current != in.ExpectedRevision {
				return app.ErrPolicyConflict
			}
			var effective time.Time
			if err := tx.QueryRow(ctx, `INSERT INTO org_tracking_policies(org_id,revision,policy,notice_version,notice_in_force_at)VALUES($1,$2,$3,$4,$5) RETURNING effective_at`,
				org, current+1, in.Policy, in.NoticeVersion, in.NoticeInForceAt).Scan(&effective); err != nil {
				return err
			}
			if in.NoticeInForceAt != nil && in.NoticeInForceAt.After(effective) {
				return errNoticeNotInForce
			}
			if _, err := tx.Exec(ctx, `INSERT INTO org_tracking_policy_commands(org_id,operation_id,actor_account_id,actor_membership_id,request_hash,result_revision)VALUES($1,$2,$3,$4,$5,$6)`,
				org, in.OperationID, actor.AccountID, member, digest, current+1); err != nil {
				return err
			}
			var err error
			ack, err = version(ctx, tx, org, current+1)
			return err
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		return s.Authority.CheckPolicyTx(ctx, tx, scope, actor, member, app.PolicyManage)
	})
	if err != nil {
		return app.PolicyAck{}, false, policyError(ctx, err)
	}
	return ack, replayed, nil
}

// ReadPolicy reads the newest version and the policy in force now, between
// two authority checks.
func (s *PolicyStore) ReadPolicy(ctx context.Context, scope kernel.OrgScope, actor app.PolicyActor, member string) (app.PolicyView, error) {
	if scope.Validate() != nil || !canonicalID(member) || actor.Validate() != nil {
		return app.PolicyView{}, app.ErrPolicyUnauthorized
	}
	if !s.ready(false) {
		return app.PolicyView{}, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	v := app.PolicyView{Policy: domain.PolicyOff, InForce: domain.PolicyOff}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckPolicyTx(ctx, tx, scope, actor, member, app.PolicyRead); err != nil {
			return err
		}
		var newest int64
		if err := tx.QueryRow(ctx, `SELECT COALESCE(max(revision),0) FROM org_tracking_policies WHERE org_id=$1`, org).Scan(&newest); err != nil {
			return err
		}
		if newest > 0 {
			stored, err := version(ctx, tx, org, newest)
			if err != nil {
				return err
			}
			effective := stored.EffectiveAt
			v.Revision, v.Policy, v.NoticeVersion, v.NoticeInForce, v.EffectiveAt = stored.Revision, stored.Policy, stored.NoticeVersion, stored.NoticeInForce, &effective
			var policy string
			err = tx.QueryRow(ctx, `SELECT policy,revision FROM org_tracking_policy_in_force($1,clock_timestamp())`, org).Scan(&policy, &v.InForceRevision)
			switch {
			case errors.Is(err, pgx.ErrNoRows):
			case err != nil:
				return err
			default:
				if v.InForce, err = domain.ParsePolicy(policy); err != nil {
					return app.ErrUnavailable
				}
			}
		}
		return s.Authority.CheckPolicyTx(ctx, tx, scope, actor, member, app.PolicyRead)
	})
	if err != nil {
		return app.PolicyView{}, policyError(ctx, err)
	}
	return v, nil
}

// ReadPolicyInForce reads the policy in force now for the requesting member's
// device, between two authority checks under trip.trip.start over the
// member's own record (EMP-GPS-002). It writes nothing.
func (s *PolicyStore) ReadPolicyInForce(ctx context.Context, scope kernel.OrgScope, actor app.PolicyActor, member string) (app.PolicyInForce, error) {
	if scope.Validate() != nil || !canonicalID(member) || actor.Validate() != nil {
		return app.PolicyInForce{}, app.ErrPolicyUnauthorized
	}
	if !s.ready(false) {
		return app.PolicyInForce{}, app.ErrUnavailable
	}
	v := app.PolicyInForce{Policy: domain.PolicyOff}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckPolicyTx(ctx, tx, scope, actor, member, app.PolicyOwn); err != nil {
			return err
		}
		var policy string
		var notice *string
		var inForceAt *time.Time
		// The EMP-GPS-004 in-force rule: the latest version effective now,
		// OFF until its notice is in force; the notice only while enabled.
		err := tx.QueryRow(ctx, `SELECT f.policy,f.revision,p.notice_version,p.notice_in_force_at FROM org_tracking_policy_in_force($1,clock_timestamp()) f
 JOIN org_tracking_policies p ON p.org_id=$1 AND p.revision=f.revision`, scope.OrgID().String()).Scan(&policy, &v.Revision, &notice, &inForceAt)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return err
		default:
			parsed, err := domain.ParsePolicy(policy)
			if err != nil {
				return app.ErrUnavailable
			}
			v.Policy = parsed
			if parsed != domain.PolicyOff {
				if notice == nil || inForceAt == nil {
					return app.ErrUnavailable
				}
				at := inForceAt.UTC()
				v.NoticeVersion, v.NoticeInForce = *notice, &at
			}
		}
		return s.Authority.CheckPolicyTx(ctx, tx, scope, actor, member, app.PolicyOwn)
	})
	if err != nil {
		return app.PolicyInForce{}, policyError(ctx, err)
	}
	return v, nil
}
