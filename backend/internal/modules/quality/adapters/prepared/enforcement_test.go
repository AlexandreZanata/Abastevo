package prepared

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

type enforcementFixture struct {
	records  []map[string]any
	identity InputIdentity
}

func enforcementHashOf(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func sampleEnforcementRecords() []map[string]any {
	company := "36754752000195"
	companyHash := enforcementHashOf(company, "DF", "BRASILIA", "Revenda de Combustíveis", "2026-10-04", "712508", "Procedimento sintético", "Resultado sintético")
	personalHash := enforcementHashOf("", "SP", "CAMPINAS", "Revenda de Combustíveis", "2019-01-02", "712509", "Procedimento sintético", "Resultado curto")
	return []map[string]any{
		{
			"schema_version": "enforcement-source-record-v1", "source": "anp-enforcement-xlsx",
			"row_locator": "synthetic.xlsx#row:2", "record_hash": companyHash,
			"tuple_hash":   "3f699e35975c1a3847deb339504c010b",
			"subject_cnpj": company, "subject_kind": "company-shaped",
			"uf": "DF", "municipality": "BRASILIA", "segment": "Revenda de Combustíveis",
			"document_date": "2026-10-04", "document_number": "712508",
			"procedure": "Procedimento sintético", "result": "Resultado sintético",
			"exact_repeat": false,
		},
		{
			"schema_version": "enforcement-source-record-v1", "source": "anp-enforcement-xlsx",
			"row_locator": "synthetic.xlsx#row:3", "record_hash": personalHash,
			"tuple_hash":   "9a26ea88c9e9c1862cec224ac6bf04ea",
			"subject_cnpj": nil, "subject_kind": "personal-shaped",
			"uf": "SP", "municipality": "CAMPINAS", "segment": "Revenda de Combustíveis",
			"document_date": "2019-01-02", "document_number": "712509",
			"procedure": "Procedimento sintético", "result": "Resultado curto",
			"exact_repeat": false,
		},
	}
}

func sealEnforcement(t *testing.T, f enforcementFixture) (manifest []byte, files map[string]string) {
	t.Helper()
	var lines strings.Builder
	for _, r := range f.records {
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		lines.Write(raw)
		lines.WriteByte('\n')
	}
	records := lines.String()
	sum := sha256.Sum256([]byte(records))
	segments := map[string]int64{}
	shapes := map[string]int64{}
	documents := map[string]bool{}
	tuples := map[string]bool{}
	results := map[string]bool{}
	var maxChars, over64, admitted int64
	first, last := "", ""
	for _, r := range f.records {
		documents[r["document_number"].(string)] = true
		tuples[r["tuple_hash"].(string)] = true
		results[r["result"].(string)] = true
		if n := int64(len([]rune(r["result"].(string)))); n > maxChars {
			maxChars = n
		}
		if len([]rune(r["result"].(string))) > 64 {
			over64++
		}
		segments[r["segment"].(string)]++
		shapes[r["subject_kind"].(string)]++
		if r["subject_cnpj"] != nil {
			admitted++
		}
		if first == "" || r["document_date"].(string) < first {
			first = r["document_date"].(string)
		}
		if last == "" || r["document_date"].(string) > last {
			last = r["document_date"].(string)
		}
	}
	n := int64(len(f.records))
	m := map[string]any{
		"schema_version": "enforcement-native-manifest-v1",
		"source_profile": "anp-enforcement-xlsx-2019-v1",
		"source_file":    "synthetic.xlsx",
		"input_bytes":    f.identity.Bytes,
		"input_sha256":   f.identity.SHA256,
		"manifest_hash":  strings.Repeat("b", 64),
		"counts":         map[string]any{"input": n, "accepted": n, "duplicates": int64(0), "quarantined": int64(0)},
		"census": map[string]any{
			"rows": n, "distinct_documents": int64(len(documents)),
			"distinct_subject_date_document_procedure": int64(len(tuples)),
			"result_vocabulary_size":                   int64(len(results)), "result_max_chars": maxChars,
			"results_over_64_chars": over64, "segments": segments, "subject_shapes": shapes,
			"subjects_admitted": admitted, "nonnumeric_dates": int64(0),
			"date_system": "EXCEL_1900", "date_precision": "DAY",
			"first_date": first, "last_date": last, "members": int64(18), "expanded_bytes": int64(127781352),
		},
		"outputs": []map[string]any{
			{"file": "enforcement-source-records.jsonl", "bytes": len(records), "sha256": hex.EncodeToString(sum[:])},
		},
	}
	manifest, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return manifest, map[string]string{enforcementRecordsFile: records}
}

func openEnforcement(files map[string]string) OpenStream {
	return func(name string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(files[name])), nil
	}
}

func TestLoadEnforcementAcceptsVerifiedBatch(t *testing.T) {
	f := enforcementFixture{records: sampleEnforcementRecords(), identity: InputIdentity{SHA256: strings.Repeat("a", 64), Bytes: 10}}
	manifest, files := sealEnforcement(t, f)
	b, err := LoadEnforcement(context.Background(), manifest, openEnforcement(files), f.identity)
	if err != nil {
		t.Fatal("verified batch refused", err)
	}
	if len(b.Records) != 2 || b.Counts.Input != 2 || b.SourceProfile != enforcementSourceProfile {
		t.Fatal("batch shape", len(b.Records), b.Counts)
	}
	first := b.Records[0]
	if first.SourceCNPJ != "36754752000195" || first.SubjectKind != "company-shaped" ||
		first.DocumentDate != "2026-10-04" || first.EvidenceRef != "synthetic.xlsx#row:2" ||
		first.RecordKey != "enforcement-source:"+first.Fingerprint {
		t.Fatal("company record mistyped", first)
	}
	second := b.Records[1]
	if second.SourceCNPJ != "" || second.SubjectKind != "personal-shaped" {
		t.Fatal("personal subject must not carry an identifier", second)
	}
}

func TestLoadEnforcementRefusesTamper(t *testing.T) {
	f := enforcementFixture{records: sampleEnforcementRecords(), identity: InputIdentity{SHA256: strings.Repeat("a", 64), Bytes: 10}}
	manifest, files := sealEnforcement(t, f)
	tampered := strings.Replace(files[enforcementRecordsFile], "Resultado sintético", "Resultado adulterado", 1)
	if _, err := LoadEnforcement(context.Background(), manifest, openEnforcement(map[string]string{enforcementRecordsFile: tampered}), f.identity); err == nil {
		t.Fatal("tampered stream accepted")
	}
	bad := sampleEnforcementRecords()
	bad[0]["record_hash"] = strings.Repeat("0", 64)
	_, files = sealEnforcement(t, enforcementFixture{records: bad, identity: f.identity})
	if _, err := LoadEnforcement(context.Background(), manifest, openEnforcement(files), f.identity); err == nil {
		t.Fatal("forged content hash accepted")
	}
}

func TestLoadEnforcementRefusesUnknownProfile(t *testing.T) {
	f := enforcementFixture{records: sampleEnforcementRecords(), identity: InputIdentity{SHA256: strings.Repeat("a", 64), Bytes: 10}}
	manifest, files := sealEnforcement(t, f)
	var m map[string]any
	if err := json.Unmarshal(manifest, &m); err != nil {
		t.Fatal(err)
	}
	m["source_profile"] = "anp-enforcement-xlsx-2030-v9"
	raw, _ := json.Marshal(m)
	if _, err := LoadEnforcement(context.Background(), raw, openEnforcement(files), f.identity); err == nil {
		t.Fatal("unknown profile accepted")
	}
	m["source_profile"] = "anp-enforcement-xlsx-2019-v1"
	m["schema_version"] = "enforcement-native-manifest-v9"
	raw, _ = json.Marshal(m)
	if _, err := LoadEnforcement(context.Background(), raw, openEnforcement(files), f.identity); err == nil {
		t.Fatal("unknown manifest schema accepted")
	}
}

func TestLoadEnforcementRefusesInvalidValues(t *testing.T) {
	identity := InputIdentity{SHA256: strings.Repeat("a", 64), Bytes: 10}
	cases := []func(map[string]any){
		func(r map[string]any) { r["subject_cnpj"] = "12345678901" },
		func(r map[string]any) { r["document_date"] = "2026-13-40" },
		func(r map[string]any) { r["uf"] = "SPO" },
		func(r map[string]any) { r["document_number"] = "" },
		func(r map[string]any) { r["row_locator"] = "other.xlsx#row:2" },
		func(r map[string]any) { r["subject_kind"] = "company-shaped"; r["subject_cnpj"] = nil },
	}
	for i, mutate := range cases {
		bad := sampleEnforcementRecords()
		mutate(bad[0])
		// The manifest is sealed from the bad content so only field
		// validation can refuse the batch, not stream integrity.
		manifest, files := sealEnforcement(t, enforcementFixture{records: bad, identity: identity})
		if _, err := LoadEnforcement(context.Background(), manifest, openEnforcement(files), identity); err == nil {
			t.Fatal("invalid record accepted", i)
		}
	}
}

func TestLoadEnforcementReturnsNoPartialBatch(t *testing.T) {
	f := enforcementFixture{records: sampleEnforcementRecords(), identity: InputIdentity{SHA256: strings.Repeat("a", 64), Bytes: 10}}
	manifest, files := sealEnforcement(t, f)
	truncated := files[enforcementRecordsFile][:len(files[enforcementRecordsFile])/2]
	if b, err := LoadEnforcement(context.Background(), manifest, openEnforcement(map[string]string{enforcementRecordsFile: truncated}), f.identity); err == nil || len(b.Records) != 0 {
		t.Fatal("truncated stream yielded a partial batch")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if b, err := LoadEnforcement(ctx, manifest, openEnforcement(files), f.identity); err == nil || len(b.Records) != 0 {
		t.Fatal("cancelled load yielded a partial batch")
	}
	if _, err := LoadEnforcement(context.Background(), manifest, nil, f.identity); err == nil {
		t.Fatal("missing stream port accepted")
	}
}
