package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	membership "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrUnavailable       = errors.New("orgsync: unavailable")
	ErrSourceDenied      = errors.New("orgsync: current source denied")
	ErrDependencyPending = errors.New("orgsync: dependency pending")
)

type privateValue struct{}

func (privateValue) String() string { return "EnterpriseIntakeValue(redacted)" }
func (value privateValue) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, value.String())
}
func (value privateValue) MarshalJSON() ([]byte, error) { return json.Marshal(value.String()) }
func (value privateValue) LogValue() slog.Value         { return slog.StringValue(value.String()) }

type sourceDenial struct {
	privateValue
	cause error
}

func (sourceDenial) Error() string        { return "orgsync: current source denied" }
func (value sourceDenial) Unwrap() error  { return value.cause }
func (sourceDenial) Is(target error) bool { return target == ErrSourceDenied }

// SourceDenied marks a current-source refusal or inability to verify the current
// source. Callers must discard buffered private acknowledgements. Its cause maps
// only to bounded transport errors (unknown causes map to unavailable/503).
// Feature/storage availability outside source verification uses ErrUnavailable.
func SourceDenied(cause error) error { return sourceDenial{cause: cause} }

type Source struct {
	privateValue
	Scope                      kernel.OrgScope
	Origin                     string
	Actor                      membership.DeviceActor
	MembershipID, EnrollmentID string
}

func (source Source) Binding() (protocol.SourceBinding, error) {
	if source.Scope.Validate() != nil || source.Actor.Validate() != nil {
		return protocol.SourceBinding{}, protocol.ErrBinding
	}
	return protocol.NewSourceBinding(source.Actor.AccountID, source.Origin, source.Scope.OrgID().String(), source.MembershipID, source.EnrollmentID)
}
func (source Source) Validate() error { _, err := source.Binding(); return err }

// DBTX deliberately excludes transaction lifecycle and connection access.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type borrowedDBTX struct {
	privateValue
	source DBTX
}

func (value borrowedDBTX) Exec(ctx context.Context, query string, arguments ...any) (pgconn.CommandTag, error) {
	return value.source.Exec(ctx, query, arguments...)
}
func (value borrowedDBTX) Query(ctx context.Context, query string, arguments ...any) (pgx.Rows, error) {
	return value.source.Query(ctx, query, arguments...)
}
func (value borrowedDBTX) QueryRow(ctx context.Context, query string, arguments ...any) pgx.Row {
	return value.source.QueryRow(ctx, query, arguments...)
}
func nilPort(value any) bool {
	if value == nil {
		return true
	}
	reflection := reflect.ValueOf(value)
	switch reflection.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflection.IsNil()
	}
	return false
}
func Borrow(source DBTX) DBTX {
	if nilPort(source) {
		return nil
	}
	return borrowedDBTX{source: source}
}

type Mode uint8

const (
	Write Mode = iota + 1
	ReadReceipt
)

type Descriptor struct {
	privateValue
	Type                    string
	Schema                  int64
	WritePolicy, ReadPolicy access.Policy
}

func (descriptor Descriptor) Validate() error {
	if !protocol.ValidOperationType(descriptor.Type) || descriptor.Schema != 1 || descriptor.WritePolicy.Validate() != nil ||
		!descriptor.WritePolicy.Effect.Write() || descriptor.ReadPolicy.Validate() != nil || descriptor.ReadPolicy.Effect != access.Read {
		return ErrUnavailable
	}
	return nil
}

type Outcome struct {
	privateValue
	Result, Code, ReviewItemID string
	EntityRevision             *int64
	Warnings                   []string
}

func (outcome Outcome) EffectReady() bool { return outcome.Result == protocol.Accepted }
func (outcome Outcome) Copy() Outcome {
	value := outcome
	value.Warnings = append([]string(nil), outcome.Warnings...)
	if outcome.EntityRevision != nil {
		revision := *outcome.EntityRevision
		value.EntityRevision = &revision
	}
	return value
}

// Admit checks required real producers and current owning authority before a
// claim. Confirm repeats fresh source/record coverage after waits/projection.
// Write admission/confirmation also checks actual owning dependency and parent
// record read coverage. After base command admission, missing/foreign/unreadable
// dependency coverage returns ErrDependencyPending without private parent data;
// the store rolls back this operation and creates no new claim. Missing owning
// producers/history instead return ErrUnavailable. Cross-device references are
// permitted when the owning source proves current record coverage.
type Handler interface {
	Descriptor() Descriptor
	Admit(context.Context, DBTX, Source, protocol.Operation, Mode) error
	Apply(context.Context, DBTX, Source, protocol.Operation) (Outcome, error)
	Confirm(context.Context, DBTX, Source, protocol.Operation, Mode, Outcome) error
}

type Store interface {
	Process(context.Context, Source, protocol.Operation, protocol.Limits, Handler) (protocol.Acknowledgement, error)
}
type Admission interface {
	AdmitBatch(context.Context, Source, int) error
}

type Registry struct {
	privateValue
	handlers map[string]Handler
}

type registeredHandler struct {
	privateValue
	descriptor Descriptor
	source     Handler
}

func (handler registeredHandler) Descriptor() Descriptor { return handler.descriptor }
func (handler registeredHandler) Admit(ctx context.Context, tx DBTX, source Source, operation protocol.Operation, mode Mode) error {
	return handler.source.Admit(ctx, Borrow(tx), source, operation, mode)
}
func (handler registeredHandler) Apply(ctx context.Context, tx DBTX, source Source, operation protocol.Operation) (Outcome, error) {
	outcome, err := handler.source.Apply(ctx, Borrow(tx), source, operation)
	return outcome.Copy(), err
}
func (handler registeredHandler) Confirm(ctx context.Context, tx DBTX, source Source, operation protocol.Operation, mode Mode, outcome Outcome) error {
	return handler.source.Confirm(ctx, Borrow(tx), source, operation, mode, outcome.Copy())
}

func NewRegistry(handlers ...Handler) (Registry, error) {
	registry := Registry{handlers: make(map[string]Handler)}
	for _, handler := range handlers {
		if nilPort(handler) {
			return Registry{}, ErrUnavailable
		}
		descriptor := handler.Descriptor()
		if descriptor.Validate() != nil || registry.handlers[descriptor.Type] != nil {
			return Registry{}, ErrUnavailable
		}
		registry.handlers[descriptor.Type] = registeredHandler{descriptor: descriptor, source: handler}
	}
	return registry, nil
}
func (registry Registry) Lookup(kind string) (Handler, bool) {
	handler, ok := registry.handlers[kind]
	return handler, ok
}

type Service struct {
	privateValue
	Store     Store
	Admission Admission
	Registry  Registry
}

func (service Service) Intake(ctx context.Context, source Source, batch protocol.Batch) ([]protocol.Acknowledgement, error) {
	if ctx == nil {
		return nil, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	binding, err := source.Binding()
	if err != nil {
		return nil, err
	}
	proof := batch.Proof()
	if proof.FamilyID != source.Actor.FamilyID || proof.AccessToken != source.Actor.AccessToken {
		return nil, protocol.ErrBinding
	}
	prepared, err := protocol.Preflight(batch, binding)
	if err != nil {
		return nil, err
	}
	if nilPort(service.Store) || nilPort(service.Admission) {
		return nil, ErrUnavailable
	}
	operations := prepared.Operations()
	if err := service.Admission.AdmitBatch(ctx, source, max(1, len(operations))); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	acknowledgements := make([]protocol.Acknowledgement, 0, len(operations))
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		handler, installed := service.Registry.Lookup(operation.Snapshot().Type)
		var acknowledgement protocol.Acknowledgement
		if installed {
			acknowledgement, err = service.Store.Process(ctx, source, operation, prepared.Limits(), handler)
			if errors.Is(err, ErrSourceDenied) {
				return nil, err
			}
			if canceled := ctx.Err(); canceled != nil {
				return nil, canceled
			}
			if err == nil {
				acknowledgement, err = protocol.ParseAcknowledgement(acknowledgement.CanonicalBytes(), operation, prepared.Limits())
			}
		} else {
			err = ErrUnavailable
		}
		if err != nil {
			acknowledgement, err = protocol.NewAcknowledgement(operation,
				protocol.AcknowledgementSnapshot{Result: protocol.Retry, Code: "UNAVAILABLE"}, prepared.Limits())
			if err != nil {
				return nil, ErrUnavailable
			}
		}
		acknowledgements = append(acknowledgements, acknowledgement)
	}
	return acknowledgements, nil
}
