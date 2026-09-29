package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	communityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters"
	communitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
	moderationadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/adapters"
	moderationapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/application"
	moderationdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
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
		Clock: time.Now,
		NewID: newUUID,
		Store: store,
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

// moderationInvalidate applies a reviewed observation invalidation
// (BUC-006): first the moderation audit action, then the community
// VALIDATED→REJECTED decision bound to the same case ID with its
// consensus recompute job. Each step is atomic; a downstream failure
// after a recorded audit is reported for audited retry (the audit shows
// the attempt, the follow-up completes it), never silently dropped.
func moderationInvalidate(c caseArgs, getenv func(string) string) error {
	operator, err := resolveOperatorID(c.operator, getenv)
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
	modStore := moderationadapters.NewStore(raw)
	enqueue := func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
		_, err := platformjobs.Enqueue(ctx, tx, kind, payload, dedupe, 5, time.Time{})
		return err
	}
	actRes, err := moderationapp.Act(ctx, moderationapp.ActPorts{
		Clock: time.Now, NewID: newUUID, Store: modStore, EnqueueJob: enqueue,
	}, moderationapp.Caller{OperatorID: operator}, moderationapp.ActDTO{
		CaseID: c.caseID, Action: moderationdomain.ActionInvalidate, Reason: c.reason,
	})
	if err != nil {
		return fmt.Errorf("audit action: %w", err)
	}
	target, err := modStore.Get(ctx, c.caseID)
	if err != nil {
		return err
	}
	if target.TargetType != moderationdomain.TargetObservation {
		return fmt.Errorf("invalidate applies to OBSERVATION targets (case targets %s); use act for other targets", target.TargetType)
	}
	communityStore := communityadapters.NewStore(raw)
	obs, err := communityStore.Observation(ctx, target.TargetID)
	if err != nil {
		return fmt.Errorf("audit %s recorded; observation load: %w", actRes.ActionID, err)
	}
	decisions, err := communityStore.Decisions(ctx, obs.ID)
	if err != nil {
		return fmt.Errorf("audit %s recorded; decision history: %w", actRes.ActionID, err)
	}
	state := communitydomain.StateReceived
	for _, d := range decisions {
		state = d.ToState
	}
	decision, err := communitydomain.Invalidate(obs, state, communitydomain.ActorModerator,
		c.caseID, []string{"moderation-invalid"}, time.Now(), int64(len(decisions)+1))
	if err != nil {
		return fmt.Errorf("audit %s recorded; invalidation refused (state %s): %w", actRes.ActionID, state, err)
	}
	raw2, _ := json.Marshal(map[string]any{"version": 1, "observation_id": obs.ID})
	if err := communityStore.RecordDecisionWithJob(ctx, decision,
		"community-consensus", raw2, "consensus:"+obs.ID, enqueue); err != nil {
		return fmt.Errorf("audit %s recorded; decision commit: %w", actRes.ActionID, err)
	}
	fmt.Fprintf(os.Stdout, "action=%s case=%s status=%s observation=%s invalidated\n",
		actRes.ActionID, actRes.CaseID, actRes.ToStatus, obs.ID)
	return nil
}

// moderationBlock applies a reviewed contributor block (BUC-006): first
// the moderation audit action, then the trust BLOCKED verdict bound to
// the same case ID. Each step is atomic with its downstream job;
// failures after the audit are reported for audited retry.
func moderationBlock(c caseArgs, getenv func(string) string) error {
	operator, err := resolveOperatorID(c.operator, getenv)
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
	modStore := moderationadapters.NewStore(raw)
	enqueue := func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
		_, err := platformjobs.Enqueue(ctx, tx, kind, payload, dedupe, 5, time.Time{})
		return err
	}
	actRes, err := moderationapp.Act(ctx, moderationapp.ActPorts{
		Clock: time.Now, NewID: newUUID, Store: modStore, EnqueueJob: enqueue,
	}, moderationapp.Caller{OperatorID: operator}, moderationapp.ActDTO{
		CaseID: c.caseID, Action: moderationdomain.ActionBlock, Reason: c.reason,
	})
	if err != nil {
		return fmt.Errorf("audit action: %w", err)
	}
	target, err := modStore.Get(ctx, c.caseID)
	if err != nil {
		return err
	}
	if target.TargetType != moderationdomain.TargetContributor {
		return fmt.Errorf("block applies to CONTRIBUTOR targets (case targets %s); use act for other targets", target.TargetType)
	}
	id, err := newUUID()
	if err != nil {
		return fmt.Errorf("audit %s recorded; id: %w", actRes.ActionID, err)
	}
	verdict, _, err := trustdomain.NewDecision(trustdomain.DecisionParams{
		ID: id, ContributorRef: target.TargetID, Tier: trustdomain.TierBlocked,
		Reason: c.reason, CaseRefs: []string{c.caseID},
		OccurredAt: time.Now(), PolicyVersion: trustdomain.PolicyV1,
	})
	if err != nil {
		return fmt.Errorf("audit %s recorded; verdict: %w", actRes.ActionID, err)
	}
	trustStore := trustadapters.NewStore(raw)
	if _, err := trustStore.AppendDecision(ctx, verdict, enqueue); err != nil {
		return fmt.Errorf("audit %s recorded; verdict commit: %w", actRes.ActionID, err)
	}
	fmt.Fprintf(os.Stdout, "action=%s case=%s status=%s contributor=%s blocked\n",
		actRes.ActionID, actRes.CaseID, actRes.ToStatus, target.TargetID)
	return nil
}
