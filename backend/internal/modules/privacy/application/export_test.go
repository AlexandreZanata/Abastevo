package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

type fakeExportStore struct {
	requests map[string]domain.Request
	archives map[string][]byte
	jobs     []string
	failJob  bool
}

func newFakeExportStore() *fakeExportStore {
	return &fakeExportStore{requests: map[string]domain.Request{}, archives: map[string][]byte{}}
}

func (f *fakeExportStore) RequestExport(_ context.Context, r domain.Request, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (string, bool, error) {
	for _, existing := range f.requests {
		if existing.ContributorID == r.ContributorID && existing.ClientSubmissionID == r.ClientSubmissionID {
			if existing.Type != r.Type || existing.ContributorRef != r.ContributorRef {
				return "", false, domain.ErrConflict
			}
			return existing.ID, true, nil
		}
	}
	if f.failJob {
		return "", false, errors.New("enqueue down")
	}
	f.requests[r.ID] = r
	if enqueue != nil {
		_ = enqueue(context.Background(), nil, "privacy-export-build", []byte(`{}`), "privacy-export:"+r.ID)
		f.jobs = append(f.jobs, "privacy-export-build")
	}
	return r.ID, false, nil
}

func (f *fakeExportStore) CompleteExport(_ context.Context, id string, archive []byte, sha string, readyAt time.Time) error {
	r, ok := f.requests[id]
	if !ok || r.Status != domain.StatusRequested {
		return domain.ErrBadState
	}
	r.Status = domain.StatusReady
	r.ArchiveSHA256 = sha
	r.ReadyAt = readyAt
	r.ExpiresAt = readyAt.Add(domain.DownloadTTL)
	r.CompletedAt = readyAt
	f.requests[id] = r
	f.archives[id] = append([]byte{}, archive...)
	return nil
}

func (f *fakeExportStore) FailExport(_ context.Context, id string, at time.Time) error {
	r, ok := f.requests[id]
	if !ok || r.Status != domain.StatusRequested {
		return domain.ErrBadState
	}
	r.Status = domain.StatusFailed
	r.CompletedAt = at
	f.requests[id] = r
	return nil
}

func (f *fakeExportStore) Get(_ context.Context, id string) (domain.Request, error) {
	r, ok := f.requests[id]
	if !ok {
		return domain.Request{}, domain.ErrNotFound
	}
	return r, nil
}

func (f *fakeExportStore) GetForOwner(_ context.Context, contributorID, id string) (domain.Request, []byte, error) {
	r, ok := f.requests[id]
	if !ok || r.ContributorID != contributorID {
		return domain.Request{}, nil, domain.ErrNotFound
	}
	return r, f.archives[id], nil
}

type fakeInventory struct {
	snapshot RawInventory
	err      error
}

func (f fakeInventory) Snapshot(context.Context, string, string) (RawInventory, error) {
	if f.err != nil {
		return RawInventory{}, f.err
	}
	return f.snapshot, nil
}

func exportPorts(store *fakeExportStore, inv InventoryReader) Ports {
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	n := 0
	return Ports{
		Clock: func() time.Time { return base },
		NewID: func() (string, error) {
			n++
			return "e0000000-0000-4000-8000-00000000000" + string(rune('0'+n)), nil
		},
		Attribution: func(_ context.Context, contributorID string) (string, error) {
			return "tok-" + contributorID, nil
		},
		Store:     store,
		Inventory: inv,
		EnqueueJob: func(context.Context, pgx.Tx, string, []byte, string) error {
			return nil
		},
	}
}

// ownerInventory mixes owner rows with a stray foreign row from a buggy
// join: Build must drop the foreign row, never export it.
func ownerInventory() RawInventory {
	return RawInventory{
		Contributor: ContributorView{ContributorID: "c1", Status: "active", CreatedAt: "2026-01-01T00:00:00Z"},
		Observations: []RawObservation{
			{OwnerRef: "tok-c1", View: ObservationView{ObservationID: "o1", StationID: "s1", Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: 5890, Condition: "STANDARD", State: "VALIDATED", ReceivedAt: "2026-09-01T00:00:00Z"}},
			{OwnerRef: "tok-evil", View: ObservationView{ObservationID: "o-evil", StationID: "s9", Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: 1, Condition: "STANDARD", State: "VALIDATED", ReceivedAt: "2026-09-01T00:00:00Z"}},
		},
		Trust: RawTrust{OwnerRef: "tok-c1", Tier: "NEW"},
	}
}

func TestRequestPersistsIntentAndJob(t *testing.T) {
	store := newFakeExportStore()
	res, err := Request(context.Background(), exportPorts(store, fakeInventory{}), Caller{ContributorID: "c1"}, RequestDTO{ClientSubmissionID: "export-1"})
	if err != nil {
		t.Fatalf("request = %v", err)
	}
	if res.Replayed {
		t.Error("first request replayed")
	}
	if len(store.jobs) != 1 || store.jobs[0] != "privacy-export-build" {
		t.Errorf("jobs = %v", store.jobs)
	}
}

func TestRequestConvergesIdenticalRetry(t *testing.T) {
	store := newFakeExportStore()
	p := exportPorts(store, fakeInventory{})
	if _, err := Request(context.Background(), p, Caller{ContributorID: "c1"}, RequestDTO{ClientSubmissionID: "export-1"}); err != nil {
		t.Fatal(err)
	}
	second, err := Request(context.Background(), p, Caller{ContributorID: "c1"}, RequestDTO{ClientSubmissionID: "export-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed {
		t.Errorf("identical retry did not replay: %+v", second)
	}
	if len(store.requests) != 1 {
		t.Errorf("requests = %d, want 1 converged row", len(store.requests))
	}
}

func TestRequestRejectsAnonymousAndEmptyKey(t *testing.T) {
	store := newFakeExportStore()
	if _, err := Request(context.Background(), exportPorts(store, fakeInventory{}), Caller{}, RequestDTO{ClientSubmissionID: "x"}); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("anonymous = %v", err)
	}
	if _, err := Request(context.Background(), exportPorts(store, fakeInventory{}), Caller{ContributorID: "c1"}, RequestDTO{}); !errors.Is(err, domain.ErrInvalidRequest) {
		t.Errorf("empty key = %v", err)
	}
}

func TestBuildRedactsForeignRows(t *testing.T) {
	store := newFakeExportStore()
	p := exportPorts(store, fakeInventory{snapshot: ownerInventory()})
	res, err := Request(context.Background(), p, Caller{ContributorID: "c1"}, RequestDTO{ClientSubmissionID: "export-1"})
	if err != nil {
		t.Fatal(err)
	}
	id, replayed, err := Build(context.Background(), p, res.RequestID)
	if err != nil || replayed || id != res.RequestID {
		t.Fatalf("build = %q %v %v", id, replayed, err)
	}
	if got := store.requests[id]; got.Status != domain.StatusReady {
		t.Fatalf("status = %q", got.Status)
	}
	var archive Archive
	if err := json.Unmarshal(store.archives[id], &archive); err != nil {
		t.Fatal(err)
	}
	if len(archive.Inventory.Observations) != 1 || archive.Inventory.Observations[0].ObservationID != "o1" {
		t.Errorf("redacted observations = %+v", archive.Inventory.Observations)
	}
	if archive.Format != domain.FormatV1 || archive.ContributorID != "c1" {
		t.Errorf("envelope = %+v", archive)
	}
	// Owner references are stripped from the shipped bytes.
	if strings.Contains(string(store.archives[id]), "tok-evil") {
		t.Error("foreign owner reference leaked into archive")
	}
}

func TestBuildReplaysTerminal(t *testing.T) {
	store := newFakeExportStore()
	p := exportPorts(store, fakeInventory{snapshot: ownerInventory()})
	res, err := Request(context.Background(), p, Caller{ContributorID: "c1"}, RequestDTO{ClientSubmissionID: "export-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Build(context.Background(), p, res.RequestID); err != nil {
		t.Fatal(err)
	}
	id, replayed, err := Build(context.Background(), p, res.RequestID)
	if err != nil || !replayed || id != res.RequestID {
		t.Errorf("terminal rebuild = %q %v %v", id, replayed, err)
	}
}

func TestBuildFailsInventoryUnavailable(t *testing.T) {
	store := newFakeExportStore()
	p := exportPorts(store, fakeInventory{err: errors.New("inventory down")})
	res, err := Request(context.Background(), p, Caller{ContributorID: "c1"}, RequestDTO{ClientSubmissionID: "export-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Build(context.Background(), p, res.RequestID); err == nil {
		t.Error("failed inventory accepted")
	}
	if store.requests[res.RequestID].Status != domain.StatusFailed {
		t.Errorf("status = %q", store.requests[res.RequestID].Status)
	}
}

func TestStatusAndDownloadOwnerGates(t *testing.T) {
	store := newFakeExportStore()
	p := exportPorts(store, fakeInventory{snapshot: ownerInventory()})
	res, err := Request(context.Background(), p, Caller{ContributorID: "c1"}, RequestDTO{ClientSubmissionID: "export-1"})
	if err != nil {
		t.Fatal(err)
	}
	q := exportPorts(store, fakeInventory{})
	if _, err := Status(context.Background(), q, Caller{ContributorID: "c2"}, res.RequestID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign status = %v", err)
	}
	st, err := Status(context.Background(), q, Caller{ContributorID: "c1"}, res.RequestID)
	if err != nil || st.Status != domain.StatusRequested {
		t.Errorf("status = %+v, %v", st, err)
	}
	if _, err := Download(context.Background(), q, Caller{ContributorID: "c1"}, res.RequestID); !errors.Is(err, domain.ErrBadState) {
		t.Errorf("premature download = %v", err)
	}
	if _, _, err := Build(context.Background(), p, res.RequestID); err != nil {
		t.Fatal(err)
	}
	dl, err := Download(context.Background(), q, Caller{ContributorID: "c1"}, res.RequestID)
	if err != nil || len(dl.Archive) == 0 || dl.SHA256 == "" {
		t.Errorf("download = %+v, %v", dl, err)
	}
	if _, err := Download(context.Background(), q, Caller{ContributorID: "c2"}, res.RequestID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign download = %v", err)
	}
}
