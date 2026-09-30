package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTransfersRefuseRedirectsAndDoNotLeakSignedURL(t *testing.T) {
	hits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; w.WriteHeader(200) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	ctx := context.Background()
	client := source.Client()
	raw := source.URL + "?X-Amz-Signature=synthetic-private-proof"
	_, getErr := Get(ctx, client, raw, 64)
	putErr := Put(ctx, client, raw, "image/jpeg", []byte("x"))
	delErr := Delete(ctx, client, raw)
	if hits != 0 {
		t.Fatalf("followed %d redirects", hits)
	}
	for _, err := range []error{getErr, putErr, delErr} {
		if err == nil {
			t.Error("redirect succeeded")
		}
	}
	source.Close()
	_, err := Get(ctx, client, raw, 64)
	if err == nil || strings.Contains(err.Error(), "synthetic-private-proof") {
		t.Fatalf("signed URL leaked: %v", err)
	}
}
