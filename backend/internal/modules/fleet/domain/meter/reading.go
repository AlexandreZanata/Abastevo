package meter

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"time"
)

type ReadingInput struct {
	redacted
	Reference  ReadingRef
	Value      Value
	Declared   *orgtime.Declaration
	ReceivedAt time.Time
	Author     MembershipRef
	Device     DeviceRef
	Operation  OperationRef
	Origin     Origin
	Method     Method
	Photo      *PhotoRef
	State      State
}
type Reading struct {
	redacted
	input ReadingInput
}

func copyInput(in ReadingInput) ReadingInput {
	if in.Declared != nil {
		d := *in.Declared
		in.Declared = &d
	}
	if in.Photo != nil {
		p := *in.Photo
		in.Photo = &p
	}
	return in
}
func NewReading(scope kernel.OrgScope, in ReadingInput) (Reading, error) {
	if err := in.Reference.validate(scope); err != nil {
		return Reading{}, err
	}
	for _, r := range []scopedID{in.Author.scopedID, in.Device.scopedID, in.Operation.scopedID} {
		if err := r.validate(scope); err != nil {
			return Reading{}, err
		}
	}
	if in.Photo != nil {
		if err := in.Photo.validate(scope); err != nil {
			return Reading{}, err
		}
	}
	if in.Declared == nil || in.Value.kind != in.Reference.segment.meter.kind || !validOrigin(in.Origin) || !validMethod(in.Method) || (in.State != Valid && in.State != Suspect) {
		return Reading{}, ErrInvalid
	}
	if _, err := NewValue(in.Value.kind, in.Value.count, in.Reference.segment.meter.Modulus()); err != nil {
		return Reading{}, err
	}
	// This evidence does not own a reporting-day snapshot. UTC is used only to
	// reuse the exact instant/zone/offset validator; device metadata is not an
	// assertion of the organization's reporting zone.
	record, err := orgtime.Capture(*in.Declared, in.ReceivedAt, orgtime.ReportingZones{Organization: "UTC"})
	if err != nil {
		return Reading{}, ErrInvalid
	}
	in = copyInput(in)
	in.Declared.OccurredAt = record.OccurredAt
	in.ReceivedAt = record.ReceivedAt
	return Reading{input: in}, nil
}
func (r Reading) Snapshot() ReadingInput { return copyInput(r.input) }

type FactInput struct {
	redacted
	ID         string
	Operation  OperationRef
	Author     MembershipRef
	Device     DeviceRef
	ReceivedAt time.Time
	Reason     string
}

func (f FactInput) validate(scope kernel.OrgScope) error {
	for _, r := range []scopedID{f.Operation.scopedID, f.Author.scopedID, f.Device.scopedID} {
		if err := r.validate(scope); err != nil {
			return err
		}
	}
	if !canonical(f.ID) || !reason(f.Reason) || f.ReceivedAt.IsZero() || f.ReceivedAt.UTC().Year() < 1 || f.ReceivedAt.UTC().Year() > 9999 {
		return ErrInvalid
	}
	return nil
}

type Fact struct {
	redacted
	input       FactInput
	target      ReadingRef
	replacement *ReadingRef
}

func (f Fact) Snapshot() FactInput { return f.input }
func (f Fact) Target() ReadingRef  { return f.target }
func (f Fact) Replacement() *ReadingRef {
	if f.replacement == nil {
		return nil
	}
	r := *f.replacement
	return &r
}
