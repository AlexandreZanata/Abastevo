package kernel

import (
	"errors"
	"strings"
)

// Wire product vocabulary. The supported OpenAPI FuelProduct values; ANP source labels are a subset.
type Product string

const (
	Ethanol              Product = "ETHANOL"
	GasolineRegular      Product = "GASOLINE_REGULAR"
	GasolineAdditived    Product = "GASOLINE_ADDITIVED"
	GasolinePremiumGrade Product = "GASOLINE_PREMIUM_GRADE"
	DieselS500           Product = "DIESEL_S500"
	DieselS10            Product = "DIESEL_S10"
	CNG                  Product = "CNG"
	LPGP13               Product = "LPG_P13"
)

// Physical unit. Fixed per product in v1 (A04).
type Unit string

const (
	Liter      Unit = "L"
	CubicMetre Unit = "M3"
	Kg13       Unit = "KG_13"
)

// Exact money bounds in milli-BRL (B-BR-002, OpenAPI Money 1..1000000).
const (
	MinMilliBRL = 1
	MaxMilliBRL = 1000000
)

var (
	ErrUnknownProduct  = errors.New("kernel: unknown fuel product label")
	ErrUnknownUnit     = errors.New("kernel: unknown unit")
	ErrUnitMismatch    = errors.New("kernel: unit does not match product")
	ErrEmptyPrice      = errors.New("kernel: empty price text")
	ErrNegativePrice   = errors.New("kernel: negative price")
	ErrZeroPrice       = errors.New("kernel: zero price")
	ErrOverPrecision   = errors.New("kernel: nonzero precision beyond 3 decimals")
	ErrInvalidPrice    = errors.New("kernel: invalid price text")
	ErrPriceOutOfRange = errors.New("kernel: price outside 1..1000000 milli-BRL")
)

// accentFold covers the Latin-1 accented vowels/caps found in ANP source
// labels (e.g. ÓLEO). It is intentionally narrow: unknown scripts stay
// unknown instead of being coerced into a false match.
var accentFold = strings.NewReplacer(
	"À", "A", "Á", "A", "Â", "A", "Ã", "A", "Ä", "A", "Å", "A",
	"È", "E", "É", "E", "Ê", "E", "Ë", "E",
	"Ì", "I", "Í", "I", "Î", "I", "Ï", "I",
	"Ò", "O", "Ó", "O", "Ô", "O", "Õ", "O", "Ö", "O",
	"Ù", "U", "Ú", "U", "Û", "U", "Ü", "U",
	"Ç", "C", "Ñ", "N",
)

// normalizeLabel trims, collapses inner whitespace, folds accents and
// uppercases, so "  GASOLINA  COMUM " and "ÓLEO DIESEL S10" map stably.
func normalizeLabel(label string) string {
	s := accentFold.Replace(label)
	fields := strings.Fields(s)
	return strings.ToUpper(strings.Join(fields, " "))
}

var productByLabel = map[string]Product{
	"ETANOL":             Ethanol,
	"GASOLINA COMUM":     GasolineRegular,
	"GASOLINA ADITIVADA": GasolineAdditived,
	"OLEO DIESEL S500":   DieselS500,
	"OLEO DIESEL S10":    DieselS10,
	"GNV":                CNG,
	"GLP P13":            LPGP13,
}

// ParseProduct maps a source label to the wire product. Unknown labels fail
// with ErrUnknownProduct so importers quarantine instead of coercing.
func ParseProduct(label string) (Product, error) {
	if p, ok := productByLabel[normalizeLabel(label)]; ok {
		return p, nil
	}
	return "", ErrUnknownProduct
}

// Unit returns the fixed v1 unit for a product.
func (p Product) Unit() (Unit, error) {
	switch p {
	case Ethanol, GasolineRegular, GasolineAdditived, GasolinePremiumGrade, DieselS500, DieselS10:
		return Liter, nil
	case CNG:
		return CubicMetre, nil
	case LPGP13:
		return Kg13, nil
	default:
		return "", ErrUnknownProduct
	}
}

// ParseUnit accepts exactly the wire units.
func ParseUnit(s string) (Unit, error) {
	switch Unit(strings.ToUpper(strings.TrimSpace(s))) {
	case Liter:
		return Liter, nil
	case CubicMetre:
		return CubicMetre, nil
	case Kg13:
		return Kg13, nil
	default:
		return "", ErrUnknownUnit
	}
}

// Price is an exact validated amount: integer milli-BRL plus the original
// ANP decimal text. Floats never appear; parsing is pure string/integer math.
type Price struct {
	Product Product
	Unit    Unit
	Milli   int64
	Raw     string
}

// ParsePrice validates a source price for a product and unit (B-BR-002).
// Accepted text is digits with an optional single ',' decimal separator and
// surrounding spaces. Dots (thousands separators or float points), signs
// other than a leading '-', empty, zero and negative inputs fail with typed
// errors; nonzero precision beyond 3 decimals fails instead of rounding.
func ParsePrice(product Product, unit Unit, text string) (Price, error) {
	wantUnit, err := product.Unit()
	if err != nil {
		return Price{}, err
	}
	if unit != wantUnit {
		return Price{}, ErrUnitMismatch
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return Price{}, ErrEmptyPrice
	}
	negative := false
	if strings.HasPrefix(trimmed, "-") {
		negative = true
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
	}
	if strings.HasPrefix(trimmed, "+") {
		return Price{}, ErrInvalidPrice
	}
	intPart, fracPart := trimmed, ""
	if i := strings.IndexByte(trimmed, ','); i >= 0 {
		intPart, fracPart = trimmed[:i], trimmed[i+1:]
	}
	if intPart == "" || fracPart == "" && strings.Contains(trimmed, ",") {
		return Price{}, ErrInvalidPrice
	}
	if !allDigits(intPart) || (fracPart != "" && !allDigits(fracPart)) {
		return Price{}, ErrInvalidPrice
	}
	if len(fracPart) > 3 {
		if strings.Trim(fracPart[3:], "0") != "" {
			return Price{}, ErrOverPrecision
		}
		fracPart = fracPart[:3]
	}
	for len(fracPart) < 3 {
		fracPart += "0"
	}
	var milli int64
	for _, r := range intPart {
		milli = milli*10 + int64(r-'0')
		if milli > MaxMilliBRL {
			return Price{}, ErrPriceOutOfRange
		}
	}
	var frac int64
	for _, r := range fracPart {
		frac = frac*10 + int64(r-'0')
	}
	milli = milli*1000 + frac
	if negative {
		return Price{}, ErrNegativePrice
	}
	if milli < MinMilliBRL {
		if strings.Trim(intPart+fracPart, "0") == "" {
			return Price{}, ErrZeroPrice
		}
		return Price{}, ErrInvalidPrice
	}
	if milli > MaxMilliBRL {
		return Price{}, ErrPriceOutOfRange
	}
	return Price{Product: product, Unit: unit, Milli: milli, Raw: text}, nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// QuarantineCode maps a value error to the stable quarantine reason code
// recorded by the P02-T01 fixtures. Unknown errors map to "invalid-value"
// so nothing quarantines silently without a code.
func QuarantineCode(err error) string {
	switch {
	case errors.Is(err, ErrUnknownProduct):
		return "unknown-fuel-label"
	case errors.Is(err, ErrUnknownUnit):
		return "unknown-unit"
	case errors.Is(err, ErrUnitMismatch):
		return "unit-mismatch"
	case errors.Is(err, ErrEmptyPrice):
		return "missing-price"
	case errors.Is(err, ErrNegativePrice):
		return "negative-price"
	case errors.Is(err, ErrZeroPrice):
		return "zero-price"
	case errors.Is(err, ErrOverPrecision):
		return "over-precision"
	case errors.Is(err, ErrPriceOutOfRange):
		return "over-range"
	case errors.Is(err, ErrInvalidPrice):
		return "invalid-price"
	case errors.Is(err, ErrUnknownCondition):
		return "unknown-condition"
	case errors.Is(err, ErrConditionQualifier):
		return "condition-qualifier"
	case errors.Is(err, ErrInvalidCNPJ):
		return "cnpj-checksum-invalid"
	default:
		return "invalid-value"
	}
}
