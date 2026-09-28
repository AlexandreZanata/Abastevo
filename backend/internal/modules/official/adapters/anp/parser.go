package anp

import (
	"fmt"
	"sort"
	"strings"
)

// Parse reads one ANP workbook and streams data rows of the selected sheet
// to yield. sheetHint names the expected sheet (e.g. DPC); when it is
// absent, sheets are probed in order for a header signature instead of
// failing on a renamed layout. Headers are detected within the first
// MaxHeaderScanRows rows; data starts on the following row. Unknown labels
// pass through as raw text: rejection with cause belongs to the import flow,
// which owns the quarantine codes.
func (p Parser) Parse(data []byte, sheetHint string, yield func(Row) error) (sheet string, headerRow int, err error) {
	zr, err := p.openZip(data)
	if err != nil {
		return "", 0, err
	}
	files := map[string][]byte{}
	read := func(name string) ([]byte, error) {
		if raw, ok := files[name]; ok {
			return raw, nil
		}
		for _, f := range zr.File {
			if f.Name != name {
				continue
			}
			raw, err := readCapped(f, p.Limits.MaxUncompressedBytes)
			if err != nil {
				return nil, err
			}
			if int64(len(raw)) > p.Limits.MaxUncompressedBytes {
				return nil, fmt.Errorf("%w: %s content", ErrTooLarge, name)
			}
			files[name] = raw
			return raw, nil
		}
		return nil, fmt.Errorf("%w: missing part %s", ErrMalformed, name)
	}
	sheets, err := workbookSheets(zr, read)
	if err != nil {
		return "", 0, err
	}
	if len(sheets) > p.Limits.MaxSheets {
		return "", 0, fmt.Errorf("%w: sheets", ErrTooLarge)
	}
	shared := []string{}
	for _, f := range zr.File {
		if f.Name != "xl/sharedStrings.xml" {
			continue
		}
		raw, err := read(f.Name)
		if err != nil {
			return "", 0, err
		}
		shared, err = sharedStrings(raw, p.Limits)
		if err != nil {
			return "", 0, err
		}
	}
	ordered := sheets
	if sheetHint != "" {
		var hinted, rest []sheetRef
		for _, s := range sheets {
			if s.Name == sheetHint {
				hinted = append(hinted, s)
			} else {
				rest = append(rest, s)
			}
		}
		ordered = append(hinted, rest...)
	}
	var lastErr error
	for _, s := range ordered {
		raw, err := read(s.Target)
		if err != nil {
			lastErr = err
			continue
		}
		headers, hdrRow, herr := detectHeaders(raw, shared, p.Limits)
		if herr != nil {
			lastErr = herr
			continue
		}
		if err := streamRows(raw, shared, headers, hdrRow, p.Limits, yield); err != nil {
			return "", 0, err
		}
		return s.Name, hdrRow, nil
	}
	if lastErr != nil {
		return "", 0, lastErr
	}
	return "", 0, fmt.Errorf("%w: %q", ErrMissingSheet, sheetHint)
}

// detectHeaders scans the first rows for a signature row carrying every
// required column, returning canonical header names per column plus the
// source row number. Fixed offsets are never assumed (A07).
func detectHeaders(raw []byte, shared []string, lim Limits) ([]string, int, error) {
	var found []string
	foundRow := 0
	scanned := 0
	err := sheetCells(raw, shared, lim.MaxHeaderScanRows+5, func(num int, cells []cell) error {
		scanned++
		if scanned > lim.MaxHeaderScanRows {
			return errStopScan
		}
		byCol := map[int]string{}
		for _, c := range cells {
			byCol[c.col] = c.text
		}
		var cols []int
		for col := range byCol {
			cols = append(cols, col)
		}
		sort.Ints(cols)
		var headers []string
		for _, col := range cols {
			headers = append(headers, headerKey(byCol[col]))
		}
		need := map[string]bool{}
		for _, h := range headers {
			need[h] = true
		}
		complete := true
		for _, r := range requiredHeaders {
			if !need[r] {
				complete = false
				break
			}
		}
		if complete {
			found = headers
			foundRow = num
			return errStopScan
		}
		return nil
	})
	if err != nil && err != errStopScan {
		return nil, 0, err
	}
	if found == nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrMissingColumns, requiredHeaders)
	}
	return found, foundRow, nil
}

type stopScan struct{}

func (stopScan) Error() string { return "stop" }

var errStopScan = stopScan{}

// streamRows replays the sheet, skipping through the header row and mapping
// each data row by column position. Sparse cells stay empty strings;
// error cells keep a marker prefix so the import quarantines with cause
// instead of parsing "#N/A" as a value.
func streamRows(raw []byte, shared []string, headers []string, hdrRow int, lim Limits, yield func(Row) error) error {
	data := 0
	return sheetCells(raw, shared, lim.MaxRows+hdrRow+5, func(num int, cells []cell) error {
		if num <= hdrRow {
			return nil
		}
		byCol := map[int]cell{}
		for _, c := range cells {
			byCol[c.col] = c
		}
		out := Row{Number: num, Cells: map[string]string{}}
		empty := true
		for i, h := range headers {
			c, ok := byCol[i]
			if !ok {
				continue
			}
			text := c.text
			if c.isErr {
				text = "!<err>:" + text
			}
			if strings.TrimSpace(text) != "" {
				empty = false
			}
			out.Cells[h] = text
		}
		if empty {
			return nil
		}
		data++
		if data > lim.MaxRows {
			// A run ending in error is invalid: the caller must discard
			// every row yielded so far instead of publishing a prefix.
			return fmt.Errorf("%w: data rows", ErrTooLarge)
		}
		return yield(out)
	})
}
