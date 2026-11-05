package portalgen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const SearchPartsSchema = "abastevo-search-parts-v1"
const maxSearchPartBytes = 384 << 10
const maxSearchDocumentBytes = 64 << 20
const maxSearchPartCount = 4096
const maxSearchDocumentRows = 100000

var searchPartName = regexp.MustCompile(`^parts/[a-f0-9]{16}-[0-9]{6}\.json$`)
var searchPartPath = regexp.MustCompile(`^dados/busca/[A-Za-z0-9_-]{1,64}/(?:(?:p|m|precos)/)?parts/[a-f0-9]{16}-[0-9]{6}\.json$`)
var searchArrays = map[string]string{SearchSchema: "stations", SearchPrefixSchema: "stations", SearchPricesSchema: "rows", SearchFiscalSchema: "records", SearchIndexSchema: "municipalities"}

type searchPart struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Rows   int    `json:"rows"`
}
type searchParts struct {
	Schema  string                     `json:"schema"`
	Release string                     `json:"release"`
	Array   string                     `json:"array"`
	Count   int                        `json:"count"`
	Header  map[string]json.RawMessage `json:"header"`
	Parts   []searchPart               `json:"parts"`
}

// Bound decoded bytes as well as the existing compressed-file ceiling. Stable
// ordering and hashes make a partitioned document immutable and replayable.
func partitionSearchFile(rel string, raw []byte) ([]indexFile, error) {
	if len(raw) <= maxSearchPartBytes {
		return []indexFile{{Path: rel, Data: raw}}, nil
	}
	var header map[string]json.RawMessage
	if err := json.Unmarshal(raw, &header); err != nil {
		return nil, err
	}
	var schema, release string
	_ = json.Unmarshal(header["schema"], &schema)
	_ = json.Unmarshal(header["release"], &release)
	field := searchArrays[schema]
	if field == "" {
		return nil, fmt.Errorf("portalgen: unknown search family")
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(header[field], &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 || len(rows) > maxSearchDocumentRows || len(raw) > maxSearchDocumentBytes {
		return nil, &BudgetError{Msg: "portalgen: logical search document budget exceeded"}
	}
	delete(header, field)
	envelope := searchParts{Schema: SearchPartsSchema, Release: release, Array: field, Count: len(rows), Header: header}
	sum := sha256.Sum256([]byte(rel))
	key := hex.EncodeToString(sum[:8])
	files := []indexFile{}
	for start := 0; start < len(rows); {
		end, size := start, 3
		for end < len(rows) && size+len(rows[end])+1 <= maxSearchPartBytes {
			size += len(rows[end]) + 1
			end++
		}
		if end == start || len(files) >= maxSearchPartCount {
			return nil, &BudgetError{Msg: "portalgen: search partition budget exceeded"}
		}
		body, err := json.Marshal(rows[start:end])
		if err != nil {
			return nil, err
		}
		body = append(body, '\n')
		name := fmt.Sprintf("parts/%s-%06d.json", key, len(files))
		hash := sha256.Sum256(body)
		envelope.Parts = append(envelope.Parts, searchPart{File: name, SHA256: hex.EncodeToString(hash[:]), Rows: end - start})
		files = append(files, indexFile{Path: filepath.ToSlash(filepath.Join(filepath.Dir(rel), name)), Data: body})
		start = end
	}
	root, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	root = append(root, '\n')
	if len(root) > maxSearchPartBytes {
		return nil, &BudgetError{Msg: "portalgen: search partition manifest budget exceeded"}
	}
	return append([]indexFile{{Path: rel, Data: root}}, files...), nil
}

// Reconstruct only a bounded, hash-bound public search document. Callers still
// apply the original family's strict field/type/privacy validation afterward.
func readSearchDocument(dir, rel string, files map[string]FileDigest, referenced map[string]bool) ([]byte, error) {
	raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	var identity struct {
		Schema string `json:"schema"`
	}
	if json.Unmarshal(raw, &identity) != nil || identity.Schema != SearchPartsSchema {
		return raw, nil
	}
	if len(raw) > maxSearchPartBytes {
		return nil, fmt.Errorf("portalcheck: oversized partition manifest")
	}
	var env searchParts
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&env) != nil {
		return nil, fmt.Errorf("portalcheck: invalid search partition manifest")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("portalcheck: trailing partition manifest")
	}
	var schema, release string
	_ = json.Unmarshal(env.Header["schema"], &schema)
	_ = json.Unmarshal(env.Header["release"], &release)
	segments := strings.Split(rel, "/")
	if len(segments) < 4 || env.Release != segments[2] || release != env.Release || searchArrays[schema] != env.Array || env.Array == "" || env.Count < 1 || env.Count > maxSearchDocumentRows || len(env.Parts) < 1 || len(env.Parts) > maxSearchPartCount || env.Header[env.Array] != nil {
		return nil, fmt.Errorf("portalcheck: search partition identity mismatch")
	}
	if value, ok := env.Header["count"]; ok {
		var count int
		if json.Unmarshal(value, &count) != nil || count != env.Count {
			return nil, fmt.Errorf("portalcheck: partition count mismatch")
		}
	}
	rows := []json.RawMessage{}
	seen := map[string]bool{}
	total := 0
	for _, part := range env.Parts {
		if !searchPartName.MatchString(part.File) || seen[part.File] || part.Rows < 1 || part.Rows > env.Count {
			return nil, fmt.Errorf("portalcheck: unsafe search partition")
		}
		seen[part.File] = true
		path := filepath.ToSlash(filepath.Join(filepath.Dir(rel), part.File))
		if files != nil {
			if _, ok := files[path]; !ok {
				return nil, fmt.Errorf("portalcheck: undeclared search partition")
			}
		}
		info, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxSearchPartBytes {
			return nil, fmt.Errorf("portalcheck: missing/oversized search partition")
		}
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			return nil, err
		}
		hash := sha256.Sum256(body)
		if part.SHA256 != hex.EncodeToString(hash[:]) {
			return nil, fmt.Errorf("portalcheck: search partition checksum mismatch")
		}
		total += len(body)
		if total > maxSearchDocumentBytes {
			return nil, fmt.Errorf("portalcheck: logical search budget exceeded")
		}
		var chunk []json.RawMessage
		if json.Unmarshal(body, &chunk) != nil || len(chunk) != part.Rows || len(rows)+len(chunk) > env.Count {
			return nil, fmt.Errorf("portalcheck: invalid partition rows")
		}
		rows = append(rows, chunk...)
		if referenced != nil {
			if referenced[path] {
				return nil, fmt.Errorf("portalcheck: reused search partition")
			}
			referenced[path] = true
		}
	}
	if len(rows) != env.Count {
		return nil, fmt.Errorf("portalcheck: incomplete search partitions")
	}
	body, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	env.Header[env.Array] = body
	return json.Marshal(env.Header)
}

func checkSearchParts(dir string, files map[string]FileDigest, release string) error {
	referenced := map[string]bool{}
	prefix := "dados/busca/" + release + "/"
	for rel := range files {
		if strings.HasPrefix(rel, prefix) && strings.HasSuffix(rel, ".json") && !searchPartPath.MatchString(rel) {
			if _, err := readSearchDocument(dir, rel, files, referenced); err != nil {
				return err
			}
		}
	}
	for rel := range files {
		if strings.HasPrefix(rel, prefix) && strings.Contains(rel, "/parts/") && !referenced[rel] {
			return fmt.Errorf("portalcheck: unreferenced search partition")
		}
	}
	return nil
}
