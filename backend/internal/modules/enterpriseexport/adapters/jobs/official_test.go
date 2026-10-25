package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	qualityapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
)

type fakeQualityReader struct {
	pin         qualityapp.OfficialExportPin
	records     []qualityapp.OfficialRecord
	noPublished bool
}

func (f *fakeQualityReader) ExportPin(context.Context) (qualityapp.OfficialExportPin, error) {
	if f.noPublished {
		return qualityapp.OfficialExportPin{}, fmt.Errorf("%w: no published", qualityapp.ErrNoPublishedOfficialRecords)
	}
	return f.pin, nil
}

func (f *fakeQualityReader) ExportSanctions(context.Context, string, int) ([]qualityapp.OfficialRecord, string, error) {
	return f.records, "", nil
}

func TestQualityOfficialProviderMapsPinBuildAndNoPublished(t *testing.T) {
	revision := "00000000-0000-4000-8000-0000000000f1"
	lease := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	checksum := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	pin := qualityapp.OfficialExportPin{
		SchemaVersion:     qualityapp.OfficialExportSchemaVersion,
		PublicFieldPolicy: qualityapp.OfficialExportPublicFieldPolicy,
		Source:            qualityapp.OfficialExportSource, Stream: qualityapp.OfficialExportStream,
		Edition: "2025", RevisionID: revision, LeaseID: lease,
		PublishedAt: "2026-10-24T21:55:16Z", Status: "published", ProvenanceState: "VERIFIED",
		SourceURL: "https://example.test/fines.csv", SourceChecksum: checksum, SourceBytes: 10,
		SourcePeriod: "2025", SourceClockPrecision: "MONTH", SourceClockValue: "2026-01",
		Counts:            qualityapp.OfficialExportCounts{Input: 1, Accepted: 1},
		AssociationPolicy: "current-company-v1", AssociationScope: "CURRENT_COMPANY_ONLY",
		AssociationDecisionDay: "2026-10-20",
		AssociationCensus:      qualityapp.AssociationCensus{Total: 1, Unmatched: 1},
	}
	record := qualityapp.OfficialRecord{
		NativeKey: "f1", SubjectCNPJ: "04218406000104", Scope: "sanction",
		Status: "Multa aplicada", Evidence: "fines.csv:2",
		AssociationState: "unmatched", AssociationReason: "no current station",
		RevisionID: revision, LeaseID: lease, SourceURL: pin.SourceURL, SourceChecksum: checksum,
		ProceedingRef: "48000.1/2025-11", SourceContract: "anp-sicom-cp850-9-v1",
		SourcePeriod: "2025", AsOf: "2026-01-31",
	}
	reader := &fakeQualityReader{pin: pin, records: []qualityapp.OfficialRecord{record}}
	provider := NewQualityOfficialProvider(reader)
	gotPin, err := provider.Pin(context.Background())
	if err != nil || gotPin == nil || gotPin.RevisionID != revision || gotPin.LeaseID != lease || gotPin.Rows != 1 || gotPin.Unmatched != 1 {
		t.Fatalf("pin = %+v, %v", gotPin, err)
	}
	var out bytes.Buffer
	build, err := provider.Build(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	if build.Pin.PayloadSHA256 == "" || build.Pin.Rows != 1 || len(build.Manifest) == 0 {
		t.Fatalf("build = %+v", build)
	}
	var decoded qualityapp.OfficialExportManifest
	if err := json.Unmarshal(build.Manifest, &decoded); err != nil || decoded.PayloadSHA256 != build.Pin.PayloadSHA256 {
		t.Fatalf("manifest = %+v, %v", decoded, err)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"native_key":"f1"`)) {
		t.Errorf("payload missing record")
	}

	noPublished := NewQualityOfficialProvider(&fakeQualityReader{noPublished: true})
	if got, err := noPublished.Pin(context.Background()); err != nil || got != nil {
		t.Errorf("no published pin = %+v, %v", got, err)
	}
	if _, err := noPublished.Build(context.Background(), &bytes.Buffer{}); !errors.Is(err, qualityapp.ErrNoPublishedOfficialRecords) {
		t.Errorf("no published build = %v", err)
	}
}
