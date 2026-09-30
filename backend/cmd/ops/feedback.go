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
	hide := true
	rest := args
	switch args[0] {
	case "hide":
		rest = args[1:]
	case "show":
		hide = false
		rest = args[1:]
	default:
		return fmt.Errorf("unknown feedback command %q", args[0])
	}
	parsed, err := parseFeedbackArgs(append([]string{fmt.Sprintf("--hide=%v", hide)}, rest...))
	if err != nil {
		return err
	}
	return feedbackVisibility(parsed, getenv)
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
