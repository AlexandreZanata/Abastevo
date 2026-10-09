package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type fakeStore struct {
	report registry.Report
	items  []registry.Assertion
	set    int
}

func (f *fakeStore) CreateRun(_ context.Context, id, _, snapshot, _ string) (string, bool, error) {
	return id, true, nil
}

func (f *fakeStore) GetRun(_ context.Context, _, _ string) (registry.Report, error) {
	return f.report, nil
}

func (f *fakeStore) FinishRun(_ context.Context, _ string, _ string, _, _, _ int64, _ string) error {
	return nil
}

func (f *fakeStore) StageAssertion(_ context.Context, _ registry.Assertion) (bool, error) {
	return true, nil
}

func (f *fakeStore) ListAssertions(_ context.Context, _ string) ([]registry.Assertion, error) {
	return f.items, nil
}

func (f *fakeStore) SetAssertionStation(_ context.Context, _, _ string) error {
	f.set++
	return nil
}

func (f *fakeStore) SetAssertionSuperseded(_ context.Context, _, _ string) error {
	return nil
}

type fakeCanon struct {
	station string
	err     error
	points  int
	status  map[string]string
}

func (f *fakeCanon) ResolveStation(_ context.Context, _, _ string, _ map[string]string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.station, nil
}

func (f *fakeCanon) RecordReviewedPoint(_ context.Context, _ string, _, _ float64, _ string) error {
	f.points++
	return nil
}

func (f *fakeCanon) SetStatus(_ context.Context, station, status string) error {
	if f.status == nil {
		f.status = map[string]string{}
	}
	f.status[station] = status
	return nil
}

func TestReconcileHandlerPublishesCompleteRun(t *testing.T) {
	store := &fakeStore{
		report: registry.Report{RunID: "run-1", State: "complete"},
		items: []registry.Assertion{{
			ID: "a-1", Source: registry.SourceCSV, SourceKey: "04218406000104",
			DisplayName: "ALFA", Address: map[string]string{"municipio_ibge": "3550308", "uf": "SP"},
			AuthState: "authorized", Eligibility: "eligible",
		}},
	}
	canon := &fakeCanon{station: "station-1"}
	payload, err := EncodePayload(registry.SourceCSV, "snap-1")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	handler := Reconcile{Store: store, Canon: canon}
	if err := handler.Handle(context.Background(), jobs.Job{Kind: handler.Kind(), Payload: payload}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if store.set != 1 {
		t.Fatalf("linked = %d, want 1", store.set)
	}
	if canon.status["station-1"] != "active" {
		t.Fatalf("status = %v", canon.status)
	}
	if handler.Kind() != "registry-reconcile" || handler.Version() != 1 {
		t.Fatal("handler identity changed")
	}
	var decoded Payload
	if err := json.Unmarshal(payload, &decoded); err != nil || decoded.Version != 1 {
		t.Fatalf("envelope = %s, err = %v", payload, err)
	}
	if got := DedupeKey("registry-csv", "snap-1"); got == "" {
		t.Fatal("empty dedupe key")
	}
}

func TestReconcileHandlerRefusesBadEnvelopes(t *testing.T) {
	handler := Reconcile{Store: &fakeStore{}, Canon: &fakeCanon{station: "s"}}
	for _, payload := range []string{
		`{}`,
		`{"version": 2, "source": "registry-csv", "snapshot": "s"}`,
		`{"version": 1, "source": "", "snapshot": "s"}`,
		`not-json`,
	} {
		if err := handler.Handle(context.Background(), jobs.Job{Payload: []byte(payload)}); err == nil {
			t.Fatalf("payload %q must fail", payload)
		}
	}
}

func TestReconcileHandlerSurfacesRowFailure(t *testing.T) {
	store := &fakeStore{
		report: registry.Report{RunID: "run-1", State: "complete"},
		items: []registry.Assertion{{
			ID: "a-1", Source: registry.SourceCSV, SourceKey: "04218406000104",
			DisplayName: "ALFA", AuthState: "authorized", Eligibility: "eligible",
		}},
	}
	canon := &fakeCanon{err: errors.New("canon down")}
	payload, _ := EncodePayload(registry.SourceCSV, "snap-1")
	handler := Reconcile{Store: store, Canon: canon}
	if err := handler.Handle(context.Background(), jobs.Job{Payload: payload}); err == nil {
		t.Fatal("canon failure must surface for dispatcher retry")
	}
}
