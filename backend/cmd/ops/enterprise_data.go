package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters/preparation"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

var errDataPrepareUsage = errors.New("usage: ops enterprise data prepare --dataset <key> --revision <n> --snapshot-dir <path>")

func runEnterpriseData(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "prepare" {
		return errDataPrepareUsage
	}
	flags := flag.NewFlagSet("enterprise data prepare", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dataset := flags.String("dataset", "", "reviewed dataset key")
	revision := flags.Int64("revision", 0, "reviewed catalog revision")
	directory := flags.String("snapshot-dir", "", "read-only verified enterprise export directory")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 ||
		!domain.ValidDatasetKey(*dataset) || *revision < 1 || *directory == "" {
		return errDataPrepareUsage
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, cfg, err := openPool(ctx)
	if err != nil {
		return errors.New("enterprise database unavailable")
	}
	defer pool.Close()
	if !cfg.EnterpriseEnabled || !cfg.EnterpriseBillingEnabled {
		return errors.New("enterprise Data is disabled")
	}
	result, err := preparation.Prepare(ctx, pool.Underlying(), *dataset, *revision, *directory)
	switch {
	case errors.Is(err, database.ErrUnsafeOrgRole):
		return errors.New("data preparation requires an ordinary data-preparation database login")
	case errors.Is(err, adapters.ErrDatasetNotFound), errors.Is(err, domain.ErrRightsPending),
		errors.Is(err, domain.ErrSourceBinding), errors.Is(err, enterpriseexport.ErrUnverifiedSnapshot):
		return err
	case err != nil:
		return errors.New("enterprise Data preparation unavailable")
	}
	_, err = fmt.Fprintf(out, "dataset %s revision=%d result_sha256=%s\n", *dataset, *revision, result)
	return err
}
