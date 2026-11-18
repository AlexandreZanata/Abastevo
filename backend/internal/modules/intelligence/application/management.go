package application

import (
	"context"
	"encoding/json"
	"time"

	intel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/intelligence/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	plan "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/plan"
)

type TankView struct {
	ID            string `json:"id"`
	UnitID        string `json:"unit_id,omitempty"`
	Name          string `json:"name"`
	Product       string `json:"product"`
	Unit          string `json:"unit"`
	CapacityMilli int64  `json:"capacity_milli"`
	QuantityMilli int64  `json:"quantity_milli"`
	ValueCentavos int64  `json:"value_centavos"`
	AllowNegative bool   `json:"allow_negative"`
	State         string `json:"state"`
	Revision      int64  `json:"revision"`
	UpdatedAt     string `json:"updated_at"`
}

type SaveTankInput struct {
	OperationID   string `json:"operation_id"`
	TankID        string `json:"tank_id,omitempty"`
	UnitID        string `json:"unit_id,omitempty"`
	Name          string `json:"name"`
	Product       string `json:"product"`
	Unit          string `json:"unit"`
	CapacityMilli int64  `json:"capacity_milli"`
	AllowNegative bool   `json:"allow_negative"`
	State         string `json:"state,omitempty"`
	Revision      *int64 `json:"revision,omitempty"`
}

type MovementView struct {
	ID                        string `json:"id"`
	TankID                    string `json:"tank_id"`
	Kind                      string `json:"kind"`
	Product                   string `json:"product"`
	Unit                      string `json:"unit"`
	DeltaQuantityMilli        int64  `json:"delta_quantity_milli"`
	AmountCentavos            int64  `json:"amount_centavos"`
	PoolQuantityAfterMilli    int64  `json:"pool_quantity_after_milli"`
	PoolValueAfterCentavos    int64  `json:"pool_value_after_centavos"`
	AssetID                   string `json:"asset_id,omitempty"`
	CounterpartTankID         string `json:"counterpart_tank_id,omitempty"`
	FuelingID                 string `json:"fueling_id,omitempty"`
	ExpenseID                 string `json:"expense_id,omitempty"`
	Reason                    string `json:"reason,omitempty"`
	OccurredAt                string `json:"occurred_at"`
	RecognizedExpenseCentavos int64  `json:"recognized_expense_centavos"`
}

type ReconciliationView struct {
	ID                   string `json:"id"`
	TankID               string `json:"tank_id"`
	MeasuredMilli        int64  `json:"measured_milli"`
	RecordedMilli        int64  `json:"recorded_milli"`
	VarianceMilli        int64  `json:"variance_milli"`
	State                string `json:"state"`
	AdjustmentMovementID string `json:"adjustment_movement_id,omitempty"`
	Reason               string `json:"reason,omitempty"`
	MeasuredAt           string `json:"measured_at"`
}

type RecordMovementInput struct {
	OperationID       string `json:"operation_id"`
	TankID            string `json:"tank_id"`
	Kind              string `json:"kind"` // RECEIPT | DISPENSE | TRANSFER | ADJUST | RECONCILE
	Product           string `json:"product"`
	Unit              string `json:"unit,omitempty"`
	QuantityMilli     int64  `json:"quantity_milli"`
	DeltaMilli        int64  `json:"delta_milli,omitempty"`
	AmountCentavos    int64  `json:"amount_centavos"`
	MeasuredMilli     int64  `json:"measured_milli,omitempty"`
	AssetID           string `json:"asset_id,omitempty"`
	CounterpartTankID string `json:"counterpart_tank_id,omitempty"`
	SupplierID        string `json:"supplier_id,omitempty"`
	DocumentID        string `json:"document_id,omitempty"`
	FillKind          string `json:"fill,omitempty"`
	ApplyAdjustment   bool   `json:"apply_adjustment,omitempty"`
	Reason            string `json:"reason,omitempty"`
	OccurredAt        string `json:"occurred_at"`
}

type PlanView struct {
	ID              string `json:"id"`
	AssetID         string `json:"asset_id"`
	Name            string `json:"name"`
	RebaseRule      string `json:"rebase_rule"`
	Blocking        bool   `json:"blocking"`
	State           string `json:"state"`
	OccurrenceID    string `json:"occurrence_id,omitempty"`
	OccurrenceState string `json:"occurrence_state,omitempty"`
	DueOdometerM    *int64 `json:"due_odometer_m,omitempty"`
	DueHourS        *int64 `json:"due_hour_s,omitempty"`
	DueDay          string `json:"due_day,omitempty"`
	RemainingM      *int64 `json:"remaining_m,omitempty"`
	RemainingS      *int64 `json:"remaining_s,omitempty"`
	DaysToDue       *int   `json:"days_to_due,omitempty"`
	ProjectedDueDay string `json:"projected_due_day,omitempty"`
	DecidingTrigger string `json:"deciding_trigger,omitempty"`
	ReadingRevision int64  `json:"reading_revision,omitempty"`
	Revision        int64  `json:"revision"`
}

type ComplianceView struct {
	ID              string `json:"id"`
	SubjectKind     string `json:"subject_kind"`
	SubjectID       string `json:"subject_id"`
	ItemKind        string `json:"item_kind"`
	ExpiryDay       string `json:"expiry_day"`
	LeadDays        int    `json:"lead_days"`
	State           string `json:"state"`
	DocumentID      string `json:"document_id,omitempty"`
	LicenceCategory string `json:"licence_category,omitempty"`
	Revision        int64  `json:"revision"`
}

type SavePlanInput struct {
	OperationID        string   `json:"operation_id"`
	Action             string   `json:"action"` // SAVE | RECOMPUTE | COMPLETE_OCCURRENCE | SKIP_OCCURRENCE | COMPLIANCE_UPSERT | COMPLIANCE_TOGGLE_LICENCE | INSURANCE_SAVE | INSURANCE_CLAIM | ALERT_DIGEST
	PlanID             string   `json:"plan_id,omitempty"`
	AssetID            string   `json:"asset_id,omitempty"`
	Name               string   `json:"name,omitempty"`
	DistanceIntervalM  *int64   `json:"distance_interval_m,omitempty"`
	DistanceLeadM      *int64   `json:"distance_lead_m,omitempty"`
	HourIntervalS      *int64   `json:"hour_interval_s,omitempty"`
	HourLeadS          *int64   `json:"hour_lead_s,omitempty"`
	TimeIntervalDays   *int     `json:"time_interval_days,omitempty"`
	TimeIntervalMonths *int     `json:"time_interval_months,omitempty"`
	TimeLeadDays       *int     `json:"time_lead_days,omitempty"`
	BaselineDay        string   `json:"baseline_day,omitempty"`
	BaselineOdometerM  *int64   `json:"baseline_odometer_m,omitempty"`
	BaselineHourS      *int64   `json:"baseline_hour_s,omitempty"`
	EvalDay            string   `json:"eval_day,omitempty"`
	CurrentOdometerM   *int64   `json:"current_odometer_m,omitempty"`
	CurrentHourS       *int64   `json:"current_hour_s,omitempty"`
	WindowStartDay     string   `json:"window_start_day,omitempty"`
	WindowStartOdoM    *int64   `json:"window_start_odom,omitempty"`
	ReadingRevision    int64    `json:"reading_revision,omitempty"`
	RebaseRule         string   `json:"rebase_rule,omitempty"`
	Blocking           bool     `json:"blocking,omitempty"`
	Reason             string   `json:"reason,omitempty"`
	SubjectKind        string   `json:"subject_kind,omitempty"`
	SubjectID          string   `json:"subject_id,omitempty"`
	ItemKind           string   `json:"item_kind,omitempty"`
	ExpiryDay          string   `json:"expiry_day,omitempty"`
	LeadDays           int      `json:"lead_days,omitempty"`
	DocumentID         string   `json:"document_id,omitempty"`
	LicenceCategory    string   `json:"licence_category,omitempty"`
	LicenceEnabled     *bool    `json:"licence_enabled,omitempty"`
	PolicyID           string   `json:"policy_id,omitempty"`
	PolicyNumber       string   `json:"policy_number,omitempty"`
	InsurerName        string   `json:"insurer_name,omitempty"`
	ValidFrom          string   `json:"valid_from,omitempty"`
	AssetIDs           []string `json:"asset_ids,omitempty"`
	IncidentReportID   string   `json:"incident_report_id,omitempty"`
	ClaimState         string   `json:"claim_state,omitempty"`
}

type AssetDocumentInput struct {
	OperationID  string `json:"operation_id"`
	Action       string `json:"action"` // ATTACH | DETACH
	LinkID       string `json:"link_id,omitempty"`
	DocumentID   string `json:"document_id,omitempty"`
	Kind         string `json:"kind,omitempty"`
	ExpiryDay    string `json:"expiry_day,omitempty"`
	IsCoverPhoto bool   `json:"is_cover_photo,omitempty"`
}

type AssetDocumentView struct {
	ID           string `json:"id"`
	AssetID      string `json:"asset_id"`
	DocumentID   string `json:"document_id"`
	Kind         string `json:"kind"`
	ExpiryDay    string `json:"expiry_day,omitempty"`
	IsCoverPhoto bool   `json:"is_cover_photo"`
	State        string `json:"state"`
	CreatedAt    string `json:"created_at"`
}

type WorkOrderLineInput struct {
	Kind           string `json:"kind"` // PART | SERVICE | LABOR
	QuantityMilli  int64  `json:"quantity_milli"`
	UnitPriceMilli int64  `json:"unit_price_milli"`
	Description    string `json:"description,omitempty"`
}

type WorkOrderView struct {
	ID                 string               `json:"id"`
	Number             int64                `json:"number"`
	AssetID            string               `json:"asset_id"`
	Kind               string               `json:"kind"`
	OriginKind         string               `json:"origin_kind"`
	OriginID           string               `json:"origin_id,omitempty"`
	Description        string               `json:"description"`
	State              string               `json:"state"`
	RequesterID        string               `json:"requester_membership_id"`
	ReviewerID         string               `json:"reviewer_membership_id,omitempty"`
	SelectedQuoteCents *int64               `json:"selected_quote_cents,omitempty"`
	QuoteValidUntil    string               `json:"quote_valid_until,omitempty"`
	ApprovedCents      *int64               `json:"approved_cents,omitempty"`
	TolerancePercent   int                  `json:"tolerance_percent"`
	PartsCents         int64                `json:"parts_cents"`
	ServiceCents       int64                `json:"service_cents"`
	DiscountCents      int64                `json:"discount_cents"`
	PaidCents          *int64               `json:"paid_cents,omitempty"`
	ArithmeticMismatch bool                 `json:"arithmetic_mismatch"`
	ExpenseID          string               `json:"expense_id,omitempty"`
	HoldOpen           bool                 `json:"hold_open"`
	AfterTheFact       bool                 `json:"after_the_fact"`
	Revision           int64                `json:"revision"`
	Lines              []WorkOrderLineInput `json:"lines,omitempty"`
}

type SaveWorkOrderInput struct {
	OperationID        string               `json:"operation_id"`
	Action             string               `json:"action"` // CREATE | QUOTE | APPROVE_DIRECT | SCHEDULE | START | COMPLETE | RELEASE_HOLD | CLOSE | CANCEL | AMEND | COMPONENT_SAVE | PART_MOVEMENT
	WorkOrderID        string               `json:"work_order_id,omitempty"`
	AssetID            string               `json:"asset_id,omitempty"`
	Kind               string               `json:"kind,omitempty"`
	OriginKind         string               `json:"origin_kind,omitempty"`
	OriginID           string               `json:"origin_id,omitempty"`
	SupplierID         string               `json:"supplier_id,omitempty"`
	Description        string               `json:"description,omitempty"`
	SelectedQuoteCents *int64               `json:"selected_quote_cents,omitempty"`
	QuoteValidUntil    string               `json:"quote_valid_until,omitempty"`
	DirectLimitCents   *int64               `json:"direct_limit_cents,omitempty"`
	TolerancePercent   *int                 `json:"tolerance_percent,omitempty"`
	InvoiceRequired    bool                 `json:"invoice_required,omitempty"`
	InvoiceDocumentID  string               `json:"invoice_document_id,omitempty"`
	Lines              []WorkOrderLineInput `json:"lines,omitempty"`
	DiscountCents      int64                `json:"discount_cents,omitempty"`
	PaidCents          *int64               `json:"paid_cents,omitempty"`
	ScheduledAt        string               `json:"scheduled_at,omitempty"`
	AfterTheFact       bool                 `json:"after_the_fact,omitempty"`
	Reason             string               `json:"reason,omitempty"`
	Revision           *int64               `json:"revision,omitempty"`
	ComponentID        string               `json:"component_id,omitempty"`
	ComponentKind      string               `json:"component_kind,omitempty"`
	SerialNumber       string               `json:"serial_number,omitempty"`
	Position           string               `json:"position,omitempty"`
	ComponentState     string               `json:"component_state,omitempty"`
	MeterValue         *int64               `json:"meter_value,omitempty"`
	PartCode           string               `json:"part_code,omitempty"`
	PartName           string               `json:"part_name,omitempty"`
	PartUnit           string               `json:"part_unit,omitempty"`
	PartDeltaMilli     int64                `json:"part_delta_milli,omitempty"`
	PartAmountCentavos int64                `json:"part_amount_centavos,omitempty"`
}

type ReviewWorkOrderInput struct {
	OperationID   string `json:"operation_id"`
	WorkOrderID   string `json:"work_order_id"`
	Decision      string `json:"decision"` // APPROVE | RETURN | REJECT
	ApprovedCents *int64 `json:"approved_cents,omitempty"`
	Reason        string `json:"reason"`
	Revision      int64  `json:"revision"`
}

type KPIValueView struct {
	IndicatorID         string `json:"indicator_id"`
	NamePT              string `json:"name_pt"`
	Status              string `json:"status"`
	InsufficiencyReason string `json:"insufficiency_reason,omitempty"`
	ValueRational       string `json:"value_rational,omitempty"`
	DisplayValue        string `json:"display_value,omitempty"`
	Unit                string `json:"unit"`
	Numerator           int64  `json:"numerator"`
	Denominator         int64  `json:"denominator"`
	ApprovedCount       int    `json:"approved_count"`
	PendingCount        int    `json:"pending_count"`
	ExcludedCount       int    `json:"excluded_count"`
	SampleSmall         bool   `json:"sample_small"`
	MethodVersion       string `json:"method_version"`
}

type PriceComparisonView struct {
	ID                     string `json:"id"`
	FuelingID              string `json:"fueling_id"`
	ReferenceSource        string `json:"reference_source"`
	MethodologyVersion     string `json:"methodology_version"`
	Status                 string `json:"status"`
	InsufficiencyReason    string `json:"insufficiency_reason,omitempty"`
	PlaceRule              string `json:"place_rule"`
	Product                string `json:"product"`
	PriceCondition         string `json:"price_condition"`
	CoverageStations       int    `json:"coverage_stations"`
	DifferenceCents        *int64 `json:"difference_cents,omitempty"`
	DifferencePerUnitMilli *int64 `json:"difference_per_unit_milli,omitempty"`
	DisplayNote            string `json:"display_note,omitempty"`
}

type KPIOverviewView struct {
	Dimension        string                `json:"dimension"`
	IncludePending   bool                  `json:"include_pending"`
	Registry         []intel.IndicatorSpec `json:"registry"`
	Values           []KPIValueView        `json:"values"`
	PriceComparisons []PriceComparisonView `json:"price_comparisons"`
}

type KPIOverviewQuery struct {
	Dimension      string `json:"dimension,omitempty"`
	DimensionID    string `json:"dimension_id,omitempty"`
	PeriodStart    string `json:"period_start,omitempty"`
	PeriodEnd      string `json:"period_end,omitempty"`
	IncludePending bool   `json:"include_pending,omitempty"`
}

type FindingView struct {
	ID              string          `json:"id"`
	FindingType     string          `json:"finding_type"`
	SubjectKey      string          `json:"subject_key"`
	AssetID         string          `json:"asset_id,omitempty"`
	AuthorID        string          `json:"author_membership_id,omitempty"`
	Severity        string          `json:"severity"`
	State           string          `json:"state"`
	MethodVersion   string          `json:"method_version"`
	Title           string          `json:"title"`
	NonProofNote    string          `json:"non_proof_note"`
	Evidence        json.RawMessage `json:"evidence"`
	AssignedID      string          `json:"assigned_membership_id,omitempty"`
	DecidedByID     string          `json:"decided_by_membership_id,omitempty"`
	DecisionReason  string          `json:"decision_reason,omitempty"`
	FollowupAction  string          `json:"followup_action,omitempty"`
	RunID           string          `json:"run_id,omitempty"`
	SupersededByRun string          `json:"superseded_by_run_id,omitempty"`
	Revision        int64           `json:"revision"`
	CreatedAt       string          `json:"created_at"`
}

type DecideFindingInput struct {
	OperationID             string `json:"operation_id"`
	Action                  string `json:"action"` // TAKE | RELEASE | CONFIRM | EXPLAIN | FALSE_POSITIVE | RAISE_OR_DETECT | REPROCESS | BENCHMARK
	FindingID               string `json:"finding_id,omitempty"`
	Reason                  string `json:"reason,omitempty"`
	FollowupAction          string `json:"followup_action,omitempty"`
	Revision                *int64 `json:"revision,omitempty"`
	PeriodStart             string `json:"period_start,omitempty"`
	PeriodEnd               string `json:"period_end,omitempty"`
	SyntheticSignalKind     string `json:"synthetic_signal_kind,omitempty"`
	AssetID                 string `json:"asset_id,omitempty"`
	SubjectKey              string `json:"subject_key,omitempty"`
	AuthorID                string `json:"author_membership_id,omitempty"`
	Severity                string `json:"severity,omitempty"`
	BenchmarkSyntheticCount int    `json:"benchmark_synthetic_count,omitempty"`
}

type FindingSettingsInput struct {
	OperationID             string   `json:"operation_id"`
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

type AuditEventView struct {
	ID         string `json:"id"`
	Category   string `json:"category"` // AUDIT | SENSITIVE_ACCESS
	Action     string `json:"action"`
	ActorID    string `json:"actor_id"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Reason     string `json:"reason,omitempty"`
	OccurredAt string `json:"occurred_at"`
}

type AuditQuery struct {
	ActorID    string `json:"actor_id,omitempty"`
	Action     string `json:"action,omitempty"`
	TargetType string `json:"target_type,omitempty"`
	TargetID   string `json:"target_id,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type ConsolidationGrantView struct {
	ID               string   `json:"id"`
	CounterpartOrgID string   `json:"counterpart_org_id"`
	AllowedDatasets  []string `json:"allowed_datasets"`
	State            string   `json:"state"`
	ProposedByOrgID  string   `json:"proposed_by_org_id"`
	ValidUntil       string   `json:"valid_until"`
	Revision         int64    `json:"revision"`
}

type ConsolidationSaveInput struct {
	OperationID      string   `json:"operation_id"`
	Action           string   `json:"action"` // PROPOSE | ACCEPT | END | DECLINE | RECURRING_COST | PAYABLE_STATEMENT | REPORT_SCHEDULE | RETENTION_POLICY | LEGAL_HOLD | DELETION_REPLAY | DSR_ROUTE | ERASURE_TOMBSTONE | CUSTOM_ROLE | VOLUNTARY_PRICE
	GrantID          string   `json:"grant_id,omitempty"`
	CounterpartOrgID string   `json:"counterpart_org_id,omitempty"`
	AllowedDatasets  []string `json:"allowed_datasets,omitempty"`
	ProposalCode     string   `json:"proposal_code,omitempty"`
	ValidUntil       string   `json:"valid_until,omitempty"`
	Revision         *int64   `json:"revision,omitempty"`
	AssetID          string   `json:"asset_id,omitempty"`
	Kind             string   `json:"kind,omitempty"`
	RecurrenceKey    string   `json:"recurrence_key,omitempty"`
	PeriodMonth      string   `json:"period_month,omitempty"`
	AmountCentavos   int64    `json:"amount_centavos,omitempty"`
	OccurredAt       string   `json:"occurred_at,omitempty"`
	SupplierID       string   `json:"supplier_id,omitempty"`
	ReferenceCode    string   `json:"reference_code,omitempty"`
	PayableIDs       []string `json:"payable_ids,omitempty"`
	ReportKind       string   `json:"report_kind,omitempty"`
	Cadence          string   `json:"cadence,omitempty"`
	RecipientIDs     []string `json:"recipient_membership_ids,omitempty"`
	Category         string   `json:"category,omitempty"`
	TargetID         string   `json:"target_id,omitempty"`
	RetentionDays    *int     `json:"retention_days,omitempty"`
	HoldActive       bool     `json:"hold_active,omitempty"`
	ExpiresAt        string   `json:"expires_at,omitempty"`
	ResponsibleCtx   string   `json:"responsible_context,omitempty"`
	RoleKey          string   `json:"role_key,omitempty"`
	RoleName         string   `json:"role_name,omitempty"`
	Permissions      []string `json:"permissions,omitempty"`
	FuelingID        string   `json:"fueling_id,omitempty"`
	StationID        string   `json:"station_id,omitempty"`
	Product          string   `json:"product,omitempty"`
	Unit             string   `json:"unit,omitempty"`
	UnitPriceMilli   int64    `json:"unit_price_milli,omitempty"`
	PriceCondition   string   `json:"price_condition,omitempty"`
	Reason           string   `json:"reason,omitempty"`
}

type ManagementRepository interface {
	ListTanks(ctx context.Context, scope kernel.OrgScope, actor Actor, member string) ([]TankView, error)
	SaveTank(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in SaveTankInput) (TankView, bool, error)
	ListMovements(ctx context.Context, scope kernel.OrgScope, actor Actor, member, tankID string) ([]MovementView, []ReconciliationView, error)
	RecordMovement(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in RecordMovementInput) (map[string]any, bool, error)

	ListPlans(ctx context.Context, scope kernel.OrgScope, actor Actor, member, assetID string) ([]PlanView, []ComplianceView, error)
	SavePlan(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in SavePlanInput) (map[string]any, bool, error)
	SaveAssetDocument(ctx context.Context, scope kernel.OrgScope, actor Actor, member, assetID string, in AssetDocumentInput) (AssetDocumentView, bool, error)

	ListWorkOrders(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, forReview, mineOnly bool) ([]WorkOrderView, error)
	SaveWorkOrder(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in SaveWorkOrderInput) (map[string]any, bool, error)
	ReviewWorkOrder(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in ReviewWorkOrderInput) (WorkOrderView, bool, error)

	KPIOverview(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, q KPIOverviewQuery) (KPIOverviewView, error)
	ListFindings(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, forReview bool) ([]FindingView, error)
	DecideFinding(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in DecideFindingInput) (map[string]any, bool, error)
	SaveFindingSettings(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in FindingSettingsInput) (map[string]any, bool, error)

	ListAuditEvents(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, q AuditQuery) ([]AuditEventView, error)
	ListConsolidation(ctx context.Context, scope kernel.OrgScope, actor Actor, member string) (map[string]any, error)
	SaveConsolidation(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in ConsolidationSaveInput) (map[string]any, bool, error)
}

type Service struct {
	Enabled    bool
	Repository ManagementRepository
	Now        func() time.Time
	NewID      func() (string, error)
}

func (s Service) ready() bool { return s.Enabled && s.Repository != nil }

func (s Service) ListTanks(ctx context.Context, scope kernel.OrgScope, actor Actor, member string) ([]TankView, error) {
	if !s.ready() {
		return nil, ErrUnavailable
	}
	return s.Repository.ListTanks(ctx, scope, actor, member)
}

func (s Service) SaveTank(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in SaveTankInput) (TankView, bool, error) {
	if !s.ready() {
		return TankView{}, false, ErrUnavailable
	}
	return s.Repository.SaveTank(ctx, scope, actor, member, in)
}

func (s Service) ListMovements(ctx context.Context, scope kernel.OrgScope, actor Actor, member, tankID string) ([]MovementView, []ReconciliationView, error) {
	if !s.ready() {
		return nil, nil, ErrUnavailable
	}
	return s.Repository.ListMovements(ctx, scope, actor, member, tankID)
}

func (s Service) RecordMovement(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in RecordMovementInput) (map[string]any, bool, error) {
	if !s.ready() {
		return nil, false, ErrUnavailable
	}
	return s.Repository.RecordMovement(ctx, scope, actor, member, in)
}

func (s Service) ListPlans(ctx context.Context, scope kernel.OrgScope, actor Actor, member, assetID string) ([]PlanView, []ComplianceView, error) {
	if !s.ready() {
		return nil, nil, ErrUnavailable
	}
	return s.Repository.ListPlans(ctx, scope, actor, member, assetID)
}

func (s Service) SavePlan(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in SavePlanInput) (map[string]any, bool, error) {
	if !s.ready() {
		return nil, false, ErrUnavailable
	}
	return s.Repository.SavePlan(ctx, scope, actor, member, in)
}

func (s Service) SaveAssetDocument(ctx context.Context, scope kernel.OrgScope, actor Actor, member, assetID string, in AssetDocumentInput) (AssetDocumentView, bool, error) {
	if !s.ready() {
		return AssetDocumentView{}, false, ErrUnavailable
	}
	return s.Repository.SaveAssetDocument(ctx, scope, actor, member, assetID, in)
}

func (s Service) ListWorkOrders(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, forReview, mineOnly bool) ([]WorkOrderView, error) {
	if !s.ready() {
		return nil, ErrUnavailable
	}
	return s.Repository.ListWorkOrders(ctx, scope, actor, member, forReview, mineOnly)
}

func (s Service) SaveWorkOrder(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in SaveWorkOrderInput) (map[string]any, bool, error) {
	if !s.ready() {
		return nil, false, ErrUnavailable
	}
	return s.Repository.SaveWorkOrder(ctx, scope, actor, member, in)
}

func (s Service) ReviewWorkOrder(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in ReviewWorkOrderInput) (WorkOrderView, bool, error) {
	if !s.ready() {
		return WorkOrderView{}, false, ErrUnavailable
	}
	return s.Repository.ReviewWorkOrder(ctx, scope, actor, member, in)
}

func (s Service) KPIOverview(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, q KPIOverviewQuery) (KPIOverviewView, error) {
	if !s.ready() {
		return KPIOverviewView{}, ErrUnavailable
	}
	return s.Repository.KPIOverview(ctx, scope, actor, member, q)
}

func (s Service) ListFindings(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, forReview bool) ([]FindingView, error) {
	if !s.ready() {
		return nil, ErrUnavailable
	}
	return s.Repository.ListFindings(ctx, scope, actor, member, forReview)
}

func (s Service) DecideFinding(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in DecideFindingInput) (map[string]any, bool, error) {
	if !s.ready() {
		return nil, false, ErrUnavailable
	}
	return s.Repository.DecideFinding(ctx, scope, actor, member, in)
}

func (s Service) SaveFindingSettings(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in FindingSettingsInput) (map[string]any, bool, error) {
	if !s.ready() {
		return nil, false, ErrUnavailable
	}
	return s.Repository.SaveFindingSettings(ctx, scope, actor, member, in)
}

func (s Service) ListAuditEvents(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, q AuditQuery) ([]AuditEventView, error) {
	if !s.ready() {
		return nil, ErrUnavailable
	}
	return s.Repository.ListAuditEvents(ctx, scope, actor, member, q)
}

func (s Service) ListConsolidation(ctx context.Context, scope kernel.OrgScope, actor Actor, member string) (map[string]any, error) {
	if !s.ready() {
		return nil, ErrUnavailable
	}
	return s.Repository.ListConsolidation(ctx, scope, actor, member)
}

func (s Service) SaveConsolidation(ctx context.Context, scope kernel.OrgScope, actor Actor, member string, in ConsolidationSaveInput) (map[string]any, bool, error) {
	if !s.ready() {
		return nil, false, ErrUnavailable
	}
	return s.Repository.SaveConsolidation(ctx, scope, actor, member, in)
}

var _ = plan.StateUpcoming
