// Package domain owns the pure registry of intelligence indicators (IND-001..020),
// ratio-of-sums consumption and operating metrics, price-comparison methodology,
// finding detectors (FT-001..006), multi-tank/category rules, and trip allocation
// (EMP-KPI-001..005, EMP-FIND-001..005, EMP-FUEL-009..010).
package domain

import (
	"errors"
	"slices"
)

const KPIVersion = "kpi-v1"

type UpdateClass string

const (
	UpdateEvent    UpdateClass = "EVENT"
	UpdateQuery    UpdateClass = "QUERY"
	UpdateAnalysis UpdateClass = "ANALYSIS"
)

type Dimension string

const (
	DimOrg        Dimension = "ORG"
	DimUnit       Dimension = "UNIT"
	DimCostCentre Dimension = "COST_CENTRE"
	DimDriver     Dimension = "DRIVER"
	DimAsset      Dimension = "ASSET"
	DimTrip       Dimension = "TRIP"
	DimSupplier   Dimension = "SUPPLIER"
)

type InsufficiencyReason string

const (
	ReasonNoClosedInterval      InsufficiencyReason = "NO_CLOSED_INTERVAL"
	ReasonMethodUnsupported     InsufficiencyReason = "METHOD_UNSUPPORTED"
	ReasonNoDistanceBasis       InsufficiencyReason = "NO_DISTANCE_BASIS"
	ReasonNoHourBasis           InsufficiencyReason = "NO_HOUR_BASIS"
	ReasonZeroDenominator       InsufficiencyReason = "ZERO_DENOMINATOR"
	ReasonNoAvailabilityHistory InsufficiencyReason = "NO_AVAILABILITY_HISTORY"
	ReasonMixedDrivers          InsufficiencyReason = "MIXED_DRIVERS"
	ReasonZeroBaseline          InsufficiencyReason = "ZERO_BASELINE"
	ReasonMethodMismatch        InsufficiencyReason = "METHOD_MISMATCH"
	ReasonNoReference           InsufficiencyReason = "NO_REFERENCE"
	ReasonPlaceUnknown          InsufficiencyReason = "PLACE_UNKNOWN"
	ReasonReferenceStale        InsufficiencyReason = "REFERENCE_STALE"
	ReasonLowCoverage           InsufficiencyReason = "LOW_COVERAGE"
	ReasonConditionMismatch     InsufficiencyReason = "CONDITION_MISMATCH"
	ReasonConditionUnknown      InsufficiencyReason = "CONDITION_UNKNOWN"
)

type IndicatorSpec struct {
	ID              string
	NamePT          string
	Formula         string
	Inputs          string
	Unit            string
	Inclusion       string
	MissingBehavior string
	Update          UpdateClass
	Source          string
	Limits          string
	Dimensions      []Dimension
	MethodVersion   string
}

var ErrUnknownIndicator = errors.New("intelligence: unknown indicator")

var registry = []IndicatorSpec{
	{ID: "IND-001", NamePT: "Gasto com combustível", Formula: "SUM(company_amount_cents) from fueling and internal-stock dispense expenses", Inputs: "Expense company amount, origin, reporting day, allocations", Unit: "centavos", Inclusion: "Approved non-voided fueling expenses; internal-stock dispense recognized once", MissingBehavior: "Zero sum with approved_count=0 shown as empty approved state", Update: UpdateEvent, Source: "fuelings, expenses", Limits: "Follows fueling reporting day, not payment date", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimDriver, DimAsset, DimTrip, DimSupplier}, MethodVersion: KPIVersion},
	{ID: "IND-002", NamePT: "Volume por produto", Formula: "SUM(quantity_milli) per product and compatible unit", Inputs: "Fueling quantity, product, unit", Unit: "milli", Inclusion: "Approved non-voided fuelings", MissingBehavior: "Empty state when approved_count=0; never mixed across units", Update: UpdateEvent, Source: "fuelings", Limits: "Purchased/dispensed volume, not instantaneous tank burn", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimDriver, DimAsset, DimTrip, DimSupplier}, MethodVersion: KPIVersion},
	{ID: "IND-003", NamePT: "Consumo (km/L)", Formula: "SUM(distance_m) / SUM(quantity_ml)", Inputs: "Computed full-to-full intervals", Unit: "km/L", Inclusion: "Closed intervals with approved fuelings of same compatible product", MissingBehavior: "INSUFFICIENT_DATA (NO_CLOSED_INTERVAL or METHOD_UNSUPPORTED)", Update: UpdateEvent, Source: "consumption intervals", Limits: "Purchase-to-purchase measure; excludes unclosed spans", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimDriver, DimAsset, DimTrip}, MethodVersion: KPIVersion},
	{ID: "IND-004", NamePT: "Consumo (L/100 km)", Formula: "SUM(quantity_ml) * 100 / SUM(distance_m)", Inputs: "Computed full-to-full intervals", Unit: "L/100km", Inclusion: "Same closed intervals as IND-003", MissingBehavior: "INSUFFICIENT_DATA (NO_CLOSED_INTERVAL)", Update: UpdateEvent, Source: "consumption intervals", Limits: "Same as IND-003", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimDriver, DimAsset, DimTrip}, MethodVersion: KPIVersion},
	{ID: "IND-005", NamePT: "Consumo por hora (L/h)", Formula: "SUM(quantity_ml) * 3600 / (SUM(hour_s) * 1000)", Inputs: "Computed hour-meter intervals", Unit: "L/h", Inclusion: "Closed hour-meter intervals", MissingBehavior: "INSUFFICIENT_DATA (NO_CLOSED_INTERVAL or NO_HOUR_BASIS)", Update: UpdateEvent, Source: "consumption intervals", Limits: "Engine hours include idling", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimDriver, DimAsset, DimTrip}, MethodVersion: KPIVersion},
	{ID: "IND-006", NamePT: "Custo de combustível por km", Formula: "SUM(interval_cost_cents) * 1000 / SUM(distance_m)", Inputs: "Interval cost, distance", Unit: "centavos/km", Inclusion: "Same closed intervals as IND-003", MissingBehavior: "INSUFFICIENT_DATA (NO_CLOSED_INTERVAL)", Update: UpdateEvent, Source: "consumption intervals", Limits: "Excludes opening anchor fill cost", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimDriver, DimAsset, DimTrip}, MethodVersion: KPIVersion},
	{ID: "IND-007", NamePT: "Custo de combustível por hora", Formula: "SUM(interval_cost_cents) * 3600 / SUM(hour_s)", Inputs: "Interval cost, seconds", Unit: "centavos/h", Inclusion: "Same closed intervals as IND-005", MissingBehavior: "INSUFFICIENT_DATA (NO_CLOSED_INTERVAL or NO_HOUR_BASIS)", Update: UpdateEvent, Source: "consumption intervals", Limits: "Same as IND-005", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimDriver, DimAsset, DimTrip}, MethodVersion: KPIVersion},
	{ID: "IND-008", NamePT: "Custo operacional total", Formula: "SUM(company_amount_cents) for operating categories", Inputs: "Approved expenses, operating category flag", Unit: "centavos", Inclusion: "Each cost once; stock receipts and settlement payments excluded", MissingBehavior: "Zero sum with count=0 labelled", Update: UpdateEvent, Source: "expenses", Limits: "Operational cost only; excludes capital depreciation", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimDriver, DimAsset, DimTrip, DimSupplier}, MethodVersion: KPIVersion},
	{ID: "IND-009", NamePT: "Custo operacional por km", Formula: "SUM(matched_window_operating_cents) * 1000 / SUM(window_distance_m)", Inputs: "Operating expenses within asset valid-reading window", Unit: "centavos/km", Inclusion: "Assets with >=2 valid odometer readings in period", MissingBehavior: "INSUFFICIENT_DATA (NO_DISTANCE_BASIS or ZERO_DENOMINATOR)", Update: UpdateEvent, Source: "expenses, meter readings", Limits: "Matched window excludes outside costs", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimAsset, DimTrip}, MethodVersion: KPIVersion},
	{ID: "IND-010", NamePT: "Custo operacional por hora", Formula: "SUM(matched_window_operating_cents) * 3600 / SUM(window_hours_s)", Inputs: "Operating expenses within asset hour-meter window", Unit: "centavos/h", Inclusion: "Assets with >=2 valid hour-meter readings in period", MissingBehavior: "INSUFFICIENT_DATA (NO_HOUR_BASIS or ZERO_DENOMINATOR)", Update: UpdateEvent, Source: "expenses, meter readings", Limits: "Matched window only", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimAsset, DimTrip}, MethodVersion: KPIVersion},
	{ID: "IND-011", NamePT: "Custo de manutenção", Formula: "SUM(company_amount_cents) from closed work orders and maintenance expenses", Inputs: "Work-order expenses, maintenance categories", Unit: "centavos", Inclusion: "Each closed work order posted once", MissingBehavior: "Empty state with count=0", Update: UpdateEvent, Source: "maintenance, expenses", Limits: "Recognized at approved closing, not payment", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimAsset, DimSupplier}, MethodVersion: KPIVersion},
	{ID: "IND-012", NamePT: "Disponibilidade da frota", Formula: "SUM(available_seconds) / SUM(active_seconds)", Inputs: "Asset lifecycle ACTIVE seconds minus maintenance holds", Unit: "ratio", Inclusion: "Assets with ACTIVE lifecycle in period", MissingBehavior: "INSUFFICIENT_DATA (NO_AVAILABILITY_HISTORY)", Update: UpdateEvent, Source: "fleet availability, maintenance holds", Limits: "Calendar 24h availability, not shift schedule", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimAsset}, MethodVersion: KPIVersion},
	{ID: "IND-013", NamePT: "Registros aguardando revisão", Formula: "COUNT(SUBMITTED or IN_REVIEW) and oldest age", Inputs: "Review queues across trips, fuelings, expenses, work orders", Unit: "count", Inclusion: "Permitted types in scope", MissingBehavior: "Disabled modules omitted", Update: UpdateQuery, Source: "review queues", Limits: "Workload indicator, not quality metric", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimDriver, DimAsset}, MethodVersion: KPIVersion},
	{ID: "IND-014", NamePT: "Documentos não conciliados", Formula: "COUNT and SUM(amount) of stored non-reconciled documents", Inputs: "Stored documents excluding REPLACED/DELETED", Unit: "count", Inclusion: "UNALLOCATED, PARTIALLY_ALLOCATED, OVER_ALLOCATED", MissingBehavior: "Empty count when none", Update: UpdateQuery, Source: "documents", Limits: "Not an indicator of fiscal irregularity", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimSupplier}, MethodVersion: KPIVersion},
	{ID: "IND-015", NamePT: "Envios offline aguardando decisão", Formula: "COUNT(unresolved sync review items)", Inputs: "org_sync_review_items", Unit: "count", Inclusion: "All unresolved held items in scope", MissingBehavior: "Zero when queue empty", Update: UpdateQuery, Source: "sync review queue", Limits: "While >0 other KPIs omit held records", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimDriver, DimAsset}, MethodVersion: KPIVersion},
	{ID: "IND-016", NamePT: "Diferença em relação à referência", Formula: "SUM(paid_cents - reference_cents) over comparable fuelings", Unit: "centavos", Inclusion: "Single reference source; same station/municipality, condition, coverage >= 3", Inputs: "Fuelings and immutable price comparisons", MissingBehavior: "INSUFFICIENT_DATA with reason and compared share", Update: UpdateAnalysis, Source: "price comparisons", Limits: "Difference for review, never proven loss or savings", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimDriver, DimAsset, DimTrip, DimSupplier}, MethodVersion: KPIVersion},
	{ID: "IND-017", NamePT: "Distância percorrida", Formula: "SUM(valid odometer window usage per asset)", Inputs: "VALID odometer readings", Unit: "m", Inclusion: "At least 2 valid odometer readings per segment", MissingBehavior: "INSUFFICIENT_DATA (NO_DISTANCE_BASIS)", Update: UpdateEvent, Source: "meter readings", Limits: "Observed window distance, never replaced by GPS", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimAsset, DimTrip}, MethodVersion: KPIVersion},
	{ID: "IND-018", NamePT: "Horas operadas", Formula: "SUM(valid hour-meter window usage per asset)", Inputs: "VALID hour-meter readings", Unit: "s", Inclusion: "At least 2 valid hour-meter readings per segment", MissingBehavior: "INSUFFICIENT_DATA (NO_HOUR_BASIS)", Update: UpdateEvent, Source: "meter readings", Limits: "Observed window hours", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimAsset, DimTrip}, MethodVersion: KPIVersion},
	{ID: "IND-019", NamePT: "Achados em aberto", Formula: "COUNT(OPEN or IN_REVIEW findings)", Inputs: "org_findings", Unit: "count", Inclusion: "Enabled finding types in caller scope", MissingBehavior: "Disabled finding types omitted", Update: UpdateQuery, Source: "findings", Limits: "Questions for review, never verdicts", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimDriver, DimAsset}, MethodVersion: KPIVersion},
	{ID: "IND-020", NamePT: "Variação entre períodos", Formula: "(current - baseline) / baseline", Inputs: "Two KPI snapshots with identical dimension and method version", Unit: "ratio", Inclusion: "Both snapshots COMPUTED with same method_version", MissingBehavior: "ZERO_BASELINE, INSUFFICIENT_DATA, or METHOD_MISMATCH", Update: UpdateQuery, Source: "kpi snapshots", Limits: "Two-point delta is not a statistical trend", Dimensions: []Dimension{DimOrg, DimUnit, DimCostCentre, DimDriver, DimAsset, DimTrip, DimSupplier}, MethodVersion: KPIVersion},
}

func Indicators() []IndicatorSpec {
	out := make([]IndicatorSpec, len(registry))
	for i, s := range registry {
		s.Dimensions = slices.Clone(s.Dimensions)
		out[i] = s
	}
	return out
}

func LookupIndicator(id string) (IndicatorSpec, error) {
	for _, s := range registry {
		if s.ID == id {
			s.Dimensions = slices.Clone(s.Dimensions)
			return s, nil
		}
	}
	return IndicatorSpec{}, ErrUnknownIndicator
}
