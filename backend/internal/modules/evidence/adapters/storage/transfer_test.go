package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetEnforcesBound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write([]byte("0123456789"))
	}))
	defer srv.Close()
	got, err := Get(context.Background(), srv.Client(), srv.URL, 10)
	if err != nil || string(got) != "0123456789" {
		t.Fatalf("get = %q, %v", got, err)
	}
	// One byte past the bound fails instead of truncating silently, so an
	// oversize object can never shrink into verified bytes.
	if _, err := Get(context.Background(), srv.Client(), srv.URL, 9); !errors.Is(err, ErrTooLarge) {
		t.Errorf("overflow = %v, want too-large", err)
	}
	if _, err := Get(context.Background(), srv.Client(), srv.URL, 0); !errors.Is(err, ErrTooLarge) {
		t.Errorf("zero bound = %v, want too-large", err)
	}
}

func TestTransferMapsStatusErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "AccessDenied", http.StatusForbidden)
	}))
	defer srv.Close()
	if _, err := Get(context.Background(), srv.Client(), srv.URL, 64); err == nil {
		t.Error("forbidden GET accepted")
	} else {
		var status *StatusError
		if !errors.As(err, &status) || status.Status != http.StatusForbidden {
			t.Errorf("GET error = %v", err)
		}
	}
	if err := Put(context.Background(), srv.Client(), srv.URL, "image/jpeg", []byte("x")); err == nil {
		t.Error("forbidden PUT accepted")
	} else {
		var status *StatusError
		if !errors.As(err, &status) || status.Status != http.StatusForbidden {
			t.Errorf("PUT error = %v", err)
		}
	}
}

func TestDeleteIsIdempotent(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodDelete {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		if calls == 1 {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "NoSuchKey", http.StatusNotFound)
	}))
	defer srv.Close()
	if err := Delete(context.Background(), srv.Client(), srv.URL); err != nil {
		t.Fatalf("delete = %v", err)
	}
	// Already gone converges: repeated sweeps never fail on absence.
	if err := Delete(context.Background(), srv.Client(), srv.URL); err != nil {
		t.Errorf("repeat delete = %v", err)
	}
}

func TestPutSendsExactContentType(t *testing.T) {
	var gotType string
	var gotLength int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		gotLength = int64(len(raw))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	body := []byte(strings.Repeat("j", 1024))
	if err := Put(context.Background(), srv.Client(), srv.URL, "image/jpeg", body); err != nil {
		t.Fatalf("put = %v", err)
	}
	if gotType != "image/jpeg" || gotLength != int64(len(body)) {
		t.Errorf("sent %q %d", gotType, gotLength)
	}
}

func TestDeleteConfirmedRequiresPrivateAbsence(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusOK, http.StatusForbidden, http.StatusInternalServerError, http.StatusTemporaryRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			methods := []string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method)
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.WriteHeader(status)
			}))
			defer server.Close()
			in := PresignInput{Endpoint: server.URL, Bucket: "synthetic-private", Key: "f/test", Namespace: "f/", TTL: DefaultURLTTL}
			err := DeleteConfirmed(context.Background(), server.Client(), in, Credentials{AccessKeyID: "fixture", SecretAccessKey: "synthetic-only"})
			if (err == nil) != (status == http.StatusNotFound) || len(methods) != 2 || methods[0] != http.MethodDelete || methods[1] != http.MethodHead {
				t.Fatal(methods, err)
			}
		})
	}
}
