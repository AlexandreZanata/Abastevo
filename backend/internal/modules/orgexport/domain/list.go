package domain

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"regexp"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"
)

// List packages (EMP-RPT-002, EDR-171): one CSV table and its manifest in a
// ZIP package written through the bounded Sink. Pure: no I/O beyond the sink.

const (
	// ListMediaType is the media type of every list package.
	ListMediaType = "application/zip"
	// ListManifestName is the manifest entry of a list package.
	ListManifestName = "manifest.json"
	// ListManifestSchema versions the manifest content.
	ListManifestSchema = "orgexport.list.v1"
	// InertPrefix is prepended to a text cell that could start a formula.
	InertPrefix = "'"
	// ByteOrderMark starts every CSV table (§10.10).
	ByteOrderMark = "\ufeff"
	// inertStarts are the first bytes that gain the prefix (BR-RPT-006, plus
	// LF and the prefix itself so the scheme stays reversible).
	inertStarts = "=+-@\t\r\n'"
)

var (
	columnPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	filePattern   = regexp.MustCompile(`^[a-z][a-z0-9_]*\.csv$`)
	reasonPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
)

// ColumnType is the closed set of cell types of a list column.
type ColumnType string

const (
	// TextColumn cells are written inert (B-BR-EMP-256).
	TextColumn ColumnType = "TEXT"
	// IntegerColumn cells are exact server integers in the column's unit.
	IntegerColumn ColumnType = "INTEGER"
	// InstantColumn cells are canonical RFC 3339 UTC instants.
	InstantColumn ColumnType = "INSTANT"
)

func (t ColumnType) valid() bool { return t == TextColumn || t == IntegerColumn || t == InstantColumn }

// Column describes one CSV column.
type Column struct {
	Name string     `json:"name"`
	Type ColumnType `json:"type"`
	Unit string     `json:"unit,omitempty"`
	Note string     `json:"note,omitempty"`
}

// Omission reports a sensitive column left out of a list, never its values.
type Omission struct {
	Column string `json:"column"`
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

// ReviewBasis states which review states a list includes (BR-RPT-007).
type ReviewBasis string

const (
	// ApprovedOnly lists accepted records only ("somente aprovados").
	ApprovedOnly ReviewBasis = "APPROVED_ONLY"
	// NotApplicable marks a register without a review flow.
	NotApplicable ReviewBasis = "NOT_APPLICABLE"
)

// ListSpec is the static description of one list kind.
type ListSpec struct {
	Kind, File, ReadPermission string
	Columns                    []Column
	Omitted                    []Omission
	Review                     ReviewBasis
	ReviewLabel, ContentBasis  string
	Ordering                   []string
}

func validText(v string) bool { return v != "" && utf8.ValidString(v) }

// Validate checks a spec: valid kind, a lower-snake .csv file name, unique
// lower-snake columns with a valid type (integers name their unit), omissions
// with a reason and note that are not columns, a known review basis and an
// ordering over declared columns.
func (s ListSpec) Validate() error {
	if !ValidKind(s.Kind) || !filePattern.MatchString(s.File) || s.File == ListManifestName || !validText(s.ReadPermission) || len(s.Columns) == 0 ||
		s.Review != ApprovedOnly && s.Review != NotApplicable || !validText(s.ReviewLabel) || !validText(s.ContentBasis) || len(s.Ordering) == 0 {
		return ErrInvalid
	}
	names := map[string]bool{}
	for _, c := range s.Columns {
		if !columnPattern.MatchString(c.Name) || names[c.Name] || !c.Type.valid() || (c.Type == IntegerColumn) != (c.Unit != "") || !utf8.ValidString(c.Unit) || !utf8.ValidString(c.Note) {
			return ErrInvalid
		}
		names[c.Name] = true
	}
	for _, o := range s.Omitted {
		if !columnPattern.MatchString(o.Column) || names[o.Column] || !reasonPattern.MatchString(o.Reason) || !validText(o.Note) {
			return ErrInvalid
		}
	}
	for _, name := range s.Ordering {
		if !names[name] {
			return ErrInvalid
		}
	}
	return nil
}

// ListBasis is the per-run basis stated in the manifest.
type ListBasis struct {
	ExportID, OrganizationID, OrganizationName, OrganizationTimeZone, RequesterMembershipID string
	AsOf, GeneratedAt                                                                       time.Time
	// Filters are the job's canonical parameters.
	Filters []byte
}

func validInstant(at time.Time) bool {
	year := at.UTC().Year()
	return !at.IsZero() && year >= 1 && year <= 9999
}

// Validate checks canonical identifiers, instants and filters.
func (b ListBasis) Validate() error {
	if !ValidID(b.ExportID) || !ValidID(b.OrganizationID) || !ValidID(b.RequesterMembershipID) || !utf8.ValidString(b.OrganizationName) || !utf8.ValidString(b.OrganizationTimeZone) ||
		!validInstant(b.AsOf) || !validInstant(b.GeneratedAt) {
		return ErrInvalid
	}
	canonical, err := CanonicalParameters(b.Filters)
	if err != nil || !bytes.Equal(canonical, b.Filters) {
		return ErrInvalid
	}
	return nil
}

type cellKind uint8

const (
	invalidCell cellKind = iota
	textCell
	integerCell
	instantCell
	emptyCell
)

// Cell is one typed CSV value.
type Cell struct {
	kind    cellKind
	text    string
	integer int64
}

// Text is a text cell; it is written inert.
func Text(v string) Cell { return Cell{kind: textCell, text: v} }

// Integer is an exact integer cell; it is never altered.
func Integer(v int64) Cell { return Cell{kind: integerCell, integer: v} }

// Instant is a canonical RFC 3339 UTC instant cell; a zero time or a year
// outside 1–9999 is invalid.
func Instant(at time.Time) Cell {
	if !validInstant(at) {
		return Cell{}
	}
	return Cell{kind: instantCell, text: at.UTC().Format(time.RFC3339Nano)}
}

// Empty is an absent value: an empty cell, never 0.
func Empty() Cell { return Cell{kind: emptyCell} }

// InertText returns the text written for a stored value: one apostrophe is
// prepended when the value begins with =, +, -, @, tab, CR, LF or an
// apostrophe, so no cell can start a formula (BR-RPT-006).
func InertText(v string) string {
	if v != "" && bytes.IndexByte([]byte(inertStarts), v[0]) >= 0 {
		return InertPrefix + v
	}
	return v
}

// RecoverText returns the stored value of a written text cell by removing
// exactly one leading apostrophe.
func RecoverText(v string) string {
	if len(v) > 0 && v[0] == InertPrefix[0] {
		return v[1:]
	}
	return v
}

// write returns the field text for a column of type t, or false.
func (c Cell) write(t ColumnType) (string, bool) {
	switch {
	case c.kind == emptyCell:
		return "", true
	case c.kind == textCell && t == TextColumn:
		return InertText(c.text), utf8.ValidString(c.text)
	case c.kind == integerCell && t == IntegerColumn:
		return strconv.FormatInt(c.integer, 10), true
	case c.kind == instantCell && t == InstantColumn:
		return c.text, true
	}
	return "", false
}

// ManifestFile identifies the CSV inside the package.
type ManifestFile struct {
	Name   string `json:"name"`
	Rows   int64  `json:"rows"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// Encoding states how the CSV is written.
type Encoding struct {
	Charset         string `json:"charset"`
	ByteOrderMark   bool   `json:"byte_order_mark"`
	Separator       string `json:"separator"`
	LineEnd         string `json:"line_end"`
	InertPrefix     string `json:"inert_prefix"`
	InertStarts     string `json:"inert_starts"`
	InertRule       string `json:"inert_rule"`
	IntegerRule     string `json:"integer_rule"`
	InstantFormat   string `json:"instant_format"`
	AbsentValueRule string `json:"absent_value_rule"`
}

var listEncoding = Encoding{
	Charset: "UTF-8", ByteOrderMark: true, Separator: ",", LineEnd: "LF", InertPrefix: InertPrefix,
	InertStarts:     "= + - @ TAB CR LF '",
	InertRule:       "A TEXT cell whose stored value begins with one of inert_starts is written with one leading inert_prefix; remove exactly one leading inert_prefix from a TEXT cell to recover the stored value. Fields holding the separator, a quote, a line break or a leading space are quoted (RFC 4180) and quotes are doubled.",
	IntegerRule:     "INTEGER cells are exact base-10 integers in the unit of their column and are never prefixed.",
	InstantFormat:   "RFC 3339 in UTC (Z) with the stored fractional digits",
	AbsentValueRule: "An absent value is an empty cell, never 0.",
}

// ListManifest is the content of manifest.json (B-BR-EMP-257).
type ListManifest struct {
	Schema                string         `json:"schema"`
	Kind                  string         `json:"kind"`
	ExportID              string         `json:"export_id"`
	OrganizationID        string         `json:"organization_id"`
	OrganizationName      string         `json:"organization_name"`
	OrganizationTimeZone  string         `json:"organization_time_zone"`
	RequesterMembershipID string         `json:"requester_membership_id"`
	ReadPermission        string         `json:"read_permission"`
	AsOf                  string         `json:"as_of"`
	GeneratedAt           string         `json:"generated_at"`
	Filters               map[string]any `json:"filters"`
	ReviewBasis           ReviewBasis    `json:"review_basis"`
	ReviewBasisLabel      string         `json:"review_basis_label"`
	ContentBasis          string         `json:"content_basis"`
	Ordering              []string       `json:"ordering"`
	RowCount              int64          `json:"row_count"`
	Files                 []ManifestFile `json:"files"`
	Columns               []Column       `json:"columns"`
	OmittedColumns        []Omission     `json:"omitted_columns"`
	Encoding              Encoding       `json:"encoding"`
}

// sinkWriter adapts the sink to io.Writer for the ZIP stream.
type sinkWriter struct{ sink *Sink }

func (w sinkWriter) Write(p []byte) (int, error) { return w.sink.Write(p) }

// tableWriter counts and hashes the uncompressed CSV on its way into the
// ZIP entry.
type tableWriter struct {
	entry io.Writer
	hash  hash.Hash
	bytes int64
}

func (w *tableWriter) Write(p []byte) (int, error) {
	n, err := w.entry.Write(p)
	w.hash.Write(p[:n])
	w.bytes += int64(n)
	return n, err
}

var errListState = errors.New("orgexport: list writer is closed or failed")

// ListWriter streams one CSV list and its manifest into a ZIP package
// through the sink. A refused row or ceiling leaves it failed: Close then
// fails and no complete package exists.
type ListWriter struct {
	sink   *Sink
	spec   ListSpec
	basis  ListBasis
	zip    *zip.Writer
	table  *tableWriter
	csv    *csv.Writer
	rows   int64
	record []string
	done   bool
	err    error
}

// NewListWriter validates the spec and basis and starts the package.
func NewListWriter(sink *Sink, spec ListSpec, basis ListBasis) (*ListWriter, error) {
	if sink == nil || spec.Validate() != nil || basis.Validate() != nil {
		return nil, ErrInvalid
	}
	w := &ListWriter{sink: sink, spec: spec, basis: basis, zip: zip.NewWriter(sinkWriter{sink}), record: make([]string, len(spec.Columns))}
	entry, err := w.zip.CreateHeader(&zip.FileHeader{Name: spec.File, Method: zip.Deflate, Modified: basis.AsOf.UTC()})
	if err != nil {
		return nil, w.fail(err)
	}
	w.table = &tableWriter{entry: entry, hash: sha256.New()}
	if _, err := io.WriteString(w.table, ByteOrderMark); err != nil {
		return nil, w.fail(err)
	}
	w.csv = csv.NewWriter(w.table)
	header := make([]string, len(spec.Columns))
	for i, c := range spec.Columns {
		header[i] = c.Name
	}
	if err := w.csv.Write(header); err != nil {
		return nil, w.fail(err)
	}
	return w, nil
}

// fail records the first failure; a ceiling stays ErrTooLarge.
func (w *ListWriter) fail(err error) error {
	if w.err == nil {
		if errors.Is(err, ErrTooLarge) {
			w.err = ErrTooLarge
		} else if errors.Is(err, ErrInvalid) {
			w.err = ErrInvalid
		} else {
			w.err = err
		}
	}
	return w.err
}

func (w *ListWriter) usable() error {
	if w == nil || w.done {
		return errListState
	}
	return w.err
}

// Row writes one data row: exactly one cell per column, of the column's type.
func (w *ListWriter) Row(cells ...Cell) error {
	if err := w.usable(); err != nil {
		return err
	}
	if len(cells) != len(w.spec.Columns) {
		return w.fail(ErrInvalid)
	}
	for i, c := range cells {
		field, ok := c.write(w.spec.Columns[i].Type)
		if !ok {
			return w.fail(ErrInvalid)
		}
		w.record[i] = field
	}
	if err := w.sink.CountRow(); err != nil {
		return w.fail(err)
	}
	if err := w.csv.Write(w.record); err != nil {
		return w.fail(err)
	}
	w.rows++
	return nil
}

// Close finishes the CSV, writes the manifest and closes the package.
func (w *ListWriter) Close() error {
	if err := w.usable(); err != nil {
		return err
	}
	w.done = true
	w.csv.Flush()
	if err := w.csv.Error(); err != nil {
		return w.fail(err)
	}
	manifest, err := w.manifest()
	if err != nil {
		return w.fail(err)
	}
	entry, err := w.zip.CreateHeader(&zip.FileHeader{Name: ListManifestName, Method: zip.Deflate, Modified: w.basis.AsOf.UTC()})
	if err != nil {
		return w.fail(err)
	}
	if _, err := entry.Write(manifest); err != nil {
		return w.fail(err)
	}
	if err := w.zip.Close(); err != nil {
		return w.fail(err)
	}
	return nil
}

func (w *ListWriter) manifest() ([]byte, error) {
	var filters map[string]any
	decoder := json.NewDecoder(bytes.NewReader(w.basis.Filters))
	decoder.UseNumber()
	if err := decoder.Decode(&filters); err != nil {
		return nil, ErrInvalid
	}
	omitted := slices.Clone(w.spec.Omitted)
	if omitted == nil {
		omitted = []Omission{}
	}
	m := ListManifest{Schema: ListManifestSchema, Kind: w.spec.Kind, ExportID: w.basis.ExportID, OrganizationID: w.basis.OrganizationID, OrganizationName: w.basis.OrganizationName,
		OrganizationTimeZone: w.basis.OrganizationTimeZone, RequesterMembershipID: w.basis.RequesterMembershipID, ReadPermission: w.spec.ReadPermission,
		AsOf: w.basis.AsOf.UTC().Format(time.RFC3339Nano), GeneratedAt: w.basis.GeneratedAt.UTC().Format(time.RFC3339Nano), Filters: filters,
		ReviewBasis: w.spec.Review, ReviewBasisLabel: w.spec.ReviewLabel, ContentBasis: w.spec.ContentBasis, Ordering: slices.Clone(w.spec.Ordering), RowCount: w.rows,
		Files:   []ManifestFile{{Name: w.spec.File, Rows: w.rows, Bytes: w.table.bytes, SHA256: hex.EncodeToString(w.table.hash.Sum(nil))}},
		Columns: slices.Clone(w.spec.Columns), OmittedColumns: omitted, Encoding: listEncoding}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(m); err != nil {
		return nil, ErrInvalid
	}
	return out.Bytes(), nil
}
