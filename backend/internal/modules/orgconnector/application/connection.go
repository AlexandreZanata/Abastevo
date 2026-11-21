package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	default:
		return access.Policy{}
	}
	return policy
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
	digest := sha256.Sum256(input)
	return hex.EncodeToString(digest[:])
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
