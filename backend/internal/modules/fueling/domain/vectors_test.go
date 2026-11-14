package domain_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

type fields map[string]string

var errUnknownEnumSelector = errors.New("fixture: unknown enum namespace")

func validateEnumSelector(raw string) error {
	switch raw {
	case "FILL", "PAYER", "PAYMENT", "ORIGIN", "PLACE":
		return nil
	}
	return errUnknownEnumSelector
}
func TestSharedReaderRejectsUnknownEnumSelector(t *testing.T) {
	for _, raw := range []string{"", "UNKNOWN", "fill", "FILL ", "PRODUCT"} {
		if !errors.Is(validateEnumSelector(raw), errUnknownEnumSelector) {
			t.Fatal("malformed fixture namespace reached domain dispatch")
		}
	}
	for _, raw := range []string{"FILL", "PAYER", "PAYMENT", "ORIGIN", "PLACE"} {
		if validateEnumSelector(raw) != nil {
			t.Fatal("documented fixture namespace refused")
		}
	}
}

func (f fields) get(key, other string) string {
	if s, ok := f[key]; ok {
		return s
	}
	return other
}
func fieldMap(t *testing.T, raw string) fields {
	t.Helper()
	out := fields{}
	for _, part := range strings.Split(raw, ";") {
		k, v, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(k) == "" {
			t.Fatal("malformed fixture field")
		}
		if _, has := out[k]; has {
			t.Fatal("duplicate fixture field")
		}
		out[k] = v
	}
	return out
}
func text(t *testing.T, raw string) string {
	t.Helper()
	v, e := hex.DecodeString(raw)
	if e != nil || raw != strings.ToLower(raw) || !utf8.Valid(v) {
		t.Fatal("malformed fixture UTF8 hex")
	}
	return string(v)
}
func fixtureID(alias string) string {
	n, ok := map[string]int{"A": 1, "B": 2, "X": 11, "Y": 12, "E": 21, "J": 22, "U": 31, "V": 32, "M": 41, "N": 42, "S": 51, "T": 52, "R": 61, "W": 62, "K": 71, "G": 72, "H": 81, "P": 91, "Q": 92, "Z": 111, "C": 112, "L": 121, "D": 122, "F": 131}[alias]
	if ok {
		return uuid(n)
	}
	if alias == "?" {
		return ""
	}
	return alias
}
func alias(id string) string {
	for _, a := range strings.Fields("A B X Y E J U V M N S T R W K G H P Q Z C L D F") {
		if fixtureID(a) == id {
			return a
		}
	}
	return id
}
func vectorScope(raw string) (kernel.OrgScope, error) {
	id, e := kernel.ParseOrgID(fixtureID(raw))
	if e != nil {
		return kernel.OrgScope{}, fuel.ErrScope
	}
	s, e := kernel.NewOrgScope(id)
	if e != nil {
		return kernel.OrgScope{}, fuel.ErrScope
	}
	return s, nil
}
func vectorAsset(s kernel.OrgScope, id string) (fleet.AssetRef, error) {
	v, e := fleet.ParseAssetID(fixtureID(id))
	if e != nil {
		return fleet.AssetRef{}, fuel.ErrInvalid
	}
	a, e := fleet.NewAssetRef(s, v)
	if e != nil {
		return fleet.AssetRef{}, fuel.ErrScope
	}
	return a, nil
}
func optional(f fields, key string) *string {
	v, ok := f[key]
	if !ok || v == "?" {
		return nil
	}
	return &v
}
func producerError(e error) error {
	if e == nil {
		return nil
	}
	if errors.Is(e, fleet.ErrScope) || errors.Is(e, meter.ErrScope) {
		return fuel.ErrScope
	}
	if errors.Is(e, fleet.ErrInvalid) || errors.Is(e, meter.ErrInvalid) || errors.Is(e, orgtime.ErrInvalid) {
		return fuel.ErrInvalid
	}
	return e
}
func vectorCandidate(t *testing.T, args fields) (kernel.OrgScope, fuel.Candidate, error) {
	t.Helper()
	f := fields{"org": "A", "id": "F", "asset": "X", "responsible": "E", "author": "U", "product": "ETHANOL", "unit": "L", "quantity": "150000", "paid": "97350", "fill": "PARTIAL", "payer": "COMPANY", "payment": "CASH", "origin": "APP", "effective": "2026-11-14T12:00:00Z", "entry": "UTC", "offset": "0", "reading": "REFERENCE"}
	for k, v := range args {
		f[k] = v
	}
	if f["omit"] != "" {
		for _, key := range strings.Split(f["omit"], ",") {
			delete(f, key)
		}
	}
	org := f.get("org", "A")
	s, e := vectorScope(org)
	if e != nil {
		return s, fuel.Candidate{}, e
	}
	in := fuel.CandidateInput{}
	if id, ok := f["id"]; ok {
		owner, er := vectorScope(f.get("id_org", org))
		if er != nil {
			return s, fuel.Candidate{}, er
		}
		in.Reference, e = fuel.NewFuelingRef(owner, fixtureID(id))
		if e != nil {
			return s, fuel.Candidate{}, e
		}
	}
	if id, ok := f["asset"]; ok {
		owner, er := vectorScope(f.get("asset_org", org))
		if er != nil {
			return s, fuel.Candidate{}, er
		}
		in.Asset, e = vectorAsset(owner, id)
		if e != nil {
			return s, fuel.Candidate{}, e
		}
	}
	if id, ok := f["responsible"]; ok {
		owner, er := vectorScope(f.get("responsible_org", org))
		if er != nil {
			return s, fuel.Candidate{}, er
		}
		in.Responsible, e = fuel.NewEmployeeRef(owner, fixtureID(id))
		if e != nil {
			return s, fuel.Candidate{}, e
		}
	}
	if id, ok := f["author"]; ok {
		owner, er := vectorScope(f.get("author_org", org))
		if er != nil {
			return s, fuel.Candidate{}, er
		}
		in.Author, e = fuel.NewMembershipRef(owner, fixtureID(id))
		if e != nil {
			return s, fuel.Candidate{}, e
		}
	}
	if raw, ok := f["effective"]; ok {
		at, er := orgtime.ParseInstant(raw)
		if er != nil {
			return s, fuel.Candidate{}, fuel.ErrInvalid
		}
		offset, er := strconv.ParseInt(f.get("offset", "0"), 10, 64)
		if er != nil {
			return s, fuel.Candidate{}, fuel.ErrInvalid
		}
		in.Declared = &orgtime.Declaration{OccurredAt: at, EntryZone: f.get("entry", "UTC"), DeviceOffsetSeconds: offset}
	}
	if _, ok := f["quantity"]; ok {
		in.Quantity, e = fuel.NewQuantity(fuel.ProductCode(f["product"]), fuel.Unit(f["unit"]), f["quantity"])
		if e != nil {
			return s, fuel.Candidate{}, e
		}
	}
	if _, ok := f["paid"]; ok {
		var priceText *string
		if raw := optional(f, "price_text_hex"); raw != nil {
			decoded := text(t, *raw)
			priceText = &decoded
		}
		in.Amounts, e = fuel.NewAmounts(f["paid"], optional(f, "discount"), optional(f, "unit_price"), priceText)
		if e != nil {
			return s, fuel.Candidate{}, e
		}
	}
	in.Fill = fuel.Fill(f["fill"])
	in.Payer = fuel.Payer(f["payer"])
	in.Payment = fuel.PaymentMethod(f["payment"])
	if raw := optional(f, "origin"); raw != nil {
		o := fuel.Origin(*raw)
		in.Origin = &o
	}
	switch f["reading"] {
	case "MISSING":
		m, er := meter.NewMissing(text(t, f.get("missing_hex", hex.EncodeToString([]byte("Display unavailable")))))
		if er != nil {
			return s, fuel.Candidate{}, producerError(er)
		}
		in.Observation, e = fuel.Missing(m)
		if e != nil {
			return s, fuel.Candidate{}, e
		}
	case "REFERENCE":
		owner, er := vectorScope(f.get("reading_org", org))
		if er != nil {
			return s, fuel.Candidate{}, er
		}
		a, er := vectorAsset(owner, f.get("reading_asset", "X"))
		if er != nil {
			return s, fuel.Candidate{}, er
		}
		m, er := meter.NewMeterRef(owner, a, fixtureID(f.get("reading_meter", "M")), meter.Kind(f.get("reading_kind", "ODOMETER")), nil)
		if er != nil {
			return s, fuel.Candidate{}, producerError(er)
		}
		seg, er := meter.NewSegmentRef(owner, m, fixtureID(f.get("reading_segment", "S")))
		if er != nil {
			return s, fuel.Candidate{}, producerError(er)
		}
		r, er := meter.NewReadingRef(owner, seg, fixtureID(f.get("reading_id", "R")))
		if er != nil {
			return s, fuel.Candidate{}, producerError(er)
		}
		in.Observation, e = fuel.Observed(r)
		if e != nil {
			return s, fuel.Candidate{}, e
		}
	case "?", "":
	default:
		return s, fuel.Candidate{}, fuel.ErrInvalid
	}
	if id := optional(f, "tank"); id != nil {
		owner, er := vectorScope(f.get("tank_org", org))
		if er != nil {
			return s, fuel.Candidate{}, er
		}
		a, er := vectorAsset(owner, f.get("tank_asset", "X"))
		if er != nil {
			return s, fuel.Candidate{}, er
		}
		v, er := fuel.NewDestinationTankRef(owner, a, fixtureID(*id))
		if er != nil {
			return s, fuel.Candidate{}, er
		}
		in.Tank = &v
	}
	if id := optional(f, "group"); id != nil {
		owner, er := vectorScope(f.get("group_org", org))
		if er != nil {
			return s, fuel.Candidate{}, er
		}
		a, er := vectorAsset(owner, f.get("group_asset", "X"))
		if er != nil {
			return s, fuel.Candidate{}, er
		}
		v, er := fuel.NewTankGroupRef(owner, a, fixtureID(*id))
		if er != nil {
			return s, fuel.Candidate{}, er
		}
		in.TankGroup = &v
	}
	if id := optional(f, "trip"); id != nil {
		owner, er := vectorScope(f.get("trip_org", org))
		if er != nil {
			return s, fuel.Candidate{}, er
		}
		v, er := fuel.NewTripRef(owner, fixtureID(*id))
		if er != nil {
			return s, fuel.Candidate{}, er
		}
		in.Trip = &v
	}
	if id := optional(f, "contracted"); id != nil {
		owner, er := vectorScope(f.get("contracted_org", org))
		if er != nil {
			return s, fuel.Candidate{}, er
		}
		v, er := fuel.NewSupplierRef(owner, fixtureID(*id))
		if er != nil {
			return s, fuel.Candidate{}, er
		}
		in.ContractedSupplier = &v
	}
	if kind := optional(f, "place"); kind != nil {
		owner, er := vectorScope(f.get("place_org", org))
		if er != nil {
			return s, fuel.Candidate{}, er
		}
		var p fuel.Establishment
		switch fuel.PlaceKind(*kind) {
		case fuel.CatalogStation:
			r, er := fuel.NewPublicStationRef(fixtureID(f.get("place_ref", "P")))
			if er != nil {
				return s, fuel.Candidate{}, er
			}
			p, e = fuel.AtStation(r)
		case fuel.OrganizationSupplier:
			r, er := fuel.NewSupplierRef(owner, fixtureID(f.get("place_ref", "Z")))
			if er != nil {
				return s, fuel.Candidate{}, er
			}
			p, e = fuel.AtSupplier(r)
		case fuel.SourceInternalTank:
			r, er := fuel.NewInternalSourceTankRef(owner, fixtureID(f.get("place_ref", "H")))
			if er != nil {
				return s, fuel.Candidate{}, er
			}
			p, e = fuel.AtInternalTank(r)
		case fuel.NotFound:
			p, e = fuel.AtNamedPlace(text(t, f.get("place_name_hex", "")))
		default:
			_, e = fuel.ParsePlaceKind(*kind)
		}
		if e != nil {
			return s, fuel.Candidate{}, e
		}
		in.Establishment = &p
	}
	c, e := fuel.NewCandidate(s, in)
	return s, c, e
}
func placeFields(in fuel.CandidateInput) fields {
	out := fields{"place": "?", "place_ref": "?", "place_name_hex": "", "contracted": "?"}
	if in.ContractedSupplier != nil {
		out["contracted"] = alias(in.ContractedSupplier.ID())
	}
	if in.Establishment != nil {
		p := in.Establishment
		out["place"] = string(p.Kind())
		out["place_name_hex"] = hex.EncodeToString([]byte(p.Name()))
		if r := p.Station(); r != nil {
			out["place_ref"] = alias(r.ID())
		}
		if r := p.Supplier(); r != nil {
			out["place_ref"] = alias(r.ID())
		}
		if r := p.InternalSource(); r != nil {
			out["place_ref"] = alias(r.ID())
		}
	}
	return out
}
func candidateFields(c fuel.Candidate) fields {
	in := c.Snapshot()
	a := in.Amounts.Snapshot()
	out := placeFields(in)
	out["ready"] = "true"
	out["org"] = alias(in.Reference.OrganizationID().String())
	out["id"] = alias(in.Reference.ID())
	out["asset"] = alias(in.Asset.ID().Value())
	out["responsible"] = alias(in.Responsible.ID())
	out["author"] = alias(in.Author.ID())
	out["product"] = string(in.Quantity.Product().Code())
	out["unit"] = string(in.Quantity.Product().Unit())
	out["quantity"] = strconv.FormatInt(in.Quantity.Amount(), 10)
	out["paid"] = strconv.FormatInt(a.Paid, 10)
	out["discount"], out["unit_price"], out["price_text_hex"] = "?", "?", "?"
	if a.Discount != nil {
		out["discount"] = strconv.FormatInt(*a.Discount, 10)
	}
	if a.UnitPrice != nil {
		out["unit_price"] = strconv.FormatInt(*a.UnitPrice, 10)
	}
	if a.SourcePriceText != nil {
		out["price_text_hex"] = hex.EncodeToString([]byte(*a.SourcePriceText))
	}
	out["fill"] = string(in.Fill)
	out["effective_fill"] = string(c.EffectiveFill())
	out["payer"] = string(in.Payer)
	out["payment"] = string(in.Payment)
	out["reading"], out["reading_id"], out["missing_hex"] = "?", "?", ""
	if r := in.Observation.Reading(); r != nil {
		out["reading"] = "REFERENCE"
		out["reading_id"] = alias(r.ID())
	}
	if m := in.Observation.Missing(); m != nil {
		out["reading"] = "MISSING"
		out["missing_hex"] = hex.EncodeToString([]byte(m.Reason()))
	}
	out["tank"], out["group"], out["trip"], out["origin"] = "?", "?", "?", "?"
	if in.Tank != nil {
		out["tank"] = alias(in.Tank.ID())
	}
	if in.TankGroup != nil {
		out["group"] = alias(in.TankGroup.ID())
	}
	if in.Trip != nil {
		out["trip"] = alias(in.Trip.ID())
	}
	if in.Origin != nil {
		out["origin"] = string(*in.Origin)
	}
	out["effective"] = orgtime.FormatInstant(in.Declared.OccurredAt)
	out["entry"] = in.Declared.EntryZone
	out["offset"] = strconv.FormatInt(in.Declared.DeviceOffsetSeconds, 10)
	out["additive"] = strconv.FormatBool(c.IsAdditive())
	return out
}
func runVector(t *testing.T, op string, f fields) (fields, error) {
	t.Helper()
	switch op {
	case "CATALOG":
		if f["all"] != "true" {
			t.Fatal("unknown catalog fixture syntax")
		}
		ps := fuel.Products()
		keys := make([]string, 0, len(ps))
		for _, p := range ps {
			keys = append(keys, string(p.Code()))
		}
		return fields{"version": fuel.CatalogVersion, "count": strconv.Itoa(len(ps)), "keys": strings.Join(keys, ",")}, nil
	case "PRODUCT":
		p, e := fuel.Lookup(fuel.ProductCode(f["key"]))
		if e != nil {
			return nil, e
		}
		return fields{"version": fuel.CatalogVersion, "key": string(p.Code()), "category": string(p.Category()), "unit": string(p.Unit()), "storage": p.StorageUnit(), "family": p.Family(), "additive": strconv.FormatBool(p.Category() == fuel.Additive)}, nil
	case "ENUM":
		raw := text(t, f["value_hex"])
		var value string
		var err error
		out := fields{}
		switch f["kind"] {
		case "FILL":
			v, e := fuel.ParseFill(raw)
			value, err = string(v), e
			if err == nil {
				effective, e := v.Effective()
				out["effective"], err = string(effective), e
			}
		case "PAYER":
			v, e := fuel.ParsePayer(raw)
			value, err = string(v), e
		case "PAYMENT":
			v, e := fuel.ParsePaymentMethod(raw)
			value, err = string(v), e
		case "ORIGIN":
			v, e := fuel.ParseOrigin(raw)
			value, err = string(v), e
		case "PLACE":
			v, e := fuel.ParsePlaceKind(raw)
			value, err = string(v), e
		default:
			t.Fatal("unknown fixture enum namespace")
		}
		out["value"] = value
		return out, err
	case "QUANTITY":
		q, e := fuel.NewQuantity(fuel.ProductCode(f["product"]), fuel.Unit(f["unit"]), f["amount"])
		if e != nil {
			return nil, e
		}
		return fields{"product": string(q.Product().Code()), "category": string(q.Product().Category()), "unit": string(q.Product().Unit()), "amount": strconv.FormatInt(q.Amount(), 10)}, nil
	case "CANDIDATE", "ROLES":
		s, c, e := vectorCandidate(t, f)
		if e != nil {
			return nil, e
		}
		if op == "CANDIDATE" {
			return candidateFields(c), nil
		}
		out := placeFields(c.Snapshot())
		out["doc"], out["doc_revision"], out["issuer_hex"] = "?", "?", "?"
		if raw := optional(f, "doc"); raw != nil {
			owner, er := vectorScope(f.get("doc_org", f.get("org", "A")))
			if er != nil {
				return nil, er
			}
			doc, er := fuel.NewDocumentRef(owner, fixtureID(*raw))
			if er != nil {
				return nil, er
			}
			revision, er := strconv.ParseInt(f.get("doc_revision", "1"), 10, 64)
			if er != nil {
				t.Fatal("invalid typed revision fixture")
			}
			p, er := fuel.NewIssuerProjection(s, doc, revision, text(t, f.get("issuer_hex", hex.EncodeToString([]byte("Synthetic document issuer")))))
			if er != nil {
				return nil, er
			}
			out["doc"] = alias(p.Document().ID())
			out["doc_revision"] = strconv.FormatInt(p.Revision(), 10)
			out["issuer_hex"] = hex.EncodeToString([]byte(p.Label()))
		}
		return out, nil
	case "TOTAL":
		var items []fuel.Quantity
		if f["items"] != "" {
			for _, tuple := range strings.Split(f["items"], "~") {
				p := strings.Split(tuple, ",")
				if len(p) != 3 {
					t.Fatal("malformed quantity tuple")
				}
				q, e := fuel.NewQuantity(fuel.ProductCode(p[0]), fuel.Unit(p[1]), p[2])
				if e != nil {
					return nil, e
				}
				items = append(items, q)
			}
		}
		switch f["mode"] {
		case "PRODUCT":
			totals, e := fuel.SumByProduct(items)
			if e != nil {
				return nil, e
			}
			groups := make([]string, 0, len(totals))
			for _, v := range totals {
				groups = append(groups, string(v.Product())+":"+string(v.Category())+":"+string(v.Unit())+":"+v.Amount().String())
			}
			return fields{"count": strconv.Itoa(len(totals)), "groups": strings.Join(groups, "~")}, nil
		case "COMPATIBLE":
			v, e := fuel.SumCompatible(items)
			if e != nil {
				return nil, e
			}
			return fields{"category": string(v.Category()), "unit": string(v.Unit()), "amount": v.Amount().String()}, nil
		default:
			t.Fatal("unknown total fixture mode")
		}
	}
	t.Fatal("unknown operation")
	return nil, nil
}
func errorFields(e error) fields {
	switch {
	case errors.Is(e, fuel.ErrInvalid):
		return fields{"error": "INVALID"}
	case errors.Is(e, fuel.ErrScope):
		return fields{"error": "SCOPE"}
	case errors.Is(e, fuel.ErrUnitMismatch):
		return fields{"error": "PRODUCT_UNIT_MISMATCH"}
	}
	return fields{"error": "UNEXPECTED"}
}
func TestSharedFuelingVectors(t *testing.T) {
	raw, e := os.ReadFile("../../../../../contracts/testdata/enterprise/fueling/fueling-v1.psv")
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "bca4934b4a21935cd1a1728c087fadef5d0935fa39f596b261f844464685395a" {
		t.Fatal("shared corpus hash changed")
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		t.Fatal("invalid shared corpus header")
	}
	counts := map[string]int{}
	wantCounts := map[string]int{"CATALOG": 1, "PRODUCT": 15, "ENUM": 38, "QUANTITY": 27, "CANDIDATE": 91, "ROLES": 15, "TOTAL": 13}
	ids := map[string]bool{}
	count := 0
	allowed := map[string]string{"CATALOG": "all", "PRODUCT": "key", "ENUM": "kind value_hex", "QUANTITY": "product unit amount", "TOTAL": "mode items"}
	candidateKeys := "org id id_org asset asset_org responsible responsible_org author author_org product unit quantity paid discount unit_price price_text_hex fill payer payment origin effective entry offset omit reading missing_hex reading_org reading_asset reading_meter reading_segment reading_id reading_kind tank tank_org tank_asset group group_org group_asset trip trip_org contracted contracted_org place place_org place_ref place_name_hex"
	allowed["CANDIDATE"] = candidateKeys
	allowed["ROLES"] = candidateKeys + " doc doc_org doc_revision issuer_hex"
	for scanner.Scan() {
		p := strings.Split(scanner.Text(), "|")
		if len(p) != 4 || strings.TrimSpace(p[0]) == "" || ids[p[0]] {
			t.Fatal("malformed/duplicate shared case")
		}
		if _, ok := wantCounts[p[1]]; !ok {
			t.Fatal("unknown shared operation")
		}
		ids[p[0]] = true
		counts[p[1]]++
		count++
		args, want := fieldMap(t, p[2]), fieldMap(t, p[3])
		if p[1] == "ENUM" && validateEnumSelector(args["kind"]) != nil {
			t.Fatal("malformed fixture enum namespace")
		}
		for k := range args {
			if !strings.Contains(" "+allowed[p[1]]+" ", " "+k+" ") {
				t.Fatal("unknown fixture argument")
			}
		}
		t.Run(p[0], func(t *testing.T) {
			got, err := runVector(t, p[1], args)
			if err != nil {
				got = errorFields(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("shared mismatch: got %v expected %v", got, want)
			}
		})
	}
	if e = scanner.Err(); e != nil {
		t.Fatal(e)
	}
	if count != 200 || !reflect.DeepEqual(counts, wantCounts) {
		t.Fatal("incomplete corpus", count, counts)
	}
}
