package migrate

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

const schemaMinFile = "schema_min_supported"

var schemaVersion = regexp.MustCompile(`^[0-9]{6}$`)

// loadMinSupported reads the oldest schema version this build can run against.
// The value is part of the embedded migration bundle and is also published in
// the signed release manifest.
func loadMinSupported(files fs.FS) (string, error) {
	raw, err := fs.ReadFile(files, schemaMinFile)
	if err != nil {
		return "", fmt.Errorf("migrate: read schema minimum: %w", err)
	}
	version := strings.TrimSpace(string(raw))
	if !schemaVersion.MatchString(version) {
		return "", fmt.Errorf("migrate: invalid schema minimum")
	}
	return version, nil
}

// checkLedger verifies the recorded ledger against the embedded chain and the
// declared compatibility floor. A database newer than the embedded chain is
// accepted as the additive side of the expand/contract window: every known
// migration must keep its checksum, no known migration may be missing, and the
// database may not be older than min. The deploy agent separately refuses a
// contract migration by comparing the previous and new schema_min_supported
// values before applying it.
func checkLedger(chain []migration, min string, recorded map[string]string) error {
	if len(chain) == 0 {
		return fmt.Errorf("migrate: empty migration chain")
	}
	newest := chain[len(chain)-1].version
	if min > newest {
		return fmt.Errorf("migrate: invalid schema window")
	}
	if len(recorded) == 0 {
		return fmt.Errorf("migrate: incompatible schema")
	}
	dbMax := ""
	for version := range recorded {
		if !schemaVersion.MatchString(version) {
			return fmt.Errorf("migrate: invalid ledger")
		}
		if version > dbMax {
			dbMax = version
		}
	}
	if dbMax < min {
		return fmt.Errorf("migrate: incompatible schema")
	}
	for _, m := range chain {
		if m.version > dbMax {
			break
		}
		sum, ok := recorded[m.version]
		if !ok {
			return fmt.Errorf("migrate: missing migration %s", m.version)
		}
		if sum != m.checksum {
			return fmt.Errorf("migrate: missing or modified migration %s", m.version)
		}
	}
	return nil
}
