package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	auditstore "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	auditapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
)

// AuditDefinitions is the DOCUMENT_CONTENT part of the sensitive-access
// vocabulary; its action names are unchanged from EMP-PLAT-006.
func AuditDefinitions() []audit.Definition {
	return audit.AccessDefinitions(audit.DocumentContent)
}

// AccessAudit routes private document issuance and reads through the
// organization sensitive-access log (EMP-PRIV-005).
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
	stage := audit.AccessIssued
	if intent == domain.Read {
		stage = audit.AccessServed
	}
	// The binding pins the exact verified revision without recording its
	// digest, type or size as readable facts.
	raw, _ := json.Marshal([]any{"orgmedia.binding.v1", object.SHA256, object.MIME, object.Bytes, object.Revision})
	sum := sha256.Sum256(raw)
	access := audit.SensitiveAccess{Category: audit.DocumentContent, Stage: stage, AccountID: member.AccountID,
		MembershipID: member.MembershipID, TargetID: object.DocumentID, AccessID: accessID, RequestID: requestID,
		Binding: hex.EncodeToString(sum[:])}
	if err := (auditstore.AccessLog{Runner: recorder.Runner}).RecordAccess(ctx, scope, access); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}
