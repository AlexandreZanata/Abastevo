package http

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"time"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const OwnershipTransferPrefix = "/v1/orgs/{org_id}/ownership-transfer"
const OwnershipTransferProposePath = OwnershipTransferPrefix + "/propose"
const OwnershipTransferAcceptPath = OwnershipTransferPrefix + "/accept"
const OwnershipTransferCancelPath = OwnershipTransferPrefix + "/cancel"
const OwnershipTransferReadPath = OwnershipTransferPrefix + "/read"

type ownershipTransferProposeRequest struct {
	companyRequest
	SchemaVersion      int    `json:"schema_version"`
	TargetMembershipID string `json:"target_membership_id"`
	StepDownToAdmin    bool   `json:"step_down_to_admin"`
	OperationID        string `json:"operation_id"`
}

func (v ownershipTransferProposeRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}

func (v *ownershipTransferProposeRequest) UnmarshalJSON(raw []byte) error {
	type plain ownershipTransferProposeRequest
	var input plain
	if e := decodeLegal(raw, &input); e != nil {
		return e
	}
	*v = ownershipTransferProposeRequest(input)
	return nil
}

type ownershipTransferAcceptRequest struct {
	TransferID  string `json:"transfer_id"`
	OperationID string `json:"operation_id"`
}

type ownershipTransferCancelRequest struct {
	TransferID  string `json:"transfer_id"`
	OperationID string `json:"operation_id"`
}

type ownershipTransferReadRequest struct {
	companyRequest
}

func (v *ownershipTransferReadRequest) UnmarshalJSON(raw []byte) error {
	type plain ownershipTransferReadRequest
	var input plain
	if e := decodeLegal(raw, &input); e != nil {
		return e
	}
	*v = ownershipTransferReadRequest(input)
	return nil
}

type OwnershipTransferRecords struct{}

func (OwnershipTransferRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.OrgID != scope.OrgID() || len(rules) == 0 {
		return access.ErrUnavailable
	}
	return nil
}

func ownershipTransferRule(write bool) orgapi.Rule {
	return orgapi.Rule{
		Permission:      "org.ownership.transfer",
		Write:           write,
		Fresh:           true,
		RequireRevision: false,
		CompanyOnly:     true,
		Success:         200,
	}
}

func ownershipTransferMapped(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, d.ErrOwnerRequiresPersonalAccount) {
		return orgapi.OwnerRequiresPersonal
	}
	if errors.Is(err, d.ErrTransferPending) {
		return orgapi.TransferPending
	}
	if errors.Is(err, d.ErrSelfTransferForbidden) {
		return orgapi.Conflict
	}
	if errors.Is(err, d.ErrTargetAlreadyOwner) {
		return orgapi.Conflict
	}
	if errors.Is(err, d.ErrTargetNotActive) || errors.Is(err, d.ErrTargetEmployeeNotActive) {
		return orgapi.Conflict
	}
	if errors.Is(err, d.ErrTransferNotFound) {
		return orgapi.NotFound
	}
	if errors.Is(err, d.ErrTransferNotPending) || errors.Is(err, d.ErrTransferExpired) {
		return orgapi.Conflict
	}
	if errors.Is(err, d.ErrNotTransferTarget) || errors.Is(err, d.ErrNotTransferParticipant) {
		return orgapi.Forbidden
	}
	if errors.Is(err, d.ErrTransferOperationReused) {
		return orgapi.OperationReused
	}
	if errors.Is(err, d.ErrOrganizationNotActive) {
		return orgapi.OrgSuspended
	}
	return legalMapped(err, "")
}

func ownershipTransferView(t d.OwnershipTransfer) map[string]any {
	view := map[string]any{
		"transfer_id":             t.TransferID,
		"org_id":                  t.OrgID,
		"initiator_membership_id": t.InitiatorMembershipID,
		"target_membership_id":    t.TargetMembershipID,
		"step_down_to_admin":      t.StepDownToAdmin,
		"state":                   string(t.State),
		"expires_at":              t.ExpiresAt.UTC().Format(time.RFC3339),
		"created_at":              t.CreatedAt.UTC().Format(time.RFC3339),
		"operation_id":            t.OperationID,
		"data_mode":               "SYNTHETIC",
	}
	if t.AcceptedAt != nil {
		view["accepted_at"] = t.AcceptedAt.UTC().Format(time.RFC3339)
	}
	if t.CancelledAt != nil {
		view["cancelled_at"] = t.CancelledAt.UTC().Format(time.RFC3339)
	}
	if t.DeclinedAt != nil {
		view["declined_at"] = t.DeclinedAt.UTC().Format(time.RFC3339)
	}
	return view
}

func RegisterOwnershipTransfer(
	router chi.Router,
	service app.OwnershipTransferService,
	cookies company.Handler,
	gateway orgapi.Gateway,
	authorization access.Authorization,
	roots app.Repository,
	memberships *memberpg.Store,
) error {
	if !service.Enabled {
		return nil
	}
	if service.Repository == nil || roots == nil || memberships == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := companyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}

	// 1. Propose transfer (guarded by org.ownership.transfer, fresh)
	if err := guarded.Register(
		owned,
		OwnershipTransferProposePath,
		gateway,
		ownershipTransferRule(true),
		authorization,
		app.OwnershipTransferProposePolicy(),
		RootFacts{roots},
		func(ctx context.Context, p orgapi.Principal, decision access.Decision, input ownershipTransferProposeRequest) (any, error) {
			if input.SchemaVersion != 1 || !canonical(input.OperationID) || !canonical(input.TargetMembershipID) {
				return nil, orgapi.Malformed
			}
			initiatorMemberID := decision.Subject().MembershipID()
			result, err := service.Propose(ctx, p.Scope, app.ProposeTransferInput{
				OrgID:             p.Scope.OrgID().String(),
				InitiatorMemberID: initiatorMemberID,
				TargetMemberID:    input.TargetMembershipID,
				StepDownToAdmin:   input.StepDownToAdmin,
				OperationID:       input.OperationID,
			})
			if err != nil {
				return nil, ownershipTransferMapped(err)
			}
			return map[string]any{
				"transfer": ownershipTransferView(result.Transfer),
				"replayed": result.Replayed,
			}, nil
		},
	); err != nil {
		return err
	}

	// 2. Read transfer (guarded by org.ownership.transfer)
	if err := guarded.Register(
		owned,
		OwnershipTransferReadPath,
		gateway,
		ownershipTransferRule(false),
		authorization,
		app.OwnershipTransferReadPolicy(),
		RootFacts{roots},
		func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ ownershipTransferReadRequest) (any, error) {
			tr, err := service.GetCurrent(ctx, p.Scope)
			if err != nil {
				return nil, ownershipTransferMapped(err)
			}
			return ownershipTransferView(tr), nil
		},
	); err != nil {
		return err
	}

	// 3. Accept transfer (by target member with fresh session)
	owned.Method(http.MethodPost, OwnershipTransferAcceptPath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Del("ETag")

		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 || r.URL.RawQuery != "" || r.URL.ForceQuery {
			orgapi.WriteError(w, r, orgapi.Malformed)
			return
		}

		sessionRow, err := cookies.ConfirmSession(r, true)
		if err != nil {
			orgapi.WriteError(w, r, orgapi.Unauthenticated)
			return
		}

		orgIDStr := chi.URLParam(r, "org_id")
		oid, err := kernel.ParseOrgID(orgIDStr)
		if err != nil {
			orgapi.WriteError(w, r, orgapi.NotFound)
			return
		}
		scope, err := kernel.NewOrgScope(oid)
		if err != nil {
			orgapi.WriteError(w, r, orgapi.NotFound)
			return
		}

		raw, err := httpapi.ReadBody(r, company.MaxBodyBytes)
		if err != nil {
			orgapi.WriteError(w, r, orgapi.Malformed)
			return
		}
		var input ownershipTransferAcceptRequest
		if httpapi.DecodeJSON(raw, &input) != nil || !canonical(input.TransferID) || !canonical(input.OperationID) {
			orgapi.WriteError(w, r, orgapi.Malformed)
			return
		}

		mem, err := memberships.ActiveByAccount(r.Context(), scope, sessionRow.AccountID)
		if err != nil {
			orgapi.WriteError(w, r, orgapi.Forbidden)
			return
		}
		snap, err := mem.Snapshot(scope)
		if err != nil {
			orgapi.WriteError(w, r, orgapi.Forbidden)
			return
		}
		memberID := snap.ID

		result, err := service.Accept(r.Context(), scope, app.AcceptTransferInput{
			OrgID:          scope.OrgID().String(),
			TransferID:     input.TransferID,
			TargetMemberID: memberID,
			OperationID:    input.OperationID,
		})
		if err != nil {
			orgapi.WriteError(w, r, ownershipTransferMapped(err))
			return
		}

		body := map[string]any{
			"transfer": ownershipTransferView(result.Transfer),
			"replayed": result.Replayed,
		}
		rawResp, _ := json.Marshal(body)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(rawResp)
	}))

	// 4. Cancel / Decline transfer (by initiator or target member with fresh session)
	owned.Method(http.MethodPost, OwnershipTransferCancelPath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Del("ETag")

		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 || r.URL.RawQuery != "" || r.URL.ForceQuery {
			orgapi.WriteError(w, r, orgapi.Malformed)
			return
		}

		sessionRow, err := cookies.ConfirmSession(r, true)
		if err != nil {
			orgapi.WriteError(w, r, orgapi.Unauthenticated)
			return
		}

		orgIDStr := chi.URLParam(r, "org_id")
		oid, err := kernel.ParseOrgID(orgIDStr)
		if err != nil {
			orgapi.WriteError(w, r, orgapi.NotFound)
			return
		}
		scope, err := kernel.NewOrgScope(oid)
		if err != nil {
			orgapi.WriteError(w, r, orgapi.NotFound)
			return
		}

		raw, err := httpapi.ReadBody(r, company.MaxBodyBytes)
		if err != nil {
			orgapi.WriteError(w, r, orgapi.Malformed)
			return
		}
		var input ownershipTransferCancelRequest
		if httpapi.DecodeJSON(raw, &input) != nil || !canonical(input.TransferID) || !canonical(input.OperationID) {
			orgapi.WriteError(w, r, orgapi.Malformed)
			return
		}

		mem, err := memberships.ActiveByAccount(r.Context(), scope, sessionRow.AccountID)
		if err != nil {
			orgapi.WriteError(w, r, orgapi.Forbidden)
			return
		}
		snap, err := mem.Snapshot(scope)
		if err != nil {
			orgapi.WriteError(w, r, orgapi.Forbidden)
			return
		}
		memberID := snap.ID

		// Look up current transfer to determine whether caller is initiator or target
		currentTr, err := service.GetCurrent(r.Context(), scope)
		if err != nil {
			orgapi.WriteError(w, r, ownershipTransferMapped(err))
			return
		}

		var isInitiator bool
		if memberID == currentTr.InitiatorMembershipID {
			isInitiator = true
		} else if memberID == currentTr.TargetMembershipID {
			isInitiator = false
		} else {
			orgapi.WriteError(w, r, orgapi.Forbidden)
			return
		}

		result, err := service.Cancel(r.Context(), scope, app.CancelTransferInput{
			OrgID:         scope.OrgID().String(),
			TransferID:    input.TransferID,
			ActorMemberID: memberID,
			OperationID:   input.OperationID,
			IsInitiator:   isInitiator,
		})
		if err != nil {
			orgapi.WriteError(w, r, ownershipTransferMapped(err))
			return
		}

		body := map[string]any{
			"transfer": ownershipTransferView(result.Transfer),
			"replayed": result.Replayed,
		}
		rawResp, _ := json.Marshal(body)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(rawResp)
	}))

	return nil
}
