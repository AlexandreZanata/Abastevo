package migrate

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"io/fs"
)

// Check verifies the schema ledger against the embedded chain and the declared
// compatibility floor without applying DDL. A newer additive schema is
// accepted within the expand/contract window; missing or modified known
// migrations remain incompatible. Only the explicit migrator may change the
// schema. Never disclose SQL/DSNs in the public readiness result.
func Check(ctx context.Context, pool *pgxpool.Pool, files fs.FS) error {
	chain, err := loadMigrations(files)
	if err != nil {
		return err
	}
	min, err := loadMinSupported(files)
	if err != nil {
		return err
	}
	rows, err := pool.Query(ctx, "SELECT version, checksum FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("migrate: schema unavailable")
	}
	defer rows.Close()
	recorded := map[string]string{}
	for rows.Next() {
		var version, sum string
		if err := rows.Scan(&version, &sum); err != nil {
			return fmt.Errorf("migrate: invalid ledger")
		}
		recorded[version] = sum
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("migrate: ledger unavailable")
	}
	return checkLedger(chain, min, recorded)
}
