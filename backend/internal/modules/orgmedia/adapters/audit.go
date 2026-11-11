package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	auditapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
	"github.com/jackc/pgx/v5"
)

func AuditDefinitions() []audit.Definition {
	return []audit.Definition{
		{Action: "document.access.issued", TargetType: "document", Reasons: []string{"member.authorized"}, RequireReason: true},
		{Action: "document.content.accessed", TargetType: "document", Reasons: []string{"member.authorized"}, RequireReason: true},
	}
}

type AccessAudit struct{ Runner auditapp.Runner }

var _ application.Audit = AccessAudit{}

func (recorder AccessAudit) Record(ctx context.Context, scope kernel.OrgScope, member domain.Member, object domain.Object, accessID, requestID string, intent domain.Intent) error {
	if object.Validate(scope, object.DocumentID) != nil || member.Validate() != nil || !domain.ValidID(accessID) || !domain.ValidID(requestID) ||
		(intent != domain.Issue && intent != domain.Read) {
		return domain.ErrInvalid
	}
	if recorder.Runner == nil {
		return domain.ErrUnavailable
	}
	action := "document.access.issued"
	if intent == domain.Read {
		action = "document.content.accessed"
	}
	raw, _ := json.Marshal([]any{"orgmedia.audit.v1", object.OrgID, member.AccountID, member.MembershipID, object.DocumentID,
		object.SHA256, object.MIME, object.Bytes, object.Revision, accessID, intent})
	sum := sha256.Sum256(raw)
	event := audit.Event{OperationID: requestID, ActorID: member.AccountID, Action: action, TargetType: "document", TargetID: object.DocumentID,
		ReasonCode: "member.authorized", RequestHash: hex.EncodeToString(sum[:])}
	replayed, err := recorder.Runner.RecordChange(ctx, scope, event, func(context.Context, pgx.Tx) error { return nil })
	if err != nil || replayed {
		return domain.ErrUnavailable
	}
	return nil
}
