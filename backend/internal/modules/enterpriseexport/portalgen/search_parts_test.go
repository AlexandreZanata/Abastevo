package portalgen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLargeRegionalSearchIsPartitionedInsteadOfExceedingFileBudget(t *testing.T) {
	rows := make([]searchStation, 12000)
	for i := range rows {
		rows[i] = searchStation{ID: fmt.Sprintf("00000000-0000-4000-8000-%012d", i), Name: fmt.Sprintf("[TEST] Station %08d", i), SearchName: fmt.Sprintf("test station %08d", i), LegalName: fmt.Sprintf("[TEST] Legal identity %08d", i), Municipality: "3550308", UF: "SP"}
	}
	raw, _ := json.Marshal(searchShard{Schema: SearchSchema, Release: "rel", UF: "SP", Count: len(rows), Stations: rows})
	g := generator{opts: Options{MaxBytes: 32 << 20}, outDir: t.TempDir(), files: map[string]FileDigest{}}
	if err := g.writeAsset("dados/busca/rel/SP.json", raw); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(g.outDir, "dados/busca/rel/SP.json"))
	var envelope struct {
		Schema string `json:"schema"`
		Count  int    `json:"count"`
		Parts  []struct {
			File string `json:"file"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Schema != "abastevo-search-parts-v1" || envelope.Count != len(rows) || len(envelope.Parts) < 2 {
		t.Fatal("large regional file was not partitioned", envelope.Schema, len(envelope.Parts))
	}
	for _, file := range g.files {
		if file.Bytes > 384<<10 {
			t.Fatal("decoded search file budget exceeded", file.Path, file.Bytes)
		}
	}
}

func TestSearchPartitionIntegrityAndCompatibility(t *testing.T) {
	raw, _ := json.Marshal(searchShard{Schema: SearchSchema, Release: "rel", UF: "SP", Count: 2, Stations: []searchStation{{ID: "one", Name: strings.Repeat("a", 200000)}, {ID: "two", Name: strings.Repeat("b", 200000)}}})
	parts, err := partitionSearchFile("dados/busca/rel/SP.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []string{"valid", "missing", "hash", "traversal", "duplicate", "count", "release", "orphan", "undeclared", "legacy-script"} {
		t.Run(tc, func(t *testing.T) {
			dir := t.TempDir()
			files := map[string]FileDigest{}
			for _, p := range parts {
				path := filepath.Join(dir, p.Path)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, p.Data, 0600); err != nil {
					t.Fatal(err)
				}
				files[p.Path] = FileDigest{Path: p.Path}
			}
			var env searchParts
			_ = json.Unmarshal(parts[0].Data, &env)
			switch tc {
			case "missing":
				_ = os.Remove(filepath.Join(dir, parts[1].Path))
			case "hash":
				_ = os.WriteFile(filepath.Join(dir, parts[1].Path), []byte("[]\n"), 0600)
			case "traversal":
				env.Parts[0].File = "../SP.json"
			case "duplicate":
				env.Parts = append(env.Parts, env.Parts[0])
			case "count":
				env.Count++
			case "release":
				env.Release = "other"
			case "orphan":
				files["dados/busca/rel/parts/ffffffffffffffff-000000.json"] = FileDigest{}
			case "undeclared":
				delete(files, parts[1].Path)
			case "legacy-script":
				g := generator{opts: Options{MaxBytes: 32 << 20, SearchScriptPath: "old.js"}, outDir: dir, files: map[string]FileDigest{}, scriptURL: "/old.js"}
				if g.writeAsset("dados/busca/rel/SP.json", raw) == nil {
					t.Fatal("old script accepted multipart data")
				}
				return
			}
			body, _ := json.Marshal(env)
			_ = os.WriteFile(filepath.Join(dir, parts[0].Path), body, 0600)
			err := checkSearchParts(dir, files, "rel")
			if tc != "valid" {
				if err == nil {
					t.Fatal("corruption accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := readSearchDocument(dir, parts[0].Path, files, nil)
			if err != nil {
				t.Fatal(err)
			}
			var wantValue, gotValue any
			_ = json.Unmarshal(raw, &wantValue)
			_ = json.Unmarshal(got, &gotValue)
			if !reflect.DeepEqual(wantValue, gotValue) {
				t.Fatal("partition lost rows or metadata")
			}
		})
	}
}
