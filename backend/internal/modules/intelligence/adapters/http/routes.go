// Package http exposes guarded HTTP POST endpoints for Phase F6 management,
// internal fuel stock, preventive/corrective maintenance, KPIs, findings,
// audit viewer, and consolidation governance.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	intelpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/intelligence/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/intelligence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const (
	StockTanksListPath           = "/v1/orgs/{org_id}/stock/tanks/list"
	StockTanksSavePath           = "/v1/orgs/{org_id}/stock/tanks/save"
	StockMovementsListPath       = "/v1/orgs/{org_id}/stock/movements/list"
	StockMovementsRecordPath     = "/v1/orgs/{org_id}/stock/movements/record"
	MaintenancePlansListPath     = "/v1/orgs/{org_id}/maintenance/plans/list"
	MaintenancePlansSavePath     = "/v1/orgs/{org_id}/maintenance/plans/save"
	AssetDocumentsSavePath       = "/v1/orgs/{org_id}/fleet/assets/{asset_id}/documents/save"
	WorkOrdersListPath           = "/v1/orgs/{org_id}/maintenance/work-orders/list"
	WorkOrdersSavePath           = "/v1/orgs/{org_id}/maintenance/work-orders/save"
	WorkOrdersReviewListPath     = "/v1/orgs/{org_id}/maintenance/work-orders/review/list"
	WorkOrdersReviewDecidePath   = "/v1/orgs/{org_id}/maintenance/work-orders/review/decide"
	MaintenanceRequestsMinePath  = "/v1/orgs/{org_id}/maintenance/requests/mine"
	IntelligenceKPIsOverviewPath = "/v1/orgs/{org_id}/intelligence/kpis/overview"
	IntelligenceFindingsListPath = "/v1/orgs/{org_id}/intelligence/findings/list"
	IntelligenceFindingsRevPath  = "/v1/orgs/{org_id}/intelligence/findings/review/list"
	IntelligenceFindingsDecide   = "/v1/orgs/{org_id}/intelligence/findings/decide"
	IntelligenceSettingsSavePath = "/v1/orgs/{org_id}/intelligence/settings/save"
	AuditEventsListPath          = "/v1/orgs/{org_id}/audit/events/list"
	ConsolidationGrantsListPath  = "/v1/orgs/{org_id}/consolidation/grants/list"
	ConsolidationGrantsSavePath  = "/v1/orgs/{org_id}/consolidation/grants/save"
)

// ManagementRecords validates fixed F6 policies and targets at the auth boundary.
type ManagementRecords struct{}

func (ManagementRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 {
		return access.ErrUnavailable
	}
	own := facts.ID == subject.MembershipID() && facts.AssignedTo == subject.MembershipID()
	switch policy {
	case app.MaintenanceRequestPolicy():
		if own {
			return nil
		}
		return access.ErrUnavailable
	case app.TankPolicy(false), app.TankPolicy(true),
		app.MovementPolicy(false), app.MovementPolicy(true),
		app.PlanPolicy(false), app.PlanPolicy(true),
		app.WorkOrderPolicy(false), app.WorkOrderPolicy(true),
		app.WorkOrderReviewPolicy(false), app.WorkOrderReviewPolicy(true),
		app.AssetDocumentManagePolicy(),
		app.KpiPolicy(),
		app.FindingReadPolicy(),
		app.FindingResolvePolicy(false), app.FindingResolvePolicy(true),
		app.SettingsManagePolicy(),
		app.AuditReadPolicy(),
		app.GroupManagePolicy(false), app.GroupManagePolicy(true):
		if !own {
			return nil
		}
	}
	return access.ErrUnavailable
}

type orgRootResource struct{}

func (orgRootResource) Resolve(_ context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, nil
}

type assetResource struct{}

func (assetResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	raw := chi.URLParamFromCtx(ctx, "asset_id")
	id, err := kernel.ParseOrgID(raw)
	if err != nil || id.String() != raw {
		return permissions.ResourceFacts{}, access.ErrNotFound
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: raw}, nil
}

type ownRequestResource struct{}

func (ownRequestResource) Resolve(_ context.Context, scope kernel.OrgScope, subject access.Subject) (permissions.ResourceFacts, error) {
	member := subject.MembershipID()
	id, err := kernel.ParseOrgID(member)
	if scope.Validate() != nil || err != nil || id.String() != member {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: member, AssignedTo: member}, nil
}

func missing(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan:
		return rv.IsNil()
	}
	return false
}

func strictObject[T any](raw []byte, target *T) error {
	if httpapi.DecodeJSON(raw, target) != nil {
		return orgapi.Malformed
	}
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if decoder.Decode(&fields) != nil || fields == nil {
		return orgapi.Malformed
	}
	return nil
}

func mappedError(err error) error {
	if code, ok := orgapi.Registered(err); ok {
		return code
	}
	switch {
	case errors.Is(err, app.ErrUnauthenticated):
		return orgapi.Unauthenticated
	case errors.Is(err, app.ErrNotFound), errors.Is(err, permissions.ErrResourceNotFound):
		return orgapi.NotFound
	case errors.Is(err, app.ErrOperationReused):
		return orgapi.OperationReused
	case errors.Is(err, app.ErrConflict):
		return orgapi.Conflict
	case errors.Is(err, app.ErrInsufficientStock):
		return orgapi.StockInsufficient
	case errors.Is(err, app.ErrProductMismatch):
		return orgapi.StockProductMismatch
	case errors.Is(err, intelpg.ErrMeterRequired):
		return orgapi.MeterRequired
	case errors.Is(err, intelpg.ErrQuoteRequired):
		return orgapi.QuoteRequired
	case errors.Is(err, app.ErrQuoteExpired):
		return orgapi.QuoteExpired
	case errors.Is(err, app.ErrOverrunDenied):
		return orgapi.OverrunNotApproved
	case errors.Is(err, app.ErrHoldOpen):
		return orgapi.HoldOpen
	case errors.Is(err, app.ErrInvoiceRequired):
		return orgapi.InvoiceRequired
	case errors.Is(err, intelpg.ErrControlDisabled):
		return orgapi.ControlDisabled
	case errors.Is(err, app.ErrFindingClosed):
		return orgapi.FindingClosed
	case errors.Is(err, app.ErrSelfApproval), errors.Is(err, app.ErrFindingSelfResolve):
		return orgapi.SelfApproval
	case errors.Is(err, intelpg.ErrExportTooLarge):
		return orgapi.ExportTooLarge
	case errors.Is(err, app.ErrInvalid):
		return orgapi.ValidationFailed
	case errors.Is(err, access.ErrFreshAuthRequired):
		return orgapi.FreshAuthRequired
	case errors.Is(err, access.ErrEntitlementRequired):
		return orgapi.EntitlementRequired
	case errors.Is(err, access.ErrSetupIncomplete):
		return orgapi.OrgSetupIncomplete
	case errors.Is(err, permissions.ErrPermissionDenied):
		return orgapi.Forbidden
	}
	return orgapi.Unavailable
}

func managementActor(ctx context.Context) (app.Actor, error) {
	actor, err := organization.CurrentCompanyActor(ctx)
	if err != nil {
		return app.Actor{}, err
	}
	return app.NewActor(actor.AccountID, actor.SessionID)
}

func readRule(p access.Policy) orgapi.Rule {
	return orgapi.Rule{Permission: string(p.Permission), Write: false, Fresh: p.Fresh, CompanyOnly: true, Success: 200}
}

func writeRule(p access.Policy, requireRevision bool) orgapi.Rule {
	return orgapi.Rule{Permission: string(p.Permission), Write: true, Fresh: p.Fresh, RequireRevision: requireRevision, CompanyOnly: true, Success: 200}
}

// --- Request DTOs ---

type emptyReadRequest struct{}

func (emptyReadRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v *emptyReadRequest) UnmarshalJSON(raw []byte) error {
	type plain emptyReadRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = emptyReadRequest(in)
	return nil
}

type tankSaveRequest struct {
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      *int64 `json:"revision"`
	TankID        string `json:"tank_id"`
	UnitID        string `json:"unit_id"`
	Name          string `json:"name"`
	Product       string `json:"product"`
	Unit          string `json:"unit"`
	CapacityMilli int64  `json:"capacity_milli"`
	AllowNegative bool   `json:"allow_negative"`
	State         string `json:"state"`
}

func (tankSaveRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v tankSaveRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}
func (v *tankSaveRequest) UnmarshalJSON(raw []byte) error {
	type plain tankSaveRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = tankSaveRequest(in)
	return nil
}

type movementListRequest struct {
	TankID string `json:"tank_id"`
}

func (movementListRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v *movementListRequest) UnmarshalJSON(raw []byte) error {
	type plain movementListRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = movementListRequest(in)
	return nil
}

type movementRecordRequest struct {
	SchemaVersion     int    `json:"schema_version"`
	OperationID       string `json:"operation_id"`
	Revision          *int64 `json:"revision"`
	TankID            string `json:"tank_id"`
	Kind              string `json:"kind"`
	Product           string `json:"product"`
	Unit              string `json:"unit"`
	QuantityMilli     int64  `json:"quantity_milli"`
	DeltaMilli        int64  `json:"delta_milli"`
	AmountCentavos    int64  `json:"amount_centavos"`
	MeasuredMilli     int64  `json:"measured_milli"`
	AssetID           string `json:"asset_id"`
	CounterpartTankID string `json:"counterpart_tank_id"`
	SupplierID        string `json:"supplier_id"`
	DocumentID        string `json:"document_id"`
	FillKind          string `json:"fill"`
	ApplyAdjustment   bool   `json:"apply_adjustment"`
	Reason            string `json:"reason"`
	OccurredAt        string `json:"occurred_at"`
}

func (movementRecordRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v movementRecordRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}
func (v *movementRecordRequest) UnmarshalJSON(raw []byte) error {
	type plain movementRecordRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = movementRecordRequest(in)
	return nil
}

type planListRequest struct {
	AssetID string `json:"asset_id"`
}

func (planListRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v *planListRequest) UnmarshalJSON(raw []byte) error {
	type plain planListRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = planListRequest(in)
	return nil
}

type planSaveRequest struct {
	SchemaVersion      int      `json:"schema_version"`
	OperationID        string   `json:"operation_id"`
	Revision           *int64   `json:"revision"`
	Action             string   `json:"action"`
	PlanID             string   `json:"plan_id"`
	AssetID            string   `json:"asset_id"`
	Name               string   `json:"name"`
	DistanceIntervalM  *int64   `json:"distance_interval_m"`
	DistanceLeadM      *int64   `json:"distance_lead_m"`
	HourIntervalS      *int64   `json:"hour_interval_s"`
	HourLeadS          *int64   `json:"hour_lead_s"`
	TimeIntervalDays   *int     `json:"time_interval_days"`
	TimeIntervalMonths *int     `json:"time_interval_months"`
	TimeLeadDays       *int     `json:"time_lead_days"`
	BaselineDay        string   `json:"baseline_day"`
	BaselineOdometerM  *int64   `json:"baseline_odometer_m"`
	BaselineHourS      *int64   `json:"baseline_hour_s"`
	EvalDay            string   `json:"eval_day"`
	CurrentOdometerM   *int64   `json:"current_odometer_m"`
	CurrentHourS       *int64   `json:"current_hour_s"`
	WindowStartDay     string   `json:"window_start_day"`
	WindowStartOdoM    *int64   `json:"window_start_odom"`
	ReadingRevision    int64    `json:"reading_revision"`
	RebaseRule         string   `json:"rebase_rule"`
	Blocking           bool     `json:"blocking"`
	Reason             string   `json:"reason"`
	SubjectKind        string   `json:"subject_kind"`
	SubjectID          string   `json:"subject_id"`
	ItemKind           string   `json:"item_kind"`
	ExpiryDay          string   `json:"expiry_day"`
	LeadDays           int      `json:"lead_days"`
	DocumentID         string   `json:"document_id"`
	LicenceCategory    string   `json:"licence_category"`
	LicenceEnabled     *bool    `json:"licence_enabled"`
	PolicyID           string   `json:"policy_id"`
	PolicyNumber       string   `json:"policy_number"`
	InsurerName        string   `json:"insurer_name"`
	ValidFrom          string   `json:"valid_from"`
	AssetIDs           []string `json:"asset_ids"`
	IncidentReportID   string   `json:"incident_report_id"`
	ClaimState         string   `json:"claim_state"`
}

func (planSaveRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v planSaveRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}
func (v *planSaveRequest) UnmarshalJSON(raw []byte) error {
	type plain planSaveRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = planSaveRequest(in)
	return nil
}

type assetDocSaveRequest struct {
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      *int64 `json:"revision"`
	Action        string `json:"action"`
	LinkID        string `json:"link_id"`
	DocumentID    string `json:"document_id"`
	Kind          string `json:"kind"`
	ExpiryDay     string `json:"expiry_day"`
	IsCoverPhoto  bool   `json:"is_cover_photo"`
}

func (assetDocSaveRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v assetDocSaveRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}
func (v *assetDocSaveRequest) UnmarshalJSON(raw []byte) error {
	type plain assetDocSaveRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = assetDocSaveRequest(in)
	return nil
}

type workOrderSaveRequest struct {
	SchemaVersion      int                      `json:"schema_version"`
	OperationID        string                   `json:"operation_id"`
	Revision           *int64                   `json:"revision"`
	Action             string                   `json:"action"`
	WorkOrderID        string                   `json:"work_order_id"`
	AssetID            string                   `json:"asset_id"`
	Kind               string                   `json:"kind"`
	OriginKind         string                   `json:"origin_kind"`
	OriginID           string                   `json:"origin_id"`
	SupplierID         string                   `json:"supplier_id"`
	Description        string                   `json:"description"`
	SelectedQuoteCents *int64                   `json:"selected_quote_cents"`
	QuoteValidUntil    string                   `json:"quote_valid_until"`
	DirectLimitCents   *int64                   `json:"direct_limit_cents"`
	TolerancePercent   *int                     `json:"tolerance_percent"`
	InvoiceRequired    bool                     `json:"invoice_required"`
	InvoiceDocumentID  string                   `json:"invoice_document_id"`
	Lines              []app.WorkOrderLineInput `json:"lines"`
	DiscountCents      int64                    `json:"discount_cents"`
	PaidCents          *int64                   `json:"paid_cents"`
	ScheduledAt        string                   `json:"scheduled_at"`
	AfterTheFact       bool                     `json:"after_the_fact"`
	Reason             string                   `json:"reason"`
	ComponentID        string                   `json:"component_id"`
	ComponentKind      string                   `json:"component_kind"`
	SerialNumber       string                   `json:"serial_number"`
	Position           string                   `json:"position"`
	ComponentState     string                   `json:"component_state"`
	MeterValue         *int64                   `json:"meter_value"`
	PartCode           string                   `json:"part_code"`
	PartName           string                   `json:"part_name"`
	PartUnit           string                   `json:"part_unit"`
	PartDeltaMilli     int64                    `json:"part_delta_milli"`
	PartAmountCentavos int64                    `json:"part_amount_centavos"`
}

func (workOrderSaveRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v workOrderSaveRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}
func (v *workOrderSaveRequest) UnmarshalJSON(raw []byte) error {
	type plain workOrderSaveRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = workOrderSaveRequest(in)
	return nil
}

type workOrderReviewDecideRequest struct {
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      *int64 `json:"revision"`
	WorkOrderID   string `json:"work_order_id"`
	Decision      string `json:"decision"`
	ApprovedCents *int64 `json:"approved_cents"`
	Reason        string `json:"reason"`
}

func (workOrderReviewDecideRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v workOrderReviewDecideRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}
func (v *workOrderReviewDecideRequest) UnmarshalJSON(raw []byte) error {
	type plain workOrderReviewDecideRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = workOrderReviewDecideRequest(in)
	return nil
}

type kpiOverviewRequest struct {
	Dimension      string `json:"dimension"`
	DimensionID    string `json:"dimension_id"`
	PeriodStart    string `json:"period_start"`
	PeriodEnd      string `json:"period_end"`
	IncludePending bool   `json:"include_pending"`
}

func (kpiOverviewRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v *kpiOverviewRequest) UnmarshalJSON(raw []byte) error {
	type plain kpiOverviewRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = kpiOverviewRequest(in)
	return nil
}

type findingDecideRequest struct {
	SchemaVersion           int    `json:"schema_version"`
	OperationID             string `json:"operation_id"`
	Revision                *int64 `json:"revision"`
	Action                  string `json:"action"`
	FindingID               string `json:"finding_id"`
	Reason                  string `json:"reason"`
	FollowupAction          string `json:"followup_action"`
	PeriodStart             string `json:"period_start"`
	PeriodEnd               string `json:"period_end"`
	SyntheticSignalKind     string `json:"synthetic_signal_kind"`
	AssetID                 string `json:"asset_id"`
	SubjectKey              string `json:"subject_key"`
	AuthorID                string `json:"author_membership_id"`
	Severity                string `json:"severity"`
	BenchmarkSyntheticCount int    `json:"benchmark_synthetic_count"`
}

func (findingDecideRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v findingDecideRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}
func (v *findingDecideRequest) UnmarshalJSON(raw []byte) error {
	type plain findingDecideRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = findingDecideRequest(in)
	return nil
}

type findingSettingsSaveRequest struct {
	SchemaVersion           int      `json:"schema_version"`
	OperationID             string   `json:"operation_id"`
	Revision                *int64   `json:"revision"`
	DuplicateWindowMinutes  int      `json:"duplicate_window_minutes"`
	MaxSpeedKmh             int      `json:"max_speed_kmh"`
	TankTolerancePercent    int      `json:"tank_tolerance_percent"`
	OutlierThresholdPercent int      `json:"outlier_threshold_percent"`
	DocQtyTolerancePercent  int      `json:"doc_quantity_tolerance_percent"`
	DocDateToleranceDays    int      `json:"doc_date_tolerance_days"`
	PriceThresholdPercent   int      `json:"price_threshold_percent"`
	PriceMinDiffCents       int64    `json:"price_min_diff_cents"`
	ReferenceSource         string   `json:"reference_source"`
	EnabledTypes            []string `json:"enabled_types"`
}

func (findingSettingsSaveRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v findingSettingsSaveRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}
func (v *findingSettingsSaveRequest) UnmarshalJSON(raw []byte) error {
	type plain findingSettingsSaveRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = findingSettingsSaveRequest(in)
	return nil
}

type auditEventsListRequest struct {
	ActorID    string `json:"actor_id"`
	Action     string `json:"action"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Limit      *int   `json:"limit"`
}

func (auditEventsListRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v *auditEventsListRequest) UnmarshalJSON(raw []byte) error {
	type plain auditEventsListRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = auditEventsListRequest(in)
	return nil
}

type consolidationSaveRequest struct {
	SchemaVersion    int      `json:"schema_version"`
	OperationID      string   `json:"operation_id"`
	Revision         *int64   `json:"revision"`
	Action           string   `json:"action"`
	GrantID          string   `json:"grant_id"`
	CounterpartOrgID string   `json:"counterpart_org_id"`
	AllowedDatasets  []string `json:"allowed_datasets"`
	ProposalCode     string   `json:"proposal_code"`
	ValidUntil       string   `json:"valid_until"`
	AssetID          string   `json:"asset_id"`
	Kind             string   `json:"kind"`
	RecurrenceKey    string   `json:"recurrence_key"`
	PeriodMonth      string   `json:"period_month"`
	AmountCentavos   int64    `json:"amount_centavos"`
	OccurredAt       string   `json:"occurred_at"`
	SupplierID       string   `json:"supplier_id"`
	ReferenceCode    string   `json:"reference_code"`
	PayableIDs       []string `json:"payable_ids"`
	ReportKind       string   `json:"report_kind"`
	Cadence          string   `json:"cadence"`
	RecipientIDs     []string `json:"recipient_membership_ids"`
	Category         string   `json:"category"`
	TargetID         string   `json:"target_id"`
	RetentionDays    *int     `json:"retention_days"`
	HoldActive       bool     `json:"hold_active"`
	ExpiresAt        string   `json:"expires_at"`
	ResponsibleCtx   string   `json:"responsible_context"`
	RoleKey          string   `json:"role_key"`
	RoleName         string   `json:"role_name"`
	Permissions      []string `json:"permissions"`
	FuelingID        string   `json:"fueling_id"`
	StationID        string   `json:"station_id"`
	Product          string   `json:"product"`
	Unit             string   `json:"unit"`
	UnitPriceMilli   int64    `json:"unit_price_milli"`
	PriceCondition   string   `json:"price_condition"`
	Reason           string   `json:"reason"`
}

func (consolidationSaveRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v consolidationSaveRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}
func (v *consolidationSaveRequest) UnmarshalJSON(raw []byte) error {
	type plain consolidationSaveRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = consolidationSaveRequest(in)
	return nil
}

// RegisterManagement mounts all F6 guarded POST endpoints.
func RegisterManagement(router chi.Router, service app.Service, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if !service.Enabled {
		return nil
	}
	if router == nil || missing(service.Repository) || missing(authorization.Source) || missing(authorization.Capabilities) || missing(authorization.Records) || authorization.Now == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}

	if err := guarded.Register(owned, StockTanksListPath, gateway, readRule(app.TankPolicy(false)), authorization, app.TankPolicy(false), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ emptyReadRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		tanks, err := service.ListTanks(ctx, p.Scope, actor, p.MembershipID)
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"tanks": tanks, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, StockTanksSavePath, gateway, writeRule(app.TankPolicy(true), false), authorization, app.TankPolicy(true), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in tankSaveRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		view, replayed, err := service.SaveTank(ctx, p.Scope, actor, p.MembershipID, app.SaveTankInput{
			OperationID: in.OperationID, TankID: in.TankID, UnitID: in.UnitID, Name: in.Name,
			Product: in.Product, Unit: in.Unit, CapacityMilli: in.CapacityMilli,
			AllowNegative: in.AllowNegative, State: in.State, Revision: in.Revision,
		})
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"tank": view, "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, StockMovementsListPath, gateway, readRule(app.MovementPolicy(false)), authorization, app.MovementPolicy(false), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in movementListRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		movs, recs, err := service.ListMovements(ctx, p.Scope, actor, p.MembershipID, in.TankID)
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"movements": movs, "reconciliations": recs, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, StockMovementsRecordPath, gateway, writeRule(app.MovementPolicy(true), false), authorization, app.MovementPolicy(true), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in movementRecordRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		res, replayed, err := service.RecordMovement(ctx, p.Scope, actor, p.MembershipID, app.RecordMovementInput{
			OperationID: in.OperationID, TankID: in.TankID, Kind: in.Kind, Product: in.Product, Unit: in.Unit,
			QuantityMilli: in.QuantityMilli, DeltaMilli: in.DeltaMilli, AmountCentavos: in.AmountCentavos, MeasuredMilli: in.MeasuredMilli,
			AssetID: in.AssetID, CounterpartTankID: in.CounterpartTankID, SupplierID: in.SupplierID, DocumentID: in.DocumentID,
			FillKind: in.FillKind, ApplyAdjustment: in.ApplyAdjustment, Reason: in.Reason, OccurredAt: in.OccurredAt,
		})
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"result": res, "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, MaintenancePlansListPath, gateway, readRule(app.PlanPolicy(false)), authorization, app.PlanPolicy(false), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in planListRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		plans, comp, err := service.ListPlans(ctx, p.Scope, actor, p.MembershipID, in.AssetID)
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"plans": plans, "compliance": comp, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, MaintenancePlansSavePath, gateway, writeRule(app.PlanPolicy(true), false), authorization, app.PlanPolicy(true), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in planSaveRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		res, replayed, err := service.SavePlan(ctx, p.Scope, actor, p.MembershipID, app.SavePlanInput{
			OperationID: in.OperationID, Action: in.Action, PlanID: in.PlanID, AssetID: in.AssetID, Name: in.Name,
			DistanceIntervalM: in.DistanceIntervalM, DistanceLeadM: in.DistanceLeadM, HourIntervalS: in.HourIntervalS, HourLeadS: in.HourLeadS,
			TimeIntervalDays: in.TimeIntervalDays, TimeIntervalMonths: in.TimeIntervalMonths, TimeLeadDays: in.TimeLeadDays,
			BaselineDay: in.BaselineDay, BaselineOdometerM: in.BaselineOdometerM, BaselineHourS: in.BaselineHourS, EvalDay: in.EvalDay,
			CurrentOdometerM: in.CurrentOdometerM, CurrentHourS: in.CurrentHourS, WindowStartDay: in.WindowStartDay, WindowStartOdoM: in.WindowStartOdoM,
			ReadingRevision: in.ReadingRevision, RebaseRule: in.RebaseRule, Blocking: in.Blocking, Reason: in.Reason,
			SubjectKind: in.SubjectKind, SubjectID: in.SubjectID, ItemKind: in.ItemKind, ExpiryDay: in.ExpiryDay, LeadDays: in.LeadDays,
			DocumentID: in.DocumentID, LicenceCategory: in.LicenceCategory, LicenceEnabled: in.LicenceEnabled,
			PolicyID: in.PolicyID, PolicyNumber: in.PolicyNumber, InsurerName: in.InsurerName, ValidFrom: in.ValidFrom,
			AssetIDs: in.AssetIDs, IncidentReportID: in.IncidentReportID, ClaimState: in.ClaimState,
		})
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"result": res, "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, AssetDocumentsSavePath, gateway, writeRule(app.AssetDocumentManagePolicy(), false), authorization, app.AssetDocumentManagePolicy(), assetResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in assetDocSaveRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		view, replayed, err := service.SaveAssetDocument(ctx, p.Scope, actor, p.MembershipID, chi.URLParamFromCtx(ctx, "asset_id"), app.AssetDocumentInput{
			OperationID: in.OperationID, Action: in.Action, LinkID: in.LinkID, DocumentID: in.DocumentID,
			Kind: in.Kind, ExpiryDay: in.ExpiryDay, IsCoverPhoto: in.IsCoverPhoto,
		})
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"link": view, "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, WorkOrdersListPath, gateway, readRule(app.WorkOrderPolicy(false)), authorization, app.WorkOrderPolicy(false), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ emptyReadRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		items, err := service.ListWorkOrders(ctx, p.Scope, actor, p.MembershipID, false, false)
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"work_orders": items, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, WorkOrdersSavePath, gateway, writeRule(app.WorkOrderPolicy(true), false), authorization, app.WorkOrderPolicy(true), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in workOrderSaveRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		res, replayed, err := service.SaveWorkOrder(ctx, p.Scope, actor, p.MembershipID, app.SaveWorkOrderInput{
			OperationID: in.OperationID, Action: in.Action, WorkOrderID: in.WorkOrderID, AssetID: in.AssetID, Kind: in.Kind,
			OriginKind: in.OriginKind, OriginID: in.OriginID, SupplierID: in.SupplierID, Description: in.Description,
			SelectedQuoteCents: in.SelectedQuoteCents, QuoteValidUntil: in.QuoteValidUntil, DirectLimitCents: in.DirectLimitCents,
			TolerancePercent: in.TolerancePercent, InvoiceRequired: in.InvoiceRequired, InvoiceDocumentID: in.InvoiceDocumentID,
			Lines: in.Lines, DiscountCents: in.DiscountCents, PaidCents: in.PaidCents, ScheduledAt: in.ScheduledAt,
			AfterTheFact: in.AfterTheFact, Reason: in.Reason, Revision: in.Revision,
			ComponentID: in.ComponentID, ComponentKind: in.ComponentKind, SerialNumber: in.SerialNumber,
			Position: in.Position, ComponentState: in.ComponentState, MeterValue: in.MeterValue,
			PartCode: in.PartCode, PartName: in.PartName, PartUnit: in.PartUnit, PartDeltaMilli: in.PartDeltaMilli, PartAmountCentavos: in.PartAmountCentavos,
		})
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"result": res, "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, WorkOrdersReviewListPath, gateway, readRule(app.WorkOrderReviewPolicy(false)), authorization, app.WorkOrderReviewPolicy(false), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ emptyReadRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		items, err := service.ListWorkOrders(ctx, p.Scope, actor, p.MembershipID, true, false)
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"work_orders": items, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, WorkOrdersReviewDecidePath, gateway, writeRule(app.WorkOrderReviewPolicy(true), true), authorization, app.WorkOrderReviewPolicy(true), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in workOrderReviewDecideRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		view, replayed, err := service.ReviewWorkOrder(ctx, p.Scope, actor, p.MembershipID, app.ReviewWorkOrderInput{
			OperationID: in.OperationID, WorkOrderID: in.WorkOrderID, Decision: in.Decision,
			ApprovedCents: in.ApprovedCents, Reason: in.Reason, Revision: *in.Revision,
		})
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"work_order": view, "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, MaintenanceRequestsMinePath, gateway, readRule(app.MaintenanceRequestPolicy()), authorization, app.MaintenanceRequestPolicy(), ownRequestResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ emptyReadRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		items, err := service.ListWorkOrders(ctx, p.Scope, actor, p.MembershipID, false, true)
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"requests": items, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, IntelligenceKPIsOverviewPath, gateway, readRule(app.KpiPolicy()), authorization, app.KpiPolicy(), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in kpiOverviewRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		view, err := service.KPIOverview(ctx, p.Scope, actor, p.MembershipID, app.KPIOverviewQuery{
			Dimension: in.Dimension, DimensionID: in.DimensionID, PeriodStart: in.PeriodStart, PeriodEnd: in.PeriodEnd, IncludePending: in.IncludePending,
		})
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"overview": view, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, IntelligenceFindingsListPath, gateway, readRule(app.FindingReadPolicy()), authorization, app.FindingReadPolicy(), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ emptyReadRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		items, err := service.ListFindings(ctx, p.Scope, actor, p.MembershipID, false)
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"findings": items, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, IntelligenceFindingsRevPath, gateway, readRule(app.FindingResolvePolicy(false)), authorization, app.FindingResolvePolicy(false), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ emptyReadRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		items, err := service.ListFindings(ctx, p.Scope, actor, p.MembershipID, true)
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"findings": items, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, IntelligenceFindingsDecide, gateway, writeRule(app.FindingResolvePolicy(true), false), authorization, app.FindingResolvePolicy(true), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in findingDecideRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		res, replayed, err := service.DecideFinding(ctx, p.Scope, actor, p.MembershipID, app.DecideFindingInput{
			OperationID: in.OperationID, Action: in.Action, FindingID: in.FindingID, Reason: in.Reason, FollowupAction: in.FollowupAction,
			Revision: in.Revision, PeriodStart: in.PeriodStart, PeriodEnd: in.PeriodEnd, SyntheticSignalKind: in.SyntheticSignalKind,
			AssetID: in.AssetID, SubjectKey: in.SubjectKey, AuthorID: in.AuthorID, Severity: in.Severity, BenchmarkSyntheticCount: in.BenchmarkSyntheticCount,
		})
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"result": res, "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, IntelligenceSettingsSavePath, gateway, writeRule(app.SettingsManagePolicy(), false), authorization, app.SettingsManagePolicy(), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in findingSettingsSaveRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		res, replayed, err := service.SaveFindingSettings(ctx, p.Scope, actor, p.MembershipID, app.FindingSettingsInput{
			OperationID: in.OperationID, DuplicateWindowMinutes: in.DuplicateWindowMinutes, MaxSpeedKmh: in.MaxSpeedKmh,
			TankTolerancePercent: in.TankTolerancePercent, OutlierThresholdPercent: in.OutlierThresholdPercent,
			DocQtyTolerancePercent: in.DocQtyTolerancePercent, DocDateToleranceDays: in.DocDateToleranceDays,
			PriceThresholdPercent: in.PriceThresholdPercent, PriceMinDiffCents: in.PriceMinDiffCents,
			ReferenceSource: in.ReferenceSource, EnabledTypes: in.EnabledTypes,
		})
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"settings": res, "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, AuditEventsListPath, gateway, readRule(app.AuditReadPolicy()), authorization, app.AuditReadPolicy(), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in auditEventsListRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		lim := 100
		if in.Limit != nil {
			lim = *in.Limit
		}
		items, err := service.ListAuditEvents(ctx, p.Scope, actor, p.MembershipID, app.AuditQuery{
			ActorID: in.ActorID, Action: in.Action, TargetType: in.TargetType, TargetID: in.TargetID, Limit: lim,
		})
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"events": items, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	if err := guarded.Register(owned, ConsolidationGrantsListPath, gateway, readRule(app.GroupManagePolicy(false)), authorization, app.GroupManagePolicy(false), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ emptyReadRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		res, err := service.ListConsolidation(ctx, p.Scope, actor, p.MembershipID)
		if err != nil {
			return nil, mappedError(err)
		}
		res["data_mode"] = "SYNTHETIC"
		return res, nil
	}); err != nil {
		return err
	}

	return guarded.Register(owned, ConsolidationGrantsSavePath, gateway, writeRule(app.GroupManagePolicy(true), false), authorization, app.GroupManagePolicy(true), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in consolidationSaveRequest) (any, error) {
		actor, err := managementActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		res, replayed, err := service.SaveConsolidation(ctx, p.Scope, actor, p.MembershipID, app.ConsolidationSaveInput{
			OperationID: in.OperationID, Action: in.Action, GrantID: in.GrantID, CounterpartOrgID: in.CounterpartOrgID,
			AllowedDatasets: in.AllowedDatasets, ProposalCode: in.ProposalCode, ValidUntil: in.ValidUntil, Revision: in.Revision,
			AssetID: in.AssetID, Kind: in.Kind, RecurrenceKey: in.RecurrenceKey, PeriodMonth: in.PeriodMonth, AmountCentavos: in.AmountCentavos,
			OccurredAt: in.OccurredAt, SupplierID: in.SupplierID, ReferenceCode: in.ReferenceCode, PayableIDs: in.PayableIDs,
			ReportKind: in.ReportKind, Cadence: in.Cadence, RecipientIDs: in.RecipientIDs, Category: in.Category, TargetID: in.TargetID,
			RetentionDays: in.RetentionDays, HoldActive: in.HoldActive, ExpiresAt: in.ExpiresAt, ResponsibleCtx: in.ResponsibleCtx,
			RoleKey: in.RoleKey, RoleName: in.RoleName, Permissions: in.Permissions, FuelingID: in.FuelingID, StationID: in.StationID,
			Product: in.Product, Unit: in.Unit, UnitPriceMilli: in.UnitPriceMilli, PriceCondition: in.PriceCondition, Reason: in.Reason,
		})
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"result": res, "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	})
}
