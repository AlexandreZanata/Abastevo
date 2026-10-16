// Package native owns bounded, file-backed official PMQC archive preparation.
// Network, Directory reads and publication remain explicit composition ports.
package native

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
)

const MaxArchiveBytes int64 = 100 << 20
const MaxCSVBytes uint64 = 30 << 20
const MaxEntries = 32
const resourcePrefix = "/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/"

var archiveName = regexp.MustCompile(`^pmqc_([0-9]{4})(?:_([0-9]{2}))?\.zip$`)
var memberName = regexp.MustCompile(`^(?:([0-9]{4})/)?PMQC_([0-9]{4})_([0-9]{2})\.(csv|json)$`)

func refused(code string) error { return errors.New("quality native: " + code) }

func archivePeriod(raw string) (year, month string, err error) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host != "www.gov.br" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.RawPath != "" || !strings.HasPrefix(u.Path, resourcePrefix) {
		return "", "", refused("resource URL")
	}
	name := strings.TrimPrefix(u.Path, resourcePrefix)
	parts := archiveName.FindStringSubmatch(name)
	if parts == nil || parts[1] == "0000" || (parts[2] != "" && (parts[2] < "01" || parts[2] > "12")) {
		return "", "", refused("resource period")
	}
	return parts[1], parts[2], nil
}

// ValidateURL is shared with the root's verified HTTPS fetcher allowlist.
func ValidateURL(raw string) error { _, _, e := archivePeriod(raw); return e }

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.r.Read(p)
}

func verify(ctx context.Context, f *os.File, id prepared.InputIdentity) error {
	if id.Bytes <= 0 || id.Bytes > MaxArchiveBytes || len(id.SHA256) != 64 || strings.ToLower(id.SHA256) != id.SHA256 {
		return refused("archive identity")
	}
	h := sha256.New()
	n, e := io.Copy(h, contextReader{ctx, io.NewSectionReader(f, 0, id.Bytes+1)})
	if e != nil {
		return refused("archive read")
	}
	if n != id.Bytes || hex.EncodeToString(h.Sum(nil)) != id.SHA256 {
		return refused("archive integrity")
	}
	return nil
}

// Check the bounded central directory before archive/zip allocates entry objects.
// ZIP64/multidisk/self-extracting or noncanonical trailing data is not needed for
// these small official archives and fails closed. The last EOCD must end the file.
func boundedDirectory(f *os.File, size int64) (int, error) {
	n := min(size, int64(65557))
	tail := make([]byte, n)
	if _, e := f.ReadAt(tail, size-n); e != nil {
		return 0, refused("ZIP tail")
	}
	for i := len(tail) - 22; i >= 0; i-- {
		if !bytes.Equal(tail[i:i+4], []byte{'P', 'K', 5, 6}) {
			continue
		}
		end := tail[i:]
		if len(end) != 22+int(binary.LittleEndian.Uint16(end[20:22])) {
			continue
		}
		count := int(binary.LittleEndian.Uint16(end[10:12]))
		directorySize := int64(binary.LittleEndian.Uint32(end[12:16]))
		offset := int64(binary.LittleEndian.Uint32(end[16:20]))
		eocd := size - n + int64(i)
		if binary.LittleEndian.Uint16(end[4:6]) != 0 || binary.LittleEndian.Uint16(end[6:8]) != 0 || int(binary.LittleEndian.Uint16(end[8:10])) != count || count < 1 || count > MaxEntries || directorySize > 65536 || directorySize <= 0 || offset <= 0 || offset+directorySize != eocd {
			return 0, refused("ZIP directory budget")
		}
		if i >= 20 && bytes.Equal(tail[i-20:i-16], []byte{'P', 'K', 6, 7}) {
			return 0, refused("ZIP64 unsupported")
		}
		return count, nil
	}
	return 0, refused("ZIP end record")
}

type archive struct {
	file    *os.File
	csv     map[string]*zip.File
	entries []application.InventoryEntry
}

func openArchive(ctx context.Context, filename string, id prepared.InputIdentity, resource string) (*archive, error) {
	year, month, e := archivePeriod(resource)
	if e != nil {
		return nil, e
	}
	info, e := os.Lstat(filename)
	if e != nil || !info.Mode().IsRegular() || info.Size() != id.Bytes {
		return nil, refused("archive file")
	}
	f, e := os.Open(filename)
	if e != nil {
		return nil, refused("archive open")
	}
	ok := false
	defer func() {
		if !ok {
			_ = f.Close()
		}
	}()
	actual, e := f.Stat()
	if e != nil || !os.SameFile(info, actual) || !actual.Mode().IsRegular() || actual.Size() != id.Bytes {
		return nil, refused("archive replacement")
	}
	if e = verify(ctx, f, id); e != nil {
		return nil, e
	}
	count, e := boundedDirectory(f, id.Bytes)
	if e != nil {
		return nil, e
	}
	z, e := zip.NewReader(f, id.Bytes)
	if e != nil || len(z.File) != count {
		return nil, refused("ZIP structure")
	}
	a := &archive{file: f, csv: map[string]*zip.File{}}
	seen := map[string]bool{}
	months := map[string]bool{}
	var total uint64
	for _, entry := range z.File {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		name := entry.Name
		parts := memberName.FindStringSubmatch(name)
		if len(name) > 256 || parts == nil || seen[name] || !entry.Mode().IsRegular() || entry.Flags&1 != 0 || (entry.Method != zip.Store && entry.Method != zip.Deflate) || parts[2] != year || (parts[1] != "" && parts[1] != year) || parts[3] < "01" || parts[3] > "12" || (month != "" && parts[3] != month) {
			return nil, refused("ZIP member shape")
		}
		seen[name] = true
		if entry.UncompressedSize64 > 1<<30 || total > 1<<30-entry.UncompressedSize64 {
			return nil, refused("ZIP aggregate budget")
		}
		total += entry.UncompressedSize64
		if entry.CompressedSize64 > uint64(id.Bytes) {
			return nil, refused("ZIP compressed budget")
		}
		if parts[4] != "csv" {
			continue
		}
		label := year + "-" + parts[3]
		if months[label] || entry.UncompressedSize64 == 0 || entry.UncompressedSize64 > MaxCSVBytes {
			return nil, refused("CSV member budget")
		}
		months[label] = true
		a.csv[name] = entry
		a.entries = append(a.entries, application.InventoryEntry{Label: label, Member: name, URL: resource, SHA256: id.SHA256, Bytes: uint64(id.Bytes)})
	}
	if len(a.entries) == 0 {
		return nil, refused("CSV member missing")
	}
	sort.Slice(a.entries, func(i, j int) bool { return a.entries[i].Label < a.entries[j].Label })
	ok = true
	return a, nil
}

// Inventory verifies the exact parent archive and returns each monthly CSV as a
// distinct edition. Unused JSON members are shape/budget checked, never imported.
func Inventory(ctx context.Context, filename string, id prepared.InputIdentity, resource string) ([]application.InventoryEntry, error) {
	a, e := openArchive(ctx, filename, id, resource)
	if e != nil {
		return nil, e
	}
	defer a.file.Close()
	return a.entries, nil
}

func (a *archive) extract(ctx context.Context, name, edition, destination string) (prepared.InputIdentity, error) {
	var id prepared.InputIdentity
	entry := a.csv[name]
	if entry == nil {
		return id, refused("selected member missing")
	}
	match := false
	for _, e := range a.entries {
		if e.Member == name && e.Label == edition {
			match = true
		}
	}
	if !match || path.Base(name) == "." {
		return id, refused("selected edition mismatch")
	}
	source, e := entry.Open()
	if e != nil {
		return id, refused("CSV open")
	}
	defer source.Close()
	out, e := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return id, refused("workspace CSV")
	}
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(out, h), contextReader{ctx, io.LimitReader(source, int64(entry.UncompressedSize64)+1)})
	closeErr := out.Close()
	// archive/zip verifies CRC when the selected stream reaches EOF. The +1 read
	// prevents an exact limit from masking a CRC or uncompressed-length failure.
	if e != nil || closeErr != nil || n != int64(entry.UncompressedSize64) || n > int64(MaxCSVBytes) {
		return id, refused("CSV integrity")
	}
	return prepared.InputIdentity{SHA256: hex.EncodeToString(h.Sum(nil)), Bytes: n}, nil
}
