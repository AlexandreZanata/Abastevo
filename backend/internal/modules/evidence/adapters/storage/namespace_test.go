package storage

import (
	"testing"
)

// SEC03-D namespace isolation (AUD-02): client-minted URLs stay
// inside the quarantine prefix; server-only finals and any public
// prefix never mint through a client namespace, and quarantine keys
// never mint through the finals namespace. Bytes cannot enter a
// public prefix through this primitive.
func TestPresignNamespaceIsolation(t *testing.T) {
	base := testInput("https://xxx.r2.cloudflarestorage.com")
	for _, tc := range []struct {
		name      string
		key       string
		namespace string
		wantErr   bool
	}{
		{"quarantine under quarantine", "q/abc123", "q/", false},
		{"final under quarantine refused", "f/abc123", "q/", true},
		{"quarantine under finals refused", "q/abc123", "f/", true},
		{"final under finals allowed (server path)", "f/abc123", "f/", false},
		{"public under quarantine refused", "public/abc123", "q/", true},
		{"public under finals refused", "public/abc123", "f/", true},
		{"bare key refused", "abc123", "q/", true},
		{"traversal refused", "q/../f/abc123", "q/", true},
	} {
		in := base
		in.Key, in.Namespace = tc.key, tc.namespace
		_, err := PresignPUT(in, testCred())
		if tc.wantErr && err == nil {
			t.Errorf("%s: minted", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}
