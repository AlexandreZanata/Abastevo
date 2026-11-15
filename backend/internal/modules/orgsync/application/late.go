package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

// Late intake (EMP-OFFL-009, EDR-197..199): an exact sealed enrollment period
// whose access ended keeps an intake-only source. Server history decides; the
// device clock is evidence that can only choose review over refusal.

// LateEnd names the server history that ended the period's access.
type LateEnd string

const (
	// LateEndMembership: the enrollment was revoked by its membership end;
	// the end instant is the membership's ended_at.
	LateEndMembership LateEnd = "MEMBERSHIP"
	// LateEndDevice: an ACCESS_ENDED revocation command; the end instant is
	// the enrollment's revoked_at.
	LateEndDevice LateEnd = "DEVICE"
)

// Late acknowledgement codes and the frozen default window (BR-OFFL-011).
const (
	LateHeldCode            = "ACCESS_ENDED_BEFORE_RECEIPT"
	LateMembershipEndedCode = "MEMBERSHIP_ENDED"
	LateDeviceRevokedCode   = "DEVICE_REVOKED"
	LateWindowClosedCode    = "INTAKE_WINDOW_CLOSED"
	LateDefaultWindowDays   = 15
	LateMaxWindowDays       = 60
	LatePolicy              = "late-intake-v1"
)

// LateAccess is the source-confirmed end of one exact period. It carries no
// membership, role, record or receipt authority.
type LateAccess struct {
	privateValue
	End     LateEnd
	EndedAt time.Time
}

// LateEvidence adds the window in force at the end instant. A zero settings
// revision means no settings version existed at EndedAt (default policy).
type LateEvidence struct {
	privateValue
	LateAccess
	WindowDays       int
	SettingsRevision int64
}

func (access LateAccess) Validate() error {
	if access.End != LateEndMembership && access.End != LateEndDevice || access.EndedAt.IsZero() {
		return ErrUnavailable
	}
	return nil
}

func (evidence LateEvidence) Validate() error {
	if evidence.LateAccess.Validate() != nil || evidence.WindowDays < 0 || evidence.WindowDays > LateMaxWindowDays || evidence.SettingsRevision < 0 {
		return ErrUnavailable
	}
	return nil
}

// LateDisposition is the pure time decision. HOLD_CLAIM and HOLD_DECLARED are
// eligible for review; ENDED and CLOSED are final refusals.
type LateDisposition string

const (
	LateHoldClaim    LateDisposition = "HOLD_CLAIM"
	LateHoldDeclared LateDisposition = "HOLD_DECLARED"
	LateEnded        LateDisposition = "ENDED"
	LateClosed       LateDisposition = "CLOSED"
)

// LateFacts: Declared is the device declaration (evidence only; the minimum
// OrgTime instant is a valid declaration, so absence is nil), Claimed the claim
// the server already held before this delivery (nil when none) and Received
// the server receipt instant.
type LateFacts struct {
	privateValue
	Declared *time.Time
	Claimed  *time.Time
	Received time.Time
}

type LateDecision struct {
	Disposition  LateDisposition
	Result, Code string
}

func (decision LateDecision) Holds() bool {
	return decision.Disposition == LateHoldClaim || decision.Disposition == LateHoldDeclared
}

var errLateEvidence = errors.New("orgsync: late evidence unavailable")

// DecideLate is the EDR-197 rule shared with org_sync_late_disposition: the
// window (elapsed seconds, end instant included) is checked first, then a
// claim the server already held before the end, then the declaration. Device
// time can only choose review over refusal.
func DecideLate(evidence LateEvidence, facts LateFacts) (LateDecision, error) {
	if evidence.Validate() != nil || facts.Declared == nil || facts.Received.IsZero() || facts.Received.Before(evidence.EndedAt) || facts.Claimed != nil && facts.Claimed.IsZero() {
		return LateDecision{}, errLateEvidence
	}
	closes := evidence.EndedAt.Add(time.Duration(evidence.WindowDays) * 24 * time.Hour)
	switch {
	case facts.Received.After(closes):
		return LateDecision{Disposition: LateClosed, Result: protocol.Rejected, Code: LateWindowClosedCode}, nil
	case facts.Claimed != nil && facts.Claimed.Before(evidence.EndedAt):
		return LateDecision{Disposition: LateHoldClaim, Result: protocol.AcceptedForReview, Code: LateHeldCode}, nil
	case facts.Declared.Before(evidence.EndedAt):
		return LateDecision{Disposition: LateHoldDeclared, Result: protocol.AcceptedForReview, Code: LateHeldCode}, nil
	case evidence.End == LateEndDevice:
		return LateDecision{Disposition: LateEnded, Result: protocol.Rejected, Code: LateDeviceRevokedCode}, nil
	}
	return LateDecision{Disposition: LateEnded, Result: protocol.Rejected, Code: LateMembershipEndedCode}, nil
}

// LateHolder is an installed owner's typed held-fact path that needs no
// current authority. It validates payload and target only and returns either
// ACCEPTED_FOR_REVIEW with the given code and a real review item, or a
// REJECTED MALFORMED outcome. It never applies an effect or reads a record
// for the source.
type LateHolder interface {
	HoldLate(context.Context, DBTX, Source, protocol.Operation, string) (Outcome, error)
}

// LateHolderProvider is implemented by registered handlers.
type LateHolderProvider interface{ LateHolder() LateHolder }

// LateHolderOf returns the handler's late holder, or nil when it has none.
func LateHolderOf(handler Handler) LateHolder {
	if provider, ok := handler.(LateHolderProvider); ok && !nilPort(provider) {
		holder := provider.LateHolder()
		if nilPort(holder) {
			return nil
		}
		return holder
	}
	if holder, ok := handler.(LateHolder); ok && !nilPort(holder) {
		return holder
	}
	return nil
}

// LateStore processes one operation through the restricted late source.
type LateStore interface {
	ProcessLate(context.Context, Source, protocol.Operation, protocol.Limits, Handler) (protocol.Acknowledgement, error)
}

type borrowedLateHolder struct {
	privateValue
	source LateHolder
}

func (holder borrowedLateHolder) HoldLate(ctx context.Context, db DBTX, source Source, op protocol.Operation, code string) (Outcome, error) {
	outcome, err := holder.source.HoldLate(ctx, Borrow(db), source, op, code)
	return outcome.Copy(), err
}

func captureLateHolder(handler Handler) LateHolder {
	holder, ok := handler.(LateHolder)
	if !ok || nilPort(holder) {
		return nil
	}
	return borrowedLateHolder{source: holder}
}

func (handler registeredHandler) LateHolder() LateHolder { return handler.late }
