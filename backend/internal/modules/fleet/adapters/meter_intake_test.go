package adapters_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	membership "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func intakeID(n int) string { return fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", n) }

type intakeTx struct{}

func (*intakeTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unexpected SQL in handler")
}
func (*intakeTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unexpected SQL in handler")
}
func (*intakeTx) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (*intakeTx) Commit(context.Context) error                     { return errors.New("lifecycle must not be borrowed") }

type intakeAuthority struct {
	t       *testing.T
	actions []app.MeterAction
	targets []string
	err     error
}

func (a *intakeAuthority) CheckMeterTx(_ context.Context, db syncapp.DBTX, scope kernel.OrgScope, source app.MeterSource, member, target string, action app.MeterAction) error {
	if _, leaked := db.(interface{ Commit(context.Context) error }); leaked {
		a.t.Fatal("transaction lifecycle leaked through authority port")
	}
	if source.Device == nil || source.Web != nil || member != source.Device.MembershipID || source.Device.Scope != scope {
		a.t.Fatal("payload or fabricated identity reached authority")
	}
	a.actions = append(a.actions, action)
	a.targets = append(a.targets, target)
	return a.err
}

type intakeRepository struct {
	t                             *testing.T
	view                          app.MeterView
	reading                       meter.Reading
	targetErr, recordErr, readErr error
	recorded                      int
	lookup                        int
	transform                     func(meter.ReadingInput) meter.ReadingInput
}

func (r *intakeRepository) ReadMeterTargetTx(_ context.Context, db syncapp.DBTX, _ kernel.OrgScope, _, _, _ string) (app.MeterView, error) {
	if _, leaked := db.(interface{ Commit(context.Context) error }); leaked {
		r.t.Fatal("lookup received lifecycle")
	}
	r.lookup++
	return r.view.Clone(), r.targetErr
}
func (r *intakeRepository) ReadMeterReadingTx(_ context.Context, db syncapp.DBTX, _ kernel.OrgScope, _, _, _, _ string) (meter.Reading, error) {
	if _, leaked := db.(interface{ Commit(context.Context) error }); leaked {
		r.t.Fatal("receipt lookup received lifecycle")
	}
	return r.reading, r.readErr
}
func (r *intakeRepository) RecordEmbedded(_ context.Context, db syncapp.DBTX, scope kernel.OrgScope, source app.MeterSource, member string, command app.MeterReadingCommand) (app.MeterReadingResult, bool, error) {
	if _, leaked := db.(interface{ Commit(context.Context) error }); leaked {
		r.t.Fatal("repository received lifecycle")
	}
	r.recorded++
	if r.recordErr != nil {
		return app.MeterReadingResult{}, false, r.recordErr
	}
	in, err := command.Snapshot()
	if err != nil {
		r.t.Fatal(err)
	}
	ref, err := meter.NewReadingRef(scope, r.view.Segment, in.ReadingID)
	if err != nil {
		r.t.Fatal(err)
	}
	author, err := meter.NewMembershipRef(scope, member)
	if err != nil {
		r.t.Fatal(err)
	}
	op, err := meter.NewOperationRef(scope, in.OperationID)
	if err != nil {
		r.t.Fatal(err)
	}
	capture, err := source.Capture(scope)
	if err != nil {
		r.t.Fatal(err)
	}
	value := meter.ReadingInput{Reference: ref, Value: in.Value, Declared: &in.Declared, ReceivedAt: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC), Author: author, Operation: op, Capture: &capture, Origin: in.Origin, Method: in.Method, State: meter.Valid}
	if r.transform != nil {
		value = r.transform(value)
	}
	r.reading, err = meter.NewReading(scope, value)
	if err != nil {
		r.t.Fatal("invalid unit fixture reading", err)
	}
	return app.MeterReadingResult{Reading: r.reading}, false, nil
}

type intakeMissingLookup struct{}

type intakeLateInvalid struct{ *intakeRepository }

func (r *intakeLateInvalid) RecordEmbedded(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source app.MeterSource, member string, command app.MeterReadingCommand) (app.MeterReadingResult, bool, error) {
	result, replay, err := r.intakeRepository.RecordEmbedded(ctx, db, scope, source, member, command)
	if err != nil {
		return result, replay, err
	}
	return result, replay, meter.ErrInvalid
}

func TestMeterIntakePostWriteErrorCannotBecomeFinalMalformedRefusal(t *testing.T) {
	source, authority, repository, _ := intakeFixture(t)
	handler, err := fleetpg.NewMeterIntakeHandler(authority, &intakeLateInvalid{repository})
	if err != nil {
		t.Fatal(err)
	}
	op := intakeOperation(t, source, nil)
	outcome, err := handler.Apply(t.Context(), &intakeTx{}, source, op)
	if !errors.Is(err, syncapp.ErrUnavailable) || outcome.Result != "" || repository.recorded != 1 || repository.reading.Snapshot().Reference.ID() != op.Snapshot().Entity.ID {
		t.Fatal("repository error after actual provisional reading became successful final refusal", err)
	}
}

func TestMeterIntakeMapsActualSourceCausesToBoundedTransportErrors(t *testing.T) {
	for _, test := range []struct{ cause, want error }{{permission.ErrPermissionDenied, orgapi.Forbidden}, {app.ErrNotFound, orgapi.NotFound}, {app.ErrUnauthenticated, orgapi.Unauthenticated}, {access.ErrSetupIncomplete, orgapi.OrgSetupIncomplete}} {
		source, authority, _, handler := intakeFixture(t)
		authority.err = test.cause
		err := handler.Admit(t.Context(), &intakeTx{}, source, intakeOperation(t, source, nil), syncapp.Write)
		if !errors.Is(err, syncapp.ErrSourceDenied) || !errors.Is(err, test.want) {
			t.Fatal("actual source refusal became unknown unavailable transport error", err)
		}
	}
}

func TestMeterIntakeConfirmRejectsUndeclaredRetainedReview(t *testing.T) {
	source, _, _, handler := intakeFixture(t)
	op := intakeOperation(t, source, nil)
	err := handler.Confirm(t.Context(), &intakeTx{}, source, op, syncapp.ReadReceipt, syncapp.Outcome{Result: protocol.AcceptedForReview, Code: "ORPHAN_REFERENCE", ReviewItemID: op.ID()})
	if !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("meter read confirmed undeclared review without owning producer", err)
	}
}

func (*intakeMissingLookup) RecordEmbedded(context.Context, syncapp.DBTX, kernel.OrgScope, app.MeterSource, string, app.MeterReadingCommand) (app.MeterReadingResult, bool, error) {
	return app.MeterReadingResult{}, false, app.ErrUnavailable
}

func intakeFixture(t *testing.T) (syncapp.Source, *intakeAuthority, *intakeRepository, syncapp.Handler) {
	t.Helper()
	org, err := kernel.ParseOrgID(intakeID(65))
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	source := syncapp.Source{Scope: scope, Origin: "https://api.example.invalid", MembershipID: intakeID(3), EnrollmentID: intakeID(4), Actor: membership.DeviceActor{AccountID: intakeID(1), FamilyID: intakeID(2), AccessToken: "synthetic-sensitive-token", ContributorID: intakeID(6), KeyID: intakeID(7), Fingerprint: "fp:" + strings.Repeat("a", 64), ProofExpires: time.Date(2026, 11, 14, 13, 0, 0, 0, time.UTC)}}
	assetID, err := fleet.ParseAssetID(intakeID(10))
	if err != nil {
		t.Fatal(err)
	}
	asset, err := fleet.NewAssetRef(scope, assetID)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := meter.NewMeterRef(scope, asset, intakeID(11), meter.Odometer, nil)
	if err != nil {
		t.Fatal(err)
	}
	segment, err := meter.NewSegmentRef(scope, reference, intakeID(12))
	if err != nil {
		t.Fatal(err)
	}
	authority := &intakeAuthority{t: t}
	repository := &intakeRepository{t: t, view: app.MeterView{Reference: reference, Segment: segment, InstalledAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}}
	handler, err := fleetpg.NewMeterIntakeHandler(authority, repository)
	if err != nil {
		t.Fatal(err)
	}
	return source, authority, repository, handler
}
func intakeOperation(t *testing.T, source syncapp.Source, change func(map[string]any, map[string]any)) protocol.Operation {
	t.Helper()
	payload := map[string]any{"asset_id": intakeID(10), "meter_id": intakeID(11), "segment_id": intakeID(12), "kind": "ODOMETER", "value": "184190000", "method": "TYPED"}
	envelope := map[string]any{"schema_version": 1, "operation_id": intakeID(20), "owner_account_id": source.Actor.AccountID, "api_origin": source.Origin, "org_id": source.Scope.OrgID().String(), "membership_id": source.MembershipID, "enrollment_id": source.EnrollmentID, "type": fleetpg.MeterReadingOperationType, "operation_schema": 1, "entity": map[string]any{"type": "meter_reading", "id": intakeID(13), "expected_revision": nil}, "occurred_at": "2026-11-13T12:00:00.000000123Z", "entry_zone": "America/Cuiaba", "device_offset_seconds": -14400, "depends_on": []any{}, "soft_references": []any{}, "attachments": []any{}, "payload": payload}
	if change != nil {
		change(envelope, payload)
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := protocol.NewBinding(source.Actor.AccountID, source.Origin, source.Scope.OrgID().String())
	if err != nil {
		t.Fatal(err)
	}
	op, err := protocol.Seal(raw, binding)
	if err != nil {
		t.Fatal("invalid unit operation fixture", err)
	}
	return op
}

func TestMeterIntakeNeedsActualAuthorityAndQueryOnlyLookup(t *testing.T) {
	_, authority, repository, _ := intakeFixture(t)
	var absentAuthority *intakeAuthority
	var absentRepository *intakeRepository
	for _, ports := range []struct {
		authority  app.MeterAuthority
		repository app.EmbeddedMeterRepository
	}{{nil, repository}, {absentAuthority, repository}, {authority, nil}, {authority, absentRepository}, {authority, &intakeMissingLookup{}}} {
		if handler, err := fleetpg.NewMeterIntakeHandler(ports.authority, ports.repository); handler != nil || !errors.Is(err, syncapp.ErrUnavailable) {
			t.Fatal("incomplete producer entered registry")
		}
	}
}

func TestMeterIntakeUsesRealTupleAndOriginalSourceAcrossApplyAndReplay(t *testing.T) {
	source, authority, repository, handler := intakeFixture(t)
	op := intakeOperation(t, source, nil)
	tx := &intakeTx{}
	if err := handler.Admit(t.Context(), tx, source, op, syncapp.Write); err != nil {
		t.Fatal("actual target not admitted", err)
	}
	outcome, err := handler.Apply(t.Context(), tx, source, op)
	if err != nil || outcome.Result != protocol.Accepted || outcome.EntityRevision == nil || *outcome.EntityRevision != 1 || repository.recorded != 1 {
		t.Fatal("actual immutable reading not applied", err)
	}
	if err := handler.Confirm(t.Context(), tx, source, op, syncapp.Write, outcome); err != nil {
		t.Fatal("actual exact reading not confirmed", err)
	}
	removed := time.Date(2026, 11, 14, 12, 1, 0, 0, time.UTC)
	repository.view.RemovedAt = &removed
	if err := handler.Admit(t.Context(), tx, source, op, syncapp.ReadReceipt); err != nil {
		t.Fatal("historic original receipt lost read coverage", err)
	}
	if err := handler.Confirm(t.Context(), tx, source, op, syncapp.ReadReceipt, outcome); err != nil {
		t.Fatal("historic original receipt not confirmed", err)
	}
	if len(authority.actions) < 4 || authority.actions[0] != app.MeterRecordAction || authority.actions[len(authority.actions)-1] != app.MeterReadAction {
		t.Fatal("write or receipt used wrong owning policy")
	}
	for _, target := range authority.targets {
		if target != intakeID(10) {
			t.Fatal("valid target was downgraded to root authority")
		}
	}
	in := repository.reading.Snapshot()
	if in.Author.ID() != source.MembershipID || in.Operation.ID() != op.ID() || repository.reading.CaptureSource().ID() != source.EnrollmentID || repository.reading.CaptureSource().Kind() != meter.CaptureEnrolledDevice || in.Origin != meter.Standalone || in.State != meter.Valid || in.Declared.OccurredAt.Nanosecond() != 123 {
		t.Fatal("payload manufactured identity, state or rounded declaration")
	}
}

func TestMeterIntakeMalformedShapeUsesOnlyExplicitOrgBaseRefusal(t *testing.T) {
	for _, name := range []string{"extra", "missing", "null", "numeric", "leading_zero", "kind", "method", "entity", "revision", "schema"} {
		t.Run(name, func(t *testing.T) {
			source, authority, repository, handler := intakeFixture(t)
			op := intakeOperation(t, source, func(envelope, payload map[string]any) {
				switch name {
				case "extra":
					payload["author_id"] = intakeID(99)
				case "missing":
					delete(payload, "asset_id")
				case "null":
					payload["value"] = nil
				case "numeric":
					payload["value"] = 184190000
				case "leading_zero":
					payload["value"] = "0184190000"
				case "kind":
					payload["kind"] = "UNKNOWN"
				case "method":
					payload["method"] = "UNKNOWN"
				case "entity":
					envelope["entity"].(map[string]any)["type"] = "asset"
				case "revision":
					envelope["entity"].(map[string]any)["expected_revision"] = 1
				case "schema":
					envelope["operation_schema"] = 2
				}
			})
			if err := handler.Admit(t.Context(), &intakeTx{}, source, op, syncapp.Write); err != nil {
				t.Fatal("ORG base malformed refusal unavailable", err)
			}
			outcome, err := handler.Apply(t.Context(), &intakeTx{}, source, op)
			if err != nil || outcome.Result != protocol.Rejected || outcome.Code != "MALFORMED" || repository.recorded != 0 || repository.lookup != 0 {
				t.Fatal("malformed request performed target/reading work", err)
			}
			if err := handler.Confirm(t.Context(), &intakeTx{}, source, op, syncapp.Write, outcome); err != nil {
				t.Fatal(err)
			}
			for _, target := range authority.targets {
				if target != source.Scope.OrgID().String() {
					t.Fatal("malformed payload acquired invented asset authority")
				}
			}
		})
	}
}

func TestMeterIntakeUnavailableProducersAndMissingTargetsStayClosed(t *testing.T) {
	for _, method := range []string{"TYPED_WITH_PHOTO", "IMPORTED", "INTEGRATION"} {
		t.Run(method, func(t *testing.T) {
			source, _, repository, handler := intakeFixture(t)
			op := intakeOperation(t, source, func(_, payload map[string]any) { payload["method"] = method })
			if err := handler.Admit(t.Context(), &intakeTx{}, source, op, syncapp.Write); !errors.Is(err, syncapp.ErrUnavailable) {
				t.Fatal("missing capture/evidence producer became eligible", err)
			}
			if _, err := handler.Apply(t.Context(), &intakeTx{}, source, op); !errors.Is(err, syncapp.ErrUnavailable) || repository.recorded != 0 {
				t.Fatal("unavailable source fabricated reading", err)
			}
		})
	}
	for _, field := range []string{"depends_on", "soft_references", "attachments"} {
		t.Run(field, func(t *testing.T) {
			source, _, repository, handler := intakeFixture(t)
			op := intakeOperation(t, source, func(envelope, _ map[string]any) {
				switch field {
				case "depends_on":
					envelope[field] = []string{intakeID(99)}
				case "soft_references":
					envelope[field] = []map[string]string{{"type": "meter_reading", "id": intakeID(99)}}
				case "attachments":
					envelope[field] = []map[string]any{{"id": intakeID(99), "sha256": strings.Repeat("a", 64), "bytes": 1, "mime": "image/jpeg"}}
				}
			})
			if err := handler.Admit(t.Context(), &intakeTx{}, source, op, syncapp.Write); !errors.Is(err, syncapp.ErrUnavailable) {
				t.Fatal("missing owning reference/document producer admitted claim", err)
			}
			if _, err := handler.Apply(t.Context(), &intakeTx{}, source, op); !errors.Is(err, syncapp.ErrUnavailable) || repository.recorded != 0 {
				t.Fatal("unsupported declaration was ignored", err)
			}
		})
	}
	source, authority, repository, handler := intakeFixture(t)
	op := intakeOperation(t, source, nil)
	repository.targetErr = app.ErrNotFound
	if err := handler.Admit(t.Context(), &intakeTx{}, source, op, syncapp.Write); !errors.Is(err, syncapp.ErrDependencyPending) {
		t.Fatal("missing real target did not remain pending", err)
	}
	for _, target := range authority.targets {
		if target == source.Scope.OrgID().String() {
			t.Fatal("missing real target downgraded to ORG")
		}
	}
	authority.err = orgapi.Forbidden
	if err := handler.Admit(t.Context(), &intakeTx{}, source, op, syncapp.Write); !errors.Is(err, syncapp.ErrSourceDenied) {
		t.Fatal("current source refusal was not fatal", err)
	}
}

func TestMeterIntakeConfirmsActualReceiptAndPreservesFatalSourcePriority(t *testing.T) {
	source, authority, repository, handler := intakeFixture(t)
	op := intakeOperation(t, source, nil)
	outcome, err := handler.Apply(t.Context(), &intakeTx{}, source, op)
	if err != nil {
		t.Fatal(err)
	}
	repository.readErr = app.ErrNotFound
	if err := handler.Confirm(t.Context(), &intakeTx{}, source, op, syncapp.ReadReceipt, outcome); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("receipt without actual immutable record was exposed", err)
	}
	repository.readErr = nil
	actual := repository.reading.Snapshot()
	actual.Operation, _ = meter.NewOperationRef(source.Scope, intakeID(99))
	repository.reading, err = meter.NewReading(source.Scope, actual)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.Confirm(t.Context(), &intakeTx{}, source, op, syncapp.ReadReceipt, outcome); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("another operation's real reading proved receipt coverage", err)
	}
	authority.err = errors.Join(orgapi.Forbidden, syncapp.ErrDependencyPending)
	if err := handler.Admit(t.Context(), &intakeTx{}, source, op, syncapp.Write); !errors.Is(err, syncapp.ErrSourceDenied) {
		t.Fatal("current source denial was downgraded to dependency retry", err)
	}
	foreign := source
	foreign.EnrollmentID = intakeID(99)
	if err := handler.Admit(t.Context(), &intakeTx{}, foreign, op, syncapp.Write); !errors.Is(err, syncapp.ErrSourceDenied) {
		t.Fatal("valid operation adopted another current enrollment", err)
	}
}

func TestMeterIntakeActualKindAndModulusAreNotPayloadAuthority(t *testing.T) {
	for _, name := range []string{"kind", "modulus", "missing_tuple", "target_storage", "record_storage", "record_invalid"} {
		t.Run(name, func(t *testing.T) {
			source, _, repository, handler := intakeFixture(t)
			op := intakeOperation(t, source, nil)
			switch name {
			case "kind":
				ref, err := meter.NewMeterRef(source.Scope, repository.view.Reference.Asset(), intakeID(11), meter.HourMeter, nil)
				if err != nil {
					t.Fatal(err)
				}
				segment, err := meter.NewSegmentRef(source.Scope, ref, intakeID(12))
				if err != nil {
					t.Fatal(err)
				}
				repository.view.Reference, repository.view.Segment = ref, segment
			case "modulus":
				modulus := int64(1000)
				ref, err := meter.NewMeterRef(source.Scope, repository.view.Reference.Asset(), intakeID(11), meter.Odometer, &modulus)
				if err != nil {
					t.Fatal(err)
				}
				segment, err := meter.NewSegmentRef(source.Scope, ref, intakeID(12))
				if err != nil {
					t.Fatal(err)
				}
				repository.view.Reference, repository.view.Segment = ref, segment
			case "missing_tuple":
				segment, err := meter.NewSegmentRef(source.Scope, repository.view.Reference, intakeID(99))
				if err != nil {
					t.Fatal(err)
				}
				repository.view.Segment = segment
			case "target_storage":
				repository.targetErr = errors.New("synthetic private lookup failure")
			case "record_storage":
				repository.recordErr = errors.New("synthetic private record failure")
			case "record_invalid":
				repository.recordErr = meter.ErrInvalid
			}
			outcome, err := handler.Apply(t.Context(), &intakeTx{}, source, op)
			switch name {
			case "kind", "modulus":
				if err != nil || outcome.Result != protocol.Rejected || outcome.Code != "MALFORMED" {
					t.Fatal("invalid actual meter evidence did not produce bounded refusal", err)
				}
			case "missing_tuple":
				if !errors.Is(err, syncapp.ErrDependencyPending) {
					t.Fatal("mismatched actual target was not pending", err)
				}
			default:
				if !errors.Is(err, syncapp.ErrUnavailable) {
					t.Fatal("unverified producer failure was exposed", err)
				}
			}
		})
	}
}

func TestMeterIntakeActualServiceWindowIsExactBeforeRecording(t *testing.T) {
	for _, name := range []string{"before_install", "at_install", "at_removal", "after_removal", "one_ns_before_removal", "future_within_service"} {
		t.Run(name, func(t *testing.T) {
			source, _, repository, handler := intakeFixture(t)
			op := intakeOperation(t, source, func(envelope, _ map[string]any) {
				if name == "future_within_service" {
					envelope["occurred_at"] = "9999-12-31T23:59:59.999999999Z"
				}
			})
			declared, err := time.Parse(time.RFC3339Nano, op.Snapshot().OccurredAt)
			if err != nil {
				t.Fatal(err)
			}
			wantMalformed := false
			switch name {
			case "before_install":
				repository.view.InstalledAt = declared.Add(time.Nanosecond)
				wantMalformed = true
			case "at_install":
				repository.view.InstalledAt = declared
			case "at_removal":
				removed := declared
				repository.view.RemovedAt = &removed
				wantMalformed = true
			case "after_removal":
				removed := declared.Add(-time.Nanosecond)
				repository.view.RemovedAt = &removed
				wantMalformed = true
			case "one_ns_before_removal":
				removed := declared.Add(time.Nanosecond)
				repository.view.RemovedAt = &removed
			}
			outcome, err := handler.Apply(t.Context(), &intakeTx{}, source, op)
			if wantMalformed {
				if err != nil || outcome.Result != protocol.Rejected || outcome.Code != "MALFORMED" || repository.recorded != 0 {
					t.Fatal("known outside-service input reached repository or became retryable", err)
				}
			} else if err != nil || outcome.Result != protocol.Accepted || repository.recorded != 1 {
				t.Fatal("exact valid/future service instant was rejected or inferred suspect", err)
			}
		})
	}
}

func TestMeterIntakeRejectsMismatchedActualEvidenceAndLateDenial(t *testing.T) {
	for _, name := range []string{"value", "declared", "author", "capture", "operation", "origin", "method", "state"} {
		t.Run(name, func(t *testing.T) {
			source, _, repository, handler := intakeFixture(t)
			op := intakeOperation(t, source, nil)
			repository.transform = func(in meter.ReadingInput) meter.ReadingInput {
				switch name {
				case "value":
					in.Value, _ = meter.NewValue(meter.Odometer, 42, nil)
				case "declared":
					d := *in.Declared
					d.DeviceOffsetSeconds++
					in.Declared = &d
				case "author":
					in.Author, _ = meter.NewMembershipRef(source.Scope, intakeID(99))
				case "capture":
					capture, _ := meter.NewCaptureSource(source.Scope, meter.CaptureEnrolledDevice, intakeID(99))
					in.Capture = &capture
				case "operation":
					in.Operation, _ = meter.NewOperationRef(source.Scope, intakeID(99))
				case "origin":
					in.Origin = meter.Fueling
				case "method":
					in.Method = meter.Integration
				case "state":
					in.State = meter.Suspect
				}
				return in
			}
			if _, err := handler.Apply(t.Context(), &intakeTx{}, source, op); !errors.Is(err, syncapp.ErrUnavailable) {
				t.Fatal("mismatched record became ACCEPTED", err)
			}
		})
	}
	source, authority, repository, handler := intakeFixture(t)
	op := intakeOperation(t, source, nil)
	outcome, err := handler.Apply(t.Context(), &intakeTx{}, source, op)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(repository.reading.Snapshot().Declared)
	authority.err = orgapi.Forbidden
	if err := handler.Confirm(t.Context(), &intakeTx{}, source, op, syncapp.Write, outcome); !errors.Is(err, syncapp.ErrSourceDenied) {
		t.Fatal("late source denial allowed final receipt", err)
	}
	after, _ := json.Marshal(repository.reading.Snapshot().Declared)
	if !bytes.Equal(before, after) {
		t.Fatal("refused confirmation rewrote immutable evidence")
	}
}
