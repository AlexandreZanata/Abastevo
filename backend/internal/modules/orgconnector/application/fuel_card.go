package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/domain"
)

var ErrStatementReviewDenied = errors.New("orgconnector: fueling review permission required")

type Statement struct {
	ConnectionID, SourceAccountID string
	CSV                           string
	OperationID, PreviewProof     string
}

func (s Statement) Validate(action Action) error {
	if !audit.ValidID(s.ConnectionID) || !audit.ValidID(s.SourceAccountID) || s.CSV == "" {
		return domain.ErrInvalid
	}
	switch action {
	case PreviewStatement:
		if s.OperationID == "" && s.PreviewProof == "" {
			return nil
		}
	case ConfirmStatement:
		if audit.ValidID(s.OperationID) && len(s.PreviewProof) == sha256.Size*2 {
			return nil
		}
	}
	return domain.ErrInvalid
}

func (s Statement) PayloadDigest() string {
	raw, _ := json.Marshal([]string{"fuel-card-csv-v1", s.ConnectionID, s.SourceAccountID, s.CSV})
	return Digest(raw)
}

func Digest(raw []byte) string {
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

type StatementOutcome struct {
	Row          int    `json:"row"`
	PaidCentavos int64  `json:"paid_centavos"`
	Status       string `json:"status"`
}

type StatementResult struct {
	Rows     []StatementOutcome
	Proof    string
	Replayed bool
}

type StatementCommands interface {
	ProcessStatement(context.Context, kernel.OrgScope, organization.CompanyActor, string, Action, Statement) (StatementResult, error)
}
