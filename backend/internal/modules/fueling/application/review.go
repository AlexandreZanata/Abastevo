package application

import (
	"context"
	"errors"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
)

var (
	ErrWarningNotesRequired = errors.New("fueling: notes required for recorded warnings")
	ErrDuplicateUnresolved  = errors.New("fueling: duplicate suspicion unresolved")
)

func ReviewPolicy(write bool) access.Policy {
	policy := access.Policy{Permission: "fuel.fueling.review", Capability: "organizations", Effect: access.Read}
	if write {
		policy.Effect, policy.Fresh = access.ConfigurationWrite, true
	}
	return policy
}

type ReviewInput struct {
	Kind             review.Kind
	ExpectedSequence int64
	Reason           string
	WarningNotes     map[string]string
	FuelEnteredTank  *bool
}

type ReviewStatus struct {
	Sequence int64        `json:"sequence"`
	State    review.State `json:"state"`
}

type ReviewView struct {
	ReviewStatus
	Warnings           []string `json:"warnings"`
	SuspectedDuplicate bool     `json:"suspected_duplicate"`
}

type ReviewService interface {
	ReadReview(context.Context, kernel.OrgScope, organization.CompanyActor, string, string) (ReviewView, error)
	WriteReview(context.Context, kernel.OrgScope, organization.CompanyActor, string, string, string, ReviewInput) (ReviewStatus, bool, error)
}

func ApplyReview(history []review.Entry, facts review.Facts, warnings []string, suspectedDuplicate bool, input ReviewInput) (review.Entry, error) {
	if input.Kind.Role() != review.RoleReviewer || input.ExpectedSequence < 1 ||
		(input.Kind == review.Reject) != (input.FuelEnteredTank != nil) ||
		input.Kind != review.Approve && len(input.WarningNotes) != 0 {
		return review.Entry{}, review.ErrInvalid
	}
	current, err := review.Replay(history)
	if err != nil || len(history) == 0 {
		return review.Entry{}, ErrUnavailable
	}
	if input.ExpectedSequence != int64(len(history)) {
		return review.Entry{}, review.ErrConflict
	}
	next, err := current.Apply(review.Command{Kind: input.Kind, Revision: current.Revision(), Reason: input.Reason}, facts)
	if err != nil {
		return review.Entry{}, err
	}
	if input.Kind == review.Approve {
		if suspectedDuplicate {
			return review.Entry{}, ErrDuplicateUnresolved
		}
		if len(input.WarningNotes) != len(warnings) {
			return review.Entry{}, ErrWarningNotesRequired
		}
		for _, code := range warnings {
			if !fleet.ValidReason(input.WarningNotes[code]) {
				return review.Entry{}, ErrWarningNotesRequired
			}
		}
	}
	entries := next.History()
	return entries[len(entries)-1], nil
}
