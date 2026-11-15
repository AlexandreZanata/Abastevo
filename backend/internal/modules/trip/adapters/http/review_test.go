package http

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	amend "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgamend/domain"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	period "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain/period"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

// EDR-222: strict lossless bodies, the closed refusal vocabulary and the two
// policies this module admits.
func TestReviewRequestsRefusalsAndRecords(t *testing.T) {
	var decide reviewDecideRequest
	if err := decide.UnmarshalJSON([]byte(`{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008601","revision":1,"decision":"RETURN","reason":"Informe o motivo"}`)); err != nil || decide.Reason != "Informe o motivo" || *decide.Revision != 1 {
		t.Fatal("valid decision body refused", err)
	}
	for name, raw := range map[string]string{
		"unknown key":    `{"schema_version":1,"operation_id":"x","revision":1,"decision":"APPROVE","reason":"","reviewer":"x"}`,
		"lone surrogate": `{"schema_version":1,"operation_id":"x","revision":1,"decision":"RETURN","reason":"\ud800"}`,
		"null reason":    `{"schema_version":1,"operation_id":"x","revision":1,"decision":"RETURN","reason":null}`,
		"upper key":      `{"Schema_version":1}`,
	} {
		var v reviewDecideRequest
		if err := v.UnmarshalJSON([]byte(raw)); !errors.Is(err, orgapi.Malformed) {
			t.Fatal("malformed decision body admitted", name, err)
		}
	}
	var read reviewReadRequest
	if err := read.UnmarshalJSON([]byte(`{"limit":1}`)); !errors.Is(err, orgapi.Malformed) {
		t.Fatal("a read body with a key was admitted", err)
	}
	var list reviewListRequest
	if err := list.UnmarshalJSON([]byte(`{"limit":5}`)); err != nil || list.Limit != 5 {
		t.Fatal("valid list body refused", err)
	}
	for _, c := range []struct {
		in   error
		want orgapi.Code
	}{
		{review.ErrSelfApproval, orgapi.SelfApproval}, {review.ErrConflict, orgapi.Conflict}, {review.ErrInvalidTransition, orgapi.Conflict},
		{review.ErrReasonRequired, orgapi.ValidationFailed}, {review.ErrInvalid, orgapi.ValidationFailed}, {app.ErrNotFound, orgapi.NotFound},
		{app.ErrOperationReused, orgapi.OperationReused}, {access.ErrFreshAuthRequired, orgapi.FreshAuthRequired}, {errors.New("private provider failure"), orgapi.Unavailable},
		{app.ErrPeriodLocked, orgapi.PeriodLocked}, // EMP-TRIP-016, B-BR-EMP-359
		{app.ErrUnavailable, orgapi.Unavailable},
	} {
		if got := reviewMapped(c.in); !errors.Is(got, c.want) {
			t.Fatal("refusal mapping", c.in, got)
		}
	}
	org, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000065")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	facts := permissions.ResourceFacts{OrgID: org, ID: org.String()}
	if err := (ReviewRecords{}).Check(context.Background(), scope, access.Subject{}, app.ReviewPolicy(app.ReviewReadAction), facts, nil); !errors.Is(err, access.ErrUnavailable) {
		t.Fatal("a check without a subject or rules passed", err)
	}
	if v, err := (queueRoot{}).Resolve(context.Background(), scope, access.Subject{}); err != nil || v.ID != org.String() {
		t.Fatal("the queue resolves another resource", err)
	}
}

// EMP-TRIP-015 (EDR-236): the amendment body is strict and lossless, its
// readings are canonical decimals of exact base units, and the kernel
// refusals keep their bounded codes.
func TestAmendmentRequestReadingsAndRefusals(t *testing.T) {
	var in amendRequest
	if err := in.UnmarshalJSON([]byte(`{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008801","revision":1,"reason":"Leitura final digitada errada","end_reading":{"kind":"ODOMETER","value":"184600000"}}`)); err != nil ||
		in.EndReading == nil || in.StartReading != nil || *in.Revision != 1 {
		t.Fatal("valid amendment body refused", err)
	}
	for name, raw := range map[string]string{
		"unknown key":    `{"schema_version":1,"operation_id":"x","revision":1,"reason":"r","end_reading":{"kind":"ODOMETER","value":"1"},"review_state":"APPROVED"}`,
		"null reading":   `{"schema_version":1,"operation_id":"x","revision":1,"reason":"r","end_reading":null}`,
		"lone surrogate": `{"schema_version":1,"operation_id":"x","revision":1,"reason":"\udc00","end_reading":{"kind":"ODOMETER","value":"1"}}`,
	} {
		var v amendRequest
		if err := v.UnmarshalJSON([]byte(raw)); !errors.Is(err, orgapi.Malformed) {
			t.Fatal("malformed amendment body admitted", name, err)
		}
	}
	for value, want := range map[string]int64{"0": 0, "184600000": 184_600_000, "9223372036854775807": 9223372036854775807} {
		if c, err := correction(&amendReading{Kind: "ODOMETER", Value: value}); err != nil || c.Value != want {
			t.Fatal("canonical reading refused", value, err)
		}
	}
	for _, value := range []string{"", "01", "-1", "1.5", "1e3", " 1", "9223372036854775808", "99999999999999999999"} {
		if _, err := correction(&amendReading{Kind: "ODOMETER", Value: value}); !errors.Is(err, orgapi.Malformed) {
			t.Fatal("non-canonical reading admitted", value, err)
		}
	}
	if c, err := correction(nil); c != nil || err != nil {
		t.Fatal("an absent reading is not absent")
	}
	for _, c := range []struct {
		in   error
		want orgapi.Code
	}{
		{amend.ErrConflict, orgapi.Conflict}, {amend.ErrNotApproved, orgapi.Conflict}, {amend.ErrAmendmentPending, orgapi.Conflict},
		{amend.ErrPeriodLocked, orgapi.PeriodLocked}, {period.ErrReasonRequired, orgapi.ValidationFailed}, {period.ErrReasonInvalid, orgapi.ValidationFailed},
		{app.ErrInvalidAmendment, orgapi.ValidationFailed}, {amend.ErrLostOriginal, orgapi.Unavailable},
	} {
		if got := reviewMapped(c.in); !errors.Is(got, c.want) {
			t.Fatal("amendment refusal mapping", c.in, got)
		}
	}
	org, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000065")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	facts := permissions.ResourceFacts{OrgID: org, ID: org.String()}
	if err := (ReviewRecords{}).Check(context.Background(), scope, access.Subject{}, app.AmendPolicy(), facts, nil); !errors.Is(err, access.ErrUnavailable) {
		t.Fatal("an amendment check without a subject or rules passed", err)
	}
}
