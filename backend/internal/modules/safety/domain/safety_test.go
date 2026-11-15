package domain

import (
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	actor  = "11111111-1111-4111-8111-111111111111"
	author = "22222222-2222-4222-8222-222222222222"
	itemA  = "33333333-3333-4333-8333-333333333333"
	itemB  = "44444444-4444-4444-8444-444444444444"
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

// B-BR-SAFE-02: only the three user-attributed content kinds name a user.
func TestParseContentKind(t *testing.T) {
	for _, raw := range []string{"FORUM_POST", "FORUM_COMMENT", "STATION_COMMENT"} {
		kind, err := ParseContentKind(raw)
		if err != nil || string(kind) != raw {
			t.Fatalf("ParseContentKind(%q) = %q, %v", raw, kind, err)
		}
	}
	for _, raw := range []string{"", "forum_post", "COMMENT", "EDITORIAL", "ACCOUNT", " FORUM_POST"} {
		if _, err := ParseContentKind(raw); !errors.Is(err, ErrInvalid) {
			t.Fatalf("ParseContentKind(%q) err = %v, want ErrInvalid", raw, err)
		}
	}
}

func TestParseContentRef(t *testing.T) {
	ref, err := ParseContentRef("STATION_COMMENT", strings.ToUpper(itemA))
	if err != nil {
		t.Fatal(err)
	}
	if ref.Kind != KindStationComment || ref.ID != itemA {
		t.Fatalf("ref = %+v, want lower-case canonical id", ref)
	}
	for _, id := range []string{"", "not-a-uuid", itemA + "0", "33333333333343338333333333333333"} {
		if _, err := ParseContentRef("FORUM_COMMENT", id); !errors.Is(err, ErrInvalid) {
			t.Fatalf("ParseContentRef(id %q) err = %v, want ErrInvalid", id, err)
		}
	}
}

func TestParseID(t *testing.T) {
	if got, err := ParseID(strings.ToUpper(itemB)); err != nil || got != itemB {
		t.Fatalf("ParseID = %q, %v", got, err)
	}
	if _, err := ParseID("x"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ParseID(x) err = %v", err)
	}
}

// B-BR-SAFE-04: closed reason set.
func TestParseReportReason(t *testing.T) {
	for _, raw := range []string{"SPAM", "HARASSMENT", "HATE", "IMPERSONATION", "ILLEGAL", "OTHER"} {
		if r, err := ParseReportReason(raw); err != nil || string(r) != raw {
			t.Fatalf("ParseReportReason(%q) = %q, %v", raw, r, err)
		}
	}
	for _, raw := range []string{"", "spam", "OFF_TOPIC", "DEFAMATION", "VOTE_GAMING"} {
		if _, err := ParseReportReason(raw); !errors.Is(err, ErrInvalid) {
			t.Fatalf("ParseReportReason(%q) err = %v, want ErrInvalid", raw, err)
		}
	}
}

// B-BR-SAFE-04: detail is optional plain text, at most 200 scalars.
func TestNormalizeDetail(t *testing.T) {
	got, err := NormalizeDetail("  ok  \n")
	if err != nil || got != "ok" {
		t.Fatalf("NormalizeDetail trim = %q, %v", got, err)
	}
	if got, err := NormalizeDetail("   "); err != nil || got != "" {
		t.Fatalf("blank detail = %q, %v, want empty", got, err)
	}
	if _, err := NormalizeDetail(strings.Repeat("é", MaxDetailScalars)); err != nil {
		t.Fatalf("200 scalars refused: %v", err)
	}
	for name, raw := range map[string]string{
		"too long":    strings.Repeat("a", MaxDetailScalars+1),
		"nul":         "a\x00b",
		"bad utf8":    "a\xffb",
		"bidi format": "a\u202eb",
	} {
		if _, err := NormalizeDetail(raw); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

// B-BR-SAFE-02: the actor can never target their own account.
func TestCheckTarget(t *testing.T) {
	if err := CheckTarget(actor, Author{AccountID: author}); err != nil {
		t.Fatalf("foreign author refused: %v", err)
	}
	if err := CheckTarget(actor, Author{AccountID: actor}); !errors.Is(err, ErrSelfTarget) {
		t.Fatalf("self target err = %v, want ErrSelfTarget", err)
	}
	if err := CheckTarget(actor, Author{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("authorless content err = %v, want ErrNotFound", err)
	}
}

// B-BR-SAFE-05: the active cap refuses with 409, never as a rate limit.
func TestAdmitBlock(t *testing.T) {
	if err := AdmitBlock(MaxActiveBlocks - 1); err != nil {
		t.Fatalf("within cap refused: %v", err)
	}
	err := AdmitBlock(MaxActiveBlocks)
	if !errors.Is(err, ErrBlockLimit) || errors.Is(err, ErrRateLimited) {
		t.Fatalf("active cap err = %v, want ErrBlockLimit only", err)
	}
}

// B-BR-SAFE-04: 10 new user reports per rolling day.
func TestAdmitReport(t *testing.T) {
	if err := AdmitReport(ReportDailyQuota - 1); err != nil {
		t.Fatalf("within quota refused: %v", err)
	}
	var quota *QuotaError
	if err := AdmitReport(ReportDailyQuota); !errors.As(err, &quota) || quota.RetryAfterSeconds != 86400 {
		t.Fatalf("report quota err = %v, want QuotaError", err)
	}
	if !errors.Is(&QuotaError{RetryAfterSeconds: 1}, ErrRateLimited) {
		t.Fatal("QuotaError must match ErrRateLimited")
	}
}

// B-BR-SAFE-06: 1..100 refs, deduplicated, all valid or refused.
func TestParseHiddenQuery(t *testing.T) {
	refs, err := ParseHiddenQuery([]RawRef{
		{Kind: "FORUM_COMMENT", ID: itemA},
		{Kind: "FORUM_COMMENT", ID: strings.ToUpper(itemA)},
		{Kind: "STATION_COMMENT", ID: itemA},
		{Kind: "FORUM_POST", ID: itemB},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 3 {
		t.Fatalf("refs = %+v, want 3 after dedupe", refs)
	}
	if _, err := ParseHiddenQuery(nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty query err = %v", err)
	}
	tooMany := make([]RawRef, MaxHiddenRefs+1)
	for i := range tooMany {
		tooMany[i] = RawRef{Kind: "FORUM_COMMENT", ID: itemA}
	}
	if _, err := ParseHiddenQuery(tooMany); !errors.Is(err, ErrInvalid) {
		t.Fatalf("101 refs err = %v, want ErrInvalid", err)
	}
	if _, err := ParseHiddenQuery([]RawRef{{Kind: "FORUM_COMMENT", ID: itemA}, {Kind: "X", ID: itemB}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("one bad ref err = %v, want ErrInvalid", err)
	}
}

// B-BR-SAFE-06: the answer is the subset whose author is blocked; authors
// never leave the function.
func TestHiddenSubset(t *testing.T) {
	refs := []ContentRef{{Kind: KindForumComment, ID: itemA}, {Kind: KindForumComment, ID: itemB}, {Kind: KindStationComment, ID: itemA}}
	authors := map[ContentRef]string{
		{Kind: KindForumComment, ID: itemA}:   author,
		{Kind: KindForumComment, ID: itemB}:   actor,
		{Kind: KindStationComment, ID: itemA}: "55555555-5555-4555-8555-555555555555",
	}
	blocked := map[string]bool{author: true}
	got := HiddenSubset(refs, authors, blocked)
	if len(got) != 1 || got[0] != refs[0] {
		t.Fatalf("hidden = %+v, want only the blocked author's item", got)
	}
	if got := HiddenSubset(refs, authors, nil); len(got) != 0 {
		t.Fatalf("no blocks hidden = %+v", got)
	}
}

// B-BR-SAFE-07: labels are the public alias, bounded and single-line.
func TestBlockLabel(t *testing.T) {
	if got := BlockLabel("  k7q2-m9zx-42ab "); got != "k7q2-m9zx-42ab" {
		t.Fatalf("label = %q", got)
	}
	if got := BlockLabel(strings.Repeat("a", 100)); len([]rune(got)) != MaxLabelScalars {
		t.Fatalf("label scalars = %d, want %d", len([]rune(got)), MaxLabelScalars)
	}
	if got := BlockLabel("a\nb\x00c"); got != "a b c" {
		t.Fatalf("control chars label = %q", got)
	}
}

func TestCaseDetailHasNoFreeText(t *testing.T) {
	ref := ContentRef{Kind: KindForumComment, ID: itemA}
	got := CaseDetail(ref)
	if got != "community user report via FORUM_COMMENT "+itemA {
		t.Fatalf("case detail = %q", got)
	}
}
