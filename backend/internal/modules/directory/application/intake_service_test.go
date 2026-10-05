package application

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeIntakeStore struct {
	mu    sync.Mutex
	rows  map[string]SuggestionRow
	order []string
	quota int64
}

func (f *fakeIntakeStore) CreateSuggestion(_ context.Context, id, accountID, clientKey string, proposal []byte, evidenceRef string) (SuggestionRow, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := accountID + "|" + clientKey
	if _, ok := f.rows[key]; ok {
		return SuggestionRow{}, false, nil
	}
	row := SuggestionRow{ID: id, AccountID: accountID, ClientSubmissionID: clientKey, Proposal: proposal, EvidenceRef: evidenceRef, State: SuggestionPending, CreatedAt: time.Now()}
	if f.rows == nil {
		f.rows = map[string]SuggestionRow{}
	}
	f.rows[key] = row
	f.order = append(f.order, key)
	return row, true, nil
}

func (f *fakeIntakeStore) SuggestionByKey(_ context.Context, accountID, clientKey string) (SuggestionRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[accountID+"|"+clientKey]
	if !ok {
		return SuggestionRow{}, ErrSuggestionNotFound
	}
	return row, nil
}

func (f *fakeIntakeStore) GetSuggestion(_ context.Context, id string) (SuggestionRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range f.rows {
		if row.ID == id {
			return row, nil
		}
	}
	return SuggestionRow{}, ErrSuggestionNotFound
}

func (f *fakeIntakeStore) ListOwned(_ context.Context, accountID string, limit, offset int) ([]SuggestionRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []SuggestionRow
	for _, key := range f.order {
		row := f.rows[key]
		if row.AccountID == accountID {
			out = append(out, row)
		}
	}
	if offset > len(out) {
		return nil, nil
	}
	out = out[offset:]
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeIntakeStore) CountRecent(_ context.Context, _ string) (int64, error) {
	return f.quota, nil
}

func (f *fakeIntakeStore) CancelSuggestion(_ context.Context, id, accountID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for key, row := range f.rows {
		if row.ID == id && row.AccountID == accountID && row.State == SuggestionPending {
			row.State = SuggestionCancelled
			f.rows[key] = row
			return 1, nil
		}
	}
	return 0, nil
}

func testService(store *fakeIntakeStore) IntakeService {
	n := 0
	return IntakeService{
		Store: store,
		Clock: func() time.Time { return time.Now() },
		NewID: func() (string, error) { n++; return "sug-" + string(rune('0'+n)), nil },
	}
}

func validInput() ProposalInput {
	return ProposalInput{DisplayName: "Posto Novo", MunicipalityCode: "3550308", State: "SP", CNPJ: "04218406000104"}
}

func TestSubmitIsIdempotentAndConflictSafe(t *testing.T) {
	svc := testService(&fakeIntakeStore{})
	first, created, err := svc.Submit(context.Background(), "acc-1", "key-1", validInput())
	if err != nil || !created {
		t.Fatalf("submit = %+v, %v, %v", first, created, err)
	}
	second, created, err := svc.Submit(context.Background(), "acc-1", "key-1", validInput())
	if err != nil || created || second.ID != first.ID {
		t.Fatalf("replay = %+v, %v, %v", second, created, err)
	}
	changed := validInput()
	changed.DisplayName = "Posto Trocado"
	if _, _, err := svc.Submit(context.Background(), "acc-1", "key-1", changed); err != ErrSuggestionConflict {
		t.Fatalf("changed body err = %v", err)
	}
}

func TestSubmitEnforcesQuotaAndValidation(t *testing.T) {
	store := &fakeIntakeStore{quota: MaxSuggestionsPerDay}
	svc := testService(store)
	if _, _, err := svc.Submit(context.Background(), "acc-1", "key-1", validInput()); err != ErrSuggestionQuota {
		t.Fatalf("quota err = %v", err)
	}
	svc2 := testService(&fakeIntakeStore{})
	bad := validInput()
	bad.DisplayName = ""
	if _, _, err := svc2.Submit(context.Background(), "acc-1", "key-1", bad); err == nil {
		t.Fatal("invalid proposal must fail before storage")
	}
}

func TestCancelAndOwnedAreOwnerOnly(t *testing.T) {
	svc := testService(&fakeIntakeStore{})
	created, _, err := svc.Submit(context.Background(), "acc-1", "key-1", validInput())
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := svc.Cancel(context.Background(), "acc-2", created.ID); err == nil {
		t.Fatal("foreign cancel must fail")
	}
	if _, err := svc.Owned(context.Background(), "acc-2", created.ID); err == nil {
		t.Fatal("foreign read must fail")
	}
	if err := svc.Cancel(context.Background(), "acc-1", created.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if err := svc.Cancel(context.Background(), "acc-1", created.ID); err != ErrSuggestionClosed {
		t.Fatalf("double cancel err = %v", err)
	}
	owned, err := svc.Owned(context.Background(), "acc-1", created.ID)
	if err != nil || owned.State != SuggestionCancelled {
		t.Fatalf("owned = %+v, err = %v", owned, err)
	}
	listed, err := svc.ListOwned(context.Background(), "acc-1", 20, 0)
	if err != nil || len(listed) != 1 {
		t.Fatalf("listed = %+v, err = %v", listed, err)
	}
}
