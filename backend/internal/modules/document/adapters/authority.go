package adapters

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

// AuthoritySource borrows the caller's scoped transaction (orgaccess Store).
type AuthoritySource interface {
	LoadAuthorityTx(context.Context, access.AuthorityQueries, kernel.OrgScope, string) (access.Authority, error)
}

// CurrentAuthority reuses orgaccess Authorization.Prepare inside the document
// transaction: membership, organization state and coverage over the facts.
// Proof, entitlement and record gates already ran on the guarded entry; this
// recheck catches changes made while the request was being served.
type CurrentAuthority struct {
	Source AuthoritySource
	Now    func() time.Time
}

var _ app.Authority = CurrentAuthority{}

type borrowedSource struct {
	tx     pgx.Tx
	source AuthoritySource
}

func (b borrowedSource) LoadAuthority(ctx context.Context, scope kernel.OrgScope, account string) (access.Authority, error) {
	return b.source.LoadAuthorityTx(ctx, b.tx, scope, account)
}

type documentFacts struct{ facts app.Facts }

func (r documentFacts) Resolve(_ context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: r.facts.TargetID, AuthoredBy: r.facts.AuthoredBy}, nil
}

func (a CurrentAuthority) CheckDocumentTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.Actor, policy access.Policy, facts app.Facts) error {
	if a.Source == nil || a.Now == nil || tx == nil {
		return access.ErrUnavailable
	}
	if policy != app.AttachPolicy() && policy != app.ContentPolicy() {
		return access.ErrUnavailable
	}
	if actor.Validate() != nil {
		return access.ErrUnauthenticated
	}
	if !validID(facts.TargetID) || facts.AuthoredBy != "" && !validID(facts.AuthoredBy) {
		return access.ErrUnavailable
	}
	if err := database.ValidateOrgTx(ctx, tx, scope); err != nil {
		return access.ErrUnavailable
	}
	ctx, finish := access.BeginRequest(ctx)
	defer finish()
	authorization := access.Authorization{Source: borrowedSource{tx: tx, source: a.Source}, Now: a.Now}
	prepared, err := authorization.Prepare(ctx, scope, access.Actor{AccountID: actor.AccountID, Channel: actor.Channel, ContributorID: actor.ContributorID, DeviceKeyID: actor.DeviceKeyID}, policy, documentFacts{facts})
	if err != nil {
		return err
	}
	if prepared.Subject().MembershipID() != actor.MembershipID {
		return access.ErrNoMembership
	}
	return nil
}
