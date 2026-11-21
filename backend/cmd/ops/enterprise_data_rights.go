package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters/preparation"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

var errDataRightsUsage = errors.New("usage: ops enterprise data rights record --source <code> --digest <sha256> --revision <n> --status APPROVED|WITHDRAWN --decision-ref <ref> --operation-id <uuid> [--license-ref <ref> --snapshot-dir <path> --snapshot-sha256 <sha256> --official-revision <uuid>]")

func runEnterpriseDataRights(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "record" {
		return errDataRightsUsage
	}
	flags := flag.NewFlagSet("enterprise data rights record", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var decision preparation.RightsDecision
	source := flags.String("source", "", "reviewed public source code")
	status := flags.String("status", "", "APPROVED or WITHDRAWN")
	flags.StringVar(&decision.OperationID, "operation-id", "", "stable intent UUID")
	flags.StringVar(&decision.Digest, "digest", "", "reviewed source digest")
	flags.Int64Var(&decision.Revision, "revision", 0, "next review revision of this source digest")
	flags.StringVar(&decision.DecisionRef, "decision-ref", "", "review decision identifier")
	flags.StringVar(&decision.LicenseRef, "license-ref", "", "legal basis or license reference")
	flags.StringVar(&decision.SnapshotDir, "snapshot-dir", "", "read-only enterprise export directory")
	flags.StringVar(&decision.SnapshotDigest, "snapshot-sha256", "", "reviewed manifest content digest")
	flags.StringVar(&decision.OfficialRevision, "official-revision", "", "official survey revision for S-ESP-02")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return errDataRightsUsage
	}
	decision.Source, decision.State = domain.Source(*source), domain.RightsState(*status)
	if err := decision.Validate(); err != nil {
		return errDataRightsUsage
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, _, err := openPool(ctx)
	if err != nil {
		return errors.New("enterprise database unavailable")
	}
	defer pool.Close()
	replayed, err := preparation.RecordRights(ctx, pool.Underlying(), decision)
	switch {
	case errors.Is(err, database.ErrUnsafeOrgRole):
		return errors.New("data rights recording requires an ordinary source review database login")
	case errors.Is(err, preparation.ErrRightsRevision), errors.Is(err, preparation.ErrRightsOperationReused),
		errors.Is(err, domain.ErrSourceBinding), errors.Is(err, domain.ErrRightsPending),
		errors.Is(err, enterpriseexport.ErrUnverifiedSnapshot):
		return err
	case err != nil:
		return errors.New("enterprise Data rights recording unavailable")
	}
	_, err = fmt.Fprintf(out, "source %s revision=%d status=%s replayed=%t\n", decision.Source, decision.Revision, decision.State, replayed)
	return err
}
