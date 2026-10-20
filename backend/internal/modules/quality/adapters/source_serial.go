package adapters

import (
	"context"
	"errors"
	"time"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NativeSourceSerial holds the same source advisory lock across fetch, prepare
// and publish in every worker. A separate fixed-slot connection prevents a lock
// waiter from occupying the pool connection the active publication needs.
func NativeSourceSerial(pool *pgxpool.Pool) func(context.Context, func(context.Context) error) error {
	return func(ctx context.Context, run func(context.Context) error) error {
		if pool == nil || run == nil {
			return errors.New("quality source: serial ports unavailable")
		}
		ctx, cancel := context.WithTimeout(ctx, 210*time.Second)
		defer cancel()
		cfg := pool.Config().ConnConfig.Copy()
		cfg.ConnectTimeout = 5 * time.Second
		cfg.RuntimeParams["application_name"] = "quality-native-source-serial"
		conn, e := pgx.ConnectConfig(ctx, cfg)
		if e != nil {
			return errors.New("quality source: serial connection unavailable")
		}
		defer func() {
			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = conn.Close(closeCtx)
		}()
		if e = quality.New(conn).LockNativeImport(ctx); e != nil {
			return errors.New("quality source: serial lease unavailable")
		}
		return run(ctx)
	}
}
