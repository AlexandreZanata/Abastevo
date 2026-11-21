package application

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/domain"
	"github.com/jackc/pgx/v5"
)

var ErrUnavailable = errors.New("orgconnector: unavailable")

type Action uint8

const (
	List Action = iota + 1
	Configure
	Activate
	Disable
	PreviewStatement
	ConfirmStatement
)

func Policy(action Action) access.Policy {
	policy := access.Policy{Permission: "integ.connection.manage", Capability: "integrations", Fresh: true}
	switch action {
	case List:
		policy.Fresh, policy.Effect = false, access.Read
	case Configure, Activate:
		policy.Effect = access.ConfigurationWrite
	case Disable:
		policy.Effect = access.ReduceAccess
	case PreviewStatement, ConfirmStatement:
		policy.Permission, policy.Effect = "import.batch.run", access.OperationalWrite
	default:
		return access.Policy{}
	}
	return policy
}

func StatementReviewPolicy() access.Policy {
	return access.Policy{Permission: "fuel.fueling.review", Capability: "integrations", Fresh: true, Effect: access.OperationalWrite}
}

func CompanyPolicy(policy access.Policy) bool {
	return policy == Policy(List) || policy == Policy(Configure) || policy == Policy(Activate) ||
		policy == Policy(Disable) || policy == Policy(PreviewStatement) || policy == StatementReviewPolicy()
}

type Authority interface {
	CheckConnectionTx(context.Context, pgx.Tx, kernel.OrgScope, organization.CompanyActor, string, Action) error
}

type Change struct {
	Action           Action
	OperationID      string
	TargetID         string
	ExpectedRevision int64
	ApprovalID       string
	ApprovalRevision int64
	Label            string
}

func (c Change) Validate() error {
	if !audit.ValidID(c.OperationID) {
		return domain.ErrInvalid
	}
	switch c.Action {
	case Configure:
		if c.TargetID == "" && c.ExpectedRevision == 0 && audit.ValidID(c.ApprovalID) &&
			c.ApprovalRevision > 0 && domain.ValidLabel(c.Label) {
			return nil
		}
	case Activate, Disable:
		if audit.ValidID(c.TargetID) && c.ExpectedRevision > 0 && c.ApprovalID == "" &&
			c.ApprovalRevision == 0 && c.Label == "" {
			return nil
		}
	}
	return domain.ErrInvalid
}

func (c Change) Digest(scope kernel.OrgScope) string {
	input, _ := json.Marshal(struct {
		Organization string
		Command      Change
	}{scope.OrgID().String(), c})
	return Digest(input)
}

type Result struct {
	Connection domain.Connection
	Replayed   bool
}

type Page struct {
	Connections []domain.Connection
	After       string
}

type Commands interface {
	Change(context.Context, kernel.OrgScope, organization.CompanyActor, string, Change) (Result, error)
	List(context.Context, kernel.OrgScope, organization.CompanyActor, string, string, int) (Page, error)
}
