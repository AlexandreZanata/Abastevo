package application

import (
	"encoding/json"
	"strings"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

// payloadKeys is the exact schema 1 payload (EDR-175). Identity, author,
// declaration, receipt, state and origin belong to the envelope or server.
var payloadKeys = []string{"asset_id", "responsible_employee_id", "product", "unit", "quantity", "paid_total", "discount", "unit_price", "unit_price_text", "fill", "payer", "payment_method", "reading", "missing_reading_reason", "place", "trip_id", "contracted_supplier_id"}

var readingKeys = []string{"meter_id", "segment_id", "reading_id", "kind", "value"}

// Places whose owners do not exist yet: recognised, never accepted as facts.
var retainedPlaces = map[string]bool{string(fuel.CatalogStation): true, string(fuel.OrganizationSupplier): true, string(fuel.SourceInternalTank): true}

type decoder struct {
	fields      map[string]json.RawMessage
	malformed   bool
	unsupported bool
}

func object(raw json.RawMessage, keys []string) (map[string]json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil || len(fields) != len(keys) {
		return nil, false
	}
	for _, key := range keys {
		if _, ok := fields[key]; !ok {
			return nil, false
		}
	}
	return fields, true
}

func isNull(raw json.RawMessage) bool { return strings.TrimSpace(string(raw)) == "null" }

func (d *decoder) text(fields map[string]json.RawMessage, key string) string {
	var value string
	if raw, ok := fields[key]; !ok || isNull(raw) || json.Unmarshal(raw, &value) != nil {
		d.malformed = true
	}
	return value
}

func (d *decoder) optional(fields map[string]json.RawMessage, key string) *string {
	raw, ok := fields[key]
	if !ok {
		d.malformed = true
		return nil
	}
	if isNull(raw) {
		return nil
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		d.malformed = true
		return nil
	}
	return &value
}

// retained marks a recognised optional reference whose owner is absent: a
// canonical identifier stays unavailable, anything else is malformed.
func (d *decoder) retained(key string) {
	if value := d.optional(d.fields, key); value != nil {
		if canonicalID(*value) {
			d.unsupported = true
		} else {
			d.malformed = true
		}
	}
}

// ParseOperation decodes one fueling.submit schema 1 operation through the
// FUEL-001 candidate (B-BR-EMP-263/265/267). ErrMalformed is final;
// ErrUnsupported keeps a recognised future form unavailable.
func ParseOperation(op protocol.Operation) (Command, error) {
	v := op.Snapshot()
	if v.Type != OperationType || !op.SupportsSchema() || v.Entity.Type != EntityType || v.Entity.ExpectedRevision != nil || !canonicalID(v.Entity.ID) {
		return Command{}, ErrMalformed
	}
	org, err := kernel.ParseOrgID(v.OrgID)
	if err != nil {
		return Command{}, ErrMalformed
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		return Command{}, ErrMalformed
	}
	fields, ok := object(op.PayloadBytes(), payloadKeys)
	if !ok {
		return Command{}, ErrMalformed
	}
	d := &decoder{fields: fields}
	asset, responsible := d.text(fields, "asset_id"), d.text(fields, "responsible_employee_id")
	product, unit, quantity, paid := d.text(fields, "product"), d.text(fields, "unit"), d.text(fields, "quantity"), d.text(fields, "paid_total")
	discount, price, priceText := d.optional(fields, "discount"), d.optional(fields, "unit_price"), d.optional(fields, "unit_price_text")
	fill, payer, payment := d.text(fields, "fill"), d.text(fields, "payer"), d.text(fields, "payment_method")
	reason := d.optional(fields, "missing_reading_reason")
	d.retained("trip_id")
	d.retained("contracted_supplier_id")
	if len(v.Attachments) != 0 || len(v.DependsOn) != 0 || len(v.SoftReferences) != 0 {
		d.unsupported = true
	}
	if !canonicalID(asset) || !canonicalID(responsible) || priceText != nil && (price == nil || *priceText == "" || strings.ContainsRune(*priceText, 0)) {
		d.malformed = true
	}
	var reading *Reading
	if raw := fields["reading"]; !isNull(raw) {
		r, ok := object(raw, readingKeys)
		if !ok {
			return Command{}, ErrMalformed
		}
		kind, value := d.text(r, "kind"), d.text(r, "value")
		count, err := meter.ParseBase(meter.Kind(kind), value, nil)
		if err != nil {
			d.malformed = true
		}
		reading = &Reading{MeterID: d.text(r, "meter_id"), SegmentID: d.text(r, "segment_id"), ReadingID: d.text(r, "reading_id"), Value: count}
	}
	var place *string
	if raw := fields["place"]; !isNull(raw) {
		var shape map[string]json.RawMessage
		if json.Unmarshal(raw, &shape) != nil || shape == nil {
			return Command{}, ErrMalformed
		}
		switch kind := d.text(shape, "kind"); {
		case kind == string(fuel.NotFound):
			p, ok := object(raw, []string{"kind", "name"})
			if !ok {
				return Command{}, ErrMalformed
			}
			name := d.text(p, "name")
			place = &name
		case retainedPlaces[kind]:
			d.unsupported = true
		default:
			d.malformed = true
		}
	}
	if d.malformed {
		return Command{}, ErrMalformed
	}
	input, err := candidateInput(scope, v, asset, responsible, product, unit, quantity, paid, discount, price, priceText, fill, payer, payment, reading, reason, place)
	if err != nil {
		return Command{}, ErrMalformed
	}
	candidate, err := fuel.NewCandidate(scope, input)
	if err != nil {
		return Command{}, ErrMalformed
	}
	if d.unsupported {
		return Command{}, ErrUnsupported
	}
	return Command{valid: true, value: CommandInput{OperationID: op.ID(), PayloadDigest: op.PayloadDigest(), FuelingID: v.Entity.ID, AssetID: asset, ResponsibleID: responsible, AuthorMembershipID: v.MembershipID,
		Declared: *input.Declared, Candidate: candidate, Reading: reading, MissingReason: reason, PlaceName: place}}, nil
}

func candidateInput(scope kernel.OrgScope, v protocol.Envelope, asset, responsible, product, unit, quantity, paid string, discount, price, priceText *string, fill, payer, payment string, reading *Reading, reason, place *string) (fuel.CandidateInput, error) {
	in := fuel.CandidateInput{}
	assetID, err := fleet.ParseAssetID(asset)
	if err != nil {
		return in, err
	}
	if in.Asset, err = fleet.NewAssetRef(scope, assetID); err != nil {
		return in, err
	}
	if in.Reference, err = fuel.NewFuelingRef(scope, v.Entity.ID); err != nil {
		return in, err
	}
	if in.Responsible, err = fuel.NewEmployeeRef(scope, responsible); err != nil {
		return in, err
	}
	if in.Author, err = fuel.NewMembershipRef(scope, v.MembershipID); err != nil {
		return in, err
	}
	at, err := orgtime.ParseInstant(v.OccurredAt)
	if err != nil {
		return in, err
	}
	declared, err := orgtime.ValidateDeclaration(orgtime.Declaration{OccurredAt: at, EntryZone: v.EntryZone, DeviceOffsetSeconds: v.DeviceOffsetSeconds})
	if err != nil {
		return in, err
	}
	in.Declared = &declared
	if in.Quantity, err = fuel.NewQuantity(fuel.ProductCode(product), fleet.Unit(unit), quantity); err != nil {
		return in, err
	}
	if in.Amounts, err = fuel.NewAmounts(paid, discount, price, priceText); err != nil {
		return in, err
	}
	in.Fill, in.Payer, in.Payment = fuel.Fill(fill), fuel.Payer(payer), fuel.PaymentMethod(payment)
	if (reading == nil) == (reason == nil) {
		return in, fuel.ErrInvalid
	}
	if reading != nil {
		m, err := meter.NewMeterRef(scope, in.Asset, reading.MeterID, reading.Value.Kind(), nil)
		if err != nil {
			return in, err
		}
		segment, err := meter.NewSegmentRef(scope, m, reading.SegmentID)
		if err != nil {
			return in, err
		}
		ref, err := meter.NewReadingRef(scope, segment, reading.ReadingID)
		if err != nil {
			return in, err
		}
		if in.Observation, err = fuel.Observed(ref); err != nil {
			return in, err
		}
	} else {
		missing, err := meter.NewMissing(*reason)
		if err != nil {
			return in, err
		}
		if in.Observation, err = fuel.Missing(missing); err != nil {
			return in, err
		}
	}
	if place != nil {
		establishment, err := fuel.AtNamedPlace(*place)
		if err != nil {
			return in, err
		}
		in.Establishment = &establishment
	}
	origin := fuel.App
	in.Origin = &origin
	return in, nil
}
