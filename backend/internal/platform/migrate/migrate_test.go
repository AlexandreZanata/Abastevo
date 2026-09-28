package migrate

import (
	"testing"
)

func TestParseMigrationName(t *testing.T) {
	version, name, err := parseMigrationName("000001_enable_postgis.sql")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if version != "000001" || name != "enable_postgis" {
		t.Errorf("got %q/%q", version, name)
	}
	for _, bad := range []string{
		"1_enable.sql",
		"000001.sql",
		"000001_Enable_PostGIS.SQL",
		"000001_enable_postgis.up.sql",
		"README.md",
	} {
		if _, _, err := parseMigrationName(bad); err == nil {
			t.Errorf("expected error for %q, got nil", bad)
		}
	}
}
