package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const exportRevision = "aaaaaaaa-1111-4111-8111-000000000001"
const exportLease = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const exportStation = "eeeeeeee-5555-4555-8555-000000000001"

func exportRecord(key, cnpj, station, state, reason string) OfficialRecord {
	amount := int64(150000)
	year := int32(2025)
	return OfficialRecord{
		NativeKey: key, SubjectCNPJ: cnpj, Scope: OfficialExportScope,
		Status: "aplicada", Evidence: "synthetic.csv:2", StationID: station,
		AssociationState: state, AssociationReason: reason,
		RevisionID: exportRevision, LeaseID: exportLease,
		SourceURL: "https://www.gov.br/anp/fines-2025.csv", SourceChecksum: strings.Repeat("a", 64),
		ProceedingRef: "48000.1/2025-11", AmountMinor: &amount, AmountText: "1500,00",
		DueOn: "2025-02-28", SourceContract: "anp-sicom-cp850-9-v1",
		SourcePeriod: "2025", ExtractionMonth: "2026-01", ReportYear: &year,
	}
}

func TestOfficialExportCursorSealsRevision(t *testing.T) {
	cursor := SealOfficialExportCursor(exportRevision, "key|with|pipes")
	revision, key, err := OpenOfficialExportCursor(cursor)
	if err != nil || revision != exportRevision || key != "key|with|pipes" {
		t.Fatalf("cursor = %q %q %v", revision, key, err)
	}
	if _, _, err := OpenOfficialExportCursor("not-a-cursor"); !errors.Is(err, ErrOfficialExportCursor) {
		t.Fatal("malformed cursor accepted", err)
	}
	if _, _, err := OpenOfficialExportCursor(exportRevision + "|"); !errors.Is(err, ErrOfficialExportCursor) {
		t.Fatal("empty native key accepted", err)
	}
	if _, _, err := OpenOfficialExportCursor("not-a-uuid|key"); !errors.Is(err, ErrOfficialExportCursor) {
		t.Fatal("malformed revision accepted", err)
	}
}

func TestAssociationStateFor(t *testing.T) {
	for kind, want := range map[string]AssociationState{
		"bound": AssociationLinked, "unmatched": AssociationUnmatched,
		"wrong_branch": AssociationUnmatched, "succession_review": AssociationUnmatched,
		"ambiguous": AssociationAmbiguous,
	} {
		got, err := AssociationStateFor(kind)
		if err != nil || got != want {
			t.Fatalf("%s = %q %v", kind, got, err)
		}
	}
	if _, err := AssociationStateFor("guessed"); !errors.Is(err, ErrOfficialExportUnavailable) {
		t.Fatal("unknown association kind accepted")
	}
}

func TestValidateOfficialRecordRejectsPrivateOrInferredData(t *testing.T) {
	valid := exportRecord("key", "11222333000181", exportStation, string(AssociationLinked), "exact current cnpj; historical business window unknown")
	if err := ValidateOfficialRecord(valid); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*OfficialRecord){
		"cpf":        func(r *OfficialRecord) { r.SubjectCNPJ = "12345678901" },
		"empty cnpj": func(r *OfficialRecord) { r.SubjectCNPJ = "" },
		"unlinked station": func(r *OfficialRecord) {
			r.AssociationState = string(AssociationUnmatched)
			r.AssociationReason = "unmatched_cnpj"
		},
		"missing station": func(r *OfficialRecord) { r.StationID = "" },
		"bad checksum":    func(r *OfficialRecord) { r.SourceChecksum = "not-a-hash" },
		"bad date":        func(r *OfficialRecord) { r.DecidedOn = "2025-02-30" },
		"empty period":    func(r *OfficialRecord) { r.SourcePeriod = "" },
		"empty lease":     func(r *OfficialRecord) { r.LeaseID = "" },
	}
	for name, mutate := range cases {
		record := valid
		mutate(&record)
		if err := ValidateOfficialRecord(record); !errors.Is(err, ErrOfficialExportIncomplete) {
			t.Fatalf("%s accepted: %v", name, err)
		}
	}
}

type fakeExportReader struct {
	pin   OfficialExportPin
	pages [][]OfficialRecord
	calls int
}

func (f *fakeExportReader) ExportPin(context.Context) (OfficialExportPin, error) {
	return f.pin, nil
}

func (f *fakeExportReader) ExportSanctions(_ context.Context, after string, _ int) ([]OfficialRecord, string, error) {
	if f.calls >= len(f.pages) {
		return nil, "", nil
	}
	page := f.pages[f.calls]
	f.calls++
	next := ""
	if f.calls < len(f.pages) {
		next = SealOfficialExportCursor(f.pin.RevisionID, page[len(page)-1].NativeKey)
	}
	return page, next, nil
}

func fakePin(rows int) OfficialExportPin {
	return OfficialExportPin{
		SchemaVersion: OfficialExportSchemaVersion, PublicFieldPolicy: OfficialExportPublicFieldPolicy,
		Source: OfficialExportSource,
		Stream: OfficialExportStream, Edition: "2025", RevisionID: exportRevision,
		LeaseID: exportLease, PublishedAt: "2026-01-02T03:04:05Z", Status: "published",
		ProvenanceState: "VERIFIED", SourceURL: "https://www.gov.br/anp/fines-2025.csv",
		SourceChecksum: strings.Repeat("a", 64), SourceBytes: 100, SourcePeriod: "2025",
		SourceClockPrecision: "MONTH", SourceClockValue: "2026-01",
		Counts:            OfficialExportCounts{Input: rows, Accepted: rows},
		AssociationPolicy: "current-company-v1", AssociationScope: "CURRENT_COMPANY_ONLY",
		AssociationDecisionDay: "2026-10-20",
		AssociationCensus:      AssociationCensus{Total: rows, Linked: rows},
	}
}

func TestBuildOfficialExportDeterministicExactlyOnce(t *testing.T) {
	first := exportRecord("key-1", "11222333000181", exportStation, string(AssociationLinked), "exact current cnpj; historical business window unknown")
	second := exportRecord("key-2", "04218406000104", "", string(AssociationUnmatched), "unmatched_cnpj")
	reader := &fakeExportReader{pin: fakePin(2), pages: [][]OfficialRecord{{first}, {second}}}
	var output bytes.Buffer
	manifest, err := BuildOfficialExport(context.Background(), reader, &output, 1)
	if err != nil {
		t.Fatal(err)
	}
	if reader.calls != 2 {
		t.Fatalf("pages read = %d, want exactly 2", reader.calls)
	}
	if manifest.Rows != 2 || manifest.PayloadSHA256 == "" || manifest.AssociationCensus.Total != 2 {
		t.Fatalf("manifest = %+v", manifest)
	}
	expected := new(bytes.Buffer)
	encoder := json.NewEncoder(expected)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(first)
	_ = encoder.Encode(second)
	sum := sha256.Sum256(expected.Bytes())
	if manifest.PayloadSHA256 != hex.EncodeToString(sum[:]) || !bytes.Equal(output.Bytes(), expected.Bytes()) {
		t.Fatal("payload hash or JSONL bytes are not deterministic")
	}
	var again bytes.Buffer
	secondManifest, err := BuildOfficialExport(context.Background(), &fakeExportReader{pin: fakePin(2), pages: [][]OfficialRecord{{first}, {second}}}, &again, 1)
	if err != nil || secondManifest.PayloadSHA256 != manifest.PayloadSHA256 || !bytes.Equal(again.Bytes(), output.Bytes()) {
		t.Fatal("repeat build diverged", err)
	}
}

func TestBuildOfficialExportFailsClosed(t *testing.T) {
	record := exportRecord("key-1", "11222333000181", exportStation, string(AssociationLinked), "exact current cnpj; historical business window unknown")
	other := exportRecord("key-2", "11222333000181", exportStation, string(AssociationLinked), "exact current cnpj; historical business window unknown")
	other.RevisionID = "cccccccc-2222-4222-8222-000000000002"
	cases := []struct {
		name   string
		reader *fakeExportReader
	}{
		{"duplicate", &fakeExportReader{pin: fakePin(2), pages: [][]OfficialRecord{{record}, {record, other}}}},
		{"short", &fakeExportReader{pin: fakePin(2), pages: [][]OfficialRecord{{record}}}},
		{"changed revision", &fakeExportReader{pin: fakePin(2), pages: [][]OfficialRecord{{record}, {other}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			if _, err := BuildOfficialExport(context.Background(), tc.reader, &output, 1); !errors.Is(err, ErrOfficialExportIncomplete) {
				t.Fatal("incomplete export accepted", err)
			}
		})
	}
}
