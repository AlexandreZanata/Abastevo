package registry

import (
	"context"
	"strings"
	"testing"
)

// fakeStore is an in-memory Store: replay converges without duplicates
// exactly like the (source, source_key, checksum) unique key.
type fakeStore struct {
	runs       map[string]Report
	assertions map[string]Assertion
	failStage  bool
	nextID     int
}

func newFakeStore() *fakeStore {
	return &fakeStore{runs: map[string]Report{}, assertions: map[string]Assertion{}}
}

func (f *fakeStore) CreateRun(_ context.Context, id, _, snapshot, _ string) (string, bool, error) {
	if _, ok := f.runs[snapshot]; ok {
		return "", false, nil
	}
	f.runs[snapshot] = Report{RunID: id, State: "running"}
	return id, true, nil
}

func (f *fakeStore) GetRun(_ context.Context, _, snapshot string) (Report, error) {
	return f.runs[snapshot], nil
}

func (f *fakeStore) FinishRun(_ context.Context, runID, state string, accepted, duplicates, rejected int64, errorCode string) error {
	for snapshot, rep := range f.runs {
		if rep.RunID == runID {
			rep.State, rep.Accepted, rep.Duplicates, rep.Rejected, rep.ErrorCode =
				state, accepted, duplicates, rejected, errorCode
			f.runs[snapshot] = rep
		}
	}
	return nil
}

func (f *fakeStore) StageAssertion(_ context.Context, a Assertion) (bool, error) {
	if f.failStage {
		return false, errTest
	}
	key := a.Source + "|" + a.SourceKey + "|" + a.Checksum
	if _, ok := f.assertions[key]; ok {
		return false, nil
	}
	f.nextID++
	a.ID = "fake-assertion-" + itoa(f.nextID)
	f.assertions[key] = a
	return true, nil
}

func (f *fakeStore) ListAssertions(_ context.Context, runID string) ([]Assertion, error) {
	var out []Assertion
	for _, a := range f.assertions {
		if a.runID == runID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeStore) SetAssertionStation(_ context.Context, _, _ string) error {
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

var errTest = errTestSentinel()

type testError string

func (e testError) Error() string { return string(e) }

func errTestSentinel() error { return testError("stage boom") }

const sampleCSV = `CNPJ;RAZAO_SOCIAL;COD_IBGE;UF;SITUACAO;ATO_AUTORIZACAO
04218406000104;[P25-TEST] ALFA;3550308;SP;ATIVA;PRC-1
12ABC34501DE35;[P25-TEST] GAMA;3550308;SP;DESCONHECIDA;
00428184000195;[P25-TEST] ZERO;3550308;SP;ATIVA;PRC-3
04218406000104;[P25-TEST] ALFA;3550308;SP;ATIVA;PRC-1
123;[P25-TEST] CURTO;3550308;SP;ATIVA;
`

func TestStageCSVAccountsEveryRow(t *testing.T) {
	store := newFakeStore()
	report, err := StageCSV(context.Background(), store, "snap-1", strings.NewReader(sampleCSV), Limits{MaxBytes: 1 << 20, MaxRows: 100, BatchSize: 2})
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if report.State != "complete" {
		t.Fatalf("state = %q", report.State)
	}
	// 3 accepted (alfa, gama-alnum, zero-leading), 1 duplicate (alfa
	// replay inside the batch), 1 rejected (short CNPJ).
	if report.Accepted != 3 || report.Duplicates != 1 || report.Rejected != 1 {
		t.Fatalf("counts = %+v", report)
	}
}

func TestStageCSVReplayIsIdempotent(t *testing.T) {
	store := newFakeStore()
	limits := Limits{MaxBytes: 1 << 20, MaxRows: 100, BatchSize: 10}
	first, err := StageCSV(context.Background(), store, "snap-1", strings.NewReader(sampleCSV), limits)
	if err != nil {
		t.Fatalf("first stage: %v", err)
	}
	second, err := StageCSV(context.Background(), store, "snap-1", strings.NewReader(sampleCSV), limits)
	if err != nil {
		t.Fatalf("replay stage: %v", err)
	}
	if second.RunID != first.RunID || second.Accepted != first.Accepted {
		t.Fatalf("replay diverged: %+v vs %+v", first, second)
	}
	if got := len(store.assertions); got != 3 {
		t.Fatalf("assertions = %d, want 3", got)
	}
}

func TestStageCSVQuarantinesUnknownAndMissingHeaders(t *testing.T) {
	store := newFakeStore()
	report, err := StageCSV(context.Background(), store, "snap-u", strings.NewReader("CNPJ;RAZAO_SOCIAL;COD_IBGE;UF;SITUACAO;EXTRA\n"), Limits{MaxBytes: 1 << 20, MaxRows: 100, BatchSize: 10})
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if report.State != "quarantined" || report.ErrorCode != "unknown_columns" {
		t.Fatalf("report = %+v", report)
	}
	report, err = StageCSV(context.Background(), store, "snap-m", strings.NewReader("CNPJ;UF\n"), Limits{MaxBytes: 1 << 20, MaxRows: 100, BatchSize: 10})
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if report.State != "quarantined" || report.ErrorCode != "missing_columns" {
		t.Fatalf("report = %+v", report)
	}
}

func TestStageCSVRejectsOversizeTruncatedAndEmpty(t *testing.T) {
	store := newFakeStore()
	report, err := StageCSV(context.Background(), store, "snap-big", strings.NewReader(sampleCSV), Limits{MaxBytes: 10, MaxRows: 100, BatchSize: 10})
	if err != nil || report.State != "failed" || report.ErrorCode != "oversize" {
		t.Fatalf("oversize = %+v, err = %v", report, err)
	}
	report, err = StageCSV(context.Background(), store, "snap-tr", strings.NewReader("CNPJ;RAZAO_SOCIAL;COD_IBGE;UF;SITUACAO\n\"unterminated"), Limits{MaxBytes: 1 << 20, MaxRows: 100, BatchSize: 10})
	if err != nil || report.State != "failed" || report.ErrorCode != "truncated" {
		t.Fatalf("truncated = %+v, err = %v", report, err)
	}
	report, err = StageCSV(context.Background(), store, "snap-empty", strings.NewReader(""), Limits{MaxBytes: 1 << 20, MaxRows: 100, BatchSize: 10})
	if err != nil || report.State != "quarantined" || report.ErrorCode != "empty_snapshot" {
		t.Fatalf("empty = %+v, err = %v", report, err)
	}
	report, err = StageCSV(context.Background(), store, "snap-cap", strings.NewReader(sampleCSV), Limits{MaxBytes: 1 << 20, MaxRows: 2, BatchSize: 10})
	if err != nil || report.State != "failed" || report.ErrorCode != "row_cap" {
		t.Fatalf("row_cap = %+v, err = %v", report, err)
	}
}

func TestStageCSVStageErrorFailsRun(t *testing.T) {
	store := newFakeStore()
	store.failStage = true
	report, err := StageCSV(context.Background(), store, "snap-err", strings.NewReader(sampleCSV), Limits{MaxBytes: 1 << 20, MaxRows: 100, BatchSize: 10})
	if err != nil || report.State != "failed" || report.ErrorCode != "stage_error" {
		t.Fatalf("stage_error = %+v, err = %v", report, err)
	}
}
