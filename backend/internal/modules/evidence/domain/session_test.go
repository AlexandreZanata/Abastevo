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

func validParams() Params {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	return Params{
		ID: "e0000000-0000-4000-8000-000000000001", ContributorRef: "tok-c1",
		ClientSessionID: "upl-1", MIME: "image/jpeg", DeclaredBytes: 512 << 10,
		ClaimedSHA256: strings.Repeat("a", 64), QuarantineKey: "q/e0000000000000000000000000000001",
		CreatedAt: now,
	}
}

func TestReserveHappyPath(t *testing.T) {
	s, evt, err := NewSession(validParams())
	if err != nil {
		t.Fatalf("valid reservation rejected: %v", err)
	}
	if s.Status != StateIssued {
		t.Errorf("status = %q, want ISSUED", s.Status)
	}
	if !s.ExpiresAt.Equal(s.CreatedAt.Add(SessionTTL)) {
		t.Errorf("expires = %v, want created+24h", s.ExpiresAt)
	}
	if s.PolicyVersion != PolicyV1 || s.MaxBytes != MaxUploadBytes {
		t.Errorf("session = %+v", s)
	}
	if evt.SessionID != s.ID || evt.Name() != "SessionReserved" {
		t.Errorf("event = %+v", evt)
	}
	// Server attribution is preserved exactly; the client never supplies it.
	if s.ContributorRef != "tok-c1" {
		t.Errorf("contributor = %q", s.ContributorRef)
	}
}

func TestReserveRejectsNonJPEGIntent(t *testing.T) {
	for _, mime := range []string{"", "image/png", "image/jpg", "application/octet-stream", "IMAGE/JPEG"} {
		p := validParams()
		p.MIME = mime
		if _, _, err := NewSession(p); !errors.Is(err, ErrUnsupportedMedia) {
			t.Errorf("mime %q = %v, want unsupported", mime, err)
		}
	}
}

func TestReserveBoundsDeclaredSize(t *testing.T) {
	for _, n := range []int64{0, -10, MaxUploadBytes + 1} {
		p := validParams()
		p.DeclaredBytes = n
		if _, _, err := NewSession(p); !errors.Is(err, ErrSizeOutOfBounds) {
			t.Errorf("size %d = %v, want out of bounds", n, err)
		}
	}
	for _, n := range []int64{1, MaxUploadBytes} {
		p := validParams()
		p.DeclaredBytes = n
		if _, _, err := NewSession(p); err != nil {
			t.Errorf("size %d rejected: %v", n, err)
		}
	}
}

func TestReserveRequiresWellFormedHashClaim(t *testing.T) {
	// The client hash is a claim verified later, but garbage never
	// reserves quota: only 64 hex characters are admitted.
	for _, h := range []string{"", "xyz", strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("z", 64)} {
		p := validParams()
		p.ClaimedSHA256 = h
		if _, _, err := NewSession(p); !errors.Is(err, ErrBadHashClaim) {
			t.Errorf("hash %q = %v, want bad claim", h, err)
		}
	}
	p := validParams()
	p.ClaimedSHA256 = strings.ToUpper(strings.Repeat("b", 64))
	s, _, err := NewSession(p)
	if err != nil {
		t.Fatalf("uppercase hash rejected: %v", err)
	}
	if s.ClaimedSHA256 != strings.Repeat("b", 64) {
		t.Errorf("hash not normalized to lowercase: %q", s.ClaimedSHA256)
	}
}

func TestReserveRequiresServerFields(t *testing.T) {
	cases := map[string]func(*Params){
		"id":           func(p *Params) { p.ID = " " },
		"contributor":  func(p *Params) { p.ContributorRef = "" },
		"client-id":    func(p *Params) { p.ClientSessionID = "" },
		"quarantine":   func(p *Params) { p.QuarantineKey = "" },
		"key-traverse": func(p *Params) { p.QuarantineKey = "q/../evil" },
		"clock":        func(p *Params) { p.CreatedAt = time.Time{} },
	}
	for name, mutate := range cases {
		p := validParams()
		mutate(&p)
		if _, _, err := NewSession(p); !errors.Is(err, ErrInvalidSession) {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
}

func TestSessionStateMachine(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	s, _, err := NewSession(validParams())
	if err != nil {
		t.Fatal(err)
	}
	v, evt, err := RequestVerification(s, now)
	if err != nil {
		t.Fatalf("request verification: %v", err)
	}
	if v.Status != StateVerifying || evt.Name() != "VerificationRequested" {
		t.Errorf("verifying = %+v %+v", v, evt)
	}
	if _, _, err := RequestVerification(v, now); !errors.Is(err, ErrBadTransition) {
		t.Errorf("second claim = %v, want bad transition", err)
	}
	r, evt, err := MarkReady(v, now)
	if err != nil {
		t.Fatalf("mark ready: %v", err)
	}
	if r.Status != StateReady || evt.Name() != "EvidenceReady" {
		t.Errorf("ready = %+v %+v", r, evt)
	}
	if _, _, err := MarkReady(s, now); !errors.Is(err, ErrBadTransition) {
		t.Errorf("ready from issued = %v, want bad transition", err)
	}
}

func TestRejectNeedsReason(t *testing.T) {
	s, _, err := NewSession(validParams())
	if err != nil {
		t.Fatal(err)
	}
	v, _, err := RequestVerification(s, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Reject(v, nil, time.Now()); !errors.Is(err, ErrBadReason) {
		t.Errorf("reasonless reject = %v", err)
	}
	j, evt, err := Reject(v, []string{"invalid-image"}, time.Now())
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if j.Status != StateRejected || evt.Name() != "EvidenceRejected" {
		t.Errorf("rejected = %+v %+v", j, evt)
	}
	if _, _, err := Reject(s, []string{"x"}, time.Now()); !errors.Is(err, ErrBadTransition) {
		t.Errorf("reject from issued = %v, want bad transition", err)
	}
}

func TestIdleSessionExpiresAfterPolicyDeadline(t *testing.T) {
	s, _, err := NewSession(validParams())
	if err != nil {
		t.Fatal(err)
	}
	// Still fresh: expiry refuses, leaving ISSUED untouched.
	if _, _, err := Expire(s, s.CreatedAt.Add(time.Hour)); !errors.Is(err, ErrNotExpired) {
		t.Errorf("early expiry = %v, want not-expired", err)
	}
	x, evt, err := Expire(s, s.ExpiresAt.Add(time.Second))
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if x.Status != StateExpired || evt.Name() != "SessionExpired" {
		t.Errorf("expired = %+v %+v", x, evt)
	}
	// Only idle ISSUED sessions expire; active work never does here.
	v, _, err := RequestVerification(s, s.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Expire(v, s.ExpiresAt.Add(48*time.Hour)); !errors.Is(err, ErrBadTransition) {
		t.Errorf("expiry of verifying = %v, want bad transition", err)
	}
}
