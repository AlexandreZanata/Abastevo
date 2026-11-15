package application

import (
	"context"
	"errors"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
)

// ErrAttachmentOverlap is the AC1 refusal: the implement is already attached
// to an asset in an overlapping period (CONFLICT, UC-FLEET-10).
var ErrAttachmentOverlap = errors.New("fleet: implement already attached in the period")

type AttachmentCreateInput struct {
	private
	OperationID, ImplementID, PoweredAssetID string
	// Start nil means the server instant of the command.
	Start, End *time.Time
}
type AttachmentCreate struct {
	private
	value AttachmentCreateInput
}

// NewAttachmentCreate validates the exact B-BR-EMP-325/326 shape; the start
// against the server clock and the kinds are the repository's checks.
func NewAttachmentCreate(v AttachmentCreateInput) (AttachmentCreate, error) {
	if !canonicalID(v.OperationID) || !canonicalID(v.ImplementID) || !canonicalID(v.PoweredAssetID) || v.ImplementID == v.PoweredAssetID {
		return AttachmentCreate{}, fleet.ErrInvalid
	}
	if !exactInstant(v.Start) || !exactInstant(v.End) || v.Start != nil && v.End != nil && !v.End.After(*v.Start) {
		return AttachmentCreate{}, fleet.ErrInvalid
	}
	v.Start, v.End = copyInstant(v.Start), copyInstant(v.End)
	return AttachmentCreate{value: v}, nil
}
func (c AttachmentCreate) Snapshot() (AttachmentCreateInput, error) {
	copy, err := NewAttachmentCreate(c.value)
	if err != nil {
		return AttachmentCreateInput{}, err
	}
	return copy.value, nil
}
func (c AttachmentCreate) Digest(scope kernel.OrgScope) string {
	in, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	return digest(1, "fleet.asset.attachment.attach", scope.OrgID().String(), in.OperationID, in.ImplementID, in.PoweredAssetID, instantText(in.Start), instantText(in.End))
}

// AttachmentDetachInput detaches one attachment of the path asset (as the
// implement or the powered asset) at End, or at the server instant.
type AttachmentDetachInput struct {
	private
	OperationID, AssetID, AttachmentID string
	End                                *time.Time
	Reason                             string
}
type AttachmentDetach struct {
	private
	value AttachmentDetachInput
}

func NewAttachmentDetach(v AttachmentDetachInput) (AttachmentDetach, error) {
	if !canonicalID(v.OperationID) || !canonicalID(v.AssetID) || !canonicalID(v.AttachmentID) || !validOptionalReason(v.Reason) || !exactInstant(v.End) {
		return AttachmentDetach{}, fleet.ErrInvalid
	}
	v.End = copyInstant(v.End)
	return AttachmentDetach{value: v}, nil
}
func (c AttachmentDetach) Snapshot() (AttachmentDetachInput, error) {
	copy, err := NewAttachmentDetach(c.value)
	if err != nil {
		return AttachmentDetachInput{}, err
	}
	return copy.value, nil
}
func (c AttachmentDetach) Digest(scope kernel.OrgScope) string {
	in, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	return digest(1, "fleet.asset.attachment.detach", scope.OrgID().String(), in.OperationID, in.AssetID, in.AttachmentID, instantText(in.End), in.Reason)
}

// AttachmentResult acknowledges one command on the path asset.
type AttachmentResult struct {
	private
	AssetID, AttachmentID string
}

// AttachmentView is one stored attachment with its state derived at the
// page instant (SCHEDULED, ACTIVE or ENDED, as an assignment period).
type AttachmentView struct {
	private
	ID, ImplementID, PoweredAssetID string
	Period                          fleet.AssignmentPeriod
	State                           fleet.AssignmentState
	RecordedAt                      time.Time
	DetachedAt                      *time.Time
	DetachReason                    string
}

type AttachmentPage struct {
	private
	Attachments []AttachmentView
	At          time.Time
	NextFrom    *time.Time
	NextID      string
}

type AttachmentRepository interface {
	AttachImplement(context.Context, kernel.OrgScope, FleetActor, string, string, AttachmentCreate) (AttachmentResult, bool, error)
	DetachImplement(context.Context, kernel.OrgScope, FleetActor, string, AttachmentDetach) (AttachmentResult, bool, error)
	ListAttachments(context.Context, kernel.OrgScope, FleetActor, string, string, PeriodFilter) (AttachmentPage, error)
}

// AttachedImplementsPort answers BR-FLEET-033 inside the caller's
// organization transaction (a pgx.Tx or the borrowed intake port): the
// implements attached to a powered asset at an instant (a trip start lists
// them, EMP-TRIP-002) and the powered asset an implement is attached to.
type AttachedImplementsPort interface {
	AttachedImplementsDBTX(context.Context, syncapp.DBTX, kernel.OrgScope, string, time.Time) ([]string, error)
	PoweredAssetDBTX(context.Context, syncapp.DBTX, kernel.OrgScope, string, time.Time) (string, bool, error)
}

// AttachmentService runs attach and detach under fresh fleet.asset.manage
// (UC-FLEET-10, fleet.md §7) and the list under fleet.asset.read.
type AttachmentService struct {
	Enabled    bool
	Repository AttachmentRepository
	NewID      func() (string, error)
}

func (s AttachmentService) guarded(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, asset string, policy access.Policy, call func() error) error {
	return guardedCall(ctx, s.Enabled, s.Repository, scope, actor, decision, asset, policy, call)
}

func (s AttachmentService) Attach(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, command AttachmentCreate) (AttachmentResult, bool, error) {
	in, err := command.Snapshot()
	if err != nil {
		return AttachmentResult{}, false, err
	}
	var v AttachmentResult
	replayed := false
	err = s.guarded(ctx, scope, actor, decision, in.ImplementID, Policy(true), func() error {
		if s.NewID == nil {
			return ErrUnavailable
		}
		id, err := s.NewID()
		if err != nil || !canonicalID(id) {
			return ErrUnavailable
		}
		v, replayed, err = s.Repository.AttachImplement(ctx, scope, actor, decision.Subject().MembershipID(), id, command)
		return err
	})
	if err != nil {
		return AttachmentResult{}, false, err
	}
	if v.AssetID != in.ImplementID || !canonicalID(v.AttachmentID) {
		return AttachmentResult{}, false, ErrUnavailable
	}
	return v, replayed, nil
}

func (s AttachmentService) Detach(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, command AttachmentDetach) (AttachmentResult, bool, error) {
	in, err := command.Snapshot()
	if err != nil {
		return AttachmentResult{}, false, err
	}
	var v AttachmentResult
	replayed := false
	err = s.guarded(ctx, scope, actor, decision, in.AssetID, Policy(true), func() error {
		var err error
		v, replayed, err = s.Repository.DetachImplement(ctx, scope, actor, decision.Subject().MembershipID(), command)
		return err
	})
	if err != nil {
		return AttachmentResult{}, false, err
	}
	if v.AssetID != in.AssetID || v.AttachmentID != in.AttachmentID {
		return AttachmentResult{}, false, ErrUnavailable
	}
	return v, replayed, nil
}

func validAttachmentView(v AttachmentView, asset string, at time.Time) bool {
	if !canonicalID(v.ID) || !canonicalID(v.ImplementID) || !canonicalID(v.PoweredAssetID) || v.ImplementID == v.PoweredAssetID || asset != v.ImplementID && asset != v.PoweredAssetID || !validInstant(v.RecordedAt) || !validOptionalReason(v.DetachReason) {
		return false
	}
	if _, err := fleet.NewAssignmentPeriod(v.Period.From, v.Period.To); err != nil || v.State != v.Period.StateAt(at) {
		return false
	}
	if v.DetachedAt == nil {
		return v.DetachReason == "" && !v.Period.Empty()
	}
	return validInstant(*v.DetachedAt) && v.Period.To != nil
}

func (s AttachmentService) List(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, asset string, filter PeriodFilter) (AttachmentPage, error) {
	if !canonicalID(asset) || filter.Validate() != nil {
		return AttachmentPage{}, fleet.ErrInvalid
	}
	var v AttachmentPage
	err := s.guarded(ctx, scope, actor, decision, asset, Policy(false), func() error {
		var err error
		v, err = s.Repository.ListAttachments(ctx, scope, actor, decision.Subject().MembershipID(), asset, filter)
		return err
	})
	if err != nil {
		return AttachmentPage{}, err
	}
	rows := v.Attachments
	if !validInstant(v.At) || !validPeriodOrder(len(rows), filter.Limit, func(i int) time.Time { return rows[i].Period.From }, func(i int) string { return rows[i].ID }, v.NextFrom, v.NextID) {
		return AttachmentPage{}, ErrUnavailable
	}
	out := AttachmentPage{Attachments: make([]AttachmentView, 0, len(rows)), At: v.At, NextFrom: copyInstant(v.NextFrom), NextID: v.NextID}
	for _, row := range rows {
		if !validAttachmentView(row, asset, v.At) {
			return AttachmentPage{}, ErrUnavailable
		}
		row.Period.To, row.DetachedAt = copyInstant(row.Period.To), copyInstant(row.DetachedAt)
		out.Attachments = append(out.Attachments, row)
	}
	return out, nil
}
