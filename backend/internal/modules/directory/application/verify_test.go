package application

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

type verifyStore struct {
	mu        sync.Mutex
	rows      map[string]SuggestionRow
	decisions int
}

func (f *verifyStore) GetSuggestion(_ context.Context, id string) (SuggestionRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[id]
	if !ok {
		return SuggestionRow{}, ErrSuggestionNotFound
	}
	return row, nil
}

func (f *verifyStore) CreateDecision(_ context.Context, _, suggestionID, _, _, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.decisions++
	return nil
}

func (f *verifyStore) SetSuggestionState(_ context.Context, id, state string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[id]
	if !ok || row.State != SuggestionPending {
		return 0, nil
	}
	row.State = state
	f.rows[id] = row
	return 1, nil
}

func (f *verifyStore) ListPending(_ context.Context, _ int) ([]SuggestionRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []SuggestionRow
	for _, row := range f.rows {
		if row.State == SuggestionPending {
			out = append(out, row)
		}
	}
	return out, nil
}

func storedRow(id, cnpj, ibge string) SuggestionRow {
	body, _ := json.Marshal(Proposal{DisplayName: "Posto", MunicipalityCode: ibge, State: "SP", CNPJ: cnpj})
	return SuggestionRow{ID: id, AccountID: "acc-1", Proposal: body, State: SuggestionPending}
}

func verifyPorts(store *verifyStore) VerifyPorts {
	n := 0
	return VerifyPorts{
		Store: store,
		Resolve: func(_ context.Context, cnpj, _ string, _ map[string]string) (string, string, string, error) {
			if cnpj == "04218406000104" {
				return "station-alfa", "3550308", "SP", nil
			}
			return "", "", "", ErrStationUnknown
		},
		RecordPin: func(_ context.Context, _ string, _, _ float64, _ string) error { return nil },
		NewID: func() (string, error) {
			n++
			return "dec-" + string(rune('0'+n)), nil
		},
	}
}

func TestAutoVerifyApprovesExactMatchOnly(t *testing.T) {
	store := &verifyStore{rows: map[string]SuggestionRow{
		"s-1": storedRow("s-1", "04218406000104", "3550308"),
		"s-2": storedRow("s-2", "12ABC34501DE35", "3550308"),
		"s-3": storedRow("s-3", "04218406000104", "3106200"),
	}}
	ports := verifyPorts(store)

	approved, err := AutoVerify(context.Background(), ports, "s-1")
	if err != nil || !approved.Approved || approved.StationID != "station-alfa" {
		t.Fatalf("exact = %+v, err = %v", approved, err)
	}
	// Unknown station and conflicting municipality stay pending.
	deferred, err := AutoVerify(context.Background(), ports, "s-2")
	if err != nil || deferred.Approved {
		t.Fatalf("unknown = %+v, err = %v", deferred, err)
	}
	conflict, err := AutoVerify(context.Background(), ports, "s-3")
	if err != nil || conflict.Approved {
		t.Fatalf("conflict = %+v, err = %v", conflict, err)
	}
	if store.rows["s-1"].State != "approved" {
		t.Fatalf("state = %q", store.rows["s-1"].State)
	}
	// Replay of a decided record is a harmless no-op.
	replay, err := AutoVerify(context.Background(), ports, "s-1")
	if err != nil || replay.Approved {
		t.Fatalf("replay = %+v, err = %v", replay, err)
	}
}

func TestDecideEnforcesReviewerReasonAndPending(t *testing.T) {
	store := &verifyStore{rows: map[string]SuggestionRow{
		"s-1": storedRow("s-1", "04218406000104", "3550308"),
	}}
	ports := verifyPorts(store)

	if err := Decide(context.Background(), ports, "s-1", "", true, "ok", "station-alfa"); err != ErrVerifyReviewer {
		t.Fatalf("reviewer err = %v", err)
	}
	if err := Decide(context.Background(), ports, "s-1", "op-1", true, "  ", "station-alfa"); err != ErrVerifyReason {
		t.Fatalf("reason err = %v", err)
	}
	if err := Decide(context.Background(), ports, "s-1", "op-1", false, "not a station", ""); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if store.rows["s-1"].State != "rejected" {
		t.Fatalf("state = %q", store.rows["s-1"].State)
	}
	if err := Decide(context.Background(), ports, "s-1", "op-2", true, "override", "station-alfa"); err != ErrVerifyClosed {
		t.Fatalf("second decision err = %v", err)
	}
	if err := Decide(context.Background(), ports, "missing", "op-1", true, "ok", "s"); err == nil {
		t.Fatal("missing record must fail")
	}
}

func TestGoneAuthorDefersAutomationAndFailsReviewClosed(t *testing.T) {
	store := &verifyStore{rows: map[string]SuggestionRow{
		"s-1": storedRow("s-1", "04218406000104", "3550308"),
	}}
	ports := verifyPorts(store)
	ports.AccountLive = func(_ context.Context, _ string) (bool, error) { return false, nil }

	deferred, err := AutoVerify(context.Background(), ports, "s-1")
	if err != nil || deferred.Approved {
		t.Fatalf("gone author = %+v, err = %v (must defer)", deferred, err)
	}
	if err := Decide(context.Background(), ports, "s-1", "op-1", true, "ok", "station-alfa"); err != ErrAccountGone {
		t.Fatalf("review err = %v (must fail closed)", err)
	}
	if store.rows["s-1"].State != SuggestionPending {
		t.Fatalf("state = %q (must stay pending)", store.rows["s-1"].State)
	}
}
