package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/jackc/pgx/v5"
)

var ErrUnavailable = errors.New("orgdata: unavailable")

type Action uint8

const (
	ListKeys Action = iota + 1
	IssueKey
	RotateKey
	RevokeKey
)

func KeyPolicy(action Action) access.Policy {
	policy := access.Policy{Permission: "data.key.manage", Capability: "data", Fresh: true}
	switch action {
	case ListKeys:
		policy.Effect = access.Read
	case IssueKey, RotateKey:
		policy.Effect = access.ConfigurationWrite
	case RevokeKey:
		policy.Effect = access.ReduceAccess
	default:
		return access.Policy{}
	}
	return policy
}

type KeyAuthority interface {
	CheckKeyActionTx(context.Context, pgx.Tx, kernel.OrgScope, organization.CompanyActor, string, Action) error
}

type Command struct {
	Action           Action
	OperationID      string
	TargetID         string
	ExpectedRevision int64
	Label            string
	Scopes           []domain.Scope
	ExpiresAt        time.Time
	OverlapUntil     time.Time
}

func (c Command) Validate() error {
	if !audit.ValidID(c.OperationID) {
		return domain.ErrInvalidKey
	}
	switch c.Action {
	case IssueKey:
		if c.TargetID != "" || c.ExpectedRevision != 0 || !c.OverlapUntil.IsZero() {
			return domain.ErrInvalidKey
		}
		return domain.KeyTerms(c.Label, c.Scopes, c.ExpiresAt)
	case RotateKey:
		if !audit.ValidID(c.TargetID) || c.ExpectedRevision < 1 || c.TargetID == c.OperationID {
			return domain.ErrInvalidKey
		}
		return domain.KeyTerms(c.Label, c.Scopes, c.ExpiresAt)
	case RevokeKey:
		if audit.ValidID(c.TargetID) && c.ExpectedRevision > 0 && c.Label == "" &&
			len(c.Scopes) == 0 && c.ExpiresAt.IsZero() && c.OverlapUntil.IsZero() {
			return nil
		}
	}
	return domain.ErrInvalidKey
}

func (c Command) Digest(scope kernel.OrgScope) string {
	c.ExpiresAt, c.OverlapUntil = c.ExpiresAt.UTC(), c.OverlapUntil.UTC()
	bound, _ := json.Marshal(struct {
		OrgID string
		Input Command
	}{scope.OrgID().String(), c})
	hash := sha256.Sum256(bound)
	return hex.EncodeToString(hash[:])
}

type KeyResult struct {
	Key      domain.Key
	Bearer   domain.Bearer
	Replayed bool
}

type KeyPage struct {
	Keys    []domain.Key
	AfterID string
}

type KeyCommands interface {
	WriteKey(context.Context, kernel.OrgScope, organization.CompanyActor, string, Command) (KeyResult, error)
	ListKeys(context.Context, kernel.OrgScope, organization.CompanyActor, string, string, int) (KeyPage, error)
}
