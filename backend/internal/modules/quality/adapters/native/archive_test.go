package native

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
)

const annualURL = "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc_2025.zip"

func archiveFixture(t *testing.T, names ...string) (string, prepared.InputIdentity) {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, name := range names {
		f, e := w.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write([]byte("synthetic CSV content")); e != nil {
			t.Fatal(e)
		}
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "archive.zip")
	if e := os.WriteFile(p, b.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(b.Bytes())
	return p, prepared.InputIdentity{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(b.Len())}
}
func TestInventoryKeepsMonthlyMembers(t *testing.T) {
	p, id := archiveFixture(t, "2025/PMQC_2025_01.csv", "2025/PMQC_2025_01.json", "2025/PMQC_2025_02.csv")
	entries, e := Inventory(context.Background(), p, id, annualURL)
	if e != nil || len(entries) != 2 || entries[0].Label != "2025-01" || entries[1].Label != "2025-02" || entries[0].Member == entries[1].Member || entries[0].SHA256 != id.SHA256 {
		t.Fatal(entries, e)
	}
}
func TestArchiveRefusals(t *testing.T) {
	for _, names := range [][]string{{"../PMQC_2025_01.csv"}, {"/PMQC_2025_01.csv"}, {"2024/PMQC_2025_01.csv"}, {"PMQC_2025_13.csv"}, {"PMQC_2024_01.csv"}, {"PMQC_2025_01.exe"}, {"PMQC_2025_01.csv", "PMQC_2025_01.csv"}, {"PMQC_2025_01.json"}} {
		t.Run(strings.Join(names, "_"), func(t *testing.T) {
			p, id := archiveFixture(t, names...)
			if _, e := Inventory(context.Background(), p, id, annualURL); e == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
	p, id := archiveFixture(t, "PMQC_2025_01.csv")
	id.SHA256 = strings.Repeat("a", 64)
	if _, e := Inventory(context.Background(), p, id, annualURL); e == nil {
		t.Fatal("changed parent accepted")
	}
}
func TestOfficialResourceAllowlist(t *testing.T) {
	for _, u := range []string{annualURL, strings.Replace(annualURL, "2025.zip", "2026_09.zip", 1)} {
		if _, _, e := archivePeriod(u); e != nil {
			t.Fatal(e)
		}
	}
	for _, u := range []string{strings.Replace(annualURL, "https:", "http:", 1), annualURL + "?token=private", strings.Replace(annualURL, "www.gov.br", "example.invalid", 1), strings.Replace(annualURL, "www.gov.br", "user:pass@www.gov.br", 1), annualURL + "#fragment", strings.Replace(annualURL, "2025.zip", "2025_13.zip", 1), strings.Replace(annualURL, "www.gov.br", "www.gov.br:443", 1)} {
		if _, _, e := archivePeriod(u); e == nil {
			t.Fatal("noncanonical URL accepted")
		}
	}
}

func rewriteArchive(t *testing.T, p string, mutate func([]byte)) prepared.InputIdentity {
	t.Helper()
	raw, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	mutate(raw)
	if e = os.WriteFile(p, raw, 0600); e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	return prepared.InputIdentity{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(raw))}
}
func TestArchiveBudgetsBeforeAllocation(t *testing.T) {
	for _, mutate := range []func([]byte){
		func(raw []byte) {
			i := bytes.LastIndex(raw, []byte{'P', 'K', 5, 6})
			binary.LittleEndian.PutUint16(raw[i+8:i+10], 65535)
			binary.LittleEndian.PutUint16(raw[i+10:i+12], 65535)
		},
		func(raw []byte) {
			i := bytes.Index(raw, []byte{'P', 'K', 1, 2})
			binary.LittleEndian.PutUint32(raw[i+24:i+28], uint32(MaxCSVBytes+1))
		},
		func(raw []byte) {
			i := bytes.Index(raw, []byte{'P', 'K', 1, 2})
			binary.LittleEndian.PutUint16(raw[i+8:i+10], 1)
		},
		func(raw []byte) {
			i := bytes.LastIndex(raw, []byte{'P', 'K', 5, 6})
			binary.LittleEndian.PutUint32(raw[i+12:i+16], 65537)
		},
	} {
		p, _ := archiveFixture(t, "PMQC_2025_01.csv")
		id := rewriteArchive(t, p, mutate)
		if _, e := Inventory(context.Background(), p, id, annualURL); e == nil {
			t.Fatal("hostile ZIP budget accepted")
		}
	}
	names := make([]string, MaxEntries+1)
	for i := range names {
		names[i] = fmt.Sprintf("invalid%d.csv", i)
	}
	p, id := archiveFixture(t, names...)
	if _, e := Inventory(context.Background(), p, id, annualURL); e == nil {
		t.Fatal("entry budget accepted")
	}
}
func TestSelectedCRCTruncationAndCancellation(t *testing.T) {
	p, _ := archiveFixture(t, "PMQC_2025_01.csv")
	id := rewriteArchive(t, p, func(raw []byte) { i := bytes.Index(raw, []byte{'P', 'K', 1, 2}); raw[i+16] ^= 1 })
	a, e := openArchive(context.Background(), p, id, annualURL)
	if e != nil {
		t.Fatal(e)
	}
	defer a.file.Close()
	if _, e = a.extract(context.Background(), "PMQC_2025_01.csv", "2025-01", filepath.Join(t.TempDir(), "input.csv")); e == nil {
		t.Fatal("bad CSV CRC accepted")
	}
	raw, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p, raw[:len(raw)-5], 0600); e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw[:len(raw)-5])
	id = prepared.InputIdentity{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(raw) - 5)}
	if _, e = Inventory(context.Background(), p, id, annualURL); e == nil {
		t.Fatal("truncated archive accepted")
	}
	p, id = archiveFixture(t, "PMQC_2025_01.csv")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = Inventory(ctx, p, id, annualURL); e == nil {
		t.Fatal("cancelled scan continued")
	}
}
func TestArchiveSymlinkAndDuplicateMonthlyScope(t *testing.T) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: "PMQC_2025_01.csv", Method: zip.Store}
	h.SetMode(os.ModeSymlink | 0777)
	f, e := w.CreateHeader(h)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.Write([]byte("/private")); e != nil {
		t.Fatal(e)
	}
	if e = w.Close(); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "archive.zip")
	if e = os.WriteFile(p, b.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(b.Bytes())
	id := prepared.InputIdentity{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(b.Len())}
	if _, e = Inventory(context.Background(), p, id, annualURL); e == nil {
		t.Fatal("symlink member accepted")
	}
	p, id = archiveFixture(t, "PMQC_2025_01.csv", "2025/PMQC_2025_01.csv")
	if _, e = Inventory(context.Background(), p, id, annualURL); e == nil {
		t.Fatal("two CSVs for monthly edition accepted")
	}
}
