package application

import (
	"context"
	"time"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/jackc/pgx/v5"
)

// MeterReviewPolicy is fixed by the owner, never selected by a submitted item.
func MeterReviewPolicy(write bool) access.Policy {
	if write {
		return access.Policy{Permission: "meter.reading.correct", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	}
	return access.Policy{Permission: "meter.reading.correct", Capability: "organizations", Effect: access.Read}
}

type MeterReviewReviewer struct {
	private
	AccountID, MembershipID, EmployeeID string
}

// MeterReviewAuthority retains current company proof and the original lock
// order. Target is an actual review ID or the explicit organization list root.
type MeterReviewAuthority interface {
	CheckMeterReviewTx(context.Context, pgx.Tx, kernel.OrgScope, FleetActor, string, string, bool) (MeterReviewReviewer, error)
}

const (
	MeterReviewOpen     = "OPEN"
	MeterReviewExpired  = "EXPIRED_UNRESOLVED"
	MeterReviewAccepted = "RESOLVED_ACCEPTED"
	MeterReviewRefused  = "RESOLVED_REFUSED"
)

type MeterReviewView struct {
	private
	ReviewItemID, OperationID, PayloadDigest, Code, State               string
	Revision                                                            int64
	AuthorAccountID, AuthorMembershipID, AuthorEmployeeID, EnrollmentID string
	ReceivedAt, ExpiresAt                                               time.Time
	Fact                                                                MeterReadingInput
	Resolution                                                          *protocol.ReviewResolution
}

func (v MeterReviewView) Clone() MeterReviewView {
	if v.Resolution != nil {
		value := *v.Resolution
		v.Resolution = &value
	}
	return v
}

// Validate checks the complete owner projection. Organization isolation is
// supplied by the repository's retained scope and current reviewer proof.
func (v MeterReviewView) Validate() error {
	if !validReviewView(v) {
		return ErrUnavailable
	}
	return nil
}

type MeterReviewFilter struct {
	private
	AfterID, State string
	Limit          int
}

func (v MeterReviewFilter) Validate() error {
	if v.Limit < 1 || v.Limit > 100 || v.AfterID != "" && !canonicalID(v.AfterID) {
		return meter.ErrInvalid
	}
	switch v.State {
	case "", MeterReviewOpen, MeterReviewExpired, MeterReviewAccepted, MeterReviewRefused:
		return nil
	}
	return meter.ErrInvalid
}

type MeterReviewPage struct {
	private
	Items  []MeterReviewView
	NextID string
}

type MeterReviewRepository interface {
	ListMeterReviews(context.Context, kernel.OrgScope, FleetActor, string, MeterReviewFilter) (MeterReviewPage, error)
	ReadMeterReview(context.Context, kernel.OrgScope, FleetActor, string, string) (MeterReviewView, error)
	ResolveMeterReview(context.Context, kernel.OrgScope, FleetActor, string, MeterReviewDecision) (protocol.ReviewResolution, bool, error)
}

// MeterOwnReviewRepository returns only this current enrolled author's minimal
// immutable outcomes. It must not grant access to a business record or preview.
type MeterOwnReviewRepository interface {
	OwnMeterReviewResults(context.Context, syncapp.Source, []protocol.ReviewSelector) ([]protocol.ReviewResult, error)
}

type MeterReviewService struct {
	Enabled    bool
	Repository MeterReviewRepository
}
