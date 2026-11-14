package application_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	membership "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	protocol "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const prefix = "7b0e2a54-0000-4000-8000-"

func id(suffix int) string { return fmt.Sprintf("%s%012d", prefix, suffix) }
func source(t *testing.T) app.Source {
	t.Helper()
	org, err := kernel.ParseOrgID(id(65))
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	return app.Source{Scope: scope, Origin: "https://api.example.invalid", MembershipID: id(3), EnrollmentID: id(4),
		Actor: membership.DeviceActor{AccountID: id(1), FamilyID: id(9), AccessToken: "synthetic-sensitive-token", ContributorID: id(10),
			KeyID: id(11), Fingerprint: "fp:" + strings.Repeat("a", 64), ProofExpires: time.Date(2026, 11, 13, 12, 5, 0, 0, time.UTC)}}
}

func operation(t *testing.T, number int, kind string) protocol.Operation {
	t.Helper()
	binding, err := protocol.NewBinding(id(1), "https://api.example.invalid", id(65))
	if err != nil {
		t.Fatal(err)
	}
	raw := fmt.Sprintf(`{"schema_version":1,"operation_id":%q,"owner_account_id":%q,"api_origin":"https://api.example.invalid","org_id":%q,"membership_id":%q,"enrollment_id":%q,"type":%q,"operation_schema":1,"entity":{"type":"synthetic","id":%q,"expected_revision":null},"occurred_at":"2026-11-13T12:00:00Z","entry_zone":"America/Cuiaba","device_offset_seconds":-14400,"depends_on":[],"soft_references":[],"attachments":[],"payload":{"note":"synthetic-sensitive-payload"}}`, id(number), id(1), id(65), id(3), id(4), kind, id(5))
	value, err := protocol.Seal([]byte(raw), binding)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func batch(t *testing.T, operations ...protocol.Operation) protocol.Batch {
	t.Helper()
	values := make([]string, len(operations))
	for i, value := range operations {
		values[i] = string(value.CanonicalBytes())
	}
	return rawBatch(t, strings.Join(values, ","))
}
func rawBatch(t *testing.T, operations string) protocol.Batch {
	t.Helper()
	value, err := protocol.ParseBatch([]byte(`{"schema_version":1,"family_id":"` + id(9) + `","access_token":"synthetic-sensitive-token","operations":[` + operations + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

type handler struct{ descriptor app.Descriptor }

func newHandler() *handler {
	return &handler{app.Descriptor{Type: "synthetic.record", Schema: 1,
		TerminalResults: []string{protocol.Accepted, protocol.Rejected},
		WritePolicy:     access.Policy{Permission: "fuel.fueling.record", Capability: "fuelings", Effect: access.OperationalWrite},
		ReadPolicy:      access.Policy{Permission: "fuel.fueling.read", Capability: "fuelings", Effect: access.Read}}}
}
func (h *handler) Descriptor() app.Descriptor { return h.descriptor }
func (*handler) Admit(context.Context, app.DBTX, app.Source, protocol.Operation, app.Mode) error {
	return nil
}
func (*handler) Apply(context.Context, app.DBTX, app.Source, protocol.Operation) (app.Outcome, error) {
	return app.Outcome{}, app.ErrUnavailable // Unit fixtures never claim business effects.
}
func (*handler) Confirm(context.Context, app.DBTX, app.Source, protocol.Operation, app.Mode, app.Outcome) error {
	return nil
}

type admission struct {
	units []int
	err   error
}

type admissionFunc func(context.Context, app.Source, int) error

func (function admissionFunc) AdmitBatch(ctx context.Context, source app.Source, units int) error {
	return function(ctx, source, units)
}

func (a *admission) AdmitBatch(_ context.Context, _ app.Source, units int) error {
	a.units = append(a.units, units)
	return a.err
}

type store struct {
	called  []string
	process func(context.Context, protocol.Operation) (protocol.Acknowledgement, error)
}

func (s *store) Process(ctx context.Context, _ app.Source, operation protocol.Operation, _ protocol.Limits, _ app.Handler) (protocol.Acknowledgement, error) {
	s.called = append(s.called, operation.ID())
	return s.process(ctx, operation)
}
func retry(t *testing.T, operation protocol.Operation, code string) protocol.Acknowledgement {
	t.Helper()
	value, err := protocol.NewAcknowledgement(operation, protocol.AcknowledgementSnapshot{Result: protocol.Retry, Code: code})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func registry(t *testing.T) app.Registry {
	t.Helper()
	value, err := app.NewRegistry(newHandler())
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestRegistryCapturesPoliciesAndRejectsAbsentOrDuplicateConsumers(t *testing.T) {
	h := newHandler()
	registered, err := app.NewRegistry(h)
	if err != nil {
		t.Fatal("real fixed descriptor refused", err)
	}
	h.descriptor.Type = "changed.record"
	h.descriptor.WritePolicy.Permission = "unknown.permission"
	frozen, ok := registered.Lookup("synthetic.record")
	if !ok || frozen.Descriptor().Type != "synthetic.record" || frozen.Descriptor().Validate() != nil {
		t.Fatal("runtime mutation changed registered source policy")
	}
	var absent *handler
	for _, values := range [][]app.Handler{{nil}, {absent}, {newHandler(), newHandler()}, {&handler{app.Descriptor{Type: "synthetic.record"}}}} {
		if _, err := app.NewRegistry(values...); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("invalid or duplicate producer registered")
		}
	}
}

func TestIntakePreflightsWholeBodyAndTransientProofBeforeAnyAdmission(t *testing.T) {
	first := operation(t, 191, "synthetic.record")
	for _, change := range []func(*app.Source){func(value *app.Source) {}, func(value *app.Source) { value.Actor.FamilyID = id(99) }, func(value *app.Source) { value.Actor.AccessToken = "different-sensitive-token" }, func(value *app.Source) { value.MembershipID = id(99) }} {
		a := &admission{}
		s := &store{}
		service := app.Service{Store: s, Admission: a}
		origin := source(t)
		change(&origin)
		input := batch(t, first)
		want := protocol.ErrBinding
		if origin.Actor.FamilyID == id(9) && origin.Actor.AccessToken == "synthetic-sensitive-token" && origin.MembershipID == id(3) {
			input = rawBatch(t, string(first.CanonicalBytes())+",{}")
			want = protocol.ErrInvalid
		}
		if _, err := service.Intake(t.Context(), origin, input); !errors.Is(err, want) || len(a.units) != 0 || len(s.called) != 0 {
			t.Fatal("untrusted trailing item or proof reached admission")
		}
	}
}

func TestIntakePreservesIndependentRetryAndDoesNotInventUnknownEffects(t *testing.T) {
	first, second, unknown := operation(t, 191, "synthetic.record"), operation(t, 192, "synthetic.record"), operation(t, 193, "future.record")
	a := &admission{}
	s := &store{process: func(_ context.Context, value protocol.Operation) (protocol.Acknowledgement, error) {
		if value.ID() == first.ID() {
			return protocol.Acknowledgement{}, errors.New("synthetic unavailable storage")
		}
		return retry(t, value, "DEPENDENCY_PENDING"), nil
	}}
	service := app.Service{Store: s, Admission: a, Registry: registry(t)}
	values, err := service.Intake(t.Context(), source(t), batch(t, first, second, unknown))
	if err != nil || len(values) != 3 || len(s.called) != 2 || len(a.units) != 1 || a.units[0] != 3 || values[0].Code() != "UNAVAILABLE" ||
		values[1].Code() != "DEPENDENCY_PENDING" || values[2].Code() != "UNAVAILABLE" {
		t.Fatal("one unavailable item aborted independent progress or fabricated a future producer", err)
	}
	for _, ack := range values {
		if ack.IsFinal() {
			t.Fatal("unavailable/dependency retry became final")
		}
	}
}

func TestLaterSourceDenialWithholdsBufferedReceiptsWithoutReplayingEarlierOperation(t *testing.T) {
	first, second, third := operation(t, 191, "synthetic.record"), operation(t, 192, "synthetic.record"), operation(t, 193, "synthetic.record")
	s := &store{process: func(_ context.Context, value protocol.Operation) (protocol.Acknowledgement, error) {
		if value.ID() == second.ID() {
			return protocol.Acknowledgement{}, app.SourceDenied(errors.New("synthetic private denial cause"))
		}
		return retry(t, value, "DEPENDENCY_PENDING"), nil
	}}
	values, err := (app.Service{Store: s, Admission: &admission{}, Registry: registry(t)}).Intake(t.Context(), source(t), batch(t, first, second, third))
	if values != nil || !errors.Is(err, app.ErrSourceDenied) || len(s.called) != 2 || strings.Contains(fmt.Sprintf("%+v", err), "private denial") {
		t.Fatal("source denial exposed prefix acknowledgements or continued after lost authority")
	}
}

func TestIntakeRejectsUncorrelatedStoreResultAndContinues(t *testing.T) {
	first, second := operation(t, 191, "synthetic.record"), operation(t, 192, "synthetic.record")
	s := &store{process: func(_ context.Context, value protocol.Operation) (protocol.Acknowledgement, error) {
		return retry(t, second, "DEPENDENCY_PENDING"), nil
	}}
	values, err := (app.Service{Store: s, Admission: &admission{}, Registry: registry(t)}).Intake(t.Context(), source(t), batch(t, first, second))
	if err != nil || len(values) != 2 || values[0].Code() != "UNAVAILABLE" || values[1].Code() != "DEPENDENCY_PENDING" {
		t.Fatal("foreign stored receipt was exposed", err)
	}
}

func TestEmptyBatchStillCostsOneAndMissingPortsOrCancellationNeverReturnSuccess(t *testing.T) {
	a := &admission{}
	service := app.Service{Store: &store{}, Admission: a}
	values, err := service.Intake(t.Context(), source(t), batch(t))
	if err != nil || len(values) != 0 || len(a.units) != 1 || a.units[0] != 1 {
		t.Fatal("empty source probe bypassed admission", err)
	}
	for _, broken := range []app.Service{{}, {Store: (*store)(nil), Admission: &admission{}}, {Store: &store{}, Admission: (*admission)(nil)}} {
		if values, err := broken.Intake(t.Context(), source(t), batch(t)); values != nil || !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("missing runtime port appeared available")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	a.units = nil
	if values, err := service.Intake(ctx, source(t), batch(t)); values != nil || !errors.Is(err, context.Canceled) || len(a.units) != 0 {
		t.Fatal("canceled request performed intake")
	}
	//lint:ignore SA1012 Intentional missing-context negative test; production callers always provide a context.
	if values, err := service.Intake(nil, source(t), batch(t)); values != nil || !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("nil request context appeared available")
	}
}

func TestSourceAndDenialRemainPrivateAcrossFormattingJSONAndStructuredLogs(t *testing.T) {
	cause := errors.New("synthetic-sensitive-source-denial")
	denial := app.SourceDenied(cause)
	if !errors.Is(denial, cause) || !errors.Is(denial, app.ErrSourceDenied) || strings.Contains(denial.Error(), cause.Error()) {
		t.Fatal("source transport classification was lost or exposed the private cause")
	}
	for _, value := range []any{source(t), registry(t), app.Outcome{ReviewItemID: id(99)}, denial} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var log bytes.Buffer
		slog.New(slog.NewTextHandler(&log, nil)).Info("synthetic privacy assertion", slog.Any("private", value))
		for _, output := range []string{fmt.Sprintf("%#v", value), fmt.Sprint(value), string(encoded), log.String()} {
			for _, private := range []string{id(1), id(65), id(99), "synthetic-sensitive-token", cause.Error()} {
				if strings.Contains(output, private) {
					t.Fatal("intake private authority entered a diagnostic surface")
				}
			}
		}
	}
}

func TestCancellationDuringAdmissionCannotReturnEvenEmptySuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	service := app.Service{Store: &store{}, Admission: admissionFunc(func(context.Context, app.Source, int) error { cancel(); return nil })}
	if values, err := service.Intake(ctx, source(t), batch(t)); values != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("canceled admission returned a successful empty result")
	}
}

func TestBudgetFailureInvalidSourceAndCanceledStoreExposeNoAcknowledgements(t *testing.T) {
	first := operation(t, 191, "synthetic.record")
	denied := errors.New("synthetic budget exhausted")
	a := &admission{err: denied}
	s := &store{}
	if values, err := (app.Service{Store: s, Admission: a, Registry: registry(t)}).Intake(t.Context(), source(t), batch(t, first)); values != nil || !errors.Is(err, denied) || len(s.called) != 0 {
		t.Fatal("failed admission reached an operation")
	}
	for _, mutate := range []func(*app.Source){
		func(value *app.Source) { value.Scope = kernel.OrgScope{} },
		func(value *app.Source) { value.Actor.Fingerprint = "invalid" },
		func(value *app.Source) { value.Origin = "https://foreign.example.invalid/" },
		func(value *app.Source) { value.EnrollmentID = "invalid" },
	} {
		invalid := source(t)
		mutate(&invalid)
		if invalid.Validate() == nil {
			t.Fatal("invalid source shape accepted")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s.process = func(_ context.Context, operation protocol.Operation) (protocol.Acknowledgement, error) {
		cancel()
		return retry(t, operation, "DEPENDENCY_PENDING"), nil
	}
	if values, err := (app.Service{Store: s, Admission: &admission{}, Registry: registry(t)}).Intake(ctx, source(t), batch(t, first)); values != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("canceled operation exposed its result")
	}
}

func TestFinalStoredReceiptRemainsOriginalAndDoesNotAcquireInventedWarnings(t *testing.T) {
	operation := operation(t, 191, "synthetic.record")
	original, err := protocol.NewAcknowledgement(operation, protocol.AcknowledgementSnapshot{Result: protocol.Rejected, Code: "MALFORMED", ReceivedAt: "2026-11-13T12:10:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	s := &store{process: func(context.Context, protocol.Operation) (protocol.Acknowledgement, error) { return original, nil }}
	values, err := (app.Service{Store: s, Admission: &admission{}, Registry: registry(t)}).Intake(t.Context(), source(t), batch(t, operation))
	if err != nil || len(values) != 1 || string(values[0].CanonicalBytes()) != string(original.CanonicalBytes()) || len(values[0].Warnings()) != 0 || !values[0].IsFinal() {
		t.Fatal("service changed an original receipt or inferred skew from offline delay", err)
	}
}

type querySource struct{ calls int }

func (q *querySource) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	q.calls++
	return pgconn.CommandTag{}, nil
}
func (*querySource) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil }
func (*querySource) QueryRow(context.Context, string, ...any) pgx.Row        { return nil }
func (*querySource) Commit(context.Context) error                            { return nil }
func (*querySource) Rollback(context.Context) error                          { return nil }

type observingHandler struct {
	*handler
	query app.DBTX
}

func (h *observingHandler) Admit(_ context.Context, query app.DBTX, _ app.Source, _ protocol.Operation, _ app.Mode) error {
	h.query = query
	return nil
}
func (h *observingHandler) Apply(_ context.Context, query app.DBTX, _ app.Source, _ protocol.Operation) (app.Outcome, error) {
	h.query = query
	return app.Outcome{}, app.ErrUnavailable
}
func (h *observingHandler) Confirm(_ context.Context, query app.DBTX, _ app.Source, _ protocol.Operation, _ app.Mode, _ app.Outcome) error {
	h.query = query
	return nil
}

func TestRegisteredConsumerNeverReceivesRawLifecycleEvenFromCarelessAdapter(t *testing.T) {
	consumer := &observingHandler{handler: newHandler()}
	registry, err := app.NewRegistry(consumer)
	if err != nil {
		t.Fatal(err)
	}
	installed, _ := registry.Lookup("synthetic.record")
	query := &querySource{}
	operation := operation(t, 191, "synthetic.record")
	for _, call := range []func(){
		func() { _ = installed.Admit(t.Context(), query, source(t), operation, app.Write) },
		func() { _, _ = installed.Apply(t.Context(), query, source(t), operation) },
		func() {
			_ = installed.Confirm(t.Context(), query, source(t), operation, app.ReadReceipt, app.Outcome{})
		},
	} {
		call()
		if _, raw := consumer.query.(interface{ Commit(context.Context) error }); raw {
			t.Fatal("registered consumer inherited adapter transaction lifecycle")
		}
	}
}

func TestBorrowedQueryPortHidesDynamicTransactionLifecycle(t *testing.T) {
	q := &querySource{}
	borrowed := app.Borrow(q)
	if _, ok := borrowed.(interface{ Commit(context.Context) error }); ok {
		t.Fatal("consumer received transaction commit authority")
	}
	if _, ok := borrowed.(interface{ Rollback(context.Context) error }); ok {
		t.Fatal("consumer received transaction rollback authority")
	}
	if _, err := borrowed.Exec(t.Context(), "SELECT 1"); err != nil || q.calls != 1 {
		t.Fatal("borrowed query port did not forward work")
	}
	var missing *querySource
	if app.Borrow(missing) != nil || app.Borrow(nil) != nil {
		t.Fatal("typed nil query source exposed")
	}
}

func TestOutcomeReadinessAndCopiesNeverFollowReviewLabel(t *testing.T) {
	revision := int64(3)
	value := app.Outcome{Result: protocol.AcceptedForReview, EntityRevision: &revision, ReviewItemID: id(99), Warnings: []string{"CLOCK_SKEW"}}
	if value.EffectReady() {
		t.Fatal("held review became a committed dependency effect")
	}
	copy := value.Copy()
	*copy.EntityRevision = 10
	copy.Warnings[0] = "changed"
	if *value.EntityRevision != 3 || value.Warnings[0] != "CLOCK_SKEW" {
		t.Fatal("outcome copy mutated original fields")
	}
	value.Result = protocol.Accepted
	if !value.EffectReady() {
		t.Fatal("applied decision lost explicit readiness")
	}
}
