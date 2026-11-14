// Package domain validates pure enterprise supply candidates, never authority or acceptance.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"io"
	"log/slog"
	"sort"
)

var (
	ErrInvalid      = errors.New("fueling: invalid candidate")
	ErrScope        = errors.New("fueling: organization scope denied")
	ErrUnitMismatch = errors.New("fueling: product unit mismatch")
)

const CatalogVersion = "fuel-products-v1"

type ProductCode string
type Category string
type Unit = fleet.Unit

const (
	Ethanol              ProductCode = "ETHANOL"
	GasolineRegular      ProductCode = "GASOLINE_REGULAR"
	GasolineAdditived    ProductCode = "GASOLINE_ADDITIVED"
	GasolinePremiumGrade ProductCode = "GASOLINE_PREMIUM_GRADE"
	DieselS10            ProductCode = "DIESEL_S10"
	DieselS500           ProductCode = "DIESEL_S500"
	CNG                  ProductCode = "CNG"
	Electricity          ProductCode = "ELECTRICITY"
	ARLA32               ProductCode = "ARLA_32"
	LiquidFuel           Category    = "LIQUID_FUEL"
	GasFuel              Category    = "GAS_FUEL"
	Electric             Category    = "ELECTRICITY"
	Additive             Category    = "ADDITIVE"
)

type redacted struct{}

func (redacted) String() string                 { return "EnterpriseFuelingValue(redacted)" }
func (r redacted) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, r.String()) }
func (r redacted) LogValue() slog.Value         { return slog.StringValue(r.String()) }
func (r redacted) MarshalJSON() ([]byte, error) { return json.Marshal(r.String()) }

type ProductDescriptor struct {
	redacted
	code            ProductCode
	category        Category
	unit            Unit
	storage, family string
}

func (p ProductDescriptor) Code() ProductCode   { return p.code }
func (p ProductDescriptor) Category() Category  { return p.category }
func (p ProductDescriptor) Unit() Unit          { return p.unit }
func (p ProductDescriptor) StorageUnit() string { return p.storage }
func (p ProductDescriptor) Family() string      { return p.family }

var descriptors = [...]ProductDescriptor{
	{code: Ethanol, category: LiquidFuel, unit: fleet.Liter, storage: "ML", family: "ETHANOL"},
	{code: GasolineRegular, category: LiquidFuel, unit: fleet.Liter, storage: "ML", family: "GASOLINE"},
	{code: GasolineAdditived, category: LiquidFuel, unit: fleet.Liter, storage: "ML", family: "GASOLINE"},
	{code: GasolinePremiumGrade, category: LiquidFuel, unit: fleet.Liter, storage: "ML", family: "GASOLINE"},
	{code: DieselS10, category: LiquidFuel, unit: fleet.Liter, storage: "ML", family: "DIESEL"},
	{code: DieselS500, category: LiquidFuel, unit: fleet.Liter, storage: "ML", family: "DIESEL"},
	{code: CNG, category: GasFuel, unit: fleet.CubicMetre, storage: "MILLI_M3", family: "NATURAL_GAS"},
	{code: Electricity, category: Electric, unit: fleet.KilowattHour, storage: "WH", family: "ELECTRIC"},
	{code: ARLA32, category: Additive, unit: fleet.Liter, storage: "ML", family: "NONE"},
}

func Lookup(code ProductCode) (ProductDescriptor, error) {
	for _, p := range descriptors {
		if p.code == code {
			return p, nil
		}
	}
	return ProductDescriptor{}, ErrInvalid
}
func Products() []ProductDescriptor {
	out := append([]ProductDescriptor(nil), descriptors[:]...)
	sort.Slice(out, func(i, j int) bool { return out[i].code < out[j].code })
	return out
}

type Catalog struct{}

func (Catalog) Product(code string) (fleet.ProductDescriptor, bool) {
	p, err := Lookup(ProductCode(code))
	if err != nil {
		return fleet.ProductDescriptor{}, false
	}
	return fleet.ProductDescriptor{Code: string(p.code), Unit: p.unit}, true
}
