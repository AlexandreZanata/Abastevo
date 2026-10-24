//go:build integration

package adapters

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
)

// expectedMigrationsFrom derives the append-only migration names at or
// after the cutoff. Upgrade tests must not pin a historical tail; a new
// append-only migration is expected to apply and be reported.
func expectedMigrationsFrom(t *testing.T, from string) []string {
	t.Helper()
	entries, err := fs.ReadDir(dbmigrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	var expected []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") || name[:6] < from {
			continue
		}
		expected = append(expected, strings.TrimSuffix(name, ".sql")[:6])
	}
	slices.Sort(expected)
	return expected
}
