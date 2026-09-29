package jobs

import (
	"strings"
	"testing"
)

func TestUUIDRoundTrip(t *testing.T) {
	id, err := NewUUIDv4()
	if err != nil {
		t.Fatalf("uuid: %v", err)
	}
	if len(id) != 36 || id[14] != '4' {
		t.Errorf("not a v4 UUID: %q", id)
	}
	parsed, err := mustUUID(id)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := uuidString(parsed); got != strings.ToLower(id) {
		t.Errorf("round trip = %q, want %q", got, id)
	}
	if _, err := mustUUID(""); err == nil {
		t.Error("empty string parsed")
	}
	for _, bad := range []string{"xyz", "d6c74c23-63db-4c24-a2e5-408cb23bad2", "zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz"} {
		if _, err := mustUUID(bad); err == nil {
			t.Errorf("malformed UUID accepted: %q", bad)
		}
	}
}
