// Command staging-report-repair restores operator cases for legacy staging
// reports that predate atomic report/case wiring. It never changes prices.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	moderation "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/adapters"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"time"
)

func main() {
	apply := flag.Bool("apply", false, "commit at most100 missing staging operator cases")
	flag.Parse()
	if err := run(*apply); err != nil {
		fmt.Fprintln(os.Stderr, "staging report repair refused or failed")
		os.Exit(1)
	}
}
func run(apply bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Env != config.EnvStaging || cfg.CanonicalHost != "teste.abastevo.com.br" {
		return errors.New("only owned staging allowed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	return repair(ctx, pool, apply)
}
func repair(ctx context.Context, pool *pgxpool.Pool, apply bool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Restricted one-shot repair composition: read legacy community report
	// metadata and write through the moderation module's owned transaction port.
	// Reporter attribution and evidence contents never leave their owner tables.
	rows, err := tx.Query(ctx, `SELECT DISTINCT ON (d.target_observation_id) d.target_observation_id::text,d.reason,d.detail,d.received_at FROM community_disputes d WHERE d.status='OPEN' AND NOT EXISTS(SELECT 1 FROM moderation_cases c WHERE c.target_type='OBSERVATION' AND c.target_id=d.target_observation_id::text AND c.status IN ('OPEN','IN_REVIEW')) ORDER BY d.target_observation_id,d.received_at,d.id LIMIT 100`)
	if err != nil {
		return err
	}
	var cases []domain.Case
	for rows.Next() {
		var target, reason, detail string
		var at time.Time
		if err := rows.Scan(&target, &reason, &detail, &at); err != nil {
			rows.Close()
			return err
		}
		id, err := newID()
		if err != nil {
			rows.Close()
			return err
		}
		c, _, err := domain.NewCase(domain.CaseParams{ID: id, TargetType: domain.TargetObservation, TargetID: target, Priority: domain.PriorityP2, Reason: reason, Detail: detail, OpenedAt: at})
		if err != nil {
			rows.Close()
			return err
		}
		cases = append(cases, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	store := moderation.NewStore(pool)
	created := 0
	if apply {
		for _, c := range cases {
			_, replay, err := store.OpenCaseTx(ctx, tx, c)
			if err != nil {
				return err
			}
			if !replay {
				created++
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"apply": apply, "missing_in_batch": len(cases), "created": created, "batch_limit": 100})
}
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	s := hex.EncodeToString(b[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:], nil
}
