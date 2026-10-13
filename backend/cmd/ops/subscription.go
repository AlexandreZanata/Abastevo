package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	sub "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"io"
	"time"
)

func runSubscription(args []string, getenv func(string) string) error {
	if len(args) == 0 || (args[0] != "stats" && args[0] != "replay") {
		return domain.ErrInvalid
	}
	fs := flag.NewFlagSet("subscription", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	job := fs.String("job", "", "durable job UUID")
	reason := fs.String("reason", "", "audited recovery code")
	operator := fs.String("operator", "", "restricted operator identity")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return domain.ErrInvalid
	}
	actor, err := resolveOperatorID(*operator, getenv)
	if err != nil {
		return err
	}
	if args[0] == "replay" && (!domain.ValidID(*job) || (*reason != "provider-fixed" && *reason != "db-fixed" && *reason != "key-rotated")) {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, _, err := openPool(ctx)
	if err != nil {
		return errors.New("subscription database unavailable")
	}
	defer pool.Close()
	store := sub.NewStore(pool.Underlying())
	if args[0] == "replay" {
		return store.ReplayDead(ctx, *job, actor, *reason)
	}
	pending, dead, err := store.Backlog(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("subscription pending=%d dead=%d\n", pending, dead)
	return nil
}
