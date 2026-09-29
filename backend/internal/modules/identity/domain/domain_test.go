package domain

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDomainStdlibOnly(t *testing.T) {
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(path, ".") {
				t.Errorf("%s imports non-stdlib %q", name, path)
			}
		}
	}
}

func TestParsePurpose(t *testing.T) {
	if _, err := ParsePurpose("REGISTER"); err != nil {
		t.Errorf("register: %v", err)
	}
	for _, s := range []string{"REGISTER", "sign", "register "} {
		if _, err := ParsePurpose(s); err != nil {
			t.Errorf("purpose %q rejected: %v", s, err)
		}
	}
	for _, s := range []string{"", "LOGIN", "REG ISTER", "delete"} {
		if _, err := ParsePurpose(s); err == nil {
			t.Errorf("purpose %q accepted", s)
		}
	}
}

func TestParseFingerprint(t *testing.T) {
	good := "fp:" + strings.Repeat("a", 64)
	if _, err := ParseFingerprint(good); err != nil {
		t.Errorf("good fingerprint: %v", err)
	}
	for _, s := range []string{"", "fp:", "fp:" + strings.Repeat("a", 63), "fp:" + strings.Repeat("a", 65), "FP:" + strings.Repeat("a", 64), "fp:" + strings.Repeat("g", 64), "fp:" + strings.Repeat("A", 64)} {
		if _, err := ParseFingerprint(s); err == nil {
			t.Errorf("fingerprint %q accepted", s)
		}
	}
}

func TestSplitNonce(t *testing.T) {
	id, cn, err := SplitNonce("ch-1.client-9")
	if err != nil || id != "ch-1" || cn != "client-9" {
		t.Errorf("split = %q %q %v", id, cn, err)
	}
	for _, s := range []string{"", "nondo", ".x", "x.", "a b.c", "a.b c"} {
		if _, _, err := SplitNonce(s); err == nil {
			t.Errorf("nonce %q accepted", s)
		}
	}
}

func TestChallengeValidate(t *testing.T) {
	now := time.Now()
	fp := "fp:" + strings.Repeat("b", 64)
	good := Challenge{ID: "ch-1", Nonce: "ch-1.cn", Fingerprint: fp, Purpose: PurposeRegister, ExpiresAt: now.Add(time.Minute)}
	if err := good.Validate(now); err != nil {
		t.Errorf("valid challenge: %v", err)
	}
	expired := good
	expired.ExpiresAt = now.Add(-time.Second)
	if err := expired.Validate(now); err == nil {
		t.Error("expired challenge accepted")
	}
	wrongPurpose := good
	wrongPurpose.Purpose = "LOGIN"
	if err := wrongPurpose.Validate(now); err == nil {
		t.Error("bad purpose accepted")
	}
}
