package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

// MeterSource describes untrusted provenance. Only MeterAuthority can verify
// that the corresponding real source is current in the retained transaction.
type MeterSource struct {
	private
	Web    *FleetActor
	Device *syncapp.Source
}

func (s MeterSource) Validate(scope kernel.OrgScope) error {
	if scope.Validate() != nil {
		return meter.ErrScope
	}
	if (s.Web == nil) == (s.Device == nil) {
		return ErrUnauthenticated
	}
	if s.Web != nil {
		return s.Web.Validate()
	}
	if s.Device.Scope != scope || s.Device.Validate() != nil {
		return ErrUnauthenticated
	}
	return nil
}
func (s MeterSource) Clone() MeterSource {
	if s.Web != nil {
		v := *s.Web
		s.Web = &v
	}
	if s.Device != nil {
		v := *s.Device
		s.Device = &v
	}
	return s
}
func (s MeterSource) AccountID() string {
	if s.Web != nil && s.Device == nil {
		return s.Web.AccountID
	}
	if s.Device != nil && s.Web == nil {
		return s.Device.Actor.AccountID
	}
	return ""
}
func (s MeterSource) Capture(scope kernel.OrgScope) (meter.CaptureSource, error) {
	if e := s.Validate(scope); e != nil {
		return meter.CaptureSource{}, e
	}
	if s.Web != nil {
		return meter.NewCaptureSource(scope, meter.CaptureWebSession, s.Web.SessionID)
	}
	return meter.NewCaptureSource(scope, meter.CaptureEnrolledDevice, s.Device.EnrollmentID)
}

type MeterAction uint8

const (
	MeterInstallAction MeterAction = iota + 1
	MeterRecordAction
	MeterReadAction
)

func MeterPolicy(action MeterAction) access.Policy {
	switch action {
	case MeterInstallAction:
		return Policy(true)
	case MeterReadAction:
		return Policy(false)
	case MeterRecordAction:
		return access.Policy{Permission: "meter.reading.record", Capability: "organizations", Effect: access.OperationalWrite}
	}
	return access.Policy{}
}

type MeterAuthority interface {
	CheckMeterTx(context.Context, syncapp.DBTX, kernel.OrgScope, MeterSource, string, string, MeterAction) error
}

type MeterInstallInput struct {
	private
	OperationID, AssetID, OpeningReadingID string
	ExpectedAssetRevision                  int64
	Kind                                   meter.Kind
	Label                                  string
	Modulus                                *int64
	Value                                  meter.Value
	Declared                               orgtime.Declaration
	Method                                 meter.Method
}
type MeterInstall struct {
	private
	value MeterInstallInput
}

func NewMeterInstall(v MeterInstallInput) (MeterInstall, error) {
	if !canonicalID(v.OperationID) || !canonicalID(v.AssetID) || !canonicalID(v.OpeningReadingID) || v.ExpectedAssetRevision < 1 || v.ExpectedAssetRevision == math.MaxInt64 || !validMeterLabel(v.Label) || v.Value.Kind() != v.Kind || !validMeterMethod(v.Method) {
		return MeterInstall{}, meter.ErrInvalid
	}
	if _, e := meter.NewValue(v.Kind, v.Value.Count(), v.Modulus); e != nil {
		return MeterInstall{}, e
	}
	declared, e := orgtime.ValidateDeclaration(v.Declared)
	if e != nil {
		return MeterInstall{}, meter.ErrInvalid
	}
	v.Declared = declared
	if v.Modulus != nil {
		n := *v.Modulus
		v.Modulus = &n
	}
	return MeterInstall{value: v}, nil
}
func (v MeterInstall) Snapshot() (MeterInstallInput, error) {
	copy, e := NewMeterInstall(v.value)
	return copy.value, e
}
func (v MeterInstall) Digest(scope kernel.OrgScope) string {
	in, e := v.Snapshot()
	if e != nil || scope.Validate() != nil {
		return ""
	}
	return meterIntentDigest([]any{1, "meter.install", scope.OrgID().String(), in.OperationID, in.AssetID, in.OpeningReadingID, in.ExpectedAssetRevision, in.Kind, in.Label, in.Modulus, in.Value.Count(), orgtime.FormatInstant(in.Declared.OccurredAt), in.Declared.EntryZone, in.Declared.DeviceOffsetSeconds, in.Method})
}

type MeterInstallResult struct {
	private
	AssetID, MeterID, SegmentID, OpeningReadingID string
	AssetRevision                                 int64
}

type MeterReadingInput struct {
	private
	OperationID, AssetID, MeterID, SegmentID, ReadingID string
	Value                                               meter.Value
	Declared                                            orgtime.Declaration
	Origin                                              meter.Origin
	Method                                              meter.Method
}
type MeterReadingCommand struct {
	private
	value MeterReadingInput
}

func NewMeterReadingCommand(v MeterReadingInput) (MeterReadingCommand, error) {
	for _, id := range []string{v.OperationID, v.AssetID, v.MeterID, v.SegmentID, v.ReadingID} {
		if !canonicalID(id) {
			return MeterReadingCommand{}, meter.ErrInvalid
		}
	}
	if _, e := meter.NewValue(v.Value.Kind(), v.Value.Count(), nil); e != nil {
		return MeterReadingCommand{}, e
	}
	if !validMeterMethod(v.Method) || !validMeterOrigin(v.Origin) {
		return MeterReadingCommand{}, meter.ErrInvalid
	}
	declared, e := orgtime.ValidateDeclaration(v.Declared)
	if e != nil {
		return MeterReadingCommand{}, meter.ErrInvalid
	}
	v.Declared = declared
	return MeterReadingCommand{value: v}, nil
}
func (v MeterReadingCommand) Snapshot() (MeterReadingInput, error) {
	copy, e := NewMeterReadingCommand(v.value)
	return copy.value, e
}
func (v MeterReadingCommand) Digest(scope kernel.OrgScope) string {
	in, e := v.Snapshot()
	if e != nil || scope.Validate() != nil {
		return ""
	}
	return meterIntentDigest([]any{1, "meter.reading", scope.OrgID().String(), in.OperationID, in.AssetID, in.MeterID, in.SegmentID, in.ReadingID, in.Value.Kind(), in.Value.Count(), orgtime.FormatInstant(in.Declared.OccurredAt), in.Declared.EntryZone, in.Declared.DeviceOffsetSeconds, in.Origin, in.Method})
}

type MeterReadingResult struct {
	private
	Reading meter.Reading
}
type MeterView struct {
	private
	Reference   meter.MeterRef
	Label       string
	InstalledAt time.Time
	RemovedAt   *time.Time
	Segment     meter.SegmentRef
	Latest      *meter.Reading
}

func (v MeterView) Clone() MeterView {
	if v.RemovedAt != nil {
		n := *v.RemovedAt
		v.RemovedAt = &n
	}
	if v.Latest != nil {
		n := *v.Latest
		v.Latest = &n
	}
	return v
}

type MeterFilter struct {
	private
	AfterID string
	Limit   int
}

func (v MeterFilter) Validate() error {
	if v.Limit < 1 || v.Limit > 100 || v.AfterID != "" && !canonicalID(v.AfterID) {
		return meter.ErrInvalid
	}
	return nil
}

type MeterPage struct {
	private
	Meters []MeterView
	NextID string
}

type MeterRepository interface {
	InstallMeter(context.Context, kernel.OrgScope, FleetActor, string, string, string, MeterInstall) (MeterInstallResult, bool, error)
	ReadLatestMeter(context.Context, kernel.OrgScope, FleetActor, string, string, string) (MeterView, error)
	ListAssetMeters(context.Context, kernel.OrgScope, FleetActor, string, string, MeterFilter) (MeterPage, error)
}
type EmbeddedMeterRepository interface {
	RecordEmbedded(context.Context, syncapp.DBTX, kernel.OrgScope, MeterSource, string, MeterReadingCommand) (MeterReadingResult, bool, error)
}

// MeterIntakeLookup proves actual parent/record reference coverage without
// creating an effect while the intake is admitting a claim or replaying an ACK.
// The handler must check current MeterAuthority before and after these reads.
type MeterIntakeLookup interface {
	ReadMeterTargetTx(context.Context, syncapp.DBTX, kernel.OrgScope, string, string, string) (MeterView, error)
	ReadMeterReadingTx(context.Context, syncapp.DBTX, kernel.OrgScope, string, string, string, string) (meter.Reading, error)
}
type MeterService struct {
	Enabled    bool
	Repository MeterRepository
	NewID      func() (string, error)
}

func validMeterLabel(raw string) bool {
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
func validMeterMethod(v meter.Method) bool {
	return v == meter.Typed || v == meter.TypedWithPhoto || v == meter.Imported || v == meter.Integration
}
func validMeterOrigin(v meter.Origin) bool {
	switch v {
	case meter.TripStart, meter.TripEnd, meter.Handover, meter.Fueling, meter.Checklist, meter.Maintenance, meter.Standalone, meter.Import, meter.Installation, meter.Removal:
		return true
	}
	return false
}
func meterIntentDigest(v []any) string {
	raw, e := json.Marshal(v)
	if e != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
