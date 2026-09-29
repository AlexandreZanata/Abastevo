package domain

import (
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

func validParams() RequestParams {
	return RequestParams{
		ID:                 "e0000000-0000-4000-8000-000000000001",
		ContributorID:      "c1111111-0000-4000-8000-000000000001",
		ContributorRef:     "tok-owner",
		ClientSubmissionID: "export-1",
		Type:               TypeExport,
		RequestedAt:        time.Now(),
	}
}

func TestNewRequestValid(t *testing.T) {
	r, err := NewRequest(validParams())
	if err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	if r.Status != StatusRequested || r.PolicyVersion != PolicyV1 {
		t.Errorf("request = %+v", r)
	}
	p := validParams()
	p.Type = TypeDeletion
	if _, err := NewRequest(p); err != nil {
		t.Errorf("deletion type rejected: %v", err)
	}
	p = validParams()
	p.Type = "BACKUP"
	if _, err := NewRequest(p); err != ErrUnknownType {
		t.Errorf("unknown type = %v", err)
	}
	for _, mutate := range []func(*RequestParams){
		func(p *RequestParams) { p.ContributorID = "" },
		func(p *RequestParams) { p.ContributorRef = "" },
		func(p *RequestParams) { p.ClientSubmissionID = " " },
	} {
		p := validParams()
		mutate(&p)
		if _, err := NewRequest(p); err != ErrInvalidRequest {
			t.Errorf("invalid request accepted: %+v", p)
		}
	}
}

func TestCompleteBoundsArchive(t *testing.T) {
	r, err := NewRequest(validParams())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	done, evt, err := Complete(r, []byte(`{"v":1}`), "ABCDEF", at)
	if err != nil {
		t.Fatalf("complete = %v", err)
	}
	if done.Status != StatusReady || done.ArchiveSHA256 != "abcdef" {
		t.Errorf("completed = %+v", done)
	}
	if !done.ExpiresAt.Equal(at.Add(DownloadTTL)) {
		t.Errorf("window = %v", done.ExpiresAt)
	}
	if evt.Name() != "PrivacyRequestCompleted" || evt.RequestID != r.ID {
		t.Errorf("event = %+v", evt)
	}
	if _, _, err := Complete(done, []byte(`{}`), "x", at); err != ErrBadState {
		t.Errorf("double complete = %v", err)
	}
	if _, _, err := Complete(r, nil, "x", at); err != ErrTooLarge {
		t.Errorf("empty archive = %v", err)
	}
	if _, _, err := Complete(r, make([]byte, MaxArchiveBytes+1), "x", at); err != ErrTooLarge {
		t.Errorf("oversize archive accepted")
	}
}

func TestFailAndExpiry(t *testing.T) {
	r, err := NewRequest(validParams())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	failed, evt, err := Fail(r, "inventory unavailable", at)
	if err != nil || failed.Status != StatusFailed {
		t.Fatalf("fail = %+v, %v", failed, err)
	}
	if evt.Status != StatusFailed {
		t.Errorf("event = %+v", evt)
	}
	if _, _, err := Fail(failed, "again", at); err != ErrBadState {
		t.Errorf("double fail = %v", err)
	}
	if _, _, err := Fail(r, "", at); err != ErrInvalidRequest {
		t.Errorf("empty reason = %v", err)
	}
	done, _, err := Complete(r, []byte(`{}`), "ff", at)
	if err != nil {
		t.Fatal(err)
	}
	if Expired(done, at.Add(DownloadTTL-time.Second)) {
		t.Error("fresh export expired")
	}
	if !Expired(done, at.Add(DownloadTTL+time.Second)) {
		t.Error("stale export live")
	}
	if Expired(r, at.Add(DownloadTTL+time.Second)) {
		t.Error("non-ready request expired")
	}
}

func TestRequestCarriesIdentifiersOnly(t *testing.T) {
	// The ledger row must stay identifier-only: no payload, media, GPS
	// or key material. Additions here fail loudly (B-BR-011).
	allowed := map[string]bool{
		"ID": true, "ContributorID": true, "ContributorRef": true,
		"ClientSubmissionID": true, "Type": true, "Status": true,
		"ArchiveSHA256": true, "RequestedAt": true, "ReadyAt": true,
		"ExpiresAt": true, "CompletedAt": true, "PolicyVersion": true,
	}
	typ := reflect.TypeOf(Request{})
	for i := 0; i < typ.NumField(); i++ {
		if !allowed[typ.Field(i).Name] {
			t.Errorf("unexpected Request field %q", typ.Field(i).Name)
		}
	}
}
