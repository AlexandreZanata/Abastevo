package jobs

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type verifyFakeStore struct {
	mu    sync.Mutex
	rows  map[string]application.SuggestionRow
	calls int
}

func storedSuggestion(id, cnpj, ibge string) application.SuggestionRow {
	body, _ := json.Marshal(application.Proposal{DisplayName: "Posto", MunicipalityCode: ibge, State: "SP", CNPJ: cnpj})
	return application.SuggestionRow{ID: id, AccountID: "acc-1", Proposal: body, State: application.SuggestionPending}
}

func (f *verifyFakeStore) GetSuggestion(_ context.Context, id string) (application.SuggestionRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[id]
	if !ok {
		return application.SuggestionRow{}, application.ErrSuggestionNotFound
	}
	return row, nil
}

func (f *verifyFakeStore) CreateDecision(_ context.Context, _, _, _, _, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return nil
}

func (f *verifyFakeStore) SetSuggestionState(_ context.Context, id, state string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[id]
	if !ok || row.State != application.SuggestionPending {
		return 0, nil
	}
	row.State = state
	f.rows[id] = row
	return 1, nil
}

func (f *verifyFakeStore) ListPending(_ context.Context, limit int) ([]application.SuggestionRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []application.SuggestionRow
	for _, row := range f.rows {
		if row.State == application.SuggestionPending && len(out) < limit {
			out = append(out, row)
		}
	}
	return out, nil
}

func TestVerifySweepApprovesExactAndDefersRest(t *testing.T) {
	store := &verifyFakeStore{rows: map[string]application.SuggestionRow{
		"s-1": storedSuggestion("s-1", "04218406000104", "3550308"),
		"s-2": storedSuggestion("s-2", "12ABC34501DE35", "3550308"),
	}}
	pins := 0
	handler := VerifySweep{
		Store: store,
		Resolve: func(_ context.Context, cnpj, _ string, _ map[string]string) (string, string, string, error) {
			if cnpj == "04218406000104" {
				return "station-alfa", "3550308", "SP", nil
			}
			return "", "", "", application.ErrStationUnknown
		},
		RecordPin: func(_ context.Context, _ string, _, _ float64, _ string) error { pins++; return nil },
		NewID:     func() (string, error) { return "dec-1", nil },
		Batch:     25,
	}
	payload, _ := json.Marshal(map[string]any{"version": 1, "batch": 25})
	if err := handler.Handle(context.Background(), jobs.Job{Kind: handler.Kind(), Payload: payload}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if store.rows["s-1"].State != "approved" {
		t.Fatalf("s-1 = %q", store.rows["s-1"].State)
	}
	if store.rows["s-2"].State != application.SuggestionPending {
		t.Fatalf("s-2 = %q (must stay pending, never auto-rejected)", store.rows["s-2"].State)
	}
	if handler.Kind() != "suggestion-verify" || handler.Version() != 1 {
		t.Fatal("handler identity changed")
	}
}

func TestVerifySweepRefusesBadEnvelope(t *testing.T) {
	handler := VerifySweep{Store: &verifyFakeStore{rows: map[string]application.SuggestionRow{}}}
	for _, payload := range []string{`{}`, `{"version": 2}`, `not-json`} {
		if err := handler.Handle(context.Background(), jobs.Job{Payload: []byte(payload)}); err == nil {
			t.Fatalf("payload %q must fail", payload)
		}
	}
}
