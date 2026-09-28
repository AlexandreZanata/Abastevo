package anp

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"
)

// Spec limits (ANP_INGESTION import stage 2). Parser.Zero value is invalid;
// use DefaultLimits.
type Limits struct {
	MaxCompressedBytes   int64
	MaxUncompressedBytes int64
	MaxSheets            int
	MaxRows              int
	MaxHeaderScanRows    int
	MaxSharedStrings     int
}

// DefaultLimits mirrors the documented import caps.
func DefaultLimits() Limits {
	return Limits{
		MaxCompressedBytes:   30 << 20,
		MaxUncompressedBytes: 250 << 20,
		MaxSheets:            100,
		MaxRows:              500000,
		MaxHeaderScanRows:    25,
		MaxSharedStrings:     500000,
	}
}

var (
	ErrTooLarge       = errors.New("anp: input exceeds bounded limits")
	ErrMalformed      = errors.New("anp: malformed workbook")
	ErrUnsafeEntry    = errors.New("anp: unsafe archive entry")
	ErrMissingSheet   = errors.New("anp: expected sheet not found")
	ErrMissingColumns = errors.New("anp: essential columns not found")
)

// Row is one data row: header name to raw cell text. Texts are preserved
// byte-exact (no float, no rounding); normalization belongs to the import
// flow, which quarantines what it cannot prove.
type Row struct {
	Number int
	Cells  map[string]string
}

// requiredHeaders are the essential columns an ANP station sheet must carry.
var requiredHeaders = []string{"CNPJ", "PRODUTO", "PRECO", "DATA"}

func normalizeHeader(h string) string {
	return strings.ToUpper(strings.Join(strings.Fields(h), " "))
}

// headerKey folds the known label variants ANP emits (accents, spaces)
// into the canonical required set. Unknown headers pass through under
// their normalized form so the import can quarantine, never coerce.
func headerKey(h string) string {
	n := normalizeHeader(accentFoldHeader(h))
	n = strings.ReplaceAll(n, "Ç", "C")
	switch {
	case strings.Contains(n, "CNPJ"):
		return "CNPJ"
	case strings.Contains(n, "PRODUTO") || strings.Contains(n, "COMBUST"):
		return "PRODUTO"
	case strings.Contains(n, "PRECO") || strings.Contains(n, "VALOR"):
		return "PRECO"
	case strings.Contains(n, "DATA"):
		return "DATA"
	default:
		return n
	}
}

var headerAccents = strings.NewReplacer(
	"À", "A", "Á", "A", "Â", "A", "Ã", "A", "Ä", "A",
	"È", "E", "É", "E", "Ê", "E", "Ë", "E",
	"Ì", "I", "Í", "I", "Î", "I", "Ï", "I",
	"Ò", "O", "Ó", "O", "Ô", "O", "Õ", "O", "Ö", "O",
	"Ù", "U", "Ú", "U", "Û", "U", "Ü", "U",
	"Ç", "C",
)

func accentFoldHeader(h string) string { return headerAccents.Replace(h) }

// Parser reads bounded ANP workbooks. The zero value is invalid.
type Parser struct {
	Limits Limits
}

// openZip guards entry traversal and pre-checks compressed and declared
// uncompressed sizes before any content is read.
func (p Parser) openZip(data []byte) (*zip.Reader, error) {
	if int64(len(data)) > p.Limits.MaxCompressedBytes {
		return nil, fmt.Errorf("%w: %d compressed bytes", ErrTooLarge, len(data))
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	var total uint64
	for _, f := range zr.File {
		name := path.Clean("/" + f.Name)
		if name != "/"+f.Name || strings.HasPrefix(f.Name, "/") || strings.Contains(f.Name, "..") {
			return nil, fmt.Errorf("%w: %q", ErrUnsafeEntry, f.Name)
		}
		total += f.UncompressedSize64
		if total > uint64(p.Limits.MaxUncompressedBytes) {
			return nil, fmt.Errorf("%w: declared content", ErrTooLarge)
		}
	}
	return zr, nil
}

func readCapped(f *zip.File, limit int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	// LimitReader bounds actual bytes even when headers lie about sizes.
	return io.ReadAll(io.LimitReader(rc, limit+1))
}

func refuseDoctype(raw []byte) error {
	if bytes.Contains(raw, []byte("<!DOCTYPE")) || bytes.Contains(raw, []byte("<!ENTITY")) {
		return fmt.Errorf("%w: document type declarations refused", ErrMalformed)
	}
	return nil
}

type sheetRef struct {
	Name   string
	Target string
}

// workbookSheets lists sheets in declaration order with their part targets.
func workbookSheets(zr *zip.Reader, read func(string) ([]byte, error)) ([]sheetRef, error) {
	raw, err := read("xl/workbook.xml")
	if err != nil {
		return nil, err
	}
	if err := refuseDoctype(raw); err != nil {
		return nil, err
	}
	rels, err := read("xl/_rels/workbook.xml.rels")
	if err != nil {
		return nil, err
	}
	if err := refuseDoctype(rels); err != nil {
		return nil, err
	}
	targetByID := map[string]string{}
	dec := xml.NewDecoder(bytes.NewReader(rels))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "Relationship" {
			continue
		}
		var id, target, typ string
		for _, a := range se.Attr {
			switch a.Name.Local {
			case "Id":
				id = a.Value
			case "Target":
				target = a.Value
			case "Type":
				typ = a.Value
			}
		}
		if strings.HasSuffix(typ, "/worksheet") {
			targetByID[id] = "xl/" + strings.TrimPrefix(target, "/")
		}
	}
	var sheets []sheetRef
	dec = xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "sheet" {
			continue
		}
		var name, rid string
		for _, a := range se.Attr {
			switch a.Name.Local {
			case "name":
				name = a.Value
			case "id":
				rid = a.Value
			}
		}
		target, ok := targetByID[rid]
		if !ok || name == "" {
			return nil, fmt.Errorf("%w: dangling sheet reference", ErrMalformed)
		}
		sheets = append(sheets, sheetRef{Name: name, Target: target})
	}
	return sheets, nil
}

// sharedStrings loads the string table with count and byte caps.
func sharedStrings(raw []byte, lim Limits) ([]string, error) {
	if err := refuseDoctype(raw); err != nil {
		return nil, err
	}
	var out []string
	var total int64
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "t" {
			continue
		}
		var text string
		if err := dec.DecodeElement(&text, &se); err != nil {
			return nil, fmt.Errorf("%w: shared strings: %v", ErrMalformed, err)
		}
		total += int64(len(text))
		out = append(out, text)
		if len(out) > lim.MaxSharedStrings || total > lim.MaxUncompressedBytes {
			return nil, fmt.Errorf("%w: string table", ErrTooLarge)
		}
	}
	return out, nil
}

type cell struct {
	col   int
	text  string
	isErr bool
}

// colIndex converts an A1 column reference to a zero-based index.
func colIndex(ref string) (int, error) {
	i := 0
	letters := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		i = i*26 + int(r-'A'+1)
		letters++
	}
	if letters == 0 {
		return 0, fmt.Errorf("%w: bad cell reference %q", ErrMalformed, ref)
	}
	return i - 1, nil
}

// sheetCells streams rows of raw cells: column index to text. Formula cells
// contribute only their cached value; cells without one stay empty. Error
// cells are flagged so the import can quarantine with cause.
func sheetCells(raw []byte, shared []string, maxRows int, yield func(num int, cells []cell) error) error {
	if err := refuseDoctype(raw); err != nil {
		return err
	}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	rows := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "row" {
			continue
		}
		rows++
		if rows > maxRows {
			return fmt.Errorf("%w: rows", ErrTooLarge)
		}
		num := rows
		for _, a := range se.Attr {
			if a.Name.Local == "r" {
				if n, cerr := strconv.Atoi(a.Value); cerr == nil {
					num = n
				}
			}
		}
		var cells []cell
	Row:
		for {
			tok, err := dec.Token()
			if err != nil {
				return fmt.Errorf("%w: truncated row %d: %v", ErrMalformed, num, err)
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local != "c" {
					if err := dec.Skip(); err != nil {
						return fmt.Errorf("%w: %v", ErrMalformed, err)
					}
					continue
				}
				var ref, typ string
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "r":
						ref = a.Value
					case "t":
						typ = a.Value
					}
				}
				col, err := colIndex(ref)
				if err != nil {
					return err
				}
				var value, cached string
				var isErr bool
				if typ == "e" {
					isErr = true
				}
			Cell:
				for {
					ctok, err := dec.Token()
					if err != nil {
						return fmt.Errorf("%w: truncated cell %s: %v", ErrMalformed, ref, err)
					}
					switch ct := ctok.(type) {
					case xml.StartElement:
						switch ct.Name.Local {
						case "v":
							var v string
							if err := dec.DecodeElement(&v, &ct); err != nil {
								return fmt.Errorf("%w: %v", ErrMalformed, err)
							}
							cached = v
						case "t":
							var s string
							if err := dec.DecodeElement(&s, &ct); err != nil {
								return fmt.Errorf("%w: %v", ErrMalformed, err)
							}
							value = s
						default:
							// <f> formulas are skipped: never evaluate,
							// keep whatever cached <v> was stored.
							if err := dec.Skip(); err != nil {
								return fmt.Errorf("%w: %v", ErrMalformed, err)
							}
						}
					case xml.EndElement:
						if ct.Name.Local == "c" {
							text := value
							if text == "" {
								text = cached
							}
							if typ == "s" {
								idx, cerr := strconv.Atoi(cached)
								if cerr != nil || idx < 0 || idx >= len(shared) {
									return fmt.Errorf("%w: shared string %q out of range", ErrMalformed, cached)
								}
								text = shared[idx]
							}
							cells = append(cells, cell{col: col, text: text, isErr: isErr})
							break Cell
						}
					}
				}
			case xml.EndElement:
				if t.Name.Local == "row" {
					break Row
				}
			}
		}
		if err := yield(num, cells); err != nil {
			return err
		}
	}
	return nil
}

// ExcelSerialToDate converts a 1900-system serial to an ISO date. Serial 60
// is the Lotus phantom 1900-02-29 and fails; the epoch math 1899-12-30 plus
// serial days is exact for every other value in range.
func ExcelSerialToDate(serial int) (string, error) {
	if serial <= 0 || serial == 60 || serial > 2958465 {
		return "", fmt.Errorf("%w: excel serial %d", ErrMalformed, serial)
	}
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	return base.AddDate(0, 0, serial).Format("2006-01-02"), nil
}
