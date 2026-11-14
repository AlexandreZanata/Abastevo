// Package domain supplies pure organization asset rules; it grants no authority or persistence.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	exact "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgvalue/domain"
)

var (
	ErrInvalid    = errors.New("fleet: invalid asset value")
	ErrScope      = errors.New("fleet: organization scope denied")
	ErrIncomplete = errors.New("fleet: activation incomplete")
	ErrKindLocked = errors.New("fleet: kind fixed by accepted operation")
)

// Private domain values never serialize themselves as a wire DTO or diagnostic content.
// Owning adapters map explicit fields only after proving current authority.
type redacted struct{}

func (redacted) String() string                 { return "EnterpriseFleetValue(redacted)" }
func (r redacted) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, r.String()) }
func (r redacted) LogValue() slog.Value         { return slog.StringValue(r.String()) }
func (r redacted) MarshalJSON() ([]byte, error) { return json.Marshal(r.String()) }

type Kind string

const (
	Vehicle   Kind = "VEHICLE"
	Machine   Kind = "MACHINE"
	Implement Kind = "IMPLEMENT"
)

func validKind(kind Kind) bool { return kind == Vehicle || kind == Machine || kind == Implement }

type State string

const (
	Draft    State = "DRAFT"
	Active   State = "ACTIVE"
	Inactive State = "INACTIVE"
	Disposed State = "DISPOSED"
	Archived State = "ARCHIVED"
)

func ParseState(raw string) (State, error) {
	switch State(raw) {
	case Draft, Active, Inactive, Disposed, Archived:
		return State(raw), nil
	}
	return "", ErrInvalid
}

type Basis string

const (
	Distance Basis = "DISTANCE"
	Hours    Basis = "HOURS"
)

func validBasis(basis Basis) bool { return basis == Distance || basis == Hours }

type Unit string

const (
	Liter        Unit = "L"
	CubicMetre   Unit = "M3"
	KilowattHour Unit = "KWH"
)

func validUnit(unit Unit) bool { return unit == Liter || unit == CubicMetre || unit == KilowattHour }

type CategoryDescriptor struct {
	redacted
	Code string
}
type ProductDescriptor struct {
	redacted
	Code string
	Unit Unit
}
type CategoryCatalog interface {
	Category(code string) (CategoryDescriptor, bool)
}
type ProductCatalog interface {
	Product(code string) (ProductDescriptor, bool)
}
type Catalog interface {
	CategoryCatalog
	ProductCatalog
}

func validMetadata(raw string, maximum int) bool {
	if !utf8.ValidString(raw) || utf8.RuneCountInString(raw) > maximum {
		return false
	}
	for _, point := range raw {
		if unicode.IsControl(point) {
			return false
		}
	}
	return true
}
func blank(raw string) bool { return strings.TrimSpace(raw) == "" }
func unavailable(port any) bool {
	if port == nil {
		return true
	}
	value := reflect.ValueOf(port)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	}
	return false
}
func product(catalog ProductCatalog, code string) (ProductDescriptor, error) {
	if unavailable(catalog) || blank(code) || !validMetadata(code, len(code)) {
		return ProductDescriptor{}, ErrInvalid
	}
	descriptor, ok := catalog.Product(code)
	if !ok || descriptor.Code != code || !validUnit(descriptor.Unit) {
		return ProductDescriptor{}, ErrInvalid
	}
	return descriptor, nil
}
func category(catalog CategoryCatalog, code string) error {
	if unavailable(catalog) || blank(code) || !validMetadata(code, len(code)) {
		return ErrInvalid
	}
	descriptor, ok := catalog.Category(code)
	if !ok || descriptor.Code != code {
		return ErrInvalid
	}
	return nil
}
func normalize(raw string, maximum int, plate bool) (string, error) {
	if !plate && len(raw) > maximum {
		return "", ErrInvalid
	}
	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		value := raw[i]
		if value < 0x20 || value > 0x7e {
			return "", ErrInvalid
		}
		if value == ' ' || value == '-' {
			continue
		}
		if value >= 'a' && value <= 'z' {
			value -= 'a' - 'A'
		}
		if plate && !(value >= 'A' && value <= 'Z' || value >= '0' && value <= '9') {
			return "", ErrInvalid
		}
		out = append(out, value)
	}
	if len(out) == 0 || plate && len(out) != 7 {
		return "", ErrInvalid
	}
	return string(out), nil
}
func NormalizePlate(raw string) (string, error)       { return normalize(raw, 7, true) }
func NormalizeFleetNumber(raw string) (string, error) { return normalize(raw, 20, false) }

// AssetID is intentionally distinct from personal vehicle IDs. A UUID cannot prove table provenance.
type AssetID struct {
	redacted
	value kernel.OrgID
}

func canonicalID(raw string) (kernel.OrgID, error) {
	id, err := kernel.ParseOrgID(raw)
	if err != nil || id.String() != raw {
		return kernel.OrgID{}, ErrInvalid
	}
	return id, nil
}
func ParseAssetID(raw string) (AssetID, error) {
	id, err := canonicalID(raw)
	if err != nil {
		return AssetID{}, err
	}
	return AssetID{value: id}, nil
}
func (id AssetID) Value() string { return id.value.String() }

type AssetRef struct {
	redacted
	organization kernel.OrgID
	id           AssetID
}

func NewAssetRef(scope kernel.OrgScope, id AssetID) (AssetRef, error) {
	if scope.Validate() != nil {
		return AssetRef{}, ErrScope
	}
	if id.value == (kernel.OrgID{}) {
		return AssetRef{}, ErrInvalid
	}
	return AssetRef{organization: scope.OrgID(), id: id}, nil
}
func (ref AssetRef) valid() bool {
	return ref.organization != (kernel.OrgID{}) && ref.id.value != (kernel.OrgID{})
}
func (ref AssetRef) Validate(scope kernel.OrgScope) error {
	if scope.Validate() != nil || ref.organization != scope.OrgID() {
		return ErrScope
	}
	if !ref.valid() {
		return ErrInvalid
	}
	return nil
}
func (ref AssetRef) OrganizationID() kernel.OrgID { return ref.organization }
func (ref AssetRef) ID() AssetID                  { return ref.id }

type TankRole string

const (
	Main      TankRole = "MAIN"
	Auxiliary TankRole = "AUXILIARY"
	Additive  TankRole = "ADDITIVE"
	Battery   TankRole = "BATTERY"
)

func validRole(role TankRole) bool {
	return role == Main || role == Auxiliary || role == Additive || role == Battery
}

type Tank struct {
	redacted
	role     TankRole
	capacity int64
	unit     Unit
	products []string
}
type TankSnapshot struct {
	redacted
	Role     TankRole
	Capacity int64
	Unit     Unit
	Products []string
}

func NewTank(catalog ProductCatalog, role TankRole, capacity string, codes []string) (Tank, error) {
	amount, err := exact.ParseQuantityMilli(capacity)
	if err != nil || !validRole(role) || len(codes) == 0 {
		return Tank{}, ErrInvalid
	}
	unit := Unit("")
	set := make(map[string]bool, len(codes))
	for _, code := range codes {
		descriptor, err := product(catalog, code)
		if err != nil || unit != "" && descriptor.Unit != unit {
			return Tank{}, ErrInvalid
		}
		unit = descriptor.Unit
		set[code] = true
	}
	keys := make([]string, 0, len(set))
	for code := range set {
		keys = append(keys, code)
	}
	sort.Strings(keys) // Valid UTF-8 sorts by Unicode scalar value, shared with Kotlin.
	return Tank{role: role, capacity: amount, unit: unit, products: keys}, nil
}
func (tank Tank) Snapshot() TankSnapshot {
	return TankSnapshot{Role: tank.role, Capacity: tank.capacity, Unit: tank.unit, Products: append([]string(nil), tank.products...)}
}

// ExpectedConsumption is an explicitly sourced comparison reference, never a measured figure.
type ExpectedConsumption struct {
	redacted
	product      string
	basis        Basis
	value        int64
	unit         Unit
	source, date string
}
type ExpectedSnapshot struct {
	redacted
	Product      string
	Basis        Basis
	Value        int64
	Unit         Unit
	Source, Date string
}

func positiveInteger(raw string) (int64, error) {
	if raw == "" || len(raw) > 19 || len(raw) > 1 && raw[0] == '0' {
		return 0, ErrInvalid
	}
	for _, digit := range raw {
		if digit < '0' || digit > '9' {
			return 0, ErrInvalid
		}
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, ErrInvalid
	}
	return value, nil
}
func validDate(raw string) bool {
	if len(raw) != 10 || raw[:4] == "0000" {
		return false
	}
	parsed, err := time.Parse("2006-01-02", raw)
	return err == nil && parsed.Format("2006-01-02") == raw && parsed.Year() >= 1 && parsed.Year() <= 9999
}
func NewExpectedConsumption(catalog ProductCatalog, code string, basis Basis, raw, source, date string) (ExpectedConsumption, error) {
	descriptor, err := product(catalog, code)
	if err != nil || !validBasis(basis) || !validMetadata(source, 280) || blank(source) || !validDate(date) {
		return ExpectedConsumption{}, ErrInvalid
	}
	value, err := positiveInteger(raw)
	if err != nil {
		return ExpectedConsumption{}, err
	}
	return ExpectedConsumption{product: code, basis: basis, value: value, unit: descriptor.Unit, source: source, date: date}, nil
}
func (reference ExpectedConsumption) Snapshot() ExpectedSnapshot {
	return ExpectedSnapshot{Product: reference.product, Basis: reference.basis, Value: reference.value, Unit: reference.unit, Source: reference.source, Date: reference.date}
}

type Profile struct {
	redacted
	Kind                                             Kind
	Category, Name, FleetNumber, Plate, SerialNumber string
	UsageBasis                                       Basis
	Tanks                                            []Tank
	Expected                                         []ExpectedConsumption
}
type Snapshot struct {
	redacted
	Ref                AssetRef
	Profile            Profile
	State              State
	FleetKey, PlateKey string
}
type Asset struct {
	redacted
	value Snapshot
}

func clone(profile Profile) Profile {
	profile.Tanks = append([]Tank(nil), profile.Tanks...)
	profile.Expected = append([]ExpectedConsumption(nil), profile.Expected...)
	return profile
}
func NewDraft(ref AssetRef, profile Profile, catalog Catalog) (Asset, error) {
	if !ref.valid() {
		return Asset{}, ErrInvalid
	}
	if !validKind(profile.Kind) || category(catalog, profile.Category) != nil || !validBasis(profile.UsageBasis) ||
		!validMetadata(profile.Name, 80) || blank(profile.Name) || !validMetadata(profile.SerialNumber, 40) {
		return Asset{}, ErrInvalid
	}
	fleetKey, plateKey := "", ""
	var err error
	if profile.FleetNumber != "" {
		fleetKey, err = NormalizeFleetNumber(profile.FleetNumber)
		if err != nil {
			return Asset{}, err
		}
	}
	if profile.Plate != "" {
		plateKey, err = NormalizePlate(profile.Plate)
		if err != nil {
			return Asset{}, err
		}
	}
	if fleetKey == "" && plateKey == "" && blank(profile.SerialNumber) {
		return Asset{}, ErrInvalid
	}
	accepted := map[string]Unit{}
	for _, tank := range profile.Tanks {
		checked, err := NewTank(catalog, tank.role, strconv.FormatInt(tank.capacity, 10), tank.products)
		if err != nil || checked.unit != tank.unit {
			return Asset{}, ErrInvalid
		}
		for _, code := range tank.products {
			if previous, exists := accepted[code]; exists && previous != tank.unit {
				return Asset{}, ErrInvalid
			}
			accepted[code] = tank.unit
		}
	}
	seen := map[string]bool{}
	for _, reference := range profile.Expected {
		checked, err := NewExpectedConsumption(catalog, reference.product, reference.basis, strconv.FormatInt(reference.value, 10), reference.source, reference.date)
		if err != nil || checked.unit != reference.unit || accepted[reference.product] != reference.unit || seen[reference.product] {
			return Asset{}, ErrInvalid
		}
		seen[reference.product] = true
	}
	return Asset{value: Snapshot{Ref: ref, Profile: clone(profile), State: Draft, FleetKey: fleetKey, PlateKey: plateKey}}, nil
}
func (asset Asset) Snapshot(scope kernel.OrgScope) (Snapshot, error) {
	if err := asset.value.Ref.Validate(scope); err != nil {
		return Snapshot{}, err
	}
	snapshot := asset.value
	snapshot.Profile = clone(snapshot.Profile)
	return snapshot, nil
}
func ChangeKind(current, next Kind, acceptedOperation bool) (Kind, error) {
	if !validKind(current) || !validKind(next) {
		return "", ErrInvalid
	}
	if current != next && acceptedOperation {
		return "", ErrKindLocked
	}
	return next, nil
}

// UnitRef and MeterFact describe source-resolved facts, not registration, eligibility or authority.
type UnitRef struct {
	redacted
	organization, id kernel.OrgID
}

func NewUnitRef(scope kernel.OrgScope, raw string) (UnitRef, error) {
	if scope.Validate() != nil {
		return UnitRef{}, ErrScope
	}
	id, err := canonicalID(raw)
	if err != nil {
		return UnitRef{}, err
	}
	return UnitRef{organization: scope.OrgID(), id: id}, nil
}

type MeterFact struct {
	redacted
	asset     AssetRef
	id        kernel.OrgID
	inService bool
}

func NewMeterFact(asset AssetRef, raw string, inService bool) (MeterFact, error) {
	if !asset.valid() {
		return MeterFact{}, ErrInvalid
	}
	id, err := canonicalID(raw)
	if err != nil {
		return MeterFact{}, err
	}
	return MeterFact{asset: asset, id: id, inService: inService}, nil
}

type ActivationFacts struct {
	redacted
	Unit   *UnitRef
	Meters []MeterFact
}
type Missing string

const (
	MissingUnit  Missing = "UNIT"
	MissingTank  Missing = "TANK"
	MissingMeter Missing = "METER"
)

type ActivationError struct{ missing []Missing }

func (*ActivationError) Error() string          { return ErrIncomplete.Error() }
func (*ActivationError) Unwrap() error          { return ErrIncomplete }
func (err *ActivationError) Missing() []Missing { return append([]Missing(nil), err.missing...) }
func (asset Asset) CheckActivation(scope kernel.OrgScope, facts ActivationFacts) error {
	if err := asset.value.Ref.Validate(scope); err != nil {
		return err
	}
	var missing []Missing
	if facts.Unit == nil {
		missing = append(missing, MissingUnit)
	} else {
		if facts.Unit.organization == (kernel.OrgID{}) || facts.Unit.id == (kernel.OrgID{}) {
			return ErrInvalid
		}
		if facts.Unit.organization != scope.OrgID() {
			return ErrScope
		}
	}
	hasMeter := false
	for _, meter := range facts.Meters {
		if !meter.asset.valid() || meter.id == (kernel.OrgID{}) {
			return ErrInvalid
		}
		if meter.asset != asset.value.Ref {
			return ErrScope
		}
		hasMeter = hasMeter || meter.inService
	}
	if asset.value.Profile.Kind != Implement {
		if len(asset.value.Profile.Tanks) == 0 {
			missing = append(missing, MissingTank)
		}
		if !hasMeter {
			missing = append(missing, MissingMeter)
		}
	}
	if len(missing) != 0 {
		return &ActivationError{missing: missing}
	}
	return nil
}
