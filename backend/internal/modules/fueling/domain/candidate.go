package domain

import (
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	exact "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgvalue/domain"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type scopedID struct {
	redacted
	org kernel.OrgID
	id  string
}

func (r scopedID) ID() string                   { return r.id }
func (r scopedID) OrganizationID() kernel.OrgID { return r.org }

type FuelingRef struct{ scopedID }
type EmployeeRef struct{ scopedID }
type MembershipRef struct{ scopedID }
type SupplierRef struct{ scopedID }
type TripRef struct{ scopedID }
type DocumentRef struct{ scopedID }
type InternalSourceTankRef struct{ scopedID }
type PublicStationRef struct {
	redacted
	id string
}

func (r PublicStationRef) ID() string { return r.id }

type DestinationTankRef struct {
	redacted
	asset fleet.AssetRef
	id    string
}
type TankGroupRef struct {
	redacted
	asset fleet.AssetRef
	id    string
}

func (r DestinationTankRef) ID() string            { return r.id }
func (r TankGroupRef) ID() string                  { return r.id }
func (r DestinationTankRef) Asset() fleet.AssetRef { return r.asset }
func (r TankGroupRef) Asset() fleet.AssetRef       { return r.asset }
func canonical(id string) bool                     { v, e := kernel.ParseOrgID(id); return e == nil && v.String() == id }
func scoped(scope kernel.OrgScope, id string) (scopedID, error) {
	if scope.Validate() != nil {
		return scopedID{}, ErrScope
	}
	if !canonical(id) {
		return scopedID{}, ErrInvalid
	}
	return scopedID{org: scope.OrgID(), id: id}, nil
}
func (r scopedID) validate(scope kernel.OrgScope) error {
	if scope.Validate() != nil {
		return ErrScope
	}
	if !canonical(r.id) || r.org == (kernel.OrgID{}) {
		return ErrInvalid
	}
	if r.org != scope.OrgID() {
		return ErrScope
	}
	return nil
}
func NewFuelingRef(scope kernel.OrgScope, id string) (FuelingRef, error) {
	r, e := scoped(scope, id)
	return FuelingRef{r}, e
}
func NewEmployeeRef(scope kernel.OrgScope, id string) (EmployeeRef, error) {
	r, e := scoped(scope, id)
	return EmployeeRef{r}, e
}
func NewMembershipRef(scope kernel.OrgScope, id string) (MembershipRef, error) {
	r, e := scoped(scope, id)
	return MembershipRef{r}, e
}
func NewSupplierRef(scope kernel.OrgScope, id string) (SupplierRef, error) {
	r, e := scoped(scope, id)
	return SupplierRef{r}, e
}
func NewTripRef(scope kernel.OrgScope, id string) (TripRef, error) {
	r, e := scoped(scope, id)
	return TripRef{r}, e
}
func NewDocumentRef(scope kernel.OrgScope, id string) (DocumentRef, error) {
	r, e := scoped(scope, id)
	return DocumentRef{r}, e
}
func NewInternalSourceTankRef(scope kernel.OrgScope, id string) (InternalSourceTankRef, error) {
	r, e := scoped(scope, id)
	return InternalSourceTankRef{r}, e
}
func NewPublicStationRef(id string) (PublicStationRef, error) {
	if !canonical(id) {
		return PublicStationRef{}, ErrInvalid
	}
	return PublicStationRef{id: id}, nil
}
func NewDestinationTankRef(scope kernel.OrgScope, asset fleet.AssetRef, id string) (DestinationTankRef, error) {
	if asset.Validate(scope) != nil {
		return DestinationTankRef{}, ErrScope
	}
	if !canonical(id) {
		return DestinationTankRef{}, ErrInvalid
	}
	return DestinationTankRef{asset: asset, id: id}, nil
}
func NewTankGroupRef(scope kernel.OrgScope, asset fleet.AssetRef, id string) (TankGroupRef, error) {
	if asset.Validate(scope) != nil {
		return TankGroupRef{}, ErrScope
	}
	if !canonical(id) {
		return TankGroupRef{}, ErrInvalid
	}
	return TankGroupRef{asset: asset, id: id}, nil
}

type Quantity struct {
	redacted
	product ProductDescriptor
	amount  int64
}

func NewQuantity(code ProductCode, unit Unit, raw string) (Quantity, error) {
	p, e := Lookup(code)
	if e != nil {
		return Quantity{}, e
	}
	if unit != p.unit {
		return Quantity{}, ErrUnitMismatch
	}
	n, e := exact.ParseQuantityMilli(raw)
	if e != nil {
		return Quantity{}, ErrInvalid
	}
	return Quantity{product: p, amount: n}, nil
}
func (q Quantity) Product() ProductDescriptor { return q.product }
func (q Quantity) Amount() int64              { return q.amount }

type Amounts struct {
	redacted
	paid                int64
	discount, unitPrice *int64
	sourceText          *string
	valid               bool
}
type AmountsSnapshot struct {
	redacted
	Paid                int64
	Discount, UnitPrice *int64
	SourcePriceText     *string
}

func copyPointer[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
func NewAmounts(paid string, discount, unitPrice, sourceText *string) (Amounts, error) {
	paidValue, e := exact.ParseCentavos(paid)
	if e != nil {
		return Amounts{}, ErrInvalid
	}
	a := Amounts{paid: paidValue, sourceText: copyPointer(sourceText), valid: true}
	if a.sourceText != nil && !utf8.ValidString(*a.sourceText) {
		return Amounts{}, ErrInvalid
	}
	if discount != nil {
		v, e := exact.ParseCentavos(*discount)
		if e != nil {
			return Amounts{}, ErrInvalid
		}
		a.discount = &v
	}
	if unitPrice != nil {
		v, e := exact.ParseUnitPriceMilliBRL(*unitPrice)
		if e != nil {
			return Amounts{}, ErrInvalid
		}
		a.unitPrice = &v
	}
	return a, nil
}
func (a Amounts) Snapshot() AmountsSnapshot {
	return AmountsSnapshot{Paid: a.paid, Discount: copyPointer(a.discount), UnitPrice: copyPointer(a.unitPrice), SourcePriceText: copyPointer(a.sourceText)}
}

type Fill string

const (
	Full    Fill = "FULL"
	Partial Fill = "PARTIAL"
	Unknown Fill = "UNKNOWN"
)

func ParseFill(raw string) (Fill, error) {
	f := Fill(raw)
	if f != Full && f != Partial && f != Unknown {
		return "", ErrInvalid
	}
	return f, nil
}

func (f Fill) Effective() (Fill, error) {
	if _, e := ParseFill(string(f)); e != nil {
		return "", e
	}
	if f == Unknown {
		return Partial, nil
	}
	return f, nil
}

type Payer string

const (
	Company  Payer = "COMPANY"
	Employee Payer = "EMPLOYEE"
)

func ParsePayer(raw string) (Payer, error) {
	p := Payer(raw)
	if p != Company && p != Employee {
		return "", ErrInvalid
	}
	return p, nil
}

type PaymentMethod string

const (
	Cash             PaymentMethod = "CASH"
	PIX              PaymentMethod = "PIX"
	DebitCard        PaymentMethod = "DEBIT_CARD"
	CreditCard       PaymentMethod = "CREDIT_CARD"
	FuelCard         PaymentMethod = "FUEL_CARD"
	SupplierAccount  PaymentMethod = "SUPPLIER_ACCOUNT"
	AgreementVoucher PaymentMethod = "AGREEMENT_VOUCHER"
	InternalStock    PaymentMethod = "INTERNAL_STOCK"
	Other            PaymentMethod = "OTHER"
)

func ParsePaymentMethod(raw string) (PaymentMethod, error) {
	p := PaymentMethod(raw)
	switch p {
	case Cash, PIX, DebitCard, CreditCard, FuelCard, SupplierAccount, AgreementVoucher, InternalStock, Other:
		return p, nil
	}
	return "", ErrInvalid
}

type Origin string

const (
	App          Origin = "APP"
	Panel        Origin = "PANEL"
	Import       Origin = "IMPORT"
	Agreement    Origin = "AGREEMENT"
	InternalTank Origin = "INTERNAL_TANK"
)

func ParseOrigin(raw string) (Origin, error) {
	o := Origin(raw)
	switch o {
	case App, Panel, Import, Agreement, InternalTank:
		return o, nil
	}
	return "", ErrInvalid
}

type Observation struct {
	redacted
	reading *meter.ReadingRef
	missing *meter.Missing
}

func validReading(r meter.ReadingRef) bool {
	m := r.Segment().Meter()
	return canonical(r.ID()) && canonical(r.Segment().ID()) && canonical(m.ID()) && (m.Kind() == meter.Odometer || m.Kind() == meter.HourMeter) && canonical(m.Asset().ID().Value())
}
func Observed(r meter.ReadingRef) (Observation, error) {
	if !validReading(r) {
		return Observation{}, ErrInvalid
	}
	return Observation{reading: &r}, nil
}
func Missing(m meter.Missing) (Observation, error) {
	if _, e := meter.NewMissing(m.Reason()); e != nil {
		return Observation{}, ErrInvalid
	}
	return Observation{missing: &m}, nil
}
func (o Observation) Reading() *meter.ReadingRef { return copyPointer(o.reading) }
func (o Observation) Missing() *meter.Missing    { return copyPointer(o.missing) }

type PlaceKind string

const (
	CatalogStation       PlaceKind = "CATALOG_STATION"
	OrganizationSupplier PlaceKind = "ORG_SUPPLIER"
	SourceInternalTank   PlaceKind = "INTERNAL_TANK"
	NotFound             PlaceKind = "NOT_FOUND"
)

func ParsePlaceKind(raw string) (PlaceKind, error) {
	p := PlaceKind(raw)
	switch p {
	case CatalogStation, OrganizationSupplier, SourceInternalTank, NotFound:
		return p, nil
	}
	return "", ErrInvalid
}

type Establishment struct {
	redacted
	kind     PlaceKind
	station  *PublicStationRef
	supplier *SupplierRef
	source   *InternalSourceTankRef
	name     string
}

func label(raw string) bool {
	if !utf8.ValidString(raw) || strings.TrimSpace(raw) == "" {
		return false
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func AtStation(r PublicStationRef) (Establishment, error) {
	if !canonical(r.id) {
		return Establishment{}, ErrInvalid
	}
	return Establishment{kind: CatalogStation, station: &r}, nil
}
func AtSupplier(r SupplierRef) (Establishment, error) {
	if !canonical(r.id) || r.org == (kernel.OrgID{}) {
		return Establishment{}, ErrInvalid
	}
	return Establishment{kind: OrganizationSupplier, supplier: &r}, nil
}
func AtInternalTank(r InternalSourceTankRef) (Establishment, error) {
	if !canonical(r.id) || r.org == (kernel.OrgID{}) {
		return Establishment{}, ErrInvalid
	}
	return Establishment{kind: SourceInternalTank, source: &r}, nil
}
func AtNamedPlace(name string) (Establishment, error) {
	if !label(name) {
		return Establishment{}, ErrInvalid
	}
	return Establishment{kind: NotFound, name: name}, nil
}
func (p Establishment) Kind() PlaceKind                        { return p.kind }
func (p Establishment) Name() string                           { return p.name }
func (p Establishment) Station() *PublicStationRef             { return copyPointer(p.station) }
func (p Establishment) Supplier() *SupplierRef                 { return copyPointer(p.supplier) }
func (p Establishment) InternalSource() *InternalSourceTankRef { return copyPointer(p.source) }

type CandidateInput struct {
	redacted
	Reference          FuelingRef
	Asset              fleet.AssetRef
	Responsible        EmployeeRef
	Author             MembershipRef
	Declared           *orgtime.Declaration
	Quantity           Quantity
	Amounts            Amounts
	Fill               Fill
	Payer              Payer
	Payment            PaymentMethod
	Observation        Observation
	Tank               *DestinationTankRef
	TankGroup          *TankGroupRef
	Trip               *TripRef
	Establishment      *Establishment
	ContractedSupplier *SupplierRef
	Origin             *Origin
}
type Candidate struct {
	redacted
	input CandidateInput
}

func copyCandidate(in CandidateInput) CandidateInput {
	in.Declared = copyPointer(in.Declared)
	in.Tank = copyPointer(in.Tank)
	in.TankGroup = copyPointer(in.TankGroup)
	in.Trip = copyPointer(in.Trip)
	in.Establishment = copyPointer(in.Establishment)
	in.ContractedSupplier = copyPointer(in.ContractedSupplier)
	in.Origin = copyPointer(in.Origin)
	return in
}
func NewCandidate(scope kernel.OrgScope, in CandidateInput) (Candidate, error) {
	in = copyCandidate(in)
	for _, r := range []scopedID{in.Reference.scopedID, in.Responsible.scopedID, in.Author.scopedID} {
		if e := r.validate(scope); e != nil {
			return Candidate{}, e
		}
	}
	if !canonical(in.Asset.ID().Value()) || in.Asset.OrganizationID() == (kernel.OrgID{}) {
		return Candidate{}, ErrInvalid
	}
	if in.Asset.Validate(scope) != nil {
		return Candidate{}, ErrScope
	}
	if in.Declared == nil {
		return Candidate{}, ErrInvalid
	}
	d, e := orgtime.ValidateDeclaration(*in.Declared)
	if e != nil {
		return Candidate{}, ErrInvalid
	}
	in.Declared = &d
	if _, e = NewQuantity(in.Quantity.product.code, in.Quantity.product.unit, strconv.FormatInt(in.Quantity.amount, 10)); e != nil {
		return Candidate{}, e
	}
	if !in.Amounts.valid {
		return Candidate{}, ErrInvalid
	}
	if _, e = ParseFill(string(in.Fill)); e != nil {
		return Candidate{}, e
	}
	if _, e = ParsePayer(string(in.Payer)); e != nil {
		return Candidate{}, e
	}
	if _, e = ParsePaymentMethod(string(in.Payment)); e != nil {
		return Candidate{}, e
	}
	if in.Origin != nil {
		if _, e = ParseOrigin(string(*in.Origin)); e != nil {
			return Candidate{}, e
		}
	}
	if (in.Observation.reading == nil) == (in.Observation.missing == nil) {
		return Candidate{}, ErrInvalid
	}
	if r := in.Observation.reading; r != nil {
		if !validReading(*r) {
			return Candidate{}, ErrInvalid
		}
		if r.Segment().Meter().Asset() != in.Asset {
			return Candidate{}, ErrScope
		}
	}
	if m := in.Observation.missing; m != nil {
		if _, e = meter.NewMissing(m.Reason()); e != nil {
			return Candidate{}, ErrInvalid
		}
	}
	if in.Tank != nil && in.TankGroup != nil {
		return Candidate{}, ErrInvalid
	}
	if t := in.Tank; t != nil {
		if !canonical(t.id) {
			return Candidate{}, ErrInvalid
		}
		if t.asset != in.Asset {
			return Candidate{}, ErrScope
		}
	}
	if t := in.TankGroup; t != nil {
		if !canonical(t.id) {
			return Candidate{}, ErrInvalid
		}
		if t.asset != in.Asset {
			return Candidate{}, ErrScope
		}
	}
	if in.Trip != nil {
		if e = in.Trip.validate(scope); e != nil {
			return Candidate{}, e
		}
	}
	if in.ContractedSupplier != nil {
		if e = in.ContractedSupplier.validate(scope); e != nil {
			return Candidate{}, e
		}
	}
	if p := in.Establishment; p != nil {
		switch p.kind {
		case CatalogStation:
			if p.station == nil || !canonical(p.station.id) {
				return Candidate{}, ErrInvalid
			}
		case OrganizationSupplier:
			if p.supplier == nil {
				return Candidate{}, ErrInvalid
			}
			if e = p.supplier.validate(scope); e != nil {
				return Candidate{}, e
			}
		case SourceInternalTank:
			if p.source == nil {
				return Candidate{}, ErrInvalid
			}
			if e = p.source.validate(scope); e != nil {
				return Candidate{}, e
			}
		case NotFound:
			if !label(p.name) {
				return Candidate{}, ErrInvalid
			}
		default:
			return Candidate{}, ErrInvalid
		}
	}
	return Candidate{input: in}, nil
}
func (c Candidate) Snapshot() CandidateInput { return copyCandidate(c.input) }
func (c Candidate) EffectiveFill() Fill {
	if c.input.Fill == Unknown {
		return Partial
	}
	return c.input.Fill
}
func (c Candidate) IsAdditive() bool { return c.input.Quantity.product.category == Additive }

type IssuerProjection struct {
	redacted
	document DocumentRef
	revision int64
	label    string
}

func NewIssuerProjection(scope kernel.OrgScope, document DocumentRef, revision int64, issuerLabel string) (IssuerProjection, error) {
	if e := document.validate(scope); e != nil {
		return IssuerProjection{}, e
	}
	if revision < 1 || !label(issuerLabel) {
		return IssuerProjection{}, ErrInvalid
	}
	return IssuerProjection{document: document, revision: revision, label: issuerLabel}, nil
}
func (p IssuerProjection) Document() DocumentRef { return p.document }
func (p IssuerProjection) Revision() int64       { return p.revision }
func (p IssuerProjection) Label() string         { return p.label }

type PurchasedTotal struct {
	redacted
	product  ProductCode
	category Category
	unit     Unit
	amount   big.Int
}

func SumByProduct(items []Quantity) ([]PurchasedTotal, error) {
	items = append([]Quantity(nil), items...)
	sums := map[ProductCode]*big.Int{}
	products := map[ProductCode]ProductDescriptor{}
	for _, q := range items {
		checked, e := NewQuantity(q.product.code, q.product.unit, strconv.FormatInt(q.amount, 10))
		if e != nil {
			return nil, e
		}
		if sums[checked.product.code] == nil {
			sums[checked.product.code] = new(big.Int)
		}
		sums[checked.product.code].Add(sums[checked.product.code], big.NewInt(checked.amount))
		products[checked.product.code] = checked.product
	}
	keys := make([]string, 0, len(sums))
	for k := range sums {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	out := make([]PurchasedTotal, 0, len(keys))
	for _, key := range keys {
		p := products[ProductCode(key)]
		out = append(out, PurchasedTotal{product: p.code, category: p.category, unit: p.unit, amount: *new(big.Int).Set(sums[p.code])})
	}
	return out, nil
}
func SumCompatible(items []Quantity) (PurchasedTotal, error) {
	items = append([]Quantity(nil), items...)
	if len(items) == 0 {
		return PurchasedTotal{}, ErrInvalid
	}
	groups, e := SumByProduct(items)
	if e != nil {
		return PurchasedTotal{}, e
	}
	total := PurchasedTotal{category: groups[0].category, unit: groups[0].unit}
	for _, g := range groups {
		if g.category != total.category || g.unit != total.unit {
			return PurchasedTotal{}, ErrInvalid
		}
		total.amount.Add(&total.amount, &g.amount)
	}
	return total, nil
}
func (t PurchasedTotal) Product() ProductCode { return t.product }
func (t PurchasedTotal) Category() Category   { return t.category }
func (t PurchasedTotal) Unit() Unit           { return t.unit }
func (t PurchasedTotal) Amount() *big.Int     { return new(big.Int).Set(&t.amount) }
