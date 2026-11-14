package adapters_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

const (
	jobOrg      = "7b0e2a54-0000-4000-8000-000000021401"
	jobDocument = "7b0e2a54-0000-4000-8000-000000021402"
)

func jobScope(t *testing.T) kernel.OrgScope {
	t.Helper()
	id, err := kernel.ParseOrgID(jobOrg)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

type recordedSafety struct {
	calls  int
	scope  kernel.OrgScope
	id     string
	finals []bool
	err    error
}

func (r *recordedSafety) Converge(_ context.Context, scope kernel.OrgScope, id string, final bool) (app.Outcome, error) {
	r.calls++
	r.scope, r.id = scope, id
	r.finals = append(r.finals, final)
	return app.Outcome{State: domain.Stored}, r.err
}

// EDR-170, BR-PRIV-003: the job carries references only, is keyed by
// organization and document per purpose, and reports static errors.
func TestSafetyJobPayloadIsReferenceOnlyAndBoundToItsKey(t *testing.T) {
	scope := jobScope(t)
	check, expiry := adapters.SafetyDedupe(adapters.CheckPurpose, scope, jobDocument), adapters.SafetyDedupe(adapters.ExpiryPurpose, scope, jobDocument)
	if check != "org-document-safety:check:"+jobOrg+":"+jobDocument || expiry != "org-document-safety:expiry:"+jobOrg+":"+jobDocument {
		t.Fatal("de-duplication keys", check, expiry)
	}
	for _, bad := range []string{adapters.SafetyDedupe("other", scope, jobDocument), adapters.SafetyDedupe(adapters.CheckPurpose, kernel.OrgScope{}, jobDocument),
		adapters.SafetyDedupe(adapters.CheckPurpose, scope, strings.ToUpper(jobDocument))} {
		if bad != "" {
			t.Fatal("invalid de-duplication input accepted", bad)
		}
	}
	payload, _ := json.Marshal(adapters.SafetyPayload{Version: 1, OrgID: jobOrg, DocumentID: jobDocument})
	parsed, id, err := adapters.ParseSafetyPayload(payload)
	if err != nil || parsed != scope || id != jobDocument {
		t.Fatal("version-1 payload refused", err)
	}
	for name, raw := range map[string]string{
		"version":   `{"version":2,"org_id":"` + jobOrg + `","document_id":"` + jobDocument + `"}`,
		"unknown":   `{"version":1,"org_id":"` + jobOrg + `","document_id":"` + jobDocument + `","sha256":"x"}`,
		"org":       `{"version":1,"org_id":"x","document_id":"` + jobDocument + `"}`,
		"document":  `{"version":1,"org_id":"` + jobOrg + `","document_id":"` + strings.ToUpper(jobDocument) + `"}`,
		"truncated": `{"version":1`,
	} {
		if _, _, err := adapters.ParseSafetyPayload([]byte(raw)); err == nil {
			t.Fatal("malformed payload accepted", name)
		}
	}
	safety := &recordedSafety{}
	handler := adapters.SafetyHandler{Safety: safety}
	if handler.Kind() != "org-document-safety" || handler.Version() != 1 || adapters.SafetyAttempts != 10 {
		t.Fatal("handler identity")
	}
	job := jobs.Job{Kind: adapters.SafetyKind, Payload: payload, DedupeKey: check, Attempts: 1, MaxAttempts: 10}
	if err := handler.Handle(t.Context(), job); err != nil || safety.calls != 1 || safety.scope != scope || safety.id != jobDocument || safety.finals[0] {
		t.Fatal("check job did not converge its document", err)
	}
	job.DedupeKey, job.Attempts = expiry, 10
	if err := handler.Handle(t.Context(), job); err != nil || safety.calls != 2 || !safety.finals[1] {
		t.Fatal("final expiry attempt", err)
	}
	job.Attempts = 11 // a final attempt reclaimed after its lease expired
	if err := handler.Handle(t.Context(), job); err != nil || !safety.finals[2] {
		t.Fatal("reclaimed final attempt", err)
	}
	for name, bad := range map[string]jobs.Job{
		"kind":      {Kind: "org-export-build", Payload: payload, DedupeKey: check, Attempts: 1, MaxAttempts: 10},
		"foreign":   {Kind: adapters.SafetyKind, Payload: payload, DedupeKey: "org-document-safety:check:" + jobOrg + ":7b0e2a54-0000-4000-8000-000000021499", Attempts: 1, MaxAttempts: 10},
		"payload":   {Kind: adapters.SafetyKind, Payload: []byte(`{"version":1}`), DedupeKey: check, Attempts: 1, MaxAttempts: 10},
		"unbounded": {Kind: adapters.SafetyKind, Payload: payload, DedupeKey: "", Attempts: 1, MaxAttempts: 10},
	} {
		if err := handler.Handle(t.Context(), bad); err == nil {
			t.Fatal("job outside its envelope ran", name)
		}
	}
	if safety.calls != 3 {
		t.Fatal("refused envelope reached the check")
	}
	for cause, quiet := range map[error]bool{app.ErrNotFound: true, app.ErrNotDue: false, app.ErrRetry: false, app.ErrUnavailable: false,
		errors.New("PRIVATE-PROVIDER-DETAIL org/" + jobOrg + "/doc/"): false} {
		safety.err = cause
		err := handler.Handle(t.Context(), jobs.Job{Kind: adapters.SafetyKind, Payload: payload, DedupeKey: check, Attempts: 1, MaxAttempts: 10})
		if quiet != (err == nil) {
			t.Fatal("job outcome mapping", cause, err)
		}
		if err != nil && (strings.Contains(err.Error(), "PRIVATE") || strings.Contains(err.Error(), jobOrg) || strings.Contains(err.Error(), "/doc/")) {
			t.Fatal("queue error carries private detail", err)
		}
	}
	if err := (adapters.SafetyHandler{}).Handle(t.Context(), jobs.Job{Kind: adapters.SafetyKind, Payload: payload, DedupeKey: check}); err == nil {
		t.Fatal("unconfigured handler acknowledged a job")
	}
}
