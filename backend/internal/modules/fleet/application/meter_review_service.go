package application

import (
	"context"
	"encoding/hex"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"strings"
)

func currentMeterReview(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, target string, write bool) error {
	if ctx == nil {
		return ErrUnauthenticated
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if scope.Validate() != nil {
		return meter.ErrScope
	}
	policy := MeterReviewPolicy(write)
	if actor.Validate() != nil || decision.Subject().Channel() != "web" || decision.Policy() != policy || decision.Check(ctx, scope, actor.AccountID, policy.Permission, target) != nil {
		return ErrUnauthenticated
	}
	return nil
}

func validReviewView(v MeterReviewView) bool {
	for _, id := range []string{v.ReviewItemID, v.OperationID, v.AuthorAccountID, v.AuthorMembershipID, v.AuthorEmployeeID, v.EnrollmentID} {
		if !canonicalID(id) {
			return false
		}
	}
	if len(v.PayloadDigest) != 71 || !strings.HasPrefix(v.PayloadDigest, "sha256:") || v.PayloadDigest != strings.ToLower(v.PayloadDigest) {
		return false
	}
	if _, e := hex.DecodeString(v.PayloadDigest[7:]); e != nil {
		return false
	}
	if v.Code != "PERMISSION_CHANGED" || v.Fact.OperationID != v.OperationID || v.Fact.Origin != meter.Standalone || v.Fact.Method != meter.Typed || v.ReceivedAt.IsZero() {
		return false
	}
	if _, e := NewMeterReadingCommand(v.Fact); e != nil {
		return false
	}
	for _, at := range []string{orgtime.FormatInstant(v.ReceivedAt), orgtime.FormatInstant(v.ExpiresAt)} {
		if _, e := orgtime.ParseInstant(at); e != nil {
			return false
		}
	}
	if !v.ExpiresAt.After(v.ReceivedAt) {
		return false
	}
	switch v.State {
	case MeterReviewOpen, MeterReviewExpired:
		return v.Revision == 1 && v.Resolution == nil
	case MeterReviewAccepted, MeterReviewRefused:
		if v.Revision != 2 || v.Resolution == nil {
			return false
		}
		r := v.Resolution.Snapshot()
		if _, e := protocol.NewReviewResolution(r); e != nil {
			return false
		}
		resolved, e := orgtime.ParseInstant(r.ResolvedAt)
		if e != nil || resolved.Before(v.ReceivedAt) || r.OperationID != v.OperationID || r.PayloadDigest != v.PayloadDigest || r.ReviewItemID != v.ReviewItemID {
			return false
		}
		return v.State == MeterReviewAccepted && r.Outcome == protocol.ReviewAccepted || v.State == MeterReviewRefused && r.Outcome == protocol.ReviewRefused
	}
	return false
}

func (s MeterReviewService) List(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, filter MeterReviewFilter) (MeterReviewPage, error) {
	if !s.Enabled {
		return MeterReviewPage{}, ErrSurfaceDisabled
	}
	if err := filter.Validate(); err != nil {
		return MeterReviewPage{}, err
	}
	target := scope.OrgID().String()
	if err := currentMeterReview(ctx, scope, actor, decision, target, false); err != nil {
		return MeterReviewPage{}, err
	}
	if unavailable(s.Repository) {
		return MeterReviewPage{}, ErrUnavailable
	}
	page, err := s.Repository.ListMeterReviews(ctx, scope, actor, decision.Subject().MembershipID(), filter)
	if guard := currentMeterReview(ctx, scope, actor, decision, target, false); guard != nil {
		return MeterReviewPage{}, guard
	}
	if err != nil {
		return MeterReviewPage{}, err
	}
	if len(page.Items) > filter.Limit {
		return MeterReviewPage{}, ErrUnavailable
	}
	prior := filter.AfterID
	items := make([]MeterReviewView, 0, len(page.Items))
	for _, item := range page.Items {
		if !validReviewView(item) || item.ReviewItemID <= prior || filter.State != "" && item.State != filter.State {
			return MeterReviewPage{}, ErrUnavailable
		}
		prior = item.ReviewItemID
		items = append(items, item.Clone())
	}
	if page.NextID != "" && (len(items) != filter.Limit || page.NextID != prior || !canonicalID(page.NextID)) {
		return MeterReviewPage{}, ErrUnavailable
	}
	return MeterReviewPage{Items: items, NextID: page.NextID}, nil
}
func (s MeterReviewService) Read(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, id string) (MeterReviewView, error) {
	if !s.Enabled {
		return MeterReviewView{}, ErrSurfaceDisabled
	}
	if !canonicalID(id) {
		return MeterReviewView{}, meter.ErrInvalid
	}
	if err := currentMeterReview(ctx, scope, actor, decision, id, false); err != nil {
		return MeterReviewView{}, err
	}
	if unavailable(s.Repository) {
		return MeterReviewView{}, ErrUnavailable
	}
	view, err := s.Repository.ReadMeterReview(ctx, scope, actor, decision.Subject().MembershipID(), id)
	if guard := currentMeterReview(ctx, scope, actor, decision, id, false); guard != nil {
		return MeterReviewView{}, guard
	}
	if err != nil {
		return MeterReviewView{}, err
	}
	if view.ReviewItemID != id || !validReviewView(view) {
		return MeterReviewView{}, ErrUnavailable
	}
	return view.Clone(), nil
}
func (s MeterReviewService) Resolve(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, command MeterReviewDecision) (protocol.ReviewResolution, bool, error) {
	if !s.Enabled {
		return protocol.ReviewResolution{}, false, ErrSurfaceDisabled
	}
	in, err := command.Snapshot()
	if err != nil {
		return protocol.ReviewResolution{}, false, err
	}
	if err := currentMeterReview(ctx, scope, actor, decision, in.ReviewItemID, true); err != nil {
		return protocol.ReviewResolution{}, false, err
	}
	if unavailable(s.Repository) {
		return protocol.ReviewResolution{}, false, ErrUnavailable
	}
	result, replay, err := s.Repository.ResolveMeterReview(ctx, scope, actor, decision.Subject().MembershipID(), command)
	if guard := currentMeterReview(ctx, scope, actor, decision, in.ReviewItemID, true); guard != nil {
		return protocol.ReviewResolution{}, false, guard
	}
	if err != nil {
		return protocol.ReviewResolution{}, false, err
	}
	v := result.Snapshot()
	if _, e := protocol.NewReviewResolution(v); e != nil || v.ReviewItemID != in.ReviewItemID || v.ResolutionID != in.OperationID || v.Reason != in.Reason || in.ExpectedRevision != 1 || in.Decision == MeterReviewAccept && v.Outcome != protocol.ReviewAccepted || in.Decision == MeterReviewRefuse && v.Outcome != protocol.ReviewRefused {
		return protocol.ReviewResolution{}, false, ErrUnavailable
	}
	return result, replay, nil
}
