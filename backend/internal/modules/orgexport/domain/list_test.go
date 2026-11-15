package domain_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
)

func listSpec() domain.ListSpec {
	return domain.ListSpec{
		Kind: "test.synthetic_list.csv", File: "synthetic.csv", ReadPermission: "fleet.asset.read",
		Columns: []domain.Column{
			{Name: "item_id", Type: domain.TextColumn},
			{Name: "label", Type: domain.TextColumn},
			{Name: "distance_m", Type: domain.IntegerColumn, Unit: "m"},
			{Name: "offset_s", Type: domain.IntegerColumn, Unit: "s"},
			{Name: "observed_at", Type: domain.InstantColumn},
		},
		Omitted:      []domain.Omission{{Column: "contact", Reason: "PURPOSE_LIMITED", Note: "Synthetic contact is never exported."}},
		Review:       domain.ApprovedOnly,
		ReviewLabel:  "somente aprovados",
		ContentBasis: "Synthetic rows.",
		Ordering:     []string{"observed_at", "item_id"},
	}
}

func listBasis() domain.ListBasis {
	return domain.ListBasis{ExportID: "7b0e2a54-0000-4000-8000-000000000050", OrganizationID: "7b0e2a54-0000-4000-8000-000000000001", OrganizationName: "Synthetic organization",
		OrganizationTimeZone: "America/Cuiaba", RequesterMembershipID: "7b0e2a54-0000-4000-8000-000000000004",
		AsOf: time.Date(2026, 11, 14, 12, 0, 0, 123456000, time.UTC), GeneratedAt: time.Date(2026, 11, 14, 12, 0, 5, 654321000, time.UTC), Filters: []byte(`{"state":"VALID"}`)}
}

type syntheticRow struct {
	id, label        string
	distance, offset int64
	at               time.Time
	empty            bool
}

var formulaLabels = []string{"=HYPERLINK(\"http://example.invalid\",\"x\")", "+5511999990000", "-2+3", "@SUM(A1:A2)", "\tlead tab", "\rlead cr", "\nlead lf", "'quoted", "plain, with \"quotes\"", "Ação ç ñ 漢字", " =spaced", ""}

func syntheticRows() []syntheticRow {
	rows := make([]syntheticRow, 0, len(formulaLabels)+1)
	base := time.Date(2026, 1, 2, 3, 4, 5, 6789, time.UTC)
	for i, label := range formulaLabels {
		rows = append(rows, syntheticRow{id: "7b0e2a54-0000-4000-8000-0000000001" + string(rune('0'+i/10)) + string(rune('0'+i%10)), label: label, distance: int64(i) * 1_000_000_007, offset: -64800 + int64(i), at: base.Add(time.Duration(i) * time.Hour)})
	}
	rows = append(rows, syntheticRow{id: "7b0e2a54-0000-4000-8000-000000000199", label: "absent values", empty: true, distance: math.MaxInt64})
	return rows
}

func writeList(t *testing.T, limits domain.Limits, spec domain.ListSpec, basis domain.ListBasis, rows []syntheticRow) (domain.File, error) {
	t.Helper()
	sink, err := domain.NewSink(limits)
	if err != nil {
		t.Fatal(err)
	}
	w, err := domain.NewListWriter(sink, spec, basis)
	if err != nil {
		return domain.File{}, err
	}
	for _, r := range rows {
		cells := []domain.Cell{domain.Text(r.id), domain.Text(r.label), domain.Integer(r.distance), domain.Integer(r.offset), domain.Instant(r.at)}
		if r.empty {
			cells[3], cells[4] = domain.Empty(), domain.Empty()
		}
		if err := w.Row(cells...); err != nil {
			return domain.File{}, err
		}
	}
	if err := w.Close(); err != nil {
		return domain.File{}, err
	}
	return sink.Result()
}

type unpacked struct {
	names    []string
	csv      []byte
	manifest []byte
	modified []time.Time
}

func unpack(t *testing.T, raw []byte) unpacked {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal("not a ZIP package", err)
	}
	var out unpacked
	for _, f := range reader.File {
		out.names = append(out.names, f.Name)
		out.modified = append(out.modified, f.Modified)
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		switch f.Name {
		case domain.ListManifestName:
			out.manifest = content
		default:
			out.csv = content
		}
	}
	return out
}

func parseCSV(t *testing.T, raw []byte) [][]string {
	t.Helper()
	if !bytes.HasPrefix(raw, []byte(domain.ByteOrderMark)) {
		t.Fatal("CSV without byte-order mark")
	}
	r := csv.NewReader(bytes.NewReader(raw[len(domain.ByteOrderMark):]))
	r.FieldsPerRecord = 0
	records, err := r.ReadAll()
	if err != nil {
		t.Fatal("CSV does not parse", err)
	}
	return records
}

// B-BR-EMP-256: neutralised starts gain exactly one apostrophe, everything
// else is untouched, and the scheme is reversible.
func TestInertTextNeutralisesFormulaStartsReversibly(t *testing.T) {
	for raw, want := range map[string]string{
		"=1+1": "'=1+1", "+5": "'+5", "-2": "'-2", "@SUM(A1)": "'@SUM(A1)", "\tx": "'\tx", "\rx": "'\rx", "\nx": "'\nx", "'x": "''x", "''": "'''",
		"plain": "plain", "": "", " =x": " =x", "1-2": "1-2", "a=b": "a=b", "ＡＢ": "ＡＢ", "７": "７",
	} {
		if got := domain.InertText(raw); got != want {
			t.Fatalf("InertText(%q) = %q, want %q", raw, got, want)
		}
		if back := domain.RecoverText(domain.InertText(raw)); back != raw {
			t.Fatalf("RecoverText(InertText(%q)) = %q", raw, back)
		}
	}
	for _, raw := range formulaLabels {
		written := domain.InertText(raw)
		if written != "" && strings.ContainsRune("=+-@\t\r\n", rune(written[0])) {
			t.Fatalf("written cell %q can start a formula", written)
		}
	}
}

// B-BR-EMP-256/257: exact integers, inert text, canonical instants, empty
// absent cells, the CSV and manifest in a ZIP, the basis and omissions stated
// without values.
func TestListWriterProducesExactInertCSVAndItsManifest(t *testing.T) {
	spec, basis, rows := listSpec(), listBasis(), syntheticRows()
	file, err := writeList(t, domain.DefaultLimits(), spec, basis, rows)
	if err != nil {
		t.Fatal(err)
	}
	if file.Rows != int64(len(rows)) {
		t.Fatal("sink did not count the data rows", file.Rows)
	}
	pkg := unpack(t, file.Bytes)
	if strings.Join(pkg.names, ",") != "synthetic.csv,manifest.json" {
		t.Fatal("package entries", pkg.names)
	}
	for _, at := range pkg.modified {
		if !at.Equal(basis.AsOf.Truncate(time.Second)) {
			t.Fatal("entry time is not the as-of instant", at)
		}
	}
	if !utf8.Valid(pkg.csv) || bytes.Contains(pkg.csv, []byte("\r\n")) {
		t.Fatal("CSV is not UTF-8 with LF line ends")
	}
	records := parseCSV(t, pkg.csv)
	if strings.Join(records[0], ",") != "item_id,label,distance_m,offset_s,observed_at" || len(records) != len(rows)+1 {
		t.Fatal("header or row count", records[0], len(records))
	}
	for i, r := range rows {
		got := records[i+1]
		if len(got) != 5 {
			t.Fatal("record width", got)
		}
		if domain.RecoverText(got[1]) != r.label || got[0] != r.id {
			t.Fatalf("text did not round-trip: %q -> %q", r.label, got[1])
		}
		if got[1] != "" && strings.ContainsRune("=+-@\t\r\n", rune(got[1][0])) {
			t.Fatalf("text cell %q can start a formula", got[1])
		}
		if got[2] != strconv.FormatInt(r.distance, 10) {
			t.Fatal("integer altered", got[2], r.distance)
		}
		if r.empty {
			if got[3] != "" || got[4] != "" {
				t.Fatal("absent value written", got)
			}
			continue
		}
		if got[3] != strconv.FormatInt(r.offset, 10) || !strings.HasPrefix(got[3], "-") {
			t.Fatal("negative integer was neutralised or altered", got[3])
		}
		at, err := time.Parse(time.RFC3339Nano, got[4])
		if err != nil || !at.Equal(r.at) || !strings.HasSuffix(got[4], "Z") || got[4] != r.at.Format(time.RFC3339Nano) {
			t.Fatal("instant not canonical UTC", got[4])
		}
	}
	var manifest domain.ListManifest
	decoder := json.NewDecoder(bytes.NewReader(pkg.manifest))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatal("manifest does not decode strictly", err)
	}
	sum := sha256.Sum256(pkg.csv)
	if manifest.Schema != domain.ListManifestSchema || manifest.Kind != spec.Kind || manifest.ExportID != basis.ExportID || manifest.OrganizationID != basis.OrganizationID ||
		manifest.OrganizationName != basis.OrganizationName || manifest.OrganizationTimeZone != basis.OrganizationTimeZone || manifest.RequesterMembershipID != basis.RequesterMembershipID ||
		manifest.ReadPermission != spec.ReadPermission || manifest.AsOf != "2026-11-14T12:00:00.123456Z" || manifest.GeneratedAt != "2026-11-14T12:00:05.654321Z" ||
		manifest.Filters["state"] != "VALID" || len(manifest.Filters) != 1 || manifest.ReviewBasis != domain.ApprovedOnly || manifest.ReviewBasisLabel != "somente aprovados" ||
		manifest.ContentBasis != spec.ContentBasis || strings.Join(manifest.Ordering, ",") != "observed_at,item_id" || manifest.RowCount != int64(len(rows)) {
		t.Fatal("manifest basis", string(pkg.manifest))
	}
	if len(manifest.Files) != 1 || manifest.Files[0] != (domain.ManifestFile{Name: "synthetic.csv", Rows: int64(len(rows)), Bytes: int64(len(pkg.csv)), SHA256: hex.EncodeToString(sum[:])}) {
		t.Fatal("manifest file identity", manifest.Files)
	}
	if len(manifest.Columns) != 5 || manifest.Columns[2] != spec.Columns[2] || len(manifest.OmittedColumns) != 1 || manifest.OmittedColumns[0] != spec.Omitted[0] {
		t.Fatal("manifest columns or omissions", manifest.Columns, manifest.OmittedColumns)
	}
	e := manifest.Encoding
	if e.Charset != "UTF-8" || !e.ByteOrderMark || e.Separator != "," || e.LineEnd != "LF" || e.InertPrefix != "'" || e.InertStarts == "" || e.InertRule == "" || e.IntegerRule == "" || e.InstantFormat == "" || e.AbsentValueRule == "" {
		t.Fatal("manifest encoding", e)
	}
}

// B-BR-EMP-256: the same data and basis give identical bytes; another
// generation instant changes only the manifest, never the CSV.
func TestListWriterIsDeterministic(t *testing.T) {
	spec, basis, rows := listSpec(), listBasis(), syntheticRows()
	first, err := writeList(t, domain.DefaultLimits(), spec, basis, rows)
	if err != nil {
		t.Fatal(err)
	}
	second, err := writeList(t, domain.DefaultLimits(), spec, basis, rows)
	if err != nil || !bytes.Equal(first.Bytes, second.Bytes) || first.SHA256 != second.SHA256 {
		t.Fatal("same input produced different packages", err)
	}
	later := basis
	later.GeneratedAt = later.GeneratedAt.Add(time.Minute)
	third, err := writeList(t, domain.DefaultLimits(), spec, later, rows)
	if err != nil || bytes.Equal(first.Bytes, third.Bytes) {
		t.Fatal("generation instant not stated", err)
	}
	a, c := unpack(t, first.Bytes), unpack(t, third.Bytes)
	if !bytes.Equal(a.csv, c.csv) {
		t.Fatal("CSV content depends on the generation instant")
	}
	if !bytes.Equal(bytes.Replace(a.manifest, []byte("2026-11-14T12:00:05.654321Z"), []byte("X"), 1), bytes.Replace(c.manifest, []byte("2026-11-14T12:01:05.654321Z"), []byte("X"), 1)) {
		t.Fatal("manifest differs beyond generated_at")
	}
}

// B-BR-EMP-258: a row or byte ceiling poisons the package; nothing complete
// remains. An empty list is a valid package with a header only.
func TestListWriterCeilingsFailWholeAndEmptyIsValid(t *testing.T) {
	spec, basis, rows := listSpec(), listBasis(), syntheticRows()
	if _, err := writeList(t, domain.Limits{Rows: 2, Bytes: domain.MaxBytes, Duration: time.Second}, spec, basis, rows); !errors.Is(err, domain.ErrTooLarge) {
		t.Fatal("row ceiling", err)
	}
	if _, err := writeList(t, domain.Limits{Rows: domain.MaxRows, Bytes: 300, Duration: time.Second}, spec, basis, rows); !errors.Is(err, domain.ErrTooLarge) {
		t.Fatal("byte ceiling", err)
	}
	sink, _ := domain.NewSink(domain.Limits{Rows: 1, Bytes: domain.MaxBytes, Duration: time.Second})
	w, err := domain.NewListWriter(sink, spec, basis)
	if err != nil {
		t.Fatal(err)
	}
	cells := []domain.Cell{domain.Text("a"), domain.Text("b"), domain.Integer(1), domain.Integer(2), domain.Instant(basis.AsOf)}
	if err := w.Row(cells...); err != nil {
		t.Fatal(err)
	}
	if err := w.Row(cells...); !errors.Is(err, domain.ErrTooLarge) {
		t.Fatal("second row over a one-row ceiling", err)
	}
	if err := w.Close(); err == nil {
		t.Fatal("a poisoned package closed")
	}
	if _, err := sink.Result(); !errors.Is(err, domain.ErrTooLarge) {
		t.Fatal("a truncated package is available", err)
	}
	empty, err := writeList(t, domain.DefaultLimits(), spec, basis, nil)
	if err != nil || empty.Rows != 0 {
		t.Fatal("empty list", err)
	}
	pkg := unpack(t, empty.Bytes)
	if records := parseCSV(t, pkg.csv); len(records) != 1 || !bytes.Contains(pkg.manifest, []byte(`"row_count": 0`)) {
		t.Fatal("empty list content", records, string(pkg.manifest))
	}
}

// B-BR-EMP-256/258: a malformed row fails the writer; nothing is closed.
func TestListWriterRefusesInvalidRows(t *testing.T) {
	spec, basis := listSpec(), listBasis()
	good := func() []domain.Cell {
		return []domain.Cell{domain.Text("a"), domain.Text("b"), domain.Integer(1), domain.Integer(2), domain.Instant(basis.AsOf)}
	}
	for name, edit := range map[string]func([]domain.Cell) []domain.Cell{
		"too few":         func(c []domain.Cell) []domain.Cell { return c[:4] },
		"too many":        func(c []domain.Cell) []domain.Cell { return append(c, domain.Text("x")) },
		"text in integer": func(c []domain.Cell) []domain.Cell { c[2] = domain.Text("1"); return c },
		"integer in text": func(c []domain.Cell) []domain.Cell { c[1] = domain.Integer(1); return c },
		"instant in text": func(c []domain.Cell) []domain.Cell { c[1] = domain.Instant(basis.AsOf); return c },
		"text in instant": func(c []domain.Cell) []domain.Cell { c[4] = domain.Text("2026-01-01T00:00:00Z"); return c },
		"zero instant":    func(c []domain.Cell) []domain.Cell { c[4] = domain.Instant(time.Time{}); return c },
		"invalid UTF-8":   func(c []domain.Cell) []domain.Cell { c[1] = domain.Text("bad\xff"); return c },
		"zero value cell": func(c []domain.Cell) []domain.Cell { c[0] = domain.Cell{}; return c },
		"year beyond 9999": func(c []domain.Cell) []domain.Cell {
			c[4] = domain.Instant(time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC))
			return c
		},
	} {
		t.Run(name, func(t *testing.T) {
			sink, _ := domain.NewSink(domain.DefaultLimits())
			w, err := domain.NewListWriter(sink, spec, basis)
			if err != nil {
				t.Fatal(err)
			}
			if err := w.Row(edit(good())...); !errors.Is(err, domain.ErrInvalid) {
				t.Fatal("invalid row accepted", err)
			}
			if err := w.Row(good()...); err == nil {
				t.Fatal("a failed writer accepted another row")
			}
			if err := w.Close(); err == nil {
				t.Fatal("a failed writer closed a package")
			}
		})
	}
	sink, _ := domain.NewSink(domain.DefaultLimits())
	w, _ := domain.NewListWriter(sink, spec, basis)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if w.Row(good()...) == nil || w.Close() == nil {
		t.Fatal("a closed writer accepted more work")
	}
}

// EDR-171: specs and bases are validated before anything is written.
func TestListWriterValidatesSpecAndBasis(t *testing.T) {
	specEdits := map[string]func(*domain.ListSpec){
		"bad kind":             func(s *domain.ListSpec) { s.Kind = "Bad" },
		"bad file":             func(s *domain.ListSpec) { s.File = "../x.csv" },
		"not csv file":         func(s *domain.ListSpec) { s.File = "list.txt" },
		"no permission":        func(s *domain.ListSpec) { s.ReadPermission = "" },
		"no columns":           func(s *domain.ListSpec) { s.Columns = nil },
		"duplicate column":     func(s *domain.ListSpec) { s.Columns = append(s.Columns, s.Columns[0]) },
		"bad column name":      func(s *domain.ListSpec) { s.Columns[1].Name = "Label" },
		"unknown column type":  func(s *domain.ListSpec) { s.Columns[1].Type = "MONEY" },
		"integer without unit": func(s *domain.ListSpec) { s.Columns[2].Unit = "" },
		"omission is a column": func(s *domain.ListSpec) { s.Omitted[0].Column = "label" },
		"omission no reason":   func(s *domain.ListSpec) { s.Omitted[0].Reason = "" },
		"omission no note":     func(s *domain.ListSpec) { s.Omitted[0].Note = "" },
		"bad reason":           func(s *domain.ListSpec) { s.Omitted[0].Reason = "lower" },
		"unknown review":       func(s *domain.ListSpec) { s.Review = "PENDING" },
		"no review label":      func(s *domain.ListSpec) { s.ReviewLabel = "" },
		"no content basis":     func(s *domain.ListSpec) { s.ContentBasis = "" },
		"no ordering":          func(s *domain.ListSpec) { s.Ordering = nil },
		"unknown ordering":     func(s *domain.ListSpec) { s.Ordering = []string{"missing"} },
	}
	for name, edit := range specEdits {
		t.Run(name, func(t *testing.T) {
			spec := listSpec()
			spec.Columns = append([]domain.Column(nil), spec.Columns...)
			spec.Omitted = append([]domain.Omission(nil), spec.Omitted...)
			edit(&spec)
			sink, _ := domain.NewSink(domain.DefaultLimits())
			if _, err := domain.NewListWriter(sink, spec, listBasis()); !errors.Is(err, domain.ErrInvalid) || sink.Bytes() != 0 {
				t.Fatal("invalid spec accepted", err)
			}
		})
	}
	basisEdits := map[string]func(*domain.ListBasis){
		"bad export":        func(b *domain.ListBasis) { b.ExportID = "x" },
		"bad organization":  func(b *domain.ListBasis) { b.OrganizationID = strings.ToUpper(b.OrganizationID) + "x" },
		"bad requester":     func(b *domain.ListBasis) { b.RequesterMembershipID = "" },
		"zero as of":        func(b *domain.ListBasis) { b.AsOf = time.Time{} },
		"zero generated":    func(b *domain.ListBasis) { b.GeneratedAt = time.Time{} },
		"non-canonical":     func(b *domain.ListBasis) { b.Filters = []byte(`{ "state":"VALID"}`) },
		"not an object":     func(b *domain.ListBasis) { b.Filters = []byte(`[]`) },
		"invalid name":      func(b *domain.ListBasis) { b.OrganizationName = "bad\xff" },
		"invalid time zone": func(b *domain.ListBasis) { b.OrganizationTimeZone = "bad\xff" },
	}
	for name, edit := range basisEdits {
		t.Run(name, func(t *testing.T) {
			basis := listBasis()
			edit(&basis)
			sink, _ := domain.NewSink(domain.DefaultLimits())
			if _, err := domain.NewListWriter(sink, listSpec(), basis); !errors.Is(err, domain.ErrInvalid) || sink.Bytes() != 0 {
				t.Fatal("invalid basis accepted", err)
			}
		})
	}
	if _, err := domain.NewListWriter(nil, listSpec(), listBasis()); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("nil sink accepted", err)
	}
	if listSpec().Validate() != nil || listBasis().Validate() != nil {
		t.Fatal("valid spec or basis refused")
	}
}

// EDR-171: CountRow counts a packaged row against the row ceiling only.
func TestSinkCountRowAppliesTheRowCeiling(t *testing.T) {
	sink, _ := domain.NewSink(domain.Limits{Rows: 2, Bytes: 4, Duration: time.Second})
	for i := 0; i < 2; i++ {
		if err := sink.CountRow(); err != nil {
			t.Fatal("row under the ceiling refused", i, err)
		}
	}
	if sink.Rows() != 2 || sink.Bytes() != 0 {
		t.Fatal("counted rows", sink.Rows())
	}
	if !errors.Is(sink.CountRow(), domain.ErrTooLarge) || sink.Rows() != 2 {
		t.Fatal("row ceiling not applied")
	}
	if _, err := sink.Write([]byte("x")); !errors.Is(err, domain.ErrTooLarge) {
		t.Fatal("sink not poisoned after a counted refusal")
	}
}
