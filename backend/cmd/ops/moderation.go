package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	communityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters"
	moderationadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/adapters"
	moderationapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/application"
	moderationdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
	qualityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	trustadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trust/adapters"
	trustdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trust/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// actArgs carries one parsed operator action invocation. Parsing stays
// pure (no I/O) so arg validation is unit-testable.
type actArgs struct {
	operator string
	caseID   string
	action   string
	reason   string
}

func parseActArgs(args []string) (actArgs, error) {
	fs := flag.NewFlagSet("moderation act", flag.ContinueOnError)
	var a actArgs
	fs.StringVar(&a.operator, "operator", "", "accountable operator identity (or ANPFUEL_OPERATOR_ID)")
	fs.StringVar(&a.caseID, "case", "", "moderation case ID")
	fs.StringVar(&a.action, "action", "", "REVIEW|INVALIDATE|BLOCK|RESOLVE|DISMISS")
	fs.StringVar(&a.reason, "reason", "", "mandatory review reason")
	if err := fs.Parse(args); err != nil {
		return actArgs{}, err
	}
	if strings.TrimSpace(a.caseID) == "" || strings.TrimSpace(a.action) == "" {
		return actArgs{}, errors.New("moderation act requires --case and --action")
	}
	return a, nil
}

// caseArgs carries the parsed reviewed-override invocations
// (invalidate/block share case+reason+operator shape).
type caseArgs struct {
	operator string
	caseID   string
	reason   string
}

func parseCaseArgs(name string, args []string) (caseArgs, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	var c caseArgs
	fs.StringVar(&c.operator, "operator", "", "accountable operator identity (or ANPFUEL_OPERATOR_ID)")
	fs.StringVar(&c.caseID, "case", "", "moderation case ID")
	fs.StringVar(&c.reason, "reason", "", "mandatory review reason")
	if err := fs.Parse(args); err != nil {
		return caseArgs{}, err
	}
	if strings.TrimSpace(c.caseID) == "" {
		return caseArgs{}, fmt.Errorf("%s requires --case", name)
	}
	return c, nil
}

func runModeration(args []string, getenv func(string) string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "act":
		parsed, err := parseActArgs(args[1:])
		if err != nil {
			return err
		}
		return moderationAct(parsed, getenv)
	case "invalidate":
		parsed, err := parseCaseArgs("moderation invalidate", args[1:])
		if err != nil {
			return err
		}
		return moderationInvalidate(parsed, getenv)
	case "block":
		parsed, err := parseCaseArgs("moderation block", args[1:])
		if err != nil {
			return err
		}
		return moderationBlock(parsed, getenv)
	default:
		return fmt.Errorf("unknown moderation command %q\n%s", args[0], usage)
	}
}

// moderationAct records one audited operator action with its case status
// move and downstream recomputation (P07-T02): audit, eligibility
// change and job commit atomically; history is never edited.
func moderationAct(a actArgs, getenv func(string) string) error {
	operator, err := resolveOperatorID(a.operator, getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, _, err := openPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	raw := pool.Underlying()
	store := moderationadapters.NewStore(raw)
	res, err := moderationapp.Act(ctx, moderationapp.ActPorts{
		ApplyQualityReport:    (&qualityadapters.ReportStore{Pool: raw}).ApplyReviewTx,
		Clock:                 time.Now,
		NewID:                 newUUID,
		Store:                 store,
		InvalidateObservation: communityadapters.NewStore(raw).InvalidateTx,
		BlockContributor: func(ctx context.Context, tx pgx.Tx, contributorRef, caseID, reason string, at time.Time) error {
			id, err := newUUID()
			if err != nil {
				return err
			}
			verdict, _, err := trustdomain.NewDecision(trustdomain.DecisionParams{
				ID: id, ContributorRef: contributorRef, Tier: trustdomain.TierBlocked, Reason: reason,
				CaseRefs: []string{caseID}, OccurredAt: at,
			})
			if err != nil {
				return err
			}
			_, err = trustadapters.NewStore(raw).AppendDecisionTx(ctx, tx, verdict)
			return err
		},
		EnqueueJob: func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
			_, err := platformjobs.Enqueue(ctx, tx, kind, payload, dedupe, 5, time.Time{})
			return err
		},
	}, moderationapp.Caller{OperatorID: operator}, moderationapp.ActDTO{
		CaseID: a.caseID, Action: a.action, Reason: a.reason,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "action=%s case=%s status=%s job=%s\n",
		res.ActionID, res.CaseID, res.ToStatus, res.JobKind)
	return nil
}

// moderationInvalidate uses the same atomic audit/decision/job path as act.
func moderationInvalidate(c caseArgs, getenv func(string) string) error {
	return moderationAct(actArgs{operator: c.operator, caseID: c.caseID, action: moderationdomain.ActionInvalidate, reason: c.reason}, getenv)
}

// moderationBlock uses the same atomic audit/trust/job path as act.
func moderationBlock(c caseArgs, getenv func(string) string) error {
	return moderationAct(actArgs{operator: c.operator, caseID: c.caseID, action: moderationdomain.ActionBlock, reason: c.reason}, getenv)
}
