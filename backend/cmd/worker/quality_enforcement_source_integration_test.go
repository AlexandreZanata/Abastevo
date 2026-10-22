//go:build integration

package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	qualityjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

const enforcementPinnedSHA = "952743f3fa2b83094ad2011c5a4a1bffc14222eabde1bb4ee3bf4816e2f7db40"
const enforcementCorrectionMarker = "SYNTHETIC CORRECTION FOR LOCAL ACCEPTANCE"

func enforcementPublicDigest(parts ...string) [32]byte {
	return sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
}

// The opt-in actual-source proof keeps every file outside Git. Only the
// delivery port is substituted: trusted image CLI, bridge, source lease,
// publisher, SQL constraints, provenance, replay and readers remain real.
func TestEnforcementPinnedWholeWorkerReplayCorrectionAndFullTraversal(t *testing.T) {
	source := os.Getenv("ANPFUEL_TEST_ENFORCEMENT_XLSX_PATH")
	binary := os.Getenv("ANPFUEL_TEST_ENFORCEMENT_PREPARER_PATH")
	censusPath := os.Getenv("ANPFUEL_TEST_ENFORCEMENT_CENSUS_PATH")
	if source == "" || censusPath == "" {
		t.Skip("private pinned source and independent census required")
	}
	if binary == "" {
		t.Fatal("trusted image preparer required")
	}
	raw, e := os.ReadFile(source)
	if e != nil {
		t.Fatal("private source unavailable")
	}
	sum := sha256.Sum256(raw)
	if len(raw) != 16188087 || hex.EncodeToString(sum[:]) != enforcementPinnedSHA {
		t.Fatal("pinned source identity mismatch")
	}
	raw = nil
	var oracle struct {
		Enforcement struct {
			Rows     int            `json:"rows"`
			Segments map[string]int `json:"segments"`
			SHA256   string         `json:"sha256"`
		}
	}
	b, e := os.ReadFile(censusPath)
	if e != nil || json.Unmarshal(b, &oracle) != nil || oracle.Enforcement.Rows != 244247 || oracle.Enforcement.SHA256 != enforcementPinnedSHA {
		t.Fatal("independent census identity mismatch")
	}
	p := freshNativeWorkerDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()
	root := t.TempDir()
	current := source
	expected := make(map[string][32]byte, 244247)
	target := ""
	preparations := 0
	im := nativeEnforcementImport(p, binary, root)
	prepare := im.Prepare
	im.Prepare = func(ctx context.Context, payload qualityjobs.ImportPayload, f qualityjobs.FileArtifact) (prepared.EnforcementBatch, error) {
		batch, e := prepare(ctx, payload, f)
		if e != nil {
			return batch, e
		}
		preparations++
		if preparations == 1 {
			if batch.Counts.Input != 244247 || batch.Counts.Accepted != 244247 || batch.Counts.Duplicates != 0 || batch.Counts.Quarantined != 0 {
				t.Fatal("full typed counts disagree")
			}
			for _, r := range batch.Records {
				if target == "" && len(r.Result) > 3 {
					target = r.Result
				}
				expected[r.RecordKey] = enforcementPublicDigest(r.SubjectKind, r.UF, r.Municipality, r.Segment, r.DocumentDate, r.DocumentNumber, r.Procedure, r.Result, r.EvidenceRef)
			}
		}
		return batch, nil
	}
	im.Fetch = func(ctx context.Context, _ string) (qualityjobs.FileArtifact, error) {
		input, e := os.Open(current)
		if e != nil {
			return qualityjobs.FileArtifact{}, fmt.Errorf("private test source unavailable")
		}
		defer input.Close()
		out, e := os.OpenFile(filepath.Join(root, "download.xlsx"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return qualityjobs.FileArtifact{}, e
		}
		hash := sha256.New()
		n, e := io.Copy(io.MultiWriter(out, hash), input)
		closed := out.Close()
		if e != nil || closed != nil {
			_ = os.Remove(out.Name())
			return qualityjobs.FileArtifact{}, fmt.Errorf("private test copy failed")
		}
		return qualityjobs.FileArtifact{Path: out.Name(), Identity: prepared.InputIdentity{SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: n}, Close: func() error { return os.Remove(out.Name()) }}, nil
	}
	job := func() platformjobs.Job {
		t.Helper()
		f, e := os.Open(current)
		if e != nil {
			t.Fatal("source unavailable")
		}
		h := sha256.New()
		_, e = io.Copy(h, f)
		f.Close()
		if e != nil {
			t.Fatal("source unreadable")
		}
		raw, _ := json.Marshal(qualityjobs.ImportPayload{Version: 1, Source: "enforcement", Stream: "enforcement-v1", Edition: application.EnforcementSourceEdition, URL: native.EnforcementResourceURL, SHA256: hex.EncodeToString(h.Sum(nil))})
		return platformjobs.Job{Payload: raw}
	}
	begin := time.Now()
	if e = im.Handle(ctx, job()); e != nil {
		t.Fatal("whole-source worker refused", e)
	}
	t.Logf("whole-source initial worker PASS (%s)", time.Since(begin).Round(time.Millisecond))
	reader := qualityread.NewReader(p)
	lease, e := reader.LeaseBlock(ctx, time.Now(), "enforcement", "enforcement-v1")
	if e != nil || !lease.Stale || lease.Counts.Accepted != 244247 || lease.SourceClockPrecision != "UNAVAILABLE" {
		t.Fatal("whole-source lease mismatch", e)
	}
	first := lease.RevisionID
	var total, documents, vocab, maxchars, over64, company, personal, other, leaks int
	var firstDay, lastDay string
	e = p.QueryRow(ctx, `SELECT count(*),count(DISTINCT document_number),count(DISTINCT result),max(char_length(result)),count(*) FILTER(WHERE char_length(result)>64),count(*) FILTER(WHERE subject_kind='company-shaped'),count(*) FILTER(WHERE subject_kind='personal-shaped'),count(*) FILTER(WHERE subject_kind='other'),count(*) FILTER(WHERE subject_kind<>'company-shaped' AND subject_cnpj IS NOT NULL),min(document_day)::text,max(document_day)::text FROM quality_enforcement_source_records WHERE revision_id=$1`, first).Scan(&total, &documents, &vocab, &maxchars, &over64, &company, &personal, &other, &leaks, &firstDay, &lastDay)
	if e != nil || total != 244247 || documents != 141290 || vocab != 495 || maxchars != 116 || over64 != 32822 || company != 242948 || personal != 1294 || other != 5 || leaks != 0 || firstDay != "2019-01-02" || lastDay != "2026-10-04" {
		t.Fatal("stored census differs from pinned independent oracle", e)
	}
	groups, e := p.Query(ctx, "SELECT segment,count(*) FROM quality_enforcement_source_records WHERE revision_id=$1 GROUP BY segment", first)
	if e != nil {
		t.Fatal(e)
	}
	segments := 0
	for groups.Next() {
		var segment string
		var n int
		if e = groups.Scan(&segment, &n); e != nil {
			t.Fatal(e)
		}
		if oracle.Enforcement.Segments[segment] != n {
			t.Fatal("stored segment census mismatch")
		}
		segments++
	}
	e = groups.Err()
	groups.Close()
	if e != nil || segments != len(oracle.Enforcement.Segments) {
		t.Fatal("segment census incomplete", e)
	}
	// Every public source field is compared, not merely the row count or first page.
	traverse := func(frozen qualityread.Lease) {
		t.Helper()
		seen := map[string]bool{}
		afterKey, afterID := "", ""
		pages := 0
		for {
			rows, next, _, e := reader.EnforcementAtLease(ctx, frozen, "", "", afterKey, afterID, 100)
			if e != nil {
				t.Fatal("whole-source traversal unavailable", e)
			}
			pages++
			for _, r := range rows {
				want, ok := expected[r.NativeKey]
				got := enforcementPublicDigest(r.SubjectKind, r.UF, r.Municipality, r.Segment, r.DocumentDay, r.DocumentNumber, r.Procedure, r.Result, r.EvidenceRef)
				if !ok || want != got || seen[r.NativeKey] || r.OccurredOn != "" || r.AsOf != "" || r.Scope != "" || r.Status != "" || r.SourceProfile != application.EnforcementSourceProfile {
					t.Fatal("public source field, privacy or identity mismatch")
				}
				seen[r.NativeKey] = true
			}
			if next == "" {
				break
			}
			afterKey, afterID, _ = strings.Cut(next, "|")
		}
		if len(seen) != 244247 || pages != 2443 {
			t.Fatal("incomplete full traversal", len(seen), pages)
		}
		t.Logf("full frozen traversal PASS: %d records, %d pages", len(seen), pages)
	}
	traverse(lease)
	if e = im.Handle(ctx, job()); e != nil {
		t.Fatal("whole-source replay refused", e)
	}
	var revisions int
	if e = p.QueryRow(ctx, "SELECT count(*) FROM quality_enforcement_source_revisions").Scan(&revisions); e != nil || revisions != 1 {
		t.Fatal("whole-source replay forked", e)
	}
	corrected := filepath.Join(t.TempDir(), "synthetic-full-correction.xlsx")
	rewriteEnforcementResult(t, source, corrected, target)
	current = corrected
	if e = im.Handle(ctx, job()); e != nil {
		t.Fatal("whole-source correction refused", e)
	}
	latest, e := reader.LeaseBlock(ctx, time.Now(), "enforcement", "enforcement-v1")
	if e != nil || latest.RevisionID == first || latest.Counts.Accepted != 244247 {
		t.Fatal("whole-source correction pointer mismatch", e)
	}
	var changed int
	if e = p.QueryRow(ctx, "SELECT count(*) FROM quality_enforcement_source_records WHERE revision_id=$1 AND result=$2", latest.RevisionID, enforcementCorrectionMarker).Scan(&changed); e != nil || changed == 0 {
		t.Fatal("synthetic correction not visible", e)
	}
	historical, e := reader.AtRevision(first).LeaseBlock(ctx, time.Now(), "enforcement", "enforcement-v1")
	if e != nil || historical.Status != "superseded" {
		t.Fatal("whole-source history unavailable", e)
	}
	traverse(historical)
	if preparations != 3 {
		t.Fatal("actual native pipeline was bypassed", preparations)
	}
	entries, e := os.ReadDir(root)
	if e != nil || len(entries) != 0 {
		t.Fatal("whole-source raw/prepared workspace retained")
	}
	// Original source stays immutable, including after correction/replay.
	f, e := os.Open(source)
	if e != nil {
		t.Fatal(e)
	}
	hash := sha256.New()
	_, e = io.Copy(hash, f)
	f.Close()
	if e != nil || hex.EncodeToString(hash.Sum(nil)) != enforcementPinnedSHA {
		t.Fatal("original source changed")
	}
	t.Logf("whole-source acceptance PASS:3 preparations,2 revisions,0 raw workspaces; total=%s", time.Since(begin).Round(time.Millisecond))
}

// Only a private synthetic correction is rewritten. All original source members
// survive; no raw data, filename or label is logged or committed.
func rewriteEnforcementResult(t *testing.T, source, destination, target string) {
	t.Helper()
	if target == "" {
		t.Fatal("correction target absent")
	}
	z, e := zip.OpenReader(source)
	if e != nil {
		t.Fatal("source archive unavailable")
	}
	defer z.Close()
	out, e := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	defer out.Close()
	w := zip.NewWriter(out)
	changes := 0
	for _, entry := range z.File {
		r, e := entry.Open()
		if e != nil {
			t.Fatal(e)
		}
		header := entry.FileHeader
		dest, e := w.CreateHeader(&header)
		if e != nil {
			t.Fatal(e)
		}
		if entry.Name == "xl/sharedStrings.xml" {
			dec := xml.NewDecoder(r)
			enc := xml.NewEncoder(dest)
			for {
				token, e := dec.Token()
				if e == io.EOF {
					break
				}
				if e != nil {
					t.Fatal("source string XML invalid")
				}

				if start, ok := token.(xml.StartElement); ok {
					// Encoder declares element/attribute namespaces itself. Copying
					// original xmlns attributes would produce duplicate declarations.
					attrs := start.Attr[:0]
					for _, attr := range start.Attr {
						if attr.Name.Space == "xmlns" || (attr.Name.Space == "" && attr.Name.Local == "xmlns") {
							continue
						}
						attrs = append(attrs, attr)
					}
					start.Attr = attrs
					token = start
				}
				if text, ok := token.(xml.CharData); ok && strings.TrimSpace(string(text)) == target {
					token = xml.CharData(enforcementCorrectionMarker)
					changes++
				}
				if e = enc.EncodeToken(token); e != nil {
					t.Fatal(e)
				}
			}
			if e = enc.Flush(); e != nil {
				t.Fatal(e)
			}
		} else {
			if _, e = io.Copy(dest, r); e != nil {
				t.Fatal(e)
			}
		}
		r.Close()
	}
	if e = w.Close(); e != nil {
		t.Fatal(e)
	}
	if e = out.Sync(); e != nil {
		t.Fatal(e)
	}
	if changes == 0 {
		t.Fatal("synthetic full-file correction did not change a source label")
	}
}

func TestEnforcementCorrectionPreservesValidXML(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "original.xlsx")
	corrected := filepath.Join(root, "corrected.xlsx")
	if e := os.WriteFile(source, workerEnforcementWorkbook("Original source result"), 0600); e != nil {
		t.Fatal(e)
	}
	rewriteEnforcementResult(t, source, corrected, "Original source result")
	z, e := zip.OpenReader(corrected)
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	found := false
	for _, entry := range z.File {
		r, e := entry.Open()
		if e != nil {
			t.Fatal(e)
		}
		dec := xml.NewDecoder(r)
		for {
			token, e := dec.Token()
			if e == io.EOF {
				break
			}
			if e != nil {
				t.Fatal("corrected XML invalid")
			}
			if start, ok := token.(xml.StartElement); ok {
				seen := map[xml.Name]bool{}
				for _, attr := range start.Attr {
					if seen[attr.Name] {
						t.Fatal("duplicate namespace/attribute in correction")
					}
					seen[attr.Name] = true
				}
			}
			if text, ok := token.(xml.CharData); ok && string(text) == enforcementCorrectionMarker {
				found = true
			}
		}
		r.Close()
	}
	if !found {
		t.Fatal("correction label missing")
	}
}
