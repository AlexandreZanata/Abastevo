package storage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerPrivatePUTRetainsCommunityPolicyAndConditions(t *testing.T) {
	in := testInput("https://storage.example.invalid")
	in.Namespace = "org/example/doc/example/"
	in.Key = in.Namespace + "snapshot"
	in.ContentType = "application/pdf"
	in.MaxBytes = 10 << 20
	if _, err := PresignPUT(in, testCred()); err == nil {
		t.Fatal("community upload policy widened")
	}
	if _, err := PresignServerPUT(in, testCred()); err != nil {
		t.Fatal(err)
	}
	for _, mime := range []string{"", "text/plain\r\nX-ACL: public", "image/jpeg; x=1"} {
		copy := in
		copy.ContentType = mime
		if _, err := PresignServerPUT(copy, testCred()); err == nil {
			t.Fatal("unsafe type accepted")
		}
	}
	in.MaxBytes = 0
	if _, err := PresignServerPUT(in, testCred()); err == nil {
		t.Fatal("absent bound accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "*" || r.Header.Get("Content-Type") != "image/jpeg" {
			t.Error("missing conditional/type header")
		}
		switch r.URL.Path {
		case "/created":
			w.WriteHeader(200)
		case "/exists":
			w.WriteHeader(412)
		case "/redirect":
			w.Header().Set("Location", "/created")
			w.WriteHeader(307)
		default:
			w.WriteHeader(503)
			_, _ = w.Write([]byte("PRIVATE-REMOTE-BODY"))
		}
	}))
	defer server.Close()
	for _, path := range []string{"/created", "/exists", "/redirect", "/error"} {
		created, err := PutIfAbsent(context.Background(), server.Client(), server.URL+path, "image/jpeg", []byte("synthetic"))
		switch path {
		case "/created":
			if err != nil || !created {
				t.Fatal(created, err)
			}
		case "/exists":
			if err != nil || created {
				t.Fatal(created, err)
			}
		default:
			if err == nil || strings.Contains(err.Error(), "PRIVATE-REMOTE-BODY") {
				t.Fatal("unsafe remote error")
			}
		}
	}
}

func TestServerPrivatePUTNeverSignsCommunityNamespaces(t *testing.T) {
	for _, namespace := range []string{QuarantinePrefix, "f/", "org/", "organization/", ""} {
		in := testInput("https://storage.example.invalid")
		in.Namespace = namespace
		in.Key = namespace + "snapshot"
		in.ContentType = "image/jpeg"
		in.MaxBytes = 1 << 20
		if _, err := PresignServerPUT(in, testCred()); !errors.Is(err, ErrServerNamespace) {
			t.Fatalf("server PUT signed namespace %q: %v", namespace, err)
		}
	}
}
