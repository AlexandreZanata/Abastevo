package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const LegalPrefix = "/v1/orgs/{org_id}/legal-entities"
const LegalCreatePath = LegalPrefix + "/create"
const LegalListPath = LegalPrefix + "/list"
const LegalReadPath = LegalPrefix + "/{legal_entity_id}/read"
const LegalIdentifierPath = LegalPrefix + "/{legal_entity_id}/identifier/read"
const LegalWithdrawPath = LegalPrefix + "/{legal_entity_id}/withdraw"
const EstablishmentCreatePath = LegalPrefix + "/{legal_entity_id}/establishments/create"
const EstablishmentListPath = LegalPrefix + "/{legal_entity_id}/establishments/list"
const EstablishmentChangePath = LegalPrefix + "/{legal_entity_id}/establishments/{establishment_id}/change"

type legalRequest struct{ privateRequest }

func (legalRequest) Credentials() orgapi.Session { return orgapi.Session{} }

type establishmentFields struct {
	privateRequest
	CNPJ             string              `json:"cnpj,omitempty"`
	Role             d.EstablishmentRole `json:"declared_role,omitempty"`
	TradeName        string              `json:"trade_name,omitempty"`
	MunicipalityCode string              `json:"municipality_code,omitempty"`
}

func (v establishmentFields) domain() d.EstablishmentInput {
	return d.EstablishmentInput{CNPJ: v.CNPJ, Role: v.Role, TradeName: v.TradeName, MunicipalityCode: v.MunicipalityCode}
}

type legalCreateRequest struct {
	legalRequest
	SchemaVersion int                  `json:"schema_version"`
	OperationID   string               `json:"operation_id"`
	Kind          d.LegalKind          `json:"kind"`
	LegalName     string               `json:"legal_name"`
	Identifier    string               `json:"identifier"`
	Purpose       d.LegalPurpose       `json:"purpose,omitempty"`
	Establishment *establishmentFields `json:"establishment,omitempty"`
}

func (v legalCreateRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}

type legalRevisionCommand struct {
	legalRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      *int64 `json:"revision"`
}

func (v legalRevisionCommand) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

type legalWithdrawRequest struct{ legalRevisionCommand }
type establishmentCreateRequest struct {
	legalRevisionCommand
	establishmentFields
}
type establishmentChangeRequest struct {
	legalRevisionCommand
	Role             d.EstablishmentRole `json:"declared_role,omitempty"`
	TradeName        string              `json:"trade_name,omitempty"`
	MunicipalityCode string              `json:"municipality_code,omitempty"`
}
type legalReadRequest struct{ legalRequest }
type legalListRequest struct {
	legalRequest
	Limit  int          `json:"limit,omitempty"`
	Cursor string       `json:"cursor,omitempty"`
	State  d.LegalState `json:"state,omitempty"`
}

func (v legalListRequest) PageFields() orgapi.Page {
	return orgapi.Page{Limit: v.Limit, Cursor: v.Cursor}
}

type establishmentListRequest struct {
	legalRequest
	orgapi.Page
}

func decodeLegal[T any](raw []byte, target *T) error {
	var fields map[string]any
	if json.Unmarshal(raw, &fields) != nil || fields == nil || !canonical(fields) {
		return orgapi.Malformed
	}
	if httpapi.DecodeJSON(raw, target) != nil {
		return orgapi.Malformed
	}
	return nil
}
func (v *legalCreateRequest) UnmarshalJSON(raw []byte) error {
	type plain legalCreateRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = legalCreateRequest(input)
	return nil
}
func (v *legalWithdrawRequest) UnmarshalJSON(raw []byte) error {
	type plain legalWithdrawRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = legalWithdrawRequest(input)
	return nil
}
func (v *establishmentCreateRequest) UnmarshalJSON(raw []byte) error {
	type plain establishmentCreateRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = establishmentCreateRequest(input)
	return nil
}
func (v *establishmentChangeRequest) UnmarshalJSON(raw []byte) error {
	type plain establishmentChangeRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = establishmentChangeRequest(input)
	return nil
}
func (v *legalReadRequest) UnmarshalJSON(raw []byte) error {
	type plain legalReadRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = legalReadRequest(input)
	return nil
}
func (v *legalListRequest) UnmarshalJSON(raw []byte) error {
	type plain legalListRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = legalListRequest(input)
	return nil
}
func (v *establishmentListRequest) UnmarshalJSON(raw []byte) error {
	type plain establishmentListRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = establishmentListRequest(input)
	return nil
}

type LegalRecords struct{}

func (LegalRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permission.ResourceFacts, rules []permission.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.OrgID != scope.OrgID() || facts.Validate(scope) != nil || len(rules) == 0 {
		return access.ErrUnavailable
	}
	if policy != app.LegalPolicy(false, false) && policy != app.LegalPolicy(false, true) && policy != app.LegalPolicy(true, true) {
		return access.ErrUnavailable
	}
	return nil
}

type legalResource struct {
	lookup        app.LegalLookup
	establishment bool
}

func legalPathID(ctx context.Context, key string) (string, error) {
	raw := chi.URLParamFromCtx(ctx, key)
	id, err := kernel.ParseOrgID(raw)
	if err != nil || id.String() != raw {
		return "", access.ErrNotFound
	}
	return raw, nil
}
func (r legalResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permission.ResourceFacts, error) {
	if r.lookup == nil {
		return permission.ResourceFacts{}, access.ErrUnavailable
	}
	entityID, err := legalPathID(ctx, "legal_entity_id")
	if err != nil {
		return permission.ResourceFacts{}, err
	}
	target, establishment := entityID, ""
	if r.establishment {
		establishment, err = legalPathID(ctx, "establishment_id")
		if err != nil {
			return permission.ResourceFacts{}, err
		}
		target = establishment
	}
	found, err := r.lookup.LegalTargetExists(ctx, scope, entityID, establishment)
	if err != nil {
		return permission.ResourceFacts{}, access.ErrUnavailable
	}
	if !found {
		return permission.ResourceFacts{}, access.ErrNotFound
	}

	return permission.ResourceFacts{OrgID: scope.OrgID(), ID: target}, nil
}
func legalRule(write, fresh, revision bool, status int) orgapi.Rule {
	return orgapi.Rule{Permission: "org.legal_entity.manage", Write: write, Fresh: fresh, RequireRevision: revision, CompanyOnly: true, Success: status}
}
func legalMapped(err error, field string) error {
	if errors.Is(err, d.ErrInvalidCPF) {
		return orgapi.IdentifierValidation(field, "INVALID_CPF")
	}
	if errors.Is(err, d.ErrInvalidCNPJ) {
		return orgapi.IdentifierValidation(field, "INVALID_CNPJ")
	}
	if errors.Is(err, app.ErrLegalDuplicate) {
		return orgapi.IdentifierValidation(field, "DUPLICATE")
	}
	if errors.Is(err, access.ErrNoMembership) {
		return orgapi.NotFound
	}
	if errors.Is(err, access.ErrOrgSuspended) {
		return orgapi.OrgSuspended
	}
	if errors.Is(err, access.ErrOrgArchived) {
		return orgapi.OrgArchived
	}
	return mapped(err)
}
func legalAck(value app.LegalResult, replay bool) map[string]any {
	result := map[string]any{"legal_entity_id": value.EntityID, "state": value.State, "revision": value.Revision, "data_mode": "SYNTHETIC", "replayed": replay}
	if value.EstablishmentID != "" {
		result["establishment_id"] = value.EstablishmentID
	}
	return result
}
func legalView(scope kernel.OrgScope, entity d.LegalEntity) (map[string]any, error) {
	value, err := entity.Snapshot(scope)
	if err != nil {
		return nil, orgapi.Unavailable
	}
	label := "Declarado pela empresa"
	if value.State == d.LegalRevoked {
		label = "Revogada"
	}
	return map[string]any{"id": value.ID, "kind": value.Kind, "legal_name": value.LegalName, "identifier": entity.MaskedIdentifier(scope), "purpose": value.Purpose, "state": value.State, "status_label": label, "revision": value.Revision, "data_mode": "SYNTHETIC"}, nil
}
func establishmentView(scope kernel.OrgScope, value d.Establishment) (map[string]any, error) {
	v, err := value.Snapshot(scope)
	if err != nil {
		return nil, orgapi.Unavailable
	}
	return map[string]any{"id": v.ID, "legal_entity_id": v.EntityID, "cnpj": v.CNPJ, "declared_role": v.Role, "trade_name": v.TradeName, "municipality_code": v.MunicipalityCode, "revision": v.Revision}, nil
}
func RegisterLegal(router chi.Router, service app.LegalService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, roots app.Repository, lookup app.LegalLookup) error {
	if !service.Enabled {
		return nil
	}
	if service.Repository == nil || service.NewID == nil || roots == nil || lookup == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := companyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	entityResource := legalResource{lookup: lookup}
	establishmentResource := legalResource{lookup: lookup, establishment: true}
	writePolicy, readPolicy, revealPolicy := app.LegalPolicy(true, true), app.LegalPolicy(false, false), app.LegalPolicy(false, true)
	if err := guarded.Register(owned, LegalCreatePath, gateway, legalRule(true, true, false, 201), authorization, writePolicy, RootFacts{roots}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input legalCreateRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		v := d.LegalDeclarationInput{Kind: input.Kind, LegalName: input.LegalName, Identifier: input.Identifier, Purpose: input.Purpose}
		if input.Kind == d.NaturalPerson && input.Establishment != nil {
			return nil, orgapi.ValidationFailed
		}
		if input.Establishment != nil {
			v.Establishment = input.Establishment.domain()
		}
		declaration, err := d.NewLegalDeclaration(input.OperationID, v)
		if err != nil {
			return nil, legalMapped(err, "identifier")
		}
		command, err := app.DeclareLegalCommand(declaration)
		if err != nil {
			return nil, legalMapped(err, "identifier")
		}
		result, replay, err := service.Write(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, legalMapped(err, "identifier")
		}
		return legalAck(result, replay), nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, LegalWithdrawPath, gateway, legalRule(true, true, true, 200), authorization, writePolicy, entityResource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input legalWithdrawRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := legalPathID(ctx, "legal_entity_id")
		if err != nil {
			return nil, orgapi.NotFound
		}
		command, err := app.WithdrawLegalCommand(id, input.OperationID, *input.Revision)
		if err != nil {
			return nil, legalMapped(err, "identifier")
		}
		result, replay, err := service.Write(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, legalMapped(err, "identifier")
		}
		return legalAck(result, replay), nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, EstablishmentCreatePath, gateway, legalRule(true, true, true, 201), authorization, writePolicy, entityResource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input establishmentCreateRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := legalPathID(ctx, "legal_entity_id")
		if err != nil {
			return nil, orgapi.NotFound
		}
		command, err := app.AddEstablishmentCommand(id, input.OperationID, *input.Revision, input.establishmentFields.domain())
		if err != nil {
			return nil, legalMapped(err, "cnpj")
		}
		result, replay, err := service.Write(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, legalMapped(err, "cnpj")
		}
		return legalAck(result, replay), nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, EstablishmentChangePath, gateway, legalRule(true, true, true, 200), authorization, writePolicy, establishmentResource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input establishmentChangeRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := legalPathID(ctx, "legal_entity_id")
		if err != nil {
			return nil, orgapi.NotFound
		}
		establishment, err := legalPathID(ctx, "establishment_id")
		if err != nil {
			return nil, orgapi.NotFound
		}
		command, err := app.ChangeEstablishmentCommand(id, establishment, input.OperationID, *input.Revision, d.EstablishmentMetadata{Role: input.Role, TradeName: input.TradeName, MunicipalityCode: input.MunicipalityCode})
		if err != nil {
			return nil, legalMapped(err, "cnpj")
		}
		result, replay, err := service.Write(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, legalMapped(err, "cnpj")
		}
		return legalAck(result, replay), nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, LegalReadPath, gateway, legalRule(false, false, false, 200), authorization, readPolicy, entityResource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ legalReadRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := legalPathID(ctx, "legal_entity_id")
		if err != nil {
			return nil, orgapi.NotFound
		}
		value, err := service.Read(ctx, p.Scope, actor, decision, id, false)
		if err != nil {
			return nil, legalMapped(err, "identifier")
		}
		return legalView(p.Scope, value.Entity)
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, LegalIdentifierPath, gateway, legalRule(false, true, false, 200), authorization, revealPolicy, entityResource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ legalReadRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := legalPathID(ctx, "legal_entity_id")
		if err != nil {
			return nil, orgapi.NotFound
		}
		value, err := service.Read(ctx, p.Scope, actor, decision, id, true)
		if err != nil {
			return nil, legalMapped(err, "identifier")
		}
		snapshot, err := value.Entity.Snapshot(p.Scope)
		if err != nil || !value.Revealed || value.AuditOperationID == "" {
			return nil, orgapi.Unavailable
		}
		return map[string]any{"legal_entity_id": snapshot.ID, "kind": snapshot.Kind, "identifier": snapshot.Identifier, "audit_operation_id": value.AuditOperationID, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, LegalListPath, gateway, legalRule(false, false, false, 200), authorization, readPolicy, RootFacts{roots}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input legalListRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		page, err := input.PageFields().Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		if input.State != "" && input.State != d.LegalDeclared && input.State != d.LegalRevoked {
			return nil, orgapi.Malformed
		}
		filters := fmt.Sprintf("legal.v1:%s:%d", input.State, page.Limit)
		after, err := gateway.OpenCursor(p, LegalListPath, filters, page.Cursor)
		if err != nil {
			return nil, orgapi.Malformed
		}
		values, err := service.List(ctx, p.Scope, actor, decision, app.LegalFilter{AfterID: after, Limit: page.Limit, State: input.State})
		if err != nil {
			return nil, legalMapped(err, "identifier")
		}
		result := make([]map[string]any, 0, len(values.Entities))
		for _, entity := range values.Entities {
			value, err := legalView(p.Scope, entity)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		next := ""
		if values.NextID != "" {
			next, err = gateway.SealCursor(p, LegalListPath, filters, values.NextID)
			if err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": result, "next_cursor": next}, nil
	}); err != nil {
		return err
	}
	return guarded.Register(owned, EstablishmentListPath, gateway, legalRule(false, false, false, 200), authorization, readPolicy, entityResource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input establishmentListRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := legalPathID(ctx, "legal_entity_id")
		if err != nil {
			return nil, orgapi.NotFound
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		filters := fmt.Sprintf("establishments.v1:%s:%d", id, page.Limit)
		after, err := gateway.OpenCursor(p, EstablishmentListPath, filters, page.Cursor)
		if err != nil {
			return nil, orgapi.Malformed
		}
		values, err := service.Establishments(ctx, p.Scope, actor, decision, id, app.LegalFilter{AfterID: after, Limit: page.Limit})
		if err != nil {
			return nil, legalMapped(err, "cnpj")
		}
		result := make([]map[string]any, 0, len(values.Establishments))
		for _, establishment := range values.Establishments {
			value, err := establishmentView(p.Scope, establishment)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		next := ""
		if values.NextID != "" {
			next, err = gateway.SealCursor(p, EstablishmentListPath, filters, values.NextID)
			if err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": result, "next_cursor": next}, nil
	})
}
