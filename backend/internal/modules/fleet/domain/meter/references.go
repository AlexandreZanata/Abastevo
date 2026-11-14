package meter

import (
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

func canonical(raw string) bool {
	id, err := kernel.ParseOrgID(raw)
	return err == nil && id.String() == raw
}

type scopedID struct {
	redacted
	organization kernel.OrgID
	id           string
}

func scoped(scope kernel.OrgScope, id string) (scopedID, error) {
	if scope.Validate() != nil {
		return scopedID{}, ErrScope
	}
	if !canonical(id) {
		return scopedID{}, ErrInvalid
	}
	return scopedID{organization: scope.OrgID(), id: id}, nil
}
func (r scopedID) validate(scope kernel.OrgScope) error {
	if scope.Validate() != nil {
		return ErrScope
	}
	if !canonical(r.id) || r.organization == (kernel.OrgID{}) {
		return ErrInvalid
	}
	if r.organization != scope.OrgID() {
		return ErrScope
	}
	return nil
}
func (r scopedID) ID() string                   { return r.id }
func (r scopedID) OrganizationID() kernel.OrgID { return r.organization }

type MembershipRef struct{ scopedID }
type DeviceRef struct{ scopedID }
type OperationRef struct{ scopedID }
type PhotoRef struct{ scopedID }

func NewMembershipRef(scope kernel.OrgScope, id string) (MembershipRef, error) {
	r, e := scoped(scope, id)
	return MembershipRef{r}, e
}
func NewDeviceRef(scope kernel.OrgScope, id string) (DeviceRef, error) {
	r, e := scoped(scope, id)
	return DeviceRef{r}, e
}
func NewOperationRef(scope kernel.OrgScope, id string) (OperationRef, error) {
	r, e := scoped(scope, id)
	return OperationRef{r}, e
}
func NewPhotoRef(scope kernel.OrgScope, id string) (PhotoRef, error) {
	r, e := scoped(scope, id)
	return PhotoRef{r}, e
}

type MeterRef struct {
	redacted
	asset   fleet.AssetRef
	id      string
	kind    Kind
	modulus int64
}

func NewMeterRef(scope kernel.OrgScope, asset fleet.AssetRef, id string, kind Kind, modulus *int64) (MeterRef, error) {
	if err := asset.Validate(scope); err != nil {
		return MeterRef{}, ErrScope
	}
	if !canonical(id) || bound(kind) < 0 || modulus != nil && *modulus <= 0 {
		return MeterRef{}, ErrInvalid
	}
	m := MeterRef{asset: asset, id: id, kind: kind}
	if modulus != nil {
		m.modulus = *modulus
	}
	return m, nil
}
func (m MeterRef) validate(scope kernel.OrgScope) error {
	if scope.Validate() != nil {
		return ErrScope
	}
	if !canonical(m.id) || bound(m.kind) < 0 {
		return ErrInvalid
	}
	if m.asset.Validate(scope) != nil {
		return ErrScope
	}
	return nil
}
func (m MeterRef) Asset() fleet.AssetRef { return m.asset }
func (m MeterRef) ID() string            { return m.id }
func (m MeterRef) Kind() Kind            { return m.kind }
func (m MeterRef) Modulus() *int64 {
	if m.modulus == 0 {
		return nil
	}
	n := m.modulus
	return &n
}

// ValidateOpenSet validates supplied in-service facts, without querying or reserving meters.
func ValidateOpenSet(scope kernel.OrgScope, meters []MeterRef) error {
	if scope.Validate() != nil {
		return ErrScope
	}
	ids := map[string]bool{}
	kinds := map[string]bool{}
	for _, m := range meters {
		if err := m.validate(scope); err != nil {
			return err
		}
		if ids[m.id] {
			return ErrInvalid
		}
		ids[m.id] = true
		k := m.asset.ID().Value() + ":" + string(m.kind)
		if kinds[k] {
			return ErrAlreadyInService
		}
		kinds[k] = true
	}
	return nil
}

type SegmentRef struct {
	redacted
	meter MeterRef
	id    string
}

func NewSegmentRef(scope kernel.OrgScope, meter MeterRef, id string) (SegmentRef, error) {
	if err := meter.validate(scope); err != nil {
		return SegmentRef{}, err
	}
	if !canonical(id) {
		return SegmentRef{}, ErrInvalid
	}
	return SegmentRef{meter: meter, id: id}, nil
}
func (s SegmentRef) validate(scope kernel.OrgScope) error {
	if err := s.meter.validate(scope); err != nil {
		return err
	}
	if !canonical(s.id) {
		return ErrInvalid
	}
	return nil
}
func (s SegmentRef) Meter() MeterRef { return s.meter }
func (s SegmentRef) ID() string      { return s.id }

type ReadingRef struct {
	redacted
	segment SegmentRef
	id      string
}

func NewReadingRef(scope kernel.OrgScope, segment SegmentRef, id string) (ReadingRef, error) {
	if err := segment.validate(scope); err != nil {
		return ReadingRef{}, err
	}
	if !canonical(id) {
		return ReadingRef{}, ErrInvalid
	}
	return ReadingRef{segment: segment, id: id}, nil
}
func (r ReadingRef) validate(scope kernel.OrgScope) error {
	if err := r.segment.validate(scope); err != nil {
		return err
	}
	if !canonical(r.id) {
		return ErrInvalid
	}
	return nil
}
func (r ReadingRef) Segment() SegmentRef { return r.segment }
func (r ReadingRef) ID() string          { return r.id }
