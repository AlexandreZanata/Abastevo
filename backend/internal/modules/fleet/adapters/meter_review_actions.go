package adapters

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *MeterReviewStore) reviewer(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.FleetActor, member, target string, write bool) (app.MeterReviewReviewer, error) {
	if s == nil || s.Authority == nil {
		return app.MeterReviewReviewer{}, app.ErrUnavailable
	}
	person, err := s.Authority.CheckMeterReviewTx(ctx, tx, scope, actor, member, target, write)
	if err != nil {
		return app.MeterReviewReviewer{}, err
	}
	if person.AccountID != actor.AccountID || person.MembershipID != member || !validID(person.EmployeeID) {
		return app.MeterReviewReviewer{}, app.ErrUnavailable
	}
	return person, nil
}
func (s *MeterReviewStore) reviewView(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, id string, now time.Time) (app.MeterReviewView, error) {
	row, err := q.New(db).ReadMeterReview(ctx, q.ReadMeterReviewParams{OrgID: scope.OrgID().String(), ID: id})
	if err != nil {
		return app.MeterReviewView{}, meterError(ctx, err)
	}
	view, err := loadMeter(ctx, db, scope, row.AssetID, row.MeterID, false)
	if err != nil {
		return app.MeterReviewView{}, meterError(ctx, err)
	}
	if view.Segment.ID() != row.SegmentID {
		return app.MeterReviewView{}, app.ErrUnavailable
	}
	value, err := meter.NewValue(view.Reference.Kind(), row.Value, view.Reference.Modulus())
	if err != nil {
		return app.MeterReviewView{}, app.ErrUnavailable
	}
	at, err := exactMeterTime(row.OccurredAt, row.OccurredSubmicro)
	if err != nil {
		return app.MeterReviewView{}, err
	}
	received, err := exactMeterTime(row.ReceivedAt, 0)
	if err != nil || received.IsZero() {
		return app.MeterReviewView{}, app.ErrUnavailable
	}
	// Expiry can pass year9999 for otherwise valid historic evidence; review
	// receipt is an actual current server instant, not the device declaration.
	expires, err := exactMeterTime(row.ExpiresAt, 0)
	if err != nil {
		return app.MeterReviewView{}, err
	}
	fact := app.MeterReadingInput{OperationID: row.OperationID, AssetID: row.AssetID, MeterID: row.MeterID, SegmentID: row.SegmentID, ReadingID: row.ReadingID, Value: value, Declared: orgtime.Declaration{OccurredAt: at, EntryZone: row.EntryZone, DeviceOffsetSeconds: row.DeviceOffsetSeconds}, Origin: meter.Origin(row.Origin), Method: meter.Method(row.Method)}
	if _, err := app.NewMeterReadingCommand(fact); err != nil {
		return app.MeterReviewView{}, app.ErrUnavailable
	}
	result := app.MeterReviewView{ReviewItemID: row.ID, OperationID: row.OperationID, PayloadDigest: row.PayloadDigest, Code: row.Code, State: app.MeterReviewOpen, Revision: 1, AuthorAccountID: row.OwnerAccountID, AuthorMembershipID: row.MembershipID, AuthorEmployeeID: row.AuthorEmployeeID, EnrollmentID: row.EnrollmentID, ReceivedAt: received, ExpiresAt: expires, Fact: fact}
	if !now.Before(expires) {
		result.State = app.MeterReviewExpired
	}
	if len(row.CanonicalResolution) > 0 {
		resolution, err := protocol.ParseReviewResolution(row.CanonicalResolution)
		if err != nil || !bytes.Equal(resolution.CanonicalBytes(), row.CanonicalResolution) {
			return app.MeterReviewView{}, app.ErrUnavailable
		}
		v := resolution.Snapshot()
		if v.OperationID != row.OperationID || v.PayloadDigest != row.PayloadDigest || v.ReviewItemID != row.ID || v.Outcome != row.Outcome {
			return app.MeterReviewView{}, app.ErrUnavailable
		}
		result.Resolution = &resolution
		result.Revision = 2
		if v.Outcome == protocol.ReviewAccepted {
			result.State = app.MeterReviewAccepted
		} else {
			result.State = app.MeterReviewRefused
		}
	}
	return result, nil
}

func (s *MeterReviewStore) ReadMeterReview(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, id string) (app.MeterReviewView, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.MeterReviewView{}, err
	}
	if !validID(id) {
		return app.MeterReviewView{}, meter.ErrInvalid
	}
	if s == nil || s.Pool == nil || s.Authority == nil {
		return app.MeterReviewView{}, app.ErrUnavailable
	}
	var result app.MeterReviewView
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if _, err := s.reviewer(ctx, tx, scope, actor, member, id, false); err != nil {
			return err
		}
		now, err := meterDBNow(ctx, q.New(tx))
		if err != nil {
			return err
		}
		result, err = s.reviewView(ctx, tx, scope, id, now)
		if err != nil {
			return err
		}
		_, err = s.reviewer(ctx, tx, scope, actor, member, id, false)
		return err
	})
	if err != nil {
		return app.MeterReviewView{}, meterError(ctx, err)
	}
	return result.Clone(), nil
}
func (s *MeterReviewStore) ListMeterReviews(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member string, filter app.MeterReviewFilter) (app.MeterReviewPage, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.MeterReviewPage{}, err
	}
	if err := filter.Validate(); err != nil {
		return app.MeterReviewPage{}, err
	}
	if s == nil || s.Pool == nil || s.Authority == nil {
		return app.MeterReviewPage{}, app.ErrUnavailable
	}
	result := app.MeterReviewPage{Items: []app.MeterReviewView{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if _, err := s.reviewer(ctx, tx, scope, actor, member, scope.OrgID().String(), false); err != nil {
			return err
		}
		now, err := meterDBNow(ctx, q.New(tx))
		if err != nil {
			return err
		}
		ids, err := q.New(tx).ListMeterReviewIDs(ctx, q.ListMeterReviewIDsParams{OrgID: scope.OrgID().String(), AfterID: filter.AfterID, State: filter.State, At: pgtype.Timestamptz{Time: now, Valid: true}, RowLimit: int32(filter.Limit)})
		if err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := s.reviewer(ctx, tx, scope, actor, member, id, false); err != nil {
				return err
			}
			view, err := s.reviewView(ctx, tx, scope, id, now)
			if err != nil {
				return err
			}
			result.Items = append(result.Items, view)
		}
		if len(ids) == filter.Limit {
			result.NextID = ids[len(ids)-1]
		}
		_, err = s.reviewer(ctx, tx, scope, actor, member, scope.OrgID().String(), false)
		return err
	})
	if err != nil {
		return app.MeterReviewPage{}, meterError(ctx, err)
	}
	return result, nil
}

func (s *MeterReviewStore) ResolveMeterReview(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member string, command app.MeterReviewDecision) (protocol.ReviewResolution, bool, error) {
	if err := validCall(scope, actor, member); err != nil {
		return protocol.ReviewResolution{}, false, err
	}
	in, err := command.Snapshot()
	if err != nil {
		return protocol.ReviewResolution{}, false, err
	}
	if s == nil || s.Pool == nil || s.Authority == nil || s.Audit == nil {
		return protocol.ReviewResolution{}, false, app.ErrUnavailable
	}
	var result protocol.ReviewResolution
	replay := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		reviewer, err := s.reviewer(ctx, tx, scope, actor, member, in.ReviewItemID, true)
		if err != nil {
			return err
		}
		queries := q.New(tx)
		org := scope.OrgID().String()
		if _, err := queries.LockMeterReview(ctx, q.LockMeterReviewParams{OrgID: org, ID: in.ReviewItemID}); err != nil {
			return err
		}
		old, err := queries.MeterReviewDecisionReceipt(ctx, q.MeterReviewDecisionReceiptParams{OrgID: org, ID: in.OperationID})
		if err == nil {
			if old.RequestHash != command.Digest(scope) || old.ReviewerAccountID != actor.AccountID || old.ReviewItemID != in.ReviewItemID {
				return app.ErrOperationReused
			}
			result, err = protocol.ParseReviewResolution(old.CanonicalResolution)
			if err != nil || !bytes.Equal(result.CanonicalBytes(), old.CanonicalResolution) {
				return app.ErrUnavailable
			}
			replay = true
			_, err = s.reviewer(ctx, tx, scope, actor, member, in.ReviewItemID, true)
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		now, err := meterDBNow(ctx, queries)
		if err != nil {
			return err
		}
		view, err := s.reviewView(ctx, tx, scope, in.ReviewItemID, now)
		if err != nil {
			return err
		}
		if view.Revision != in.ExpectedRevision || view.Revision != 1 || view.Resolution != nil {
			return app.ErrConflict
		}
		author, err := queries.CanonicalReviewEmployee(ctx, q.CanonicalReviewEmployeeParams{OrgID: org, ID: view.AuthorEmployeeID})
		if err != nil || !validID(author) {
			return app.ErrUnavailable
		}
		if reviewer.AccountID == view.AuthorAccountID || reviewer.MembershipID == view.AuthorMembershipID || reviewer.EmployeeID == author {
			return orgapi.SelfApproval
		}
		bound := audit.Event{OperationID: in.OperationID, ActorID: actor.AccountID, Action: "offl.review.resolved", TargetType: "offl.review", TargetID: in.ReviewItemID, RequestHash: command.Digest(scope)}
		auditReplay, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(ctx context.Context, tx pgx.Tx) error {
			fresh, err := s.reviewer(ctx, tx, scope, actor, member, in.ReviewItemID, true)
			if err != nil {
				return err
			}
			if fresh.AccountID != reviewer.AccountID || fresh.MembershipID != reviewer.MembershipID || fresh.EmployeeID != reviewer.EmployeeID {
				return app.ErrUnavailable
			}
			at, err := meterDBNow(ctx, q.New(tx))
			if err != nil {
				return err
			}
			value := protocol.ReviewResolutionSnapshot{OperationID: view.OperationID, PayloadDigest: view.PayloadDigest, ReviewItemID: in.ReviewItemID, ResolutionID: in.OperationID, ReviewRevision: 2, Outcome: protocol.ReviewRefused, Reason: in.Reason, ResolvedAt: orgtime.FormatInstant(at)}
			params := q.InsertMeterReviewResolutionParams{OrgID: org, ID: in.OperationID, ReviewItemID: in.ReviewItemID, ExpectedRevision: in.ExpectedRevision, Outcome: protocol.ReviewRefused, Reason: in.Reason, ReviewerAccountID: reviewer.AccountID, ReviewerMembershipID: reviewer.MembershipID, ReviewerEmployeeID: reviewer.EmployeeID, ReviewerSessionID: actor.SessionID, RequestHash: command.Digest(scope), ResolvedAt: pgtype.Timestamptz{Time: at, Valid: true}}
			if in.Decision == app.MeterReviewAccept {
				revision := int64(1)
				value.Outcome = protocol.ReviewAccepted
				value.EntityRevision = &revision
				params.Outcome = protocol.ReviewAccepted
				params.EntityRevision = pgtype.Int8{Int64: 1, Valid: true}
				params.ReadingID = view.Fact.ReadingID
			}
			result, err = protocol.NewReviewResolution(value)
			if err != nil {
				return app.ErrUnavailable
			}
			params.CanonicalResolution = result.CanonicalBytes()
			if err := q.New(tx).InsertMeterReviewResolution(ctx, params); err != nil {
				return err
			}
			if in.Decision == app.MeterReviewAccept {
				if _, err := s.ApplyReviewedMeterTx(ctx, syncapp.Borrow(tx), scope, in.OperationID); err != nil {
					return err
				}
				if err := q.New(tx).InsertMeterReviewEffect(ctx, q.InsertMeterReviewEffectParams{OrgID: org, OperationID: view.OperationID}); err != nil {
					return err
				}
			}
			_, err = s.reviewer(ctx, tx, scope, actor, member, in.ReviewItemID, true)
			return err
		})
		if err != nil {
			return err
		}
		if auditReplay {
			return app.ErrUnavailable
		}
		_, err = s.reviewer(ctx, tx, scope, actor, member, in.ReviewItemID, true)
		return err
	})
	if err != nil {
		return protocol.ReviewResolution{}, false, meterError(ctx, err)
	}
	return result, replay, nil
}

func (s *MeterReviewStore) ApplyReviewedMeterTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, resolutionID string) (meter.Reading, error) {
	db = syncapp.Borrow(db)
	if s == nil || db == nil || !validID(resolutionID) || database.ValidateOrgDBTX(ctx, db, scope) != nil {
		return meter.Reading{}, app.ErrUnavailable
	}
	queries := q.New(db)
	row, err := queries.ReadMeterReviewResolution(ctx, q.ReadMeterReviewResolutionParams{OrgID: scope.OrgID().String(), ID: resolutionID})
	if err != nil {
		return meter.Reading{}, meterError(ctx, err)
	}
	if row.Outcome != protocol.ReviewAccepted {
		return meter.Reading{}, app.ErrUnavailable
	}
	resolution, err := protocol.ParseReviewResolution(row.CanonicalResolution)
	if err != nil || resolution.Snapshot().ResolutionID != resolutionID || resolution.Snapshot().OperationID != row.OperationID {
		return meter.Reading{}, app.ErrUnavailable
	}
	view, err := loadMeter(ctx, db, scope, row.AssetID, row.MeterID, false)
	if err != nil {
		return meter.Reading{}, meterError(ctx, err)
	}
	if view.Segment.ID() != row.SegmentID {
		return meter.Reading{}, app.ErrUnavailable
	}
	reading, err := queries.GetMeterReadingByOperation(ctx, q.GetMeterReadingByOperationParams{OrgID: scope.OrgID().String(), MeterID: row.MeterID, OperationID: row.OperationID})
	if errors.Is(err, pgx.ErrNoRows) {
		if err := queries.InsertReviewedMeterReading(ctx, q.InsertReviewedMeterReadingParams{OrgID: scope.OrgID().String(), ID: resolutionID}); err != nil {
			return meter.Reading{}, meterError(ctx, err)
		}
		reading, err = queries.GetMeterReadingByOperation(ctx, q.GetMeterReadingByOperationParams{OrgID: scope.OrgID().String(), MeterID: row.MeterID, OperationID: row.OperationID})
	}
	if err != nil || reading.ID != row.ReadingID {
		return meter.Reading{}, app.ErrUnavailable
	}
	return meterReading(scope, view, reading)
}

func reviewSelectorValid(v protocol.ReviewSelector) bool {
	if !validID(v.OperationID) || len(v.PayloadDigest) != 71 || !strings.HasPrefix(v.PayloadDigest, "sha256:") {
		return false
	}
	for _, c := range v.PayloadDigest[7:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func (s *MeterReviewStore) OwnMeterReviewResults(ctx context.Context, source syncapp.Source, selectors []protocol.ReviewSelector) ([]protocol.ReviewResult, error) {
	if s == nil || s.Pool == nil || s.ConfirmSource == nil || source.Validate() != nil {
		return nil, app.ErrUnavailable
	}
	selectors = append([]protocol.ReviewSelector(nil), selectors...)
	if len(selectors) > protocol.MaxReviewResultSelectors {
		return nil, meter.ErrInvalid
	}
	seen := map[string]bool{}
	for _, selector := range selectors {
		if !reviewSelectorValid(selector) || seen[selector.OperationID] {
			return nil, meter.ErrInvalid
		}
		seen[selector.OperationID] = true
	}
	results := make([]protocol.ReviewResult, 0, len(selectors))
	err := database.WithOrgTx(ctx, s.Pool, source.Scope, func(tx pgx.Tx) error {
		if _, err := s.ConfirmSource(ctx, tx, source); err != nil {
			return syncapp.SourceDenied(err)
		}
		for _, selector := range selectors {
			proof, err := q.New(tx).ReadMeterHeldProof(ctx, q.ReadMeterHeldProofParams{OrgID: source.Scope.OrgID().String(), OperationID: selector.OperationID})
			if errors.Is(err, pgx.ErrNoRows) {
				return orgapi.NotFound
			}
			if err != nil {
				return err
			}
			if proof.OwnerAccountID != source.Actor.AccountID || proof.ApiOrigin != source.Origin || proof.MembershipID != source.MembershipID || proof.EnrollmentID != source.EnrollmentID || proof.PayloadDigest != selector.PayloadDigest || proof.ProofVersion != 1 || proof.AcknowledgementProofVersion != 1 || proof.AcknowledgementResult != protocol.AcceptedForReview || proof.AcknowledgementCode != proof.Code || proof.AcknowledgementReviewItemID != proof.ID || !proof.ReceivedAt.Valid {
				return orgapi.NotFound
			}
			raw, err := q.New(tx).OwnMeterReviewResolution(ctx, q.OwnMeterReviewResolutionParams{OrgID: source.Scope.OrgID().String(), ID: proof.ID})
			if err != nil {
				return err
			}
			result := protocol.ReviewResult{Selector: selector}
			if len(raw) > 0 {
				resolution, err := protocol.ParseReviewResolution(raw)
				if err != nil || !bytes.Equal(raw, resolution.CanonicalBytes()) {
					return app.ErrUnavailable
				}
				value := resolution.Snapshot()
				if value.OperationID != selector.OperationID || value.PayloadDigest != selector.PayloadDigest || value.ReviewItemID != proof.ID {
					return app.ErrUnavailable
				}
				result.Resolution = &resolution
			}
			results = append(results, result)
		}
		_, err := s.ConfirmSource(ctx, tx, source)
		if err != nil {
			return syncapp.SourceDenied(err)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, syncapp.ErrSourceDenied) {
			return nil, err
		}
		return nil, meterError(ctx, err)
	}
	return results, nil
}
