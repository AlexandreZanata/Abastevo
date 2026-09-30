package migrate

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"io/fs"
)

// Check verifies the complete expected schema ledger without applying DDL.
// Missing, modified or unknown migrations make a binary incompatible; only
// the explicit migrator may change the schema. Never disclose SQL/DSNs in
// the public readiness result.
func Check(ctx context.Context, pool *pgxpool.Pool, files fs.FS) error {
	chain, err := loadMigrations(files)
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
	if len(recorded) != len(chain) {
		return fmt.Errorf("migrate: incompatible schema")
	}
	for _, m := range chain {
		if recorded[m.version] != m.checksum {
			return fmt.Errorf("migrate: missing or modified migration %s", m.version)
		}
	}
	return nil
}
