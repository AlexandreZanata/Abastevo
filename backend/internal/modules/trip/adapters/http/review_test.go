package http

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
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
