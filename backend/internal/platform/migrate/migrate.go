// Package migrate applies the ordered append-only SQL chain with a
// lock/checksum ledger. This explicit runner (no migration framework) is the
// only schema-change mechanism: concurrent runners serialize on a session
// advisory lock, each file applies once inside a transaction, and any
// checksum drift of an applied file aborts loudly.
//
// Constraints: one file holds plain sequential statements executed as a
// single batch in one transaction; prefer one change per numbered file.
// Never edit an applied file; recovery is a new migration, except on a
// disposable database where the local volume may be recreated.
package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

var migrationName = regexp.MustCompile(`^([0-9]{6})_([a-z0-9_]+)\.sql$`)

// parseMigrationName validates the strict NNNNNN_label.sql convention.
func parseMigrationName(name string) (version, label string, err error) {
	m := migrationName.FindStringSubmatch(name)
	if m == nil {
		return "", "", fmt.Errorf("migrate: malformed migration name %q", name)
	}
	return m[1], m[2], nil
}

type migration struct {
	version  string
	filename string
	sql      string
	checksum string
}

func checksum(sql string) string {
	sum := sha256.Sum256([]byte(sql))
	return hex.EncodeToString(sum[:])
}

func loadMigrations(files fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate: read migrations: %w", err)
	}
	var out []migration
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, _, err := parseMigrationName(entry.Name())
		if err != nil {
			return nil, err
		}
		raw, err := fs.ReadFile(files, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("migrate: read %s: %w", entry.Name(), err)
		}
		sql := string(raw)
		out = append(out, migration{
			version:  version,
			filename: entry.Name(),
			sql:      sql,
			checksum: checksum(sql),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	for i := 1; i < len(out); i++ {
		if out[i].version == out[i-1].version {
			return nil, fmt.Errorf("migrate: duplicate version %s", out[i].version)
		}
	}
	return out, nil
}

// Apply connects with the deployment (migrator) role and brings the schema to
// the chain in files. It returns the versions applied by this call; a second
// call is a no-op. Errors never carry the DSN.
func Apply(ctx context.Context, dsn string, files fs.FS) ([]string, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("migrate: connect: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("migrate: ping: %w", err)
	}

	// Serialize concurrent runners on a session advisory lock.
	holder, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: acquire lock holder: %w", err)
	}
	defer holder.Release()
	if _, err := holder.Exec(ctx, "SELECT pg_advisory_lock(hashtext('anpfuel_schema_migrations'))"); err != nil {
		return nil, fmt.Errorf("migrate: lock: %w", err)
	}
	defer holder.Exec(context.Background(), "SELECT pg_advisory_unlock(hashtext('anpfuel_schema_migrations'))")

	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		checksum TEXT NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return nil, fmt.Errorf("migrate: ledger: %w", err)
	}

	chain, err := loadMigrations(files)
	if err != nil {
		return nil, err
	}
	recorded := map[string]string{}
	rows, err := pool.Query(ctx, "SELECT version, checksum FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("migrate: read ledger: %w", err)
	}
	for rows.Next() {
		var version, sum string
		if err := rows.Scan(&version, &sum); err != nil {
			rows.Close()
			return nil, fmt.Errorf("migrate: scan ledger: %w", err)
		}
		recorded[version] = sum
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrate: read ledger: %w", err)
	}

	var applied []string
	for _, m := range chain {
		if sum, ok := recorded[m.version]; ok {
			if sum != m.checksum {
				return nil, fmt.Errorf("migrate: checksum mismatch for %s (%s): file changed after apply", m.version, m.filename)
			}
			continue
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("migrate: begin %s: %w", m.version, err)
		}
		if _, err := tx.Exec(ctx, m.sql); err != nil {
			_ = tx.Rollback(ctx)
			return nil, fmt.Errorf("migrate: apply %s: %w", m.version, err)
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations(version, checksum) VALUES ($1, $2)",
			m.version, m.checksum); err != nil {
			_ = tx.Rollback(ctx)
			return nil, fmt.Errorf("migrate: record %s: %w", m.version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("migrate: commit %s: %w", m.version, err)
		}
		applied = append(applied, m.version)
	}
	return applied, nil
}
