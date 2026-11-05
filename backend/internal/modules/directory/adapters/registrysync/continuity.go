package registrysync

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
)

type EditionReport struct {
	Schema         string `json:"schema"`
	Rows           int    `json:"rows"`
	Added          int    `json:"added"`
	Changed        int    `json:"changed"`
	Missing        int    `json:"missing"`
	Unchanged      int    `json:"unchanged"`
	SourceSHA256   string `json:"source_sha256"`
	PreviousSHA256 string `json:"previous_sha256,omitempty"`
}

var fullRegistryCNPJ = regexp.MustCompile(`^[A-Z0-9]{12}[0-9]{2}$`)

// Read only bounded official identity/row hashes. Raw company fields never
// enter this report. Actual CNPJ check digits and source layout belong to Rust.
func registryMembership(path string) (map[string]string, error) {
	raw, err := smallFile(path, SourceCap)
	if err != nil {
		return nil, err
	}
	if !bytes.HasSuffix(raw, []byte{'\n'}) {
		return nil, fmt.Errorf("registry final record terminator missing")
	}
	// ANP contains literal quotes inside unquoted names. Rust's CSV reader
	// admits these; Go needs LazyQuotes. Still reject an unfinished quoted
	// field instead of silently accepting a truncated last record.
	fieldStart, quoted := true, false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if quoted {
			if c == '"' {
				if i+1 < len(raw) && raw[i+1] == '"' {
					i++
				} else if i+1 == len(raw) || raw[i+1] == ';' || raw[i+1] == '\n' || raw[i+1] == '\r' {
					quoted = false
				}
			}
		} else if fieldStart && c == '"' {
			quoted, fieldStart = true, false
		} else {
			fieldStart = c == ';' || c == '\n'
		}
	}
	if quoted {
		return nil, fmt.Errorf("registry quoted field incomplete")
	}
	r := csv.NewReader(bytes.NewReader(raw))
	r.Comma = ';'
	r.LazyQuotes = true
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("registry header unavailable")
	}
	column := -1
	for i, value := range header {
		if strings.TrimPrefix(strings.TrimSpace(value), "\ufeff") == "CNPJ" {
			if column >= 0 {
				return nil, fmt.Errorf("duplicate CNPJ column")
			}
			column = i
		}
	}
	if column < 0 {
		return nil, fmt.Errorf("registry CNPJ column missing")
	}
	rows := map[string]string{}
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("registry CSV incomplete")
		}
		if len(rows) >= 500000 {
			return nil, fmt.Errorf("registry row budget exceeded")
		}
		cnpj := strings.ToUpper(strings.NewReplacer(".", "", "/", "", "-", "", " ", "").Replace(row[column]))
		if !fullRegistryCNPJ.MatchString(cnpj) {
			return nil, fmt.Errorf("invalid registry establishment identity")
		}
		if _, ok := rows[cnpj]; ok {
			return nil, fmt.Errorf("duplicate registry establishment identity")
		}
		encoded, _ := json.Marshal(row)
		sum := sha256.Sum256(encoded)
		rows[cnpj] = hex.EncodeToString(sum[:])
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("empty registry edition refused")
	}
	return rows, nil
}

// Missing means absent from this comparable source edition, never closed.
func AnalyzeRegistryEdition(previous, current string) (EditionReport, error) {
	report := EditionReport{Schema: "abastevo-registry-edition-v1"}
	path := filepath.Join(current, "registry.csv")
	rows, err := registryMembership(path)
	if err != nil {
		return report, err
	}
	report.Rows = len(rows)
	report.SourceSHA256, err = fileHash(path, SourceCap)
	if err != nil {
		return report, err
	}
	before := map[string]string{}
	if previous != "" {
		path = filepath.Join(previous, "registry.csv")
		before, err = registryMembership(path)
		if err != nil {
			return report, err
		}
		report.PreviousSHA256, err = fileHash(path, SourceCap)
		if err != nil {
			return report, err
		}
	}
	for id, hash := range rows {
		old, ok := before[id]
		if !ok {
			report.Added++
		} else if old != hash {
			report.Changed++
		} else {
			report.Unchanged++
		}
	}
	for id := range before {
		if _, ok := rows[id]; !ok {
			report.Missing++
		}
	}
	if len(rows)*100 < len(before)*80 {
		return report, fmt.Errorf("registry edition dropped over 20 percent; review required")
	}
	raw, _ := json.Marshal(report)
	err = atomicFile(current, "edition-report.json", append(raw, '\n'))
	return report, err
}

// Keep the documented ANP admission threshold before any loader mutation.
func ValidateRegistryAdmission(dir string) error {
	raw, err := smallFile(filepath.Join(dir, "manifest.json"), 1<<20)
	if err != nil {
		return err
	}
	var manifest struct {
		Counts map[string]struct{ Input, Accepted, Duplicates, Quarantined int }
	}
	if json.Unmarshal(raw, &manifest) != nil {
		return fmt.Errorf("registry admission manifest invalid")
	}
	counts, ok := manifest.Counts["registry-13col"]
	if !ok || counts.Input <= 0 || counts.Input > 500000 || counts.Accepted <= 0 || counts.Accepted > counts.Input || counts.Duplicates < 0 || counts.Duplicates > counts.Input || counts.Quarantined < 0 || counts.Quarantined > counts.Input ||
		counts.Accepted+counts.Duplicates+counts.Quarantined != counts.Input || counts.Quarantined*100 > counts.Input {
		return fmt.Errorf("registry quarantine/completeness requires review")
	}
	return nil
}
