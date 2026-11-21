package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fueling"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const reviewTarget = "fueling.review"

func reviewAction(kind review.Kind) string { return reviewTarget + "." + strings.ToLower(string(kind)) }

func ReviewAuditDefinitions() []audit.Definition {
	return []audit.Definition{
		{Action: reviewAction(review.Open), TargetType: reviewTarget},
		{Action: reviewAction(review.Approve), TargetType: reviewTarget},
		{Action: reviewAction(review.Return), TargetType: reviewTarget},
		{Action: reviewAction(review.Reject), TargetType: reviewTarget},
	}
}

type ReviewAuthority interface {
	CheckFuelingReviewTx(context.Context, pgx.Tx, kernel.OrgScope, organization.CompanyActor, string, string, bool) error
}

type ReviewStore struct {
	Pool      *pgxpool.Pool
	Authority ReviewAuthority
	Audit     auditing.TransactionRunner
}

func (s ReviewStore) validate(scope kernel.OrgScope, actor organization.CompanyActor, member, fueling string) error {
	if s.Pool == nil || s.Authority == nil || s.Audit == nil {
		return app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || !audit.ValidID(member) || !audit.ValidID(fueling) {
		return app.ErrNotFound
	}
	return nil
}

func (s ReviewStore) ReadReview(ctx context.Context, scope kernel.OrgScope, actor organization.CompanyActor, member, fueling string) (app.ReviewView, error) {
	if err := s.validate(scope, actor, member, fueling); err != nil {
		return app.ReviewView{}, err
	}
	var result app.ReviewView
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckFuelingReviewTx(ctx, tx, scope, actor, member, fueling, false); err != nil {
			return err
		}
		var err error
		_, result, err = reviewContext(ctx, q.New(tx), scope.OrgID().String(), fueling)
		return err
	})
	if err != nil {
		return app.ReviewView{}, err
	}
	return result, nil
}

func (s ReviewStore) WriteReview(ctx context.Context, scope kernel.OrgScope, actor organization.CompanyActor, member, fueling, operation string,
	input app.ReviewInput,
) (app.ReviewStatus, bool, error) {
	if err := s.validate(scope, actor, member, fueling); err != nil {
		return app.ReviewStatus{}, false, err
	}
	if !audit.ValidID(operation) || input.Kind.Role() != review.RoleReviewer {
		return app.ReviewStatus{}, false, review.ErrInvalid
	}
	org := scope.OrgID().String()
	request, err := json.Marshal([]any{1, org, fueling, input})
	if err != nil {
		return app.ReviewStatus{}, false, app.ErrUnavailable
	}
	hash := sha256.Sum256(request)
	event := audit.Event{OperationID: operation, ActorID: actor.AccountID, Action: reviewAction(input.Kind),
		TargetType: reviewTarget, TargetID: fueling, RequestHash: hex.EncodeToString(hash[:])}
	var status app.ReviewStatus
	var replayed bool
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckFuelingReviewTx(ctx, tx, scope, actor, member, fueling, true); err != nil {
			return err
		}
		var err error
		replayed, err = s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			return applyFuelingReview(ctx, q.New(tx), org, member, fueling, operation, actor.AccountID, input)
		})
		if err != nil {
			return err
		}
		row, err := q.New(tx).ReadFuelingDecision(ctx, q.ReadFuelingDecisionParams{OrgID: org, FuelingID: fueling, OperationID: operation})
		if err != nil {
			return err
		}
		state, err := review.ParseState(row.State)
		if err != nil {
			return err
		}
		status = app.ReviewStatus{Sequence: row.Sequence, State: state}
		return nil
	})
	if err != nil {
		return app.ReviewStatus{}, false, err
	}
	return status, replayed, nil
}

func reviewContext(ctx context.Context, queries *q.Queries, org, fueling string) ([]review.Entry, app.ReviewView, error) {
	rows, err := queries.ListFuelingReviewEntries(ctx, q.ListFuelingReviewEntriesParams{OrgID: org, FuelingID: fueling})
	if err != nil {
		return nil, app.ReviewView{}, err
	}
	if len(rows) == 0 {
		return nil, app.ReviewView{}, app.ErrNotFound
	}
	entries := make([]review.Entry, 0, len(rows))
	for index, row := range rows {
		if row.Sequence != int64(index+1) {
			return nil, app.ReviewView{}, app.ErrUnavailable
		}
		entries = append(entries, review.Entry{Kind: review.Kind(row.Kind), Revision: row.Revision, State: review.State(row.State), Reason: row.Reason})
	}
	current, err := review.Replay(entries)
	if err != nil {
		return nil, app.ReviewView{}, app.ErrUnavailable
	}
	warnings, err := queries.ListFuelingReviewWarnings(ctx, q.ListFuelingReviewWarningsParams{OrgID: org, FuelingID: fueling})
	if err != nil {
		return nil, app.ReviewView{}, err
	}
	duplicate, err := queries.FuelingReviewDuplicatePending(ctx, q.FuelingReviewDuplicatePendingParams{OrgID: org, FuelingID: fueling})
	if err != nil {
		return nil, app.ReviewView{}, err
	}
	return entries, app.ReviewView{
		ReviewStatus: app.ReviewStatus{Sequence: int64(len(entries)), State: current.State()},
		Warnings:     append([]string{}, warnings...), SuspectedDuplicate: duplicate,
	}, nil
}

func applyFuelingReview(ctx context.Context, queries *q.Queries, org, member, fueling, operation, account string, input app.ReviewInput) error {
	people, err := queries.FuelingReviewPeople(ctx, q.FuelingReviewPeopleParams{
		OrgID: org, FuelingID: fueling, MembershipID: member, AccountID: account,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	if err != nil || people.AuthorEmployee == "" || people.ResponsibleEmployee == "" {
		return app.ErrUnavailable
	}
	history, view, err := reviewContext(ctx, queries, org, fueling)
	if err != nil {
		return err
	}
	facts := review.Facts{
		Actor:       review.Identity{Accounts: []string{account}, Memberships: []string{member}, Employees: []string{people.ReviewerEmployee}},
		Author:      review.Identity{Accounts: []string{people.AuthorAccount}, Memberships: []string{people.AuthorMembership}, Employees: []string{people.AuthorEmployee}},
		Beneficiary: &review.Identity{Employees: []string{people.ResponsibleEmployee}},
		Permitted:   true,
	}
	entry, err := app.ApplyReview(history, facts, view.Warnings, view.SuspectedDuplicate, input)
	if err != nil {
		return err
	}
	notes := []byte("{}")
	if len(input.WarningNotes) != 0 {
		notes, err = json.Marshal(input.WarningNotes)
		if err != nil {
			return app.ErrUnavailable
		}
	}
	var entered pgtype.Bool
	if input.FuelEnteredTank != nil {
		entered = pgtype.Bool{Bool: *input.FuelEnteredTank, Valid: true}
	}
	return queries.InsertFuelingReviewDecision(ctx, q.InsertFuelingReviewDecisionParams{
		OrgID: org, FuelingID: fueling, ReviewSequence: view.Sequence + 1, Kind: string(entry.Kind), State: string(entry.State),
		Reason: entry.Reason, Member: member, OperationID: operation, WarningNotes: notes, FuelEnteredTank: entered,
	})
}
