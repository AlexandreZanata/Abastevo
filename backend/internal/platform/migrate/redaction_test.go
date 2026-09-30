package migrate

import (
	"context"
	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"strings"
	"testing"
)

func TestInvalidMigrationDSNDoesNotExposePassword(t *testing.T) {
	_, err := Apply(context.Background(), "postgres://user:synthetic-private-password@localhost/db?connect_timeout=invalid", dbmigrations.Files)
	if err == nil || strings.Contains(err.Error(), "synthetic-private-password") {
		t.Fatalf("DSN redaction failed: %v", err)
	}
}
