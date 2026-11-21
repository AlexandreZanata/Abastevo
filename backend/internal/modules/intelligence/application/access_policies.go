// Package application orchestrates Phase F6 operations: internal fuel stock,
// preventive and corrective maintenance, document/licence expiries, KPI overview,
// price comparisons, findings review and detection, audit trail viewer,
// consolidation grants, retention/legal holds, and complementary controls.
package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	stock "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fuelstock/domain"
	intel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/intelligence/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	plan "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/plan"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

var (
	ErrUnauthenticated = errors.New("intelligence: current company proof required")
	ErrUnavailable     = errors.New("intelligence: unavailable")
	ErrNotFound        = errors.New("intelligence: not found")
	ErrInvalid         = errors.New("intelligence: validation failed")
	ErrConflict        = errors.New("intelligence: revision or state conflict")
	ErrOperationReused = errors.New("intelligence: operation identifier reused")
)

type private struct{}

func (private) String() string               { return "IntelligenceValue(redacted)" }
func (private) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, "IntelligenceValue(redacted)") }
func (private) MarshalJSON() ([]byte, error) { return []byte(`"IntelligenceValue(redacted)"`), nil }
func (private) LogValue() slog.Value         { return slog.StringValue("IntelligenceValue(redacted)") }

type Actor struct {
	private
	AccountID string
	SessionID string
}

func canonicalID(raw string) bool {
	id, err := kernel.ParseOrgID(raw)
	return err == nil && id.String() == raw
}

func NewActor(accountID, sessionID string) (Actor, error) {
	if !canonicalID(accountID) || !canonicalID(sessionID) {
		return Actor{}, ErrUnauthenticated
	}
	return Actor{AccountID: accountID, SessionID: sessionID}, nil
}

func TankPolicy(write bool) access.Policy {
	effect := access.Read
	if write {
		effect = access.ConfigurationWrite
	}
	return access.Policy{Permission: permissions.Permission("stock.tank.manage"), Capability: "organizations", Effect: effect, Fresh: write}
}

func MovementPolicy(write bool) access.Policy {
	effect := access.Read
	if write {
		effect = access.OperationalWrite
	}
	return access.Policy{Permission: permissions.Permission("stock.movement.record"), Capability: "organizations", Effect: effect}
}

func PlanPolicy(write bool) access.Policy {
	effect := access.Read
	if write {
		effect = access.ConfigurationWrite
	}
	return access.Policy{Permission: permissions.Permission("maint.plan.manage"), Capability: "organizations", Effect: effect, Fresh: write}
}

func WorkOrderPolicy(write bool) access.Policy {
	effect := access.Read
	if write {
		effect = access.ConfigurationWrite
	}
	return access.Policy{Permission: permissions.Permission("maint.workorder.manage"), Capability: "organizations", Effect: effect, Fresh: write}
}

func WorkOrderReviewPolicy(write bool) access.Policy {
	effect := access.Read
	if write {
		effect = access.ConfigurationWrite
	}
	return access.Policy{Permission: permissions.Permission("maint.workorder.review"), Capability: "organizations", Effect: effect, Fresh: write}
}

func MaintenanceRequestPolicy() access.Policy {
	return access.Policy{Permission: permissions.Permission("maint.request.create"), Capability: "organizations", Effect: access.Read}
}

func AssetDocumentManagePolicy() access.Policy {
	return access.Policy{Permission: permissions.Permission("fleet.asset.manage"), Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
}

func KpiPolicy() access.Policy {
	return access.Policy{Permission: permissions.Permission("kpi.dashboard.read"), Capability: "organizations", Effect: access.Read}
}

func FindingReadPolicy() access.Policy {
	return access.Policy{Permission: permissions.Permission("find.finding.read"), Capability: "organizations", Effect: access.Read}
}

func FindingResolvePolicy(write bool) access.Policy {
	effect := access.Read
	if write {
		effect = access.ConfigurationWrite
	}
	return access.Policy{Permission: permissions.Permission("find.finding.resolve"), Capability: "organizations", Effect: effect, Fresh: write}
}

func SettingsManagePolicy() access.Policy {
	return access.Policy{Permission: permissions.Permission("org.settings.manage"), Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
}

func AuditReadPolicy() access.Policy {
	return access.Policy{Permission: permissions.Permission("audit.event.read"), Capability: "organizations", Effect: access.Read}
}

func GroupManagePolicy(write bool) access.Policy {
	effect := access.Read
	if write {
		effect = access.ConfigurationWrite
	}
	return access.Policy{Permission: permissions.Permission("org.group.manage"), Capability: "organizations", Effect: effect, Fresh: true}
}

func HashRequest(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", ErrInvalid
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// Re-export domain error helpers for adapter mapping.
var (
	ErrInsufficientStock  = stock.ErrInsufficientStock
	ErrProductMismatch    = stock.ErrProductMismatch
	ErrSelfApproval       = plan.ErrSelfApproval
	ErrOverrunDenied      = plan.ErrOverrunDenied
	ErrHoldOpen           = plan.ErrHoldOpen
	ErrQuoteExpired       = plan.ErrQuoteExpired
	ErrInvoiceRequired    = plan.ErrInvoiceRequired
	ErrFindingClosed      = intel.ErrFindingClosed
	ErrFindingSelfResolve = intel.ErrFindingSelfApproval
)
