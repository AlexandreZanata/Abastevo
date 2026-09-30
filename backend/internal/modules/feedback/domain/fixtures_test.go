package domain

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type feedbackFixture struct {
	ID         string         `json:"id"`
	Version    int            `json:"version"`
	Provenance string         `json:"provenance"`
	Attack     string         `json:"attack"`
	Verdict    string         `json:"verdict"`
	Code       string         `json:"code"`
	Inputs     map[string]any `json:"inputs"`
	Note       string         `json:"note"`
}

func feedbackFixturesDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "contracts", "testdata", "feedback")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		t.Fatalf("fixtures dir missing: %s", dir)
	}
	return dir
}

// TestFeedbackAttackFixtures loads every feedback-*.json vector and
// enforces the frozen shape, then replays the executable cases (text,
// rating, agreement) against this package: fixtures and code must
// agree, never drift.
func TestFeedbackAttackFixtures(t *testing.T) {
	dir := feedbackFixturesDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "feedback-") && strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	if len(files) < 10 {
		t.Fatalf("want at least 10 feedback fixtures, got %d", len(files))
	}
	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			var f feedbackFixture
			if err := json.Unmarshal(raw, &f); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if f.ID == "" || f.Version != 1 || f.Provenance == "" {
				t.Errorf("fixture needs id/version=1/provenance: %+v", f)
			}
			if strings.TrimSuffix(name, ".json") != f.ID {
				t.Errorf("filename %q must match id %q", name, f.ID)
			}
			if f.Attack == "" || f.Note == "" || f.Code == "" {
				t.Errorf("fixture needs attack/note/code: %+v", f)
			}
			if f.Verdict != "accept" && f.Verdict != "reject" {
				t.Errorf("verdict %q must be accept or reject", f.Verdict)
			}
			if f.Verdict == "accept" && f.Code != "ok" {
				t.Errorf("accept verdict must carry code ok, got %q", f.Code)
			}
			replayFeedbackFixture(t, f)
		})
	}
}

func replayFeedbackFixture(t *testing.T, f feedbackFixture) {
	t.Helper()
	str := func(key string) string {
		s, _ := f.Inputs[key].(string)
		return s
	}
	num := func(key string) int64 {
		switch v := f.Inputs[key].(type) {
		case float64:
			return int64(v)
		default:
			return -1
		}
	}
	switch {
	case strings.HasPrefix(f.ID, "feedback-text-"):
		var got CommentText
		var err error
		if raw, ok := f.Inputs["raw_hex"].(string); ok {
			bytes, derr := hex.DecodeString(raw)
			if derr != nil {
				t.Fatalf("fixture raw_hex must decode: %v", derr)
			}
			got, err = ParseComment(string(bytes))
		} else if n := num("scalars"); f.ID == "feedback-text-281" {
			got, err = ParseComment(strings.Repeat("x", int(n)))
		} else {
			got, err = ParseComment(str("text"))
		}
		checkFeedbackVerdict(t, f, err)
		if err == nil {
			if want := str("normalized"); want != "" && got.Text != want {
				t.Errorf("normalized = %q, want %q", got.Text, want)
			}
			if n := num("scalars"); n >= 0 && int64(got.Scalars) != n {
				t.Errorf("scalars = %d, want %d", got.Scalars, n)
			}
		}
		if want := VerdictCode(err); f.Verdict == "reject" && want != f.Code {
			t.Errorf("code = %q, want %q", want, f.Code)
		}
	case strings.HasPrefix(f.ID, "feedback-rating-"):
		err := Rating(num("stars")).Validate()
		checkFeedbackVerdict(t, f, err)
		if want := VerdictCode(err); f.Verdict == "reject" && want != f.Code {
			t.Errorf("code = %q, want %q", want, f.Code)
		}
	case strings.HasPrefix(f.ID, "feedback-agreement-"):
		got, err := ComputeAgreement(num("valid"), num("invalid"))
		checkFeedbackVerdict(t, f, err)
		if err != nil {
			t.Fatalf("agreement must compute: %v", err)
		}
		if want, ok := f.Inputs["basis_points"]; ok {
			if want == nil {
				if got.HasVotes {
					t.Errorf("zero votes must be null, got %+v", got)
				}
			} else if got.BasisPoints != int64(want.(float64)) || !got.HasVotes {
				t.Errorf("basis points = %+v, want %v", got, want)
			}
		}
	case strings.HasPrefix(f.ID, "feedback-revision-"):
		if f.Verdict != "accept" {
			t.Errorf("revision vectors document accepted rules, got %q", f.Verdict)
		}
	default:
		t.Errorf("fixture family without replay: %q", f.ID)
	}
}

func checkFeedbackVerdict(t *testing.T, f feedbackFixture, err error) {
	t.Helper()
	if f.Verdict == "accept" && err != nil {
		t.Errorf("accept vector rejected: %v", err)
	}
	if f.Verdict == "reject" && err == nil {
		t.Error("reject vector accepted")
	}
}
