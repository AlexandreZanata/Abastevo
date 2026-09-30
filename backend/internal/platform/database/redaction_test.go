package database

import (
	"context"
	"strings"
	"testing"
)

func TestInvalidDSNDoesNotExposePassword(t *testing.T) {
	_, err := Open(context.Background(), "postgres://user:synthetic-private-password@localhost/db?connect_timeout=invalid", DefaultOptions())
	if err == nil || strings.Contains(err.Error(), "synthetic-private-password") {
		t.Fatalf("DSN redaction failed: %v", err)
	}
}
