package enterpriseexport

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
)

var ErrUnverifiedSnapshot = errors.New("export: snapshot integrity check failed")

const verifiedRecordLimit = 1 << 20

// ReadVerifiedCore checks the expected manifest digest and the exact bytes
// consumed by the typed official-only reader. It is an offline preparation
// entry and accepts no additional dataset sections.
func ReadVerifiedCore(ctx context.Context, dir, expected string, admit func(Manifest) error) (Manifest, Snapshot, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return Manifest{}, Snapshot{}, ErrUnverifiedSnapshot
	}
	defer root.Close()

	manifest, err := openCoreFile(root, "manifest.json")
	if err != nil {
		return Manifest{}, Snapshot{}, ErrUnverifiedSnapshot
	}
	raw, err := io.ReadAll(io.LimitReader(manifest, verifiedRecordLimit+1))
	_ = manifest.Close()
	if err != nil || len(raw) > verifiedRecordLimit {
		return Manifest{}, Snapshot{}, ErrUnverifiedSnapshot
	}
	var m Manifest
	if strictRecord(raw, &m) != nil || m.ContentSHA256 != expected || m.Sources.Forum != nil ||
		m.Counts.ForumPosts != 0 || len(m.Files) != 3 || admit == nil {
		return Manifest{}, Snapshot{}, ErrUnverifiedSnapshot
	}
	actual, err := contentDigest(m)
	if err != nil || actual != expected {
		return Manifest{}, Snapshot{}, ErrUnverifiedSnapshot
	}
	if err := admit(m); err != nil {
		return Manifest{}, Snapshot{}, err
	}
	files := make(map[string]FileDigest, 3)
	for _, f := range m.Files {
		files[f.Name] = f
	}
	var s Snapshot
	if s.Stations, err = verifiedRows[Station](ctx, root, "stations.jsonl", files["stations.jsonl"], m.Counts.Stations, defaultMaxStations); err != nil {
		return Manifest{}, Snapshot{}, err
	}
	if s.Revisions, err = verifiedRows[Revision](ctx, root, "revisions.jsonl", files["revisions.jsonl"], m.Counts.Revisions, defaultMaxRevisions); err != nil {
		return Manifest{}, Snapshot{}, err
	}
	if s.Prices, err = verifiedRows[Price](ctx, root, "prices.jsonl", files["prices.jsonl"], m.Counts.Prices, defaultMaxPrices); err != nil {
		return Manifest{}, Snapshot{}, err
	}
	return m, s, nil
}

func verifiedRows[T any](ctx context.Context, root *os.Root, name string, f FileDigest, count, maximum int) ([]T, error) {
	if f.Name != name || f.Bytes < 0 || f.Bytes == math.MaxInt64 || count < 0 || count > maximum || f.Rows != count {
		return nil, ErrUnverifiedSnapshot
	}
	file, err := openCoreFile(root, name)
	if err != nil {
		return nil, ErrUnverifiedSnapshot
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != f.Bytes {
		return nil, ErrUnverifiedSnapshot
	}
	raw := &io.LimitedReader{R: file, N: f.Bytes + 1}
	sum := sha256.New()
	lines := bufio.NewScanner(io.TeeReader(raw, sum))
	lines.Buffer(make([]byte, 16<<10), verifiedRecordLimit)
	var rows []T
	for lines.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var row T
		if len(rows) == count || strictRecord(lines.Bytes(), &row) != nil {
			return nil, ErrUnverifiedSnapshot
		}
		rows = append(rows, row)
	}
	if lines.Err() != nil || raw.N != 1 || len(rows) != count ||
		"sha256:"+hex.EncodeToString(sum.Sum(nil)) != f.SHA256 {
		return nil, ErrUnverifiedSnapshot
	}
	return rows, nil
}

func openCoreFile(root *os.Root, name string) (*os.File, error) {
	entry, err := root.Lstat(name)
	if err != nil || !entry.Mode().IsRegular() {
		return nil, ErrUnverifiedSnapshot
	}
	return root.Open(name)
}

func strictRecord(raw []byte, dst any) error {
	fields := bytes.TrimSpace(raw)
	if len(fields) == 0 || fields[0] != '{' {
		return ErrUnverifiedSnapshot
	}
	decoder := json.NewDecoder(bytes.NewReader(fields))
	decoder.DisallowUnknownFields()
	if decoder.Decode(dst) != nil || decoder.Decode(new(any)) != io.EOF {
		return ErrUnverifiedSnapshot
	}
	return nil
}
