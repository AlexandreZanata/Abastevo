package registrysync

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutomaticNamesRunForNewEditionsAndNeverRebindPending(t *testing.T) {
	r, fetches, _, _ := fixture(t)
	calls := 0
	r.GenerateNames = func(context.Context, string) (string, error) {
		calls++
		return nameFixture(t, t.TempDir(), "synthetic generated"), nil
	}
	r.Apply = func(context.Context, string) error { return os.ErrDeadlineExceeded }
	if r.Run(context.Background()) == nil {
		t.Fatal("interruption hidden")
	}
	before := *fetches
	r.GenerateNames = func(context.Context, string) (string, error) {
		t.Fatal("pending names generated again")
		return "", nil
	}
	r.Apply = func(context.Context, string) error { return nil }
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || before != *fetches {
		t.Fatal("pending source rebound")
	}
	s, _ := readState(r.Root)
	if s.Success == nil || s.Success.NameInputsHash == "" {
		t.Fatal("automatic names not pinned")
	}
}

func TestAdmissionRejectsQuarantineBeforeLoadAndPreservesPreviousSuccess(t *testing.T) {
	r, _, applied, _ := fixture(t)
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	before, _ := readState(r.Root)
	r.GenerateNames = func(context.Context, string) (string, error) { return nameFixture(t, t.TempDir(), "new edition"), nil }
	r.Prepare = func(_ context.Context, dir string, _ Snapshot) error {
		if err := os.Mkdir(filepath.Join(dir, "prepared"), 0700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "prepared", "manifest.json"), []byte(`{"counts":{"registry-13col":{"input":100,"accepted":98,"duplicates":0,"quarantined":2}}}`), 0600)
	}
	r.ValidatePrepared = ValidateRegistryAdmission
	if r.Run(context.Background()) == nil {
		t.Fatal("excess quarantine published")
	}
	after, _ := readState(r.Root)
	if *applied != 1 || after.Success.Key != before.Success.Key || after.Pending == nil {
		t.Fatal("failed edition advanced success")
	}
	for _, counts := range []map[string]int{
		{"input": 0, "accepted": 0}, {"input": 100, "accepted": 97, "quarantined": 2},
		{"input": 100, "accepted": 100, "duplicates": -1, "quarantined": 1}, {"input": 500001, "accepted": 500001},
	} {
		dir := t.TempDir()
		raw, _ := json.Marshal(map[string]any{"counts": map[string]any{"registry-13col": counts}})
		_ = os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600)
		if ValidateRegistryAdmission(dir) == nil {
			t.Fatal("invalid admission accepted", counts)
		}
	}
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"counts":{"registry-13col":{"input":100,"accepted":99,"duplicates":0,"quarantined":1}}}`), 0600)
	if err := ValidateRegistryAdmission(dir); err != nil {
		t.Fatal("documented 1% boundary refused", err)
	}
}

func TestEditionReportTamperingRefusesPendingReplay(t *testing.T) {
	r, _, applied, _ := fixture(t)
	r.Analyze = func(_, current string) error {
		return os.WriteFile(filepath.Join(current, "edition-report.json"), []byte(`{"synthetic":true}`), 0600)
	}
	r.Apply = func(context.Context, string) error { return os.ErrDeadlineExceeded }
	if r.Run(context.Background()) == nil {
		t.Fatal("interruption hidden")
	}
	s, _ := readState(r.Root)
	_ = os.WriteFile(filepath.Join(r.Root, s.Pending.Key, "edition-report.json"), []byte(`{"changed":true}`), 0600)
	r.Apply = func(context.Context, string) error { *applied++; return nil }
	if r.Run(context.Background()) == nil || *applied != 0 {
		t.Fatal("altered report published")
	}
}

func TestMembershipRejectsMalformedDuplicateAndTruncatedEditions(t *testing.T) {
	for _, body := range []string{"CNPJ;CNPJ\n00123456000100;00123456000100\n", "CNPJ;NAME\n00123456000100;One\n00123456000100;Two\n", "CNPJ;NAME\n00123456000100;\"Truncated", "CNPJ;NAME\ninvalid;One\n"} {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "registry.csv"), []byte(body), 0600)
		if _, err := AnalyzeRegistryEdition("", dir); err == nil {
			t.Fatal("unsafe edition accepted", strings.Split(body, "\n")[0])
		}
	}
}

func TestMembershipAcceptsANPLiteralQuotesAndCompleteMultilineFields(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "registry.csv"), []byte("CNPJ;NAME\n00123456000100;Synthetic \"quoted\" name\n00123456000290;\"Complete\nmultiline\"\n"), 0600)
	report, err := AnalyzeRegistryEdition("", dir)
	if err != nil || report.Rows != 2 || report.Added != 2 {
		t.Fatal(report, err)
	}
}

func TestRegistryDeltaDetectsAddChangeMissingAndRefusesTruncation(t *testing.T) {
	previous, current := t.TempDir(), t.TempDir()
	write := func(dir, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "registry.csv"), []byte("CNPJ;RAZAOSOCIAL\n"+body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(previous, "00123456000100;Old\n00123456000290;Retained\n00123456000370;Missing\n00123456000451;Retained\n00123456000532;Retained\n")
	write(current, "00123456000100;Changed\n00123456000290;Retained\n00123456000451;Retained\n00123456000532;Retained\n00123456000613;New\n")
	report, err := AnalyzeRegistryEdition(previous, current)
	if err != nil || report.Added != 1 || report.Changed != 1 || report.Missing != 1 || report.Unchanged != 3 {
		t.Fatal(report, err)
	}
	write(current, "00123456000100;Truncated\n")
	if _, err := AnalyzeRegistryEdition(previous, current); err == nil {
		t.Fatal("national drop silently admitted")
	}
	write(current, "")
	if _, err := AnalyzeRegistryEdition("", current); err == nil {
		t.Fatal("empty first edition admitted")
	}
}
