package domain

import (
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
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

func validParams() CaseParams {
	return CaseParams{
		ID:         "c0000000-0000-4000-8000-000000000001",
		TargetType: TargetObservation,
		TargetID:   "b0000000-0000-4000-8000-000000000001",
		Priority:   PriorityP2,
		Reason:     "repeated conflicting reports",
		OpenedAt:   time.Now(),
	}
}

func TestNewCaseValid(t *testing.T) {
	c, evt, err := NewCase(validParams())
	if err != nil {
		t.Fatalf("valid case rejected: %v", err)
	}
	if c.Status != StatusOpen || c.PolicyVersion != PolicyV1 {
		t.Errorf("case = %+v", c)
	}
	if evt.CaseID != c.ID || evt.Name() != "ModerationCaseOpened" {
		t.Errorf("event = %+v", evt)
	}
	if c.Reason != "repeated conflicting reports" {
		t.Errorf("reason mutated: %q", c.Reason)
	}
}

func TestNewCaseTargets(t *testing.T) {
	for _, target := range []string{TargetObservation, TargetDispute, TargetEvidence, TargetComment} {
		p := validParams()
		p.TargetType = target
		if _, _, err := NewCase(p); err != nil {
			t.Errorf("%s rejected: %v", target, err)
		}
	}
	if DefaultPriority(TargetComment) != PriorityP2 {
		t.Errorf("comment default = %q, want P2 like facts", DefaultPriority(TargetComment))
	}
	// Contributor targets accept opaque attribution tokens, not UUIDs.
	p := validParams()
	p.TargetType = TargetContributor
	p.TargetID = "tok-abc123"
	if _, _, err := NewCase(p); err != nil {
		t.Errorf("contributor token rejected: %v", err)
	}
	p.TargetType = "STATION"
	if _, _, err := NewCase(p); !errors.Is(err, ErrUnknownTarget) {
		t.Errorf("unknown target = %v", err)
	}
	p = validParams()
	p.TargetID = "not-a-uuid"
	if _, _, err := NewCase(p); !errors.Is(err, ErrInvalidCase) {
		t.Errorf("non-uuid observation target = %v", err)
	}
}

func TestNewCasePriorityAndReason(t *testing.T) {
	p := validParams()
	p.Priority = "P0"
	if _, _, err := NewCase(p); !errors.Is(err, ErrUnknownPriority) {
		t.Errorf("bad priority = %v", err)
	}
	p = validParams()
	p.Reason = "   "
	if _, _, err := NewCase(p); !errors.Is(err, ErrBadReason) {
		t.Errorf("empty reason = %v", err)
	}
	p = validParams()
	p.Reason = strings.Repeat("r", MaxReasonChars+1)
	if _, _, err := NewCase(p); !errors.Is(err, ErrBadReason) {
		t.Errorf("long reason = %v", err)
	}
	p = validParams()
	p.Detail = strings.Repeat("d", MaxDetailChars+1)
	if _, _, err := NewCase(p); !errors.Is(err, ErrInvalidCase) {
		t.Errorf("long detail = %v", err)
	}
	p = validParams()
	p.EvidenceID = "not-a-uuid"
	if _, _, err := NewCase(p); !errors.Is(err, ErrInvalidCase) {
		t.Errorf("bad evidence ref = %v", err)
	}
	p = validParams()
	p.EvidenceID = "e0000000-0000-4000-8000-000000000001"
	if _, _, err := NewCase(p); err != nil {
		t.Errorf("uuid evidence ref rejected: %v", err)
	}
	p = validParams()
	p.OpenedAt = time.Time{}
	if _, _, err := NewCase(p); !errors.Is(err, ErrInvalidCase) {
		t.Errorf("zero time = %v", err)
	}
}

func TestPriorityOrdering(t *testing.T) {
	if !(PriorityRank(PriorityP1) < PriorityRank(PriorityP2) && PriorityRank(PriorityP2) < PriorityRank(PriorityP3)) {
		t.Errorf("priority ranks out of order: %d %d %d",
			PriorityRank(PriorityP1), PriorityRank(PriorityP2), PriorityRank(PriorityP3))
	}
	if DefaultPriority(TargetContributor) != PriorityP1 {
		t.Errorf("contributor default = %q", DefaultPriority(TargetContributor))
	}
	if DefaultPriority(TargetObservation) != PriorityP2 {
		t.Errorf("observation default = %q", DefaultPriority(TargetObservation))
	}
}

func TestCaseCarriesNoSensitivePayload(t *testing.T) {
	c, _, err := NewCase(validParams())
	if err != nil {
		t.Fatal(err)
	}
	// The case struct must stay identifier-only: no coordinates, accuracy,
	// IP, media bytes or URLs. A field addition here fails loudly so the
	// privacy bound (B-BR-011) cannot drift silently.
	allowed := map[string]bool{
		"ID": true, "TargetType": true, "TargetID": true, "Status": true,
		"Priority": true, "Reason": true, "Detail": true, "EvidenceID": true,
		"OpenedAt": true, "PolicyVersion": true,
	}
	v := reflect.ValueOf(c)
	typ := v.Type()
	for i := 0; i < typ.NumField(); i++ {
		if !allowed[typ.Field(i).Name] {
			t.Errorf("unexpected Case field %q leaks beyond the identifier-only bound", typ.Field(i).Name)
		}
	}
	if c.EvidenceID != "" {
		t.Errorf("valid case should carry no evidence ref by default")
	}
}
