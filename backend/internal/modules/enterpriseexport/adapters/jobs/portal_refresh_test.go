package jobs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/portalgen"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

const refreshStationID = "00000000-0000-4000-8000-000000000001"
const refreshRevisionID = "00000000-0000-4000-8000-000000000011"
const refreshOfficialA = "00000000-0000-4000-8000-0000000000a1"
const refreshOfficialB = "00000000-0000-4000-8000-0000000000b1"

func writeRefreshJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeRefreshJSONL(t *testing.T, path string, value any) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	switch rows := value.(type) {
	case []enterpriseexport.Station:
		for _, row := range rows {
			if err := encoder.Encode(row); err != nil {
				t.Fatal(err)
			}
		}
	case []enterpriseexport.Revision:
		for _, row := range rows {
			if err := encoder.Encode(row); err != nil {
				t.Fatal(err)
			}
		}
	case []enterpriseexport.Price:
		for _, row := range rows {
			if err := encoder.Encode(row); err != nil {
				t.Fatal(err)
			}
		}
	default:
		t.Fatalf("unsupported fixture type")
	}
}

func writeRefreshExport(t *testing.T, root, name, contentHash, amount, officialRevision string) State {
	t.Helper()
	dir := filepath.Join(root, "candidates", "export-"+name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	stations := []enterpriseexport.Station{{
		StationID: refreshStationID, DisplayName: "Posto Refresh " + name, CNPJ: "04218406000104",
		State: "SP", MunicipalityCode: "3550308", OperationStatus: "active", LocationQuality: "reviewed",
		Address: map[string]string{"municipio": "São Paulo", "uf": "SP", "street": "Rua A"},
	}}
	revisions := []enterpriseexport.Revision{{
		RevisionID: refreshRevisionID, SurveyStart: "2026-10-04", SurveyEnd: "2026-10-10",
		PublishedAt: "2026-10-11T12:00:00Z", SourceURL: "https://example.test/" + name + ".xlsx",
		SourceChecksum: "sha256:" + name, ParserVersion: "refresh-test",
	}}
	prices := []enterpriseexport.Price{{
		PriceID: "price-" + name, RevisionID: refreshRevisionID, StationID: refreshStationID,
		FuelProduct: "GASOLINE_REGULAR", Unit: "L", AmountMilliBRL: 6000,
		Currency: "BRL", CollectedOn: "2026-10-08",
	}}
	if amount != "" {
		prices[0].AmountMilliBRL = 0
		_, _ = fmtSscan(amount, &prices[0].AmountMilliBRL)
	}
	manifest := enterpriseexport.Manifest{
		SchemaVersion: enterpriseexport.SchemaVersion, ContentVersion: enterpriseexport.ContentVersion,
		PublicFieldPolicy: enterpriseexport.PublicFieldPolicy, BuiltAt: "2026-10-24T12:00:00Z",
		ContentSHA256: contentHash,
	}
	writeRefreshJSON(t, filepath.Join(dir, "manifest.json"), manifest)
	writeRefreshJSONL(t, filepath.Join(dir, "stations.jsonl"), stations)
	writeRefreshJSONL(t, filepath.Join(dir, "revisions.jsonl"), revisions)
	writeRefreshJSONL(t, filepath.Join(dir, "prices.jsonl"), prices)
	projection, err := analytics.Compute(analytics.Input{
		ContentSHA256: contentHash, Stations: stations, Revisions: revisions, Prices: prices,
	}, analytics.Options{})
	if err != nil {
		t.Fatal(err)
	}
	analyticsRaw, err := json.MarshalIndent(projection, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	analyticsRaw = append(analyticsRaw, '\n')
	if err := os.WriteFile(filepath.Join(dir, "analytics-v1.json"), analyticsRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	analyticsSum := sha256.Sum256(analyticsRaw)
	state := State{
		SchemaVersion: StateSchemaVersion, BuiltAt: "2026-10-24T12:00:00Z", ObservedAt: "2026-10-24T11:00:00Z",
		CandidateDir: dir, ContentSHA256: contentHash,
		AnalyticsSHA256: "sha256:" + hex.EncodeToString(analyticsSum[:]), AnalyticsBytes: int64(len(analyticsRaw)),
		DirectoryFingerprint: "fingerprint-" + name, DirectoryStations: 1,
		FromSurvey: "2026-10-04", ToSurvey: "2026-10-10",
	}
	if officialRevision != "" {
		state.Official = writeRefreshOfficial(t, dir, officialRevision)
	}
	if err := WriteState(root, state); err != nil {
		t.Fatal(err)
	}
	return state
}

func writeRefreshOfficial(t *testing.T, dir, revision string) *OfficialPin {
	t.Helper()
	lease := strings.Repeat("b", 64)
	checksum := strings.Repeat("a", 64)
	record := map[string]any{
		"native_key": "f1", "subject_cnpj": "04218406000104", "scope": "sanction",
		"status": "Multa aplicada", "evidence": "fines.csv:2",
		"association_state": "unmatched", "association_reason": "no current station",
		"revision_id": revision, "lease_id": lease,
		"source_url": "https://www.gov.br/anp/fines.csv", "source_checksum": checksum,
		"proceeding_ref": "48000.1/2025-11", "amount_minor": 150000, "amount_text": "1500,00",
		"decided_on": "2025-01-15", "due_on": "2025-02-28",
		"source_contract": "anp-sicom-cp850-9-v1", "source_period": "2025",
		"extraction_month": "2026-01", "report_year": 2025,
	}
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(record); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "official-records.jsonl"), payload.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload.Bytes())
	manifest := map[string]any{
		"schema_version": "quality-official-export-v1", "public_field_policy": "fq-public-v1",
		"source": "sanctions", "stream": "sanction-v1", "edition": "2025",
		"revision_id": revision, "lease_id": lease, "published_at": "2026-10-24T21:55:16Z",
		"status": "published", "provenance_state": "VERIFIED",
		"source_url": "https://www.gov.br/anp/fines.csv", "source_checksum": checksum,
		"source_bytes": 100, "source_period": "2025", "source_clock_precision": "MONTH",
		"source_clock_value": "2026-01",
		"counts":             map[string]any{"input": 1, "accepted": 1, "duplicates": 0, "quarantined": 0},
		"association_policy": "current-company-v1", "association_scope": "CURRENT_COMPANY_ONLY",
		"association_decision_day": "2026-10-20",
		"association_census":       map[string]any{"total": 1, "linked": 0, "unmatched": 1, "ambiguous": 0, "wrong_branch": 0, "succession_review": 0},
		"stale":                    true, "stale_reason": "source extraction month may exceed 180 days",
		"rows": 1, "payload_sha256": hex.EncodeToString(sum[:]),
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "official-records-manifest.json"), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return &OfficialPin{
		RevisionID: revision, LeaseID: lease, Edition: "2025",
		SourceURL: "https://www.gov.br/anp/fines.csv", SourceChecksum: checksum,
		SourcePeriod: "2025", SourceClockPrecision: "MONTH", SourceClockValue: "2026-01",
		Input: 1, Accepted: 1, Rows: 1, Unmatched: 1, PayloadSHA256: hex.EncodeToString(sum[:]),
	}
}

func fmtSscan(value string, target *int64) (int, error) {
	var parsed int64
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, errors.New("bad amount")
		}
		parsed = parsed*10 + int64(r-'0')
	}
	*target = parsed
	return 1, nil
}

func refreshFixture(t *testing.T) (string, string, Refresh, string) {
	t.Helper()
	exportRoot := t.TempDir()
	portalRoot := filepath.Join(t.TempDir(), "portal")
	js := filepath.Join(t.TempDir(), "search-v1.js")
	if err := os.WriteFile(js, []byte("globalThis.__refresh = 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := Refresh{ExportRoot: exportRoot, PortalRoot: portalRoot, SiteURL: "https://portal.example", SearchScriptPath: js}
	return exportRoot, portalRoot, handler, js
}

func runRefresh(t *testing.T, handler Refresh) error {
	t.Helper()
	return handler.Handle(context.Background(), platformjobs.Job{})
}

func refreshCurrent(t *testing.T, portalRoot string) portalgen.Release {
	t.Helper()
	release, _, err := portalgen.CurrentRelease(portalRoot, "https://portal.example", false)
	if err != nil {
		t.Fatal(err)
	}
	return release
}

func TestRefreshPromotesCoalescesAndInvalidatesAssets(t *testing.T) {
	exportRoot, portalRoot, handler, js := refreshFixture(t)
	writeRefreshExport(t, exportRoot, "a", "sha256:"+strings.Repeat("a", 64), "6000", "")
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	first := refreshCurrent(t, portalRoot)
	if first.ContentSHA256 == "" {
		t.Fatal("first release missing")
	}
	firstPage, err := os.ReadFile(filepath.Join(portalRoot, "releases", strings.TrimPrefix(first.ContentSHA256, "sha256:"), "consultar", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(firstPage), "portal.") || !strings.Contains(string(firstPage), ".js") {
		t.Fatal("first release lacks search asset")
	}
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	refresh, ok := loadRefreshState(portalRoot)
	if !ok || refresh.LastOutcome != "noop" {
		t.Fatalf("pull refresh = %+v %v", refresh, ok)
	}
	state, _ := portalgen.LoadReleaseState(portalRoot)
	if len(state.Releases) != 1 {
		t.Fatalf("noop created releases: %+v", state.Releases)
	}
	// A changed script changes the content-addressed asset and the page, while
	// the old release remains intact.
	js2 := filepath.Join(filepath.Dir(js), "search-v2.js")
	if err := os.WriteFile(js2, []byte("globalThis.__refresh = 2;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler.SearchScriptPath = js2
	writeRefreshExport(t, exportRoot, "b", "sha256:"+strings.Repeat("b", 64), "6500", "")
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	second := refreshCurrent(t, portalRoot)
	if second.ContentSHA256 == first.ContentSHA256 {
		t.Fatal("source change did not create a new release")
	}
	secondPage, err := os.ReadFile(filepath.Join(portalRoot, "releases", strings.TrimPrefix(second.ContentSHA256, "sha256:"), "consultar", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(firstPage) == string(secondPage) {
		t.Fatal("new release reused the old page")
	}
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	refresh, _ = loadRefreshState(portalRoot)
	if refresh.LastOutcome != "noop" {
		t.Fatalf("post-change refresh = %+v", refresh)
	}
	state, _ = portalgen.LoadReleaseState(portalRoot)
	if len(state.Releases) != 2 {
		t.Fatalf("releases = %+v", state.Releases)
	}
}

func TestRefreshPreservesCurrentOnBuildFailure(t *testing.T) {
	exportRoot, portalRoot, handler, _ := refreshFixture(t)
	writeRefreshExport(t, exportRoot, "a", "sha256:"+strings.Repeat("a", 64), "6000", "")
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	first := refreshCurrent(t, portalRoot)
	writeRefreshExport(t, exportRoot, "b", "sha256:"+strings.Repeat("b", 64), "6500", "")
	handler.Generate = func(portalgen.Options) (portalgen.Result, error) {
		return portalgen.Result{}, errors.New("synthetic build failure")
	}
	if err := runRefresh(t, handler); err == nil {
		t.Fatal("failed build accepted")
	}
	if got := refreshCurrent(t, portalRoot); got.ContentSHA256 != first.ContentSHA256 {
		t.Fatal("failed build changed current release")
	}
	refresh, _ := loadRefreshState(portalRoot)
	if refresh.LastOutcome != "failed" || !strings.Contains(refresh.LastError, "synthetic build failure") {
		t.Fatalf("failure receipt = %+v", refresh)
	}
	state, _ := portalgen.LoadReleaseState(portalRoot)
	if len(state.Releases) != 1 {
		t.Fatalf("failure created releases: %+v", state.Releases)
	}
	handler.Generate = nil
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	if got := refreshCurrent(t, portalRoot); got.ContentSHA256 == first.ContentSHA256 {
		t.Fatal("recovery did not promote the new release")
	}
}

func TestRefreshRestartRecoveryAndOrphanCleanup(t *testing.T) {
	exportRoot, portalRoot, handler, _ := refreshFixture(t)
	writeRefreshExport(t, exportRoot, "a", "sha256:"+strings.Repeat("a", 64), "6000", "")
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	first := refreshCurrent(t, portalRoot)
	if err := os.Remove(filepath.Join(portalRoot, refreshStateFile)); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(portalRoot, refreshCandidates, "portal-orphan")
	if err := os.MkdirAll(orphan, 0o700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(orphan, old, old); err != nil {
		t.Fatal(err)
	}
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	if got := refreshCurrent(t, portalRoot); got.ContentSHA256 != first.ContentSHA256 {
		t.Fatal("restart recovery changed the release")
	}
	state, _ := portalgen.LoadReleaseState(portalRoot)
	if len(state.Releases) != 1 {
		t.Fatalf("restart created releases: %+v", state.Releases)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("orphan candidate retained")
	}
	refresh, _ := loadRefreshState(portalRoot)
	if refresh.LastOutcome != "noop" {
		t.Fatalf("restart receipt = %+v", refresh)
	}
}

func TestRefreshWithdrawsAffectedOfficialRelease(t *testing.T) {
	exportRoot, portalRoot, handler, _ := refreshFixture(t)
	writeRefreshExport(t, exportRoot, "price", "sha256:"+strings.Repeat("c", 64), "6000", "")
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	priceOnly := refreshCurrent(t, portalRoot)
	writeRefreshExport(t, exportRoot, "official-a", "sha256:"+strings.Repeat("d", 64), "6500", refreshOfficialA)
	handler.Status = func(context.Context) (SourceStatus, error) {
		return SourceStatus{HasPublished: true, RevisionID: refreshOfficialA, Status: "published"}, nil
	}
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	official := refreshCurrent(t, portalRoot)
	if official.OfficialRevisionID != refreshOfficialA {
		t.Fatalf("official release = %+v", official)
	}
	handler.Status = func(context.Context) (SourceStatus, error) {
		return SourceStatus{HasPublished: false, RevisionID: refreshOfficialA, Status: "withdrawn"}, nil
	}
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	if got := refreshCurrent(t, portalRoot); got.ContentSHA256 != priceOnly.ContentSHA256 {
		t.Fatalf("withdrawal fallback = %+v, want price-only %+v", got, priceOnly)
	}
	state, _ := portalgen.LoadReleaseState(portalRoot)
	for _, release := range state.Releases {
		if release.ContentSHA256 == official.ContentSHA256 && !release.Withdrawn {
			t.Fatal("affected release was not withdrawn")
		}
	}
	refresh, _ := loadRefreshState(portalRoot)
	if refresh.LastOutcome != "withdrawn" || refresh.WithdrawnRevisionID != refreshOfficialA {
		t.Fatalf("withdrawal receipt = %+v", refresh)
	}
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	refresh, _ = loadRefreshState(portalRoot)
	if refresh.LastOutcome != "source-unavailable" {
		t.Fatalf("post-withdrawal receipt = %+v", refresh)
	}
}

func TestRefreshAwaitsAcceptedExportThenReleases(t *testing.T) {
	exportRoot, portalRoot, handler, _ := refreshFixture(t)
	writeRefreshExport(t, exportRoot, "official-a", "sha256:"+strings.Repeat("e", 64), "6000", refreshOfficialA)
	handler.Status = func(context.Context) (SourceStatus, error) {
		return SourceStatus{HasPublished: true, RevisionID: refreshOfficialA, Status: "published"}, nil
	}
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	first := refreshCurrent(t, portalRoot)
	handler.Status = func(context.Context) (SourceStatus, error) {
		return SourceStatus{HasPublished: true, RevisionID: refreshOfficialB, Status: "published"}, nil
	}
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	refresh, _ := loadRefreshState(portalRoot)
	if refresh.LastOutcome != "awaiting-export" {
		t.Fatalf("new source did not wait for export: %+v", refresh)
	}
	if got := refreshCurrent(t, portalRoot); got.ContentSHA256 != first.ContentSHA256 {
		t.Fatal("refresh bypassed the export build")
	}
	writeRefreshExport(t, exportRoot, "official-b", "sha256:"+strings.Repeat("f", 64), "7000", refreshOfficialB)
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	second := refreshCurrent(t, portalRoot)
	if second.ContentSHA256 == first.ContentSHA256 || second.OfficialRevisionID != refreshOfficialB {
		t.Fatalf("accepted export not released: %+v", second)
	}
	if refresh, _ := loadRefreshState(portalRoot); refresh.LastOutcome != "released" || refresh.SourceRevisionID != refreshOfficialB {
		t.Fatalf("release receipt = %+v", refresh)
	}
	if refresh, _ := loadRefreshState(portalRoot); refresh.ObservedAt == "" || refresh.ExportBuiltAt == "" || refresh.ReleasedAt == "" {
		t.Fatalf("receipt timestamps missing: %+v", refresh)
	}
}

func TestRefreshLockIsExclusive(t *testing.T) {
	_, portalRoot, handler, _ := refreshFixture(t)
	if err := ensureRefreshRoot(portalRoot); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockRefresh(portalRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if err := runRefresh(t, handler); !errors.Is(err, ErrRefreshBusy) {
		t.Fatal("busy refresh accepted", err)
	}
}

func TestRefreshPriceOnlyExportWithoutOfficialSource(t *testing.T) {
	exportRoot, portalRoot, handler, _ := refreshFixture(t)
	handler.Status = func(context.Context) (SourceStatus, error) {
		return SourceStatus{HasPublished: false, Status: "withdrawn"}, nil
	}
	writeRefreshExport(t, exportRoot, "price-only", "sha256:"+strings.Repeat("9", 64), "6000", "")
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	current := refreshCurrent(t, portalRoot)
	if current.ContentSHA256 == "" || current.OfficialRevisionID != "" {
		t.Fatalf("price-only release = %+v", current)
	}
	refresh, _ := loadRefreshState(portalRoot)
	if refresh.LastOutcome != "released" || refresh.SourceRevisionID != "" {
		t.Fatalf("price-only receipt = %+v", refresh)
	}
}

func TestRefreshOfficialOnlyPropagatesMode(t *testing.T) {
	exportRoot, portalRoot, handler, _ := refreshFixture(t)
	handler.OfficialOnly = true
	writeRefreshExport(t, exportRoot, "a", "sha256:"+strings.Repeat("a", 64), "6000", refreshOfficialA)
	var sawOfficialOnly *bool
	wrapped := handler
	wrapped.Generate = func(opts portalgen.Options) (portalgen.Result, error) {
		flag := opts.OfficialOnly
		sawOfficialOnly = &flag
		return portalgen.Generate(opts)
	}
	if err := runRefresh(t, wrapped); err != nil {
		t.Fatal(err)
	}
	if sawOfficialOnly == nil || !*sawOfficialOnly {
		t.Fatal("official-only refresh generated a full portal")
	}
	release, _, err := portalgen.CurrentRelease(portalRoot, "https://portal.example", true)
	if err != nil {
		t.Fatalf("official-only current rejected: %v", err)
	}
	if release.OfficialRevisionID != refreshOfficialA {
		t.Fatalf("official release = %+v", release)
	}
	for _, dir := range []string{"precos", "postos", "dados", "consultar", "curiosidades"} {
		if _, err := os.Stat(filepath.Join(portalRoot, "current", dir)); !os.IsNotExist(err) {
			t.Fatalf("official-only release serves %s", dir)
		}
	}
	if _, _, err := portalgen.CurrentRelease(portalRoot, "https://portal.example", true); err != nil {
		t.Fatalf("official-only current rejected: %v", err)
	}
}

func TestRefreshOfficialOnlyRequiresPublishedOfficial(t *testing.T) {
	exportRoot, _, handler, _ := refreshFixture(t)
	handler.OfficialOnly = true
	writeRefreshExport(t, exportRoot, "a", "sha256:"+strings.Repeat("a", 64), "6000", "")
	if err := runRefresh(t, handler); err == nil {
		t.Fatal("official-only refresh accepted an export without official records")
	}
}

func TestRefreshOfficialOnlyWithdrawsWithoutFallback(t *testing.T) {
	exportRoot, portalRoot, handler, _ := refreshFixture(t)
	handler.OfficialOnly = true
	handler.Status = func(context.Context) (SourceStatus, error) {
		return SourceStatus{HasPublished: true, RevisionID: refreshOfficialA}, nil
	}
	writeRefreshExport(t, exportRoot, "a", "sha256:"+strings.Repeat("a", 64), "6000", refreshOfficialA)
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	handler.Status = func(context.Context) (SourceStatus, error) { return SourceStatus{HasPublished: false}, nil }
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	if _, _, err := portalgen.CurrentReleaseHeader(portalRoot); !errors.Is(err, portalgen.ErrNoCurrentRelease) {
		t.Fatalf("withdrawn official-only release still current: %v", err)
	}
	// A full release must never become the fallback of an official-only
	// withdrawal: the outcome above is 404 (no current), never wider scope.
	refresh, _ := loadRefreshState(portalRoot)
	if refresh.LastOutcome != "withdrawn" {
		t.Fatalf("official-only withdrawal receipt = %+v", refresh)
	}
}

// linkRefreshOfficial rewrites the export candidate's official files so the
// single record is linked to the snapshot station, simulating a Directory
// change accepted into a new revision. Census, payload hash and the state
// pin move together; a partial edit is a test bug, not a product path.
func linkRefreshOfficial(t *testing.T, exportRoot string, revision, stationID string) {
	t.Helper()
	state, ok := LoadState(exportRoot)
	if !ok {
		t.Fatal("export state missing")
	}
	payloadPath := filepath.Join(state.CandidateDir, "official-records.jsonl")
	raw, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(raw))), &record); err != nil {
		t.Fatal(err)
	}
	record["association_state"] = "linked"
	record["association_reason"] = "exact current cnpj; historical business window unknown"
	record["station_id"] = stationID
	record["revision_id"] = revision
	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(payloadPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	manifestPath := filepath.Join(state.CandidateDir, "official-records-manifest.json")
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["revision_id"] = revision
	manifest["payload_sha256"] = hex.EncodeToString(sum[:])
	manifest["association_census"] = map[string]any{"total": 1, "linked": 1, "unmatched": 0, "ambiguous": 0, "wrong_branch": 0, "succession_review": 0}
	remanifested, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(remanifested, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if state.Official == nil {
		t.Fatal("export state has no official pin")
	}
	state.Official.RevisionID = revision
	state.Official.Linked = 1
	state.Official.Unmatched = 0
	state.Official.PayloadSHA256 = hex.EncodeToString(sum[:])
	if err := WriteState(exportRoot, state); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshRebuildsOnAssociationChange(t *testing.T) {
	exportRoot, portalRoot, handler, _ := refreshFixture(t)
	handler.Status = func(context.Context) (SourceStatus, error) {
		return SourceStatus{HasPublished: true, RevisionID: refreshOfficialA, Status: "published"}, nil
	}
	writeRefreshExport(t, exportRoot, "unmatched", "sha256:"+strings.Repeat("e", 64), "6000", refreshOfficialA)
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	first := refreshCurrent(t, portalRoot)
	// A Directory change accepted into revision B links the record: the new
	// release must carry the link, and the link states the decision day,
	// never historical operation.
	linkRefreshOfficial(t, exportRoot, refreshOfficialB, refreshStationID)
	handler.Status = func(context.Context) (SourceStatus, error) {
		return SourceStatus{HasPublished: true, RevisionID: refreshOfficialB, Status: "published"}, nil
	}
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	second := refreshCurrent(t, portalRoot)
	if second.ContentSHA256 == first.ContentSHA256 || second.OfficialRevisionID != refreshOfficialB {
		t.Fatalf("association change not released: %+v", second)
	}
	matches, err := filepath.Glob(filepath.Join(portalRoot, "current", "fiscalizacao", "registro", "*", "index.html"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("record pages = %v, %v", matches, err)
	}
	record, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	body := string(record)
	if !strings.Contains(body, "Vinculado a posto") || !strings.Contains(body, "/postos/"+refreshStationID+"/") {
		t.Error("linked association missing from the rebuilt release")
	}
	if !strings.Contains(body, "não é histórico de propriedade") {
		t.Error("link states historical operation instead of the decision day")
	}
}
