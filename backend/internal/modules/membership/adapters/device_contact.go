package adapters

import (
	"context"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/membership"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// contactProof checks only current personal proof. It grants no membership or
// organization reads, and may therefore precede an exact owned terminal lookup.
func contactProof(ctx context.Context, tx pgx.Tx, a app.DeviceActor) (time.Time, error) {
	p, e := q.New(tx).DeviceProof(ctx, q.DeviceProofParams{AccountID: a.AccountID, FamilyID: a.FamilyID, KeyID: a.KeyID, ContributorID: a.ContributorID, Fingerprint: a.Fingerprint})
	if errors.Is(e, pgx.ErrNoRows) {
		return time.Time{}, app.ErrEmployeeUnauthenticated
	}
	if e != nil {
		return time.Time{}, e
	}
	now, e := deviceStamp(p.Now)
	if e != nil {
		return time.Time{}, e
	}
	if !(account.SHA256Hasher{}).Equal(p.AccessHash, p.AccessSalt, a.AccessToken) || !a.ProofExpires.After(now) || a.ProofExpires.After(now.Add(10*time.Minute)) {
		return time.Time{}, app.ErrEmployeeUnauthenticated
	}
	return now, nil
}

// ContactDevice observes one exact enrolled period. Unlike CheckDevice it may
// return a private terminal tombstone after membership ends; that path reads no
// employee, terms, settings, roles or operational records and never grants a lease.
func (s *DeviceStore) ContactDevice(ctx context.Context, scope kernel.OrgScope, a app.DeviceActor, r app.DeviceContactRequest) (app.DeviceContact, error) {
	if !s.ready() {
		return app.DeviceContact{}, app.ErrRegisterUnavailable
	}
	if scope.Validate() != nil || a.Validate() != nil || r.Validate() != nil {
		return app.DeviceContact{}, d.ErrInvalid
	}
	var out app.DeviceContact
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		// Account -> current family/key -> root -> exact member/enrollment is the
		// same serialization order as enrollment and revocation. No unowned root
		// metadata is returned even when the caller guessed its UUID.
		owner, e := queries.DeviceAccount(ctx, a.AccountID)
		if e != nil {
			return e
		}
		if owner.Status != "active" || owner.DeletionPending {
			return app.ErrEmployeeUnauthenticated
		}
		if owner.ManagedOrg != "" {
			return app.ErrManagedRestricted
		}
		if _, e := contactProof(ctx, tx, a); e != nil {
			return e
		}
		if _, e := queries.LockDeviceContactRoot(ctx, scope.OrgID().String()); e != nil {
			return e
		}
		row, e := queries.OwnedDeviceContact(ctx, q.OwnedDeviceContactParams{OrgID: scope.OrgID().String(), EnrollmentID: r.EnrollmentID, MembershipID: r.MembershipID, AccountID: a.AccountID, KeyID: a.KeyID, ContributorID: a.ContributorID, Fingerprint: a.Fingerprint})
		if e != nil {
			return e
		}
		record, e := deviceRecord(scope, q.GetDeviceRow(row))
		if e != nil {
			return e
		}
		state, e := record.Enrollment.Snapshot(scope)
		if e != nil {
			return e
		}
		out = app.DeviceContact{RequestID: r.RequestID, AccountID: a.AccountID, FamilyID: a.FamilyID, MembershipID: r.MembershipID, EnrollmentID: r.EnrollmentID, Revision: state.Revision}
		if state.State == d.DeviceRevoked {
			out.RevocationClass = state.Revocation
			switch state.Revocation {
			case d.LostOrCompromised:
				out.Verdict = app.ContactErase
			case d.AccessEnded:
				out.Verdict = app.ContactLock
			default:
				return app.ErrRegisterUnavailable
			}
		} else {
			member, e := s.signed(ctx, tx, scope, a, false, true)
			if e != nil {
				return e
			}
			current, e := member.Snapshot(scope)
			if e != nil || current.ID != r.MembershipID {
				return app.ErrEmployeeNotFound
			}
			out.Verdict, out.AccessModel, out.ValiditySeconds = app.ContactActive, d.ExistingAccount, app.DeviceOfflineValiditySeconds
		}
		out.ServerTime, e = contactProof(ctx, tx, a)
		if e != nil {
			return e
		}
		return out.Validate(a, r)
	})
	if e != nil {
		return app.DeviceContact{}, deviceError(ctx, e)
	}
	return out, nil
}
