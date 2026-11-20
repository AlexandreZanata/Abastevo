// Package application orchestrates Phase F7 station agreements, their
// negotiated price terms, bounded fueling authorizations and the station's
// once-only redemption confirmation (EMP-AGREE-001..006, BR-AGREE-001/002).
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/agreement/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

var (
	ErrUnauthenticated = errors.New("agreement: current company proof required")
	ErrUnavailable     = errors.New("agreement: unavailable")
	ErrNotFound        = errors.New("agreement: not found")
	ErrInvalid         = errors.New("agreement: validation failed")
	ErrConflict        = errors.New("agreement: revision or state conflict")
	ErrOperationReused = errors.New("agreement: operation identifier reused")
	ErrStationMismatch = errors.New("agreement: station does not match")
)

type private struct{}

func (private) String() string               { return "AgreementValue(redacted)" }
func (private) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, "AgreementValue(redacted)") }
func (private) MarshalJSON() ([]byte, error) { return []byte(`"AgreementValue(redacted)"`), nil }
func (private) LogValue() slog.Value         { return slog.StringValue("AgreementValue(redacted)") }

// Actor is the organization-side company actor.
type Actor struct {
	private
	AccountID string
	SessionID string
}

// StationActor is the station-side representative; the station surface
// proves the live grant scope, not an organization membership.
type StationActor struct {
	private
	AccountID string
}

func canonicalID(raw string) bool {
	id, err := kernel.ParseOrgID(raw)
	return err == nil && id.String() == raw
}

// NewActor validates the canonical identifiers of a company actor.
func NewActor(accountID, sessionID string) (Actor, error) {
	if !canonicalID(accountID) || !canonicalID(sessionID) {
		return Actor{}, ErrUnauthenticated
	}
	return Actor{AccountID: accountID, SessionID: sessionID}, nil
}

// NewStationActor validates the canonical station account identifier.
func NewStationActor(accountID string) (StationActor, error) {
	if !canonicalID(accountID) {
		return StationActor{}, ErrUnauthenticated
	}
	return StationActor{AccountID: accountID}, nil
}

// AgreementPolicy governs the fleet side of station agreements
// (agree.agreement.manage; the station side acts through the station
// surface's live grant scope).
func AgreementPolicy(write bool) access.Policy {
	effect := access.Read
	if write {
		effect = access.OperationalWrite
	}
	return access.Policy{Permission: permissions.Permission("agree.agreement.manage"), Capability: "organizations", Effect: effect, Fresh: write}
}

// AuthorizationPolicy governs bounded fueling authorizations
// (agree.authorization.issue).
func AuthorizationPolicy(write bool) access.Policy {
	effect := access.Read
	if write {
		effect = access.OperationalWrite
	}
	return access.Policy{Permission: permissions.Permission("agree.authorization.issue"), Capability: "organizations", Effect: effect, Fresh: write}
}

// TermInput is one negotiated price term of a proposal.
type TermInput struct {
	Product       string `json:"fuel_product"`
	Unit          string `json:"unit"`
	PriceMilliBRL int64  `json:"price_milli_brl"`
	Conditions    string `json:"conditions"`
}

// ProposeInput proposes one bilateral agreement with its terms.
type ProposeInput struct {
	OperationID string
	StationID   string
	OrgLabel    string
	ValidFrom   time.Time
	ValidUntil  time.Time
	Conditions  string
	Terms       []TermInput
}

// AgreementTransitionInput applies one recorded lifecycle command.
type AgreementTransitionInput struct {
	OperationID string
	AgreementID string
	Revision    int64
	Reason      string
}

// StationDecisionInput is the station party's explicit accept or decline.
type StationDecisionInput struct {
	OperationID string
	AgreementID string
	Reason      string
}

// IssueInput issues one bounded single-use voucher.
type IssueInput struct {
	OperationID      string
	AgreementID      string
	EmployeeID       string
	AssetID          string
	Product          string
	Unit             string
	MaxQuantityMilli int64
	MaxCentavos      int64
	ValidUntil       time.Time
}

// CancelInput cancels one issued voucher.
type CancelInput struct {
	OperationID     string
	AuthorizationID string
	Reason          string
}

// ConfirmInput is the station's once-only redemption confirmation.
type ConfirmInput struct {
	OperationID   string
	Code          string
	Product       string
	Unit          string
	QuantityMilli int64
}

// AgreementView is the organization-side agreement projection.
type AgreementView struct {
	ID           string     `json:"id"`
	StationID    string     `json:"station_id"`
	OrgLabel     string     `json:"org_label"`
	State        string     `json:"state"`
	DerivedState string     `json:"derived_state"`
	Revision     int64      `json:"revision"`
	ValidFrom    string     `json:"valid_from"`
	ValidUntil   string     `json:"valid_until"`
	Conditions   string     `json:"conditions"`
	Terms        []TermView `json:"terms"`
	Usable       bool       `json:"usable"`
}

// TermView is one negotiated price term of the read projection.
type TermView struct {
	Product       string `json:"fuel_product"`
	Unit          string `json:"unit"`
	PriceMilliBRL int64  `json:"price_milli_brl"`
	Conditions    string `json:"conditions"`
}

// StationAgreementView is the station-side allowlisted projection: the
// organization label, state, validity and this agreement's terms only
// (BR-AGREE-001; no fleet, employee, document or location fields).
type StationAgreementView struct {
	ID           string     `json:"id"`
	OrgLabel     string     `json:"org_label"`
	State        string     `json:"state"`
	DerivedState string     `json:"derived_state"`
	ValidFrom    string     `json:"valid_from"`
	ValidUntil   string     `json:"valid_until"`
	Conditions   string     `json:"conditions"`
	Terms        []TermView `json:"terms"`
}

// AuthorizationView is the organization-side voucher projection. The
// one-time code is never part of it.
type AuthorizationView struct {
	ID               string `json:"id"`
	AgreementID      string `json:"agreement_id"`
	StationID        string `json:"station_id"`
	EmployeeID       string `json:"employee_id"`
	AssetID          string `json:"asset_id,omitempty"`
	Product          string `json:"fuel_product"`
	Unit             string `json:"unit"`
	MaxQuantityMilli int64  `json:"max_quantity_milli"`
	MaxCentavos      int64  `json:"max_centavos"`
	ValidUntil       string `json:"valid_until"`
	State            string `json:"state"`
	DerivedState     string `json:"derived_state"`
	FailedAttempts   int    `json:"failed_attempts"`
}

// RedemptionView is the settlement projection both permitted sides read.
type RedemptionView struct {
	ID              string `json:"id"`
	AgreementID     string `json:"agreement_id"`
	AuthorizationID string `json:"authorization_id"`
	StationID       string `json:"station_id"`
	EmployeeID      string `json:"employee_id"`
	AssetID         string `json:"asset_id,omitempty"`
	Product         string `json:"fuel_product"`
	Unit            string `json:"unit"`
	QuantityMilli   int64  `json:"quantity_milli"`
	PriceMilliBRL   int64  `json:"price_milli_brl"`
	TotalCentavos   int64  `json:"total_centavos"`
	ConfirmedAt     string `json:"confirmed_at"`
}

// HashRequest digests one command request for the idempotent receipt.
func HashRequest(in any) (string, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return "", ErrUnavailable
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// IssueAcknowledgement is the one-time issuance answer carrying the code
// exactly once; reads never repeat it and logs never carry it.
type IssueAcknowledgement struct {
	Authorization AuthorizationView `json:"authorization"`
	Code          string            `json:"code"`
}

// StationAuthority proves the station surface's live grant scope for one
// action at the station: the typed port the station session adapters own.
type StationAuthority interface {
	StationGrantTx(ctx context.Context, stationID, accountID, scope string) (StationGrant, error)
}

// StationGrant is the proven station-side capability.
type StationGrant struct {
	GrantID      string `json:"grant_id"`
	OperatorCNPJ string `json:"operator_cnpj"`
	Scopes       []string
}

// Repository is the transactional persistence port. Every method runs
// inside the caller's organization scope; station-side methods resolve
// the organization from the agreement or voucher and prove the station
// grant through the StationAuthority port.
type Repository interface {
	ProposeAgreement(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in ProposeInput, agreementID string, digest string) (AgreementView, bool, error)
	ReadAgreement(ctx context.Context, scope kernel.OrgScope, actor Actor, member, agreementID string) (AgreementView, error)
	ListAgreements(ctx context.Context, scope kernel.OrgScope, actor Actor, member string) ([]AgreementView, error)
	ApplyAgreementTransition(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in AgreementTransitionInput, kind string) (AgreementView, bool, error)
	IssueAuthorization(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in IssueInput, authorizationID, code, digest string) (IssueAcknowledgement, bool, error)
	ReadAuthorization(ctx context.Context, scope kernel.OrgScope, actor Actor, member, authorizationID string) (AuthorizationView, error)
	ListAuthorizations(ctx context.Context, scope kernel.OrgScope, actor Actor, member string) ([]AuthorizationView, error)
	CancelAuthorization(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in CancelInput) (AuthorizationView, bool, error)
	ListRedemptions(ctx context.Context, scope kernel.OrgScope, actor Actor, member string) ([]RedemptionView, error)
	StationAgreements(ctx context.Context, stationID string, actor StationActor, authority StationAuthority) ([]StationAgreementView, error)
	StationAccept(ctx context.Context, stationID string, actor StationActor, authority StationAuthority, in StationDecisionInput) (StationAgreementView, bool, error)
	StationDecline(ctx context.Context, stationID string, actor StationActor, authority StationAuthority, in StationDecisionInput) (StationAgreementView, bool, error)
	StationConfirmRedemption(ctx context.Context, stationID string, actor StationActor, authority StationAuthority, in ConfirmInput, redemptionID string) (RedemptionView, bool, error)
}

// Service is the bounded Phase F7 application.
type Service struct {
	Enabled    bool
	Repository Repository
	Now        func() time.Time
	NewID      func() (string, error)
	// NewCode mints the one-time voucher code; only its digest is stored.
	NewCode func() (string, error)
}

func (s Service) ready() bool {
	return s.Enabled && s.Repository != nil && s.Now != nil && s.NewID != nil && s.NewCode != nil
}

func (s Service) now() time.Time { return s.Now().UTC() }

// Propose validates the pure proposal and records it once.
func (s Service) Propose(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in ProposeInput) (AgreementView, bool, error) {
	if !s.ready() || scope.Validate() != nil || member == "" {
		return AgreementView{}, false, ErrUnavailable
	}
	if in.OperationID == "" || !canonicalID(in.StationID) || len(in.Terms) == 0 {
		return AgreementView{}, false, ErrInvalid
	}
	terms := make([]d.Term, 0, len(in.Terms))
	for _, term := range in.Terms {
		terms = append(terms, d.Term{Product: term.Product, Unit: term.Unit, PriceMilliBRL: term.PriceMilliBRL, Conditions: term.Conditions})
	}
	head, err := d.Propose(in.ValidFrom, in.ValidUntil, terms)
	if err != nil {
		return AgreementView{}, false, ErrInvalid
	}
	_ = head
	digest, err := HashRequest(in)
	if err != nil {
		return AgreementView{}, false, err
	}
	id, err := s.NewID()
	if err != nil {
		return AgreementView{}, false, ErrUnavailable
	}
	return s.Repository.ProposeAgreement(ctx, scope, actor, member, in, id, digest)
}

// Apply executes one recorded lifecycle command (SUSPEND or END).
func (s Service) Apply(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in AgreementTransitionInput, kind string) (AgreementView, bool, error) {
	if !s.ready() || scope.Validate() != nil || member == "" {
		return AgreementView{}, false, ErrUnavailable
	}
	if in.OperationID == "" || !canonicalID(in.AgreementID) || in.Revision < 1 {
		return AgreementView{}, false, ErrInvalid
	}
	return s.Repository.ApplyAgreementTransition(ctx, scope, actor, member, in, kind)
}

// Issue validates the bounded voucher and issues it once; the code is
// answered exactly once.
func (s Service) Issue(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in IssueInput) (IssueAcknowledgement, bool, error) {
	if !s.ready() || scope.Validate() != nil || member == "" {
		return IssueAcknowledgement{}, false, ErrUnavailable
	}
	if in.OperationID == "" || !canonicalID(in.AgreementID) || !canonicalID(in.EmployeeID) || (in.AssetID != "" && !canonicalID(in.AssetID)) {
		return IssueAcknowledgement{}, false, ErrInvalid
	}
	if _, err := d.Issue(in.Product, in.Unit, in.MaxQuantityMilli, in.MaxCentavos, in.ValidUntil, s.now()); err != nil {
		return IssueAcknowledgement{}, false, ErrInvalid
	}
	digest, err := HashRequest(in)
	if err != nil {
		return IssueAcknowledgement{}, false, err
	}
	id, err := s.NewID()
	if err != nil {
		return IssueAcknowledgement{}, false, ErrUnavailable
	}
	code, err := s.NewCode()
	if err != nil {
		return IssueAcknowledgement{}, false, ErrUnavailable
	}
	return s.Repository.IssueAuthorization(ctx, scope, actor, member, in, id, code, digest)
}

// Cancel cancels one issued voucher.
func (s Service) Cancel(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in CancelInput) (AuthorizationView, bool, error) {
	if !s.ready() || scope.Validate() != nil || member == "" {
		return AuthorizationView{}, false, ErrUnavailable
	}
	if in.OperationID == "" || !canonicalID(in.AuthorizationID) {
		return AuthorizationView{}, false, ErrInvalid
	}
	return s.Repository.CancelAuthorization(ctx, scope, actor, member, in)
}

// ReadAgreement reads one agreement with its terms.
func (s Service) ReadAgreement(ctx context.Context, scope kernel.OrgScope, actor Actor, member, agreementID string) (AgreementView, error) {
	if !s.ready() || scope.Validate() != nil || member == "" {
		return AgreementView{}, ErrUnavailable
	}
	if !canonicalID(agreementID) {
		return AgreementView{}, ErrInvalid
	}
	return s.Repository.ReadAgreement(ctx, scope, actor, member, agreementID)
}

// ListAgreements lists the organization's agreements.
func (s Service) ListAgreements(ctx context.Context, scope kernel.OrgScope, actor Actor, member string) ([]AgreementView, error) {
	if !s.ready() || scope.Validate() != nil || member == "" {
		return nil, ErrUnavailable
	}
	return s.Repository.ListAgreements(ctx, scope, actor, member)
}

// ReadAuthorization reads one voucher without the code.
func (s Service) ReadAuthorization(ctx context.Context, scope kernel.OrgScope, actor Actor, member, authorizationID string) (AuthorizationView, error) {
	if !s.ready() || scope.Validate() != nil || member == "" {
		return AuthorizationView{}, ErrUnavailable
	}
	if !canonicalID(authorizationID) {
		return AuthorizationView{}, ErrInvalid
	}
	return s.Repository.ReadAuthorization(ctx, scope, actor, member, authorizationID)
}

// ListAuthorizations lists the organization's vouchers.
func (s Service) ListAuthorizations(ctx context.Context, scope kernel.OrgScope, actor Actor, member string) ([]AuthorizationView, error) {
	if !s.ready() || scope.Validate() != nil || member == "" {
		return nil, ErrUnavailable
	}
	return s.Repository.ListAuthorizations(ctx, scope, actor, member)
}

// ListRedemptions lists the station-confirmed settlements.
func (s Service) ListRedemptions(ctx context.Context, scope kernel.OrgScope, actor Actor, member string) ([]RedemptionView, error) {
	if !s.ready() || scope.Validate() != nil || member == "" {
		return nil, ErrUnavailable
	}
	return s.Repository.ListRedemptions(ctx, scope, actor, member)
}

// StationAgreements lists the station's agreements across organizations
// with only the allowlisted fields.
func (s Service) StationAgreements(ctx context.Context, stationID string, actor StationActor, authority StationAuthority) ([]StationAgreementView, error) {
	if !s.ready() || !canonicalID(stationID) {
		return nil, ErrUnavailable
	}
	return s.Repository.StationAgreements(ctx, stationID, actor, authority)
}

// StationAccept is the station party's explicit acceptance. A repeated
// command is refused by the recorded state machine and every attempt stays
// audited, so no client operation identifier is accepted.
func (s Service) StationAccept(ctx context.Context, stationID string, actor StationActor, authority StationAuthority, in StationDecisionInput) (StationAgreementView, bool, error) {
	if !s.ready() || !canonicalID(stationID) || !canonicalID(in.AgreementID) {
		return StationAgreementView{}, false, ErrInvalid
	}
	return s.Repository.StationAccept(ctx, stationID, actor, authority, in)
}

// StationDecline is the station party's explicit decline.
func (s Service) StationDecline(ctx context.Context, stationID string, actor StationActor, authority StationAuthority, in StationDecisionInput) (StationAgreementView, bool, error) {
	if !s.ready() || !canonicalID(stationID) || !canonicalID(in.AgreementID) {
		return StationAgreementView{}, false, ErrInvalid
	}
	return s.Repository.StationDecline(ctx, stationID, actor, authority, in)
}

// StationConfirmRedemption validates and consumes the voucher once under
// the agreement's negotiated price; a matching retry answers the stored
// receipt (BR-AGREE-002).
func (s Service) StationConfirmRedemption(ctx context.Context, stationID string, actor StationActor, authority StationAuthority, in ConfirmInput) (RedemptionView, bool, error) {
	if !s.ready() || !canonicalID(stationID) {
		return RedemptionView{}, false, ErrUnavailable
	}
	if in.OperationID == "" || in.Code == "" || len(in.Code) > 64 || in.QuantityMilli < 1 {
		return RedemptionView{}, false, ErrInvalid
	}
	id, err := s.NewID()
	if err != nil {
		return RedemptionView{}, false, ErrUnavailable
	}
	return s.Repository.StationConfirmRedemption(ctx, stationID, actor, authority, in, id)
}
