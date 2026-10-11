package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// CS10C: the training object store speaks SigV4 presigned URLs against
// the private bucket (MinIO/R2). These tests run the transfer path
// against a fake S3-speaking server: keys stay inside the training
// namespace, deletes are idempotent, and existence checks bound their
// reads.

type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := strings.TrimPrefix(r.URL.Path, "/validation-private/")
	switch r.Method {
	case http.MethodPut:
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		f.objects[key] = buf
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		raw, ok := f.objects[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	case http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func testObjectStore(t *testing.T, s3 *fakeS3) (*TrainingObjects, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(s3)
	t.Cleanup(srv.Close)
	return NewTrainingObjects(TrainingObjectConfig{
		Endpoint: srv.URL, Bucket: "validation-private", Region: "us-east-1",
		AccessKeyID: "test", SecretAccessKey: "test-secret-test-secret-test12",
	}), srv
}

func TestTrainingObjectsRoundTrip(t *testing.T) {
	s3 := &fakeS3{objects: map[string][]byte{}}
	store, _ := testObjectStore(t, s3)
	ctx := context.Background()
	key := "training/v1/2026-10-11/img-1-1.jpg"
	if err := store.Put(ctx, key, []byte("minimized")); err != nil {
		t.Fatalf("put = %v", err)
	}
	exists, err := store.Exists(ctx, key)
	if err != nil || !exists {
		t.Fatalf("exists = %v, %v", exists, err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("delete = %v", err)
	}
	exists, err = store.Exists(ctx, key)
	if err != nil || exists {
		t.Fatalf("post-delete exists = %v, %v", exists, err)
	}
	// Deletes stay idempotent; missing reads deny quietly.
	if err := store.Delete(ctx, key); err != nil {
		t.Errorf("repeat delete = %v", err)
	}
	if exists, err := store.Exists(ctx, key); err != nil || exists {
		t.Errorf("ghost exists = %v, %v", exists, err)
	}
}

func TestTrainingObjectsRefuseOutsideNamespace(t *testing.T) {
	s3 := &fakeS3{objects: map[string][]byte{}}
	store, _ := testObjectStore(t, s3)
	ctx := context.Background()
	for _, key := range []string{"", "../escape.jpg", "other/ns.jpg", "training/v1/../../x.jpg"} {
		if err := store.Put(ctx, key, []byte("x")); err == nil {
			t.Errorf("put %q accepted", key)
		}
		if _, err := store.Exists(ctx, key); err == nil {
			t.Errorf("exists %q accepted", key)
		}
		if err := store.Delete(ctx, key); err == nil {
			t.Errorf("delete %q accepted", key)
		}
	}
}
