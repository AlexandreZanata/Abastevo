package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	feedbackadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/adapters"
	feedbackapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/application"
	feedbackdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
	moderationadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/adapters"
	moderationdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
)

// feedbackArgs carries one parsed operator visibility invocation.
// Parsing stays pure (no I/O) so arg validation is unit-testable.
type feedbackArgs struct {
	operator string
	comment  string
	caseID   string
	hide     bool
}

func parseFeedbackArgs(args []string) (feedbackArgs, error) {
	fs := flag.NewFlagSet("feedback visibility", flag.ContinueOnError)
	var a feedbackArgs
	fs.StringVar(&a.operator, "operator", "", "accountable operator identity (or ANPFUEL_OPERATOR_ID)")
	fs.StringVar(&a.comment, "comment", "", "feedback comment ID")
	fs.StringVar(&a.caseID, "case", "", "moderation case ID bound to the comment")
	fs.BoolVar(&a.hide, "hide", true, "hide (true) or restore to visible (false)")
	if err := fs.Parse(args); err != nil {
		return feedbackArgs{}, err
	}
	if strings.TrimSpace(a.comment) == "" {
		return feedbackArgs{}, errors.New("feedback visibility requires --comment")
	}
	if strings.TrimSpace(a.caseID) == "" {
		return feedbackArgs{}, errors.New("feedback visibility requires --case for audit binding")
	}
	return a, nil
}

func runFeedback(args []string, getenv func(string) string) error {
	if len(args) == 0 {
		return errors.New("usage: ops feedback hide|show --comment ID --case ID [--operator ID] [--reason R]")
	}
	switch args[0] {
	case "hide", "show":
		hide := args[0] == "hide"
		parsed, err := parseFeedbackArgs(append([]string{fmt.Sprintf("--hide=%v", hide)}, args[1:]...))
		if err != nil {
			return err
		}
		return feedbackVisibility(parsed, getenv)
	case "export":
		parsed, err := parseFeedbackExportArgs(args[1:])
		if err != nil {
			return err
		}
		return feedbackExport(parsed, getenv)
	case "erase":
		parsed, err := parseFeedbackEraseArgs(args[1:])
		if err != nil {
			return err
		}
		return feedbackErase(parsed, getenv)
	default:
		return fmt.Errorf("unknown feedback command %q", args[0])
	}
}

// feedbackExportArgs carries one parsed owner-archive invocation.
// Parsing stays pure (no I/O) so arg validation is unit-testable.
type feedbackExportArgs struct {
	operator string
	account  string
}

func parseFeedbackExportArgs(args []string) (feedbackExportArgs, error) {
	fs := flag.NewFlagSet("feedback export", flag.ContinueOnError)
	var a feedbackExportArgs
	fs.StringVar(&a.operator, "operator", "", "accountable operator identity (or ANPFUEL_OPERATOR_ID)")
	fs.StringVar(&a.account, "account", "", "account ID whose footprint exports")
	if err := fs.Parse(args); err != nil {
		return feedbackExportArgs{}, err
	}
	if strings.TrimSpace(a.account) == "" {
		return feedbackExportArgs{}, errors.New("feedback export requires --account")
	}
	return a, nil
}

// feedbackEraseArgs carries one parsed owner-erasure invocation.
type feedbackEraseArgs struct {
	operator string
	account  string
	reason   string
}

func parseFeedbackEraseArgs(args []string) (feedbackEraseArgs, error) {
	fs := flag.NewFlagSet("feedback erase", flag.ContinueOnError)
	var a feedbackEraseArgs
	fs.StringVar(&a.operator, "operator", "", "accountable operator identity (or ANPFUEL_OPERATOR_ID)")
	fs.StringVar(&a.account, "account", "", "account ID whose footprint erases")
	fs.StringVar(&a.reason, "reason", "", "mandatory reviewed reason")
	if err := fs.Parse(args); err != nil {
		return feedbackEraseArgs{}, err
	}
	if strings.TrimSpace(a.account) == "" || strings.TrimSpace(a.reason) == "" {
		return feedbackEraseArgs{}, errors.New("feedback erase requires --account and --reason")
	}
	return a, nil
}

// feedbackVisibility binds one audited moderation case to a feedback
// visibility move (P14-T05A). The case must be OPEN, target the same
// comment as a COMMENT case; the operator identity comes from the
// restricted CLI environment, never from a public request. Record the
// matching `moderation act` on the case for the action audit trail;
// this command moves visibility only.
func feedbackVisibility(a feedbackArgs, getenv func(string) string) error {
	if _, err := resolveOperatorID(a.operator, getenv); err != nil {
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
	c, err := modStore.Get(ctx, strings.TrimSpace(a.caseID))
	if err != nil {
		return err
	}
	if c.TargetType != moderationdomain.TargetComment || c.TargetID != strings.TrimSpace(a.comment) {
		return fmt.Errorf("case %s does not target comment %s as COMMENT", c.ID, strings.TrimSpace(a.comment))
	}
	if c.Status != moderationdomain.StatusOpen && c.Status != moderationdomain.StatusInReview {
		return fmt.Errorf("case %s is %s; visibility moves only on open cases", c.ID, c.Status)
	}
	store := feedbackadapters.NewPGStore(raw)
	visibility := feedbackdomain.VisibilityVisible
	if a.hide {
		visibility = feedbackdomain.VisibilityHidden
	}
	if err := store.SetVisibility(ctx, strings.TrimSpace(a.comment), visibility); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "feedback %s visibility=%s case=%s\n", strings.TrimSpace(a.comment), visibility, c.ID)
	return nil
}

// feedbackClock is the operator-side time source for footprint reads.
type feedbackClock struct{}

func (feedbackClock) NowUnix() int64 { return time.Now().Unix() }

// feedbackExport prints one owner's deterministic feedback archive
// (P14-T05B). Operator identity is required; the archive holds only
// the requested account's rows, never other owners, GPS or secrets.
func feedbackExport(a feedbackExportArgs, getenv func(string) string) error {
	if _, err := resolveOperatorID(a.operator, getenv); err != nil {
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
	store := feedbackadapters.NewPGStore(raw)
	svc := &feedbackapp.Service{
		Clock: feedbackClock{}, Store: store, Comments: store, Votes: store,
		IDGen: newUUID,
	}
	rawJSON, err := svc.ExportAccountBytes(ctx, strings.TrimSpace(a.account))
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, string(rawJSON))
	return nil
}

// feedbackErase tombstones one account's live feedback and rebuilds
// the affected aggregates (P14-T05B). The reviewed reason is audited
// in the operator invocation; the store keeps tombstoned history for
// audit while aggregates ignore it. Re-running converges (restore
// replay uses the same command).
func feedbackErase(a feedbackEraseArgs, getenv func(string) string) error {
	operator, err := resolveOperatorID(a.operator, getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	pool, _, err := openPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	raw := pool.Underlying()
	store := feedbackadapters.NewPGStore(raw)
	svc := &feedbackapp.Service{
		Clock: feedbackClock{}, Store: store, Comments: store, Votes: store,
		IDGen: newUUID,
	}
	report, err := svc.EraseAccount(ctx, strings.TrimSpace(a.account))
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "operator=%s feedback-erase account=%s ratings=%d comments=%d votes=%d keys=%d tallies=%d reason=%q\n",
		operator, report.AccountID, report.Ratings, report.Comments, report.Votes,
		report.RatingKeys, report.Tallies, strings.TrimSpace(a.reason))
	return nil
}
