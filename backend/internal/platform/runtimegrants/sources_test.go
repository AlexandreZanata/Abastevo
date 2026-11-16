package runtimegrants

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestClosureFollowsProductionBuildOnly(t *testing.T) {
	root := writeModule(t, map[string]string{
		"go.mod":               "module example.test/m\n\ngo 1.27\n",
		"cmd/api/main.go":      "package main\n\nimport (\n\t_ \"fmt\"\n\t_ \"example.test/m/store\"\n)\n\nfunc main() {}\n",
		"cmd/ops/main.go":      "package main\n\nimport _ \"example.test/m/operator\"\n\nfunc main() {}\n",
		"store/store.go":       "package store\n\nimport _ \"example.test/m/shared\"\n\nconst q = `DELETE FROM sessions`\n",
		"store/store_test.go":  "package store\n\nconst testOnly = `DELETE FROM accounts`\n",
		"store/tagged.go":      "//go:build integration\n\npackage store\n\nconst tagged = `DELETE FROM organizations`\n",
		"store/os_windows.go":  "package store\n\nconst windows = `DELETE FROM payments`\n",
		"shared/shared.go":     "package shared\n",
		"operator/operator.go": "package operator\n\nconst op = `TRUNCATE everything`\n",
	})
	packages, err := Closure(ProductionContext(), root, "example.test/m", "cmd/api")
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, p := range packages {
		paths = append(paths, p.ImportPath)
		if p.ImportPath == "example.test/m/store" && (len(p.Files) != 1 || filepath.Base(p.Files[0]) != "store.go") {
			t.Fatal("test, tagged or foreign-OS files entered the production closure", p.Files)
		}
	}
	if !reflect.DeepEqual(paths, []string{"example.test/m/cmd/api", "example.test/m/shared", "example.test/m/store"}) {
		t.Fatal("closure", paths)
	}
	u := NewUsage()
	for _, p := range packages {
		if err := ScanPackage(u, p); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(keys(u.Delete), []string{"sessions"}) {
		t.Fatal("scanned SQL outside the production closure", keys(u.Delete))
	}
	if _, err := Closure(ProductionContext(), root, "example.test/m"); err == nil {
		t.Fatal("closure without a main package")
	}
	if _, err := Closure(ProductionContext(), root, "example.test/m", "cmd/missing"); err == nil {
		t.Fatal("missing main package accepted")
	}
}

func TestScanPackageFoldsConstantsAndConcatenations(t *testing.T) {
	root := writeModule(t, map[string]string{
		"go.mod": "module example.test/m\n\ngo 1.27\n",
		"store/a.go": "package store\n\nconst base = `SELECT id FROM org_assets WHERE org_id=$1`\n\n" +
			"func lock(extra string) string { return base + ` AND state=$2` + \" FOR UPDATE\" }\n" +
			"func partial(filter string) string { return \"UPDATE org_units \" + filter + \" SET name=$1\" }\n",
		"store/b.go": "package store\n\nconst (\n\tinsert = \"INSERT INTO \" + table\n\ttable  = `org_trips(id) VALUES($1)`\n)\n\n" +
			"var cycle = loopA\n\nconst loopA = loopB\nconst loopB = loopA\n",
	})
	pkg := Package{ImportPath: "example.test/m/store", Dir: filepath.Join(root, "store"), Files: []string{filepath.Join(root, "store", "a.go"), filepath.Join(root, "store", "b.go")}}
	u := NewUsage()
	if err := ScanPackage(u, pkg); err != nil {
		t.Fatal(err)
	}
	if !u.Update["org_assets"] || !u.Insert["org_trips"] || !u.Update["org_units"] {
		t.Fatal("constant folding lost a statement", keys(u.Update), keys(u.Insert))
	}
	embedded := writeModule(t, map[string]string{
		"go.mod":                 "module example.test/m\n\ngo 1.27\n",
		"cmd/api/main.go":        "package main\n\nimport (\n\t_ \"example.test/m/db/migrations\"\n\t_ \"example.test/m/reports\"\n)\n\nfunc main() {}\n",
		"db/migrations/embed.go": "package migrations\n\nimport \"embed\"\n\n//go:embed *.sql\nvar Files embed.FS\n",
		"db/migrations/0001.sql": "INSERT INTO owner_only(x) VALUES(1);\n",
		"reports/reports.go":     "package reports\n\nimport _ \"embed\"\n\n//go:embed report.sql\nvar Query string\n",
		"reports/report.sql":     "DELETE FROM sessions;\n",
	})
	closure, err := Closure(ProductionContext(), embedded, "example.test/m", "cmd/api")
	if err != nil {
		t.Fatal(err)
	}
	refused := 0
	for _, p := range closure {
		if err := ScanPackage(NewUsage(), p); err != nil {
			if p.ImportPath != "example.test/m/reports" {
				t.Fatal("migration chain refused", p.ImportPath, err)
			}
			refused++
		}
	}
	if refused != 1 {
		t.Fatal("embedded runtime SQL was not refused")
	}
	bad := Package{ImportPath: "example.test/m/bad", Files: []string{writeModule(t, map[string]string{"x.go": "package"}) + "/x.go"}}
	if err := ScanPackage(NewUsage(), bad); err == nil {
		t.Fatal("unparsable source accepted")
	}
}
