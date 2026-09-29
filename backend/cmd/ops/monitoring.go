package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry/alerts"
)

// Monitoring thresholds (INFRASTRUCTURE_PLAN budgets): oldest queued
// job past 5 minutes warns, any dead-lettered job pages, backup
// manifests older than 26 hours page.
const (
	stuckJobThreshold = 5 * time.Minute
	backupFreshHours  = 26
)

type monitoringArgs struct {
	webhook  string
	manifest string
}

func parseMonitoringArgs(args []string) (monitoringArgs, error) {
	fs := flag.NewFlagSet("monitoring eval", flag.ContinueOnError)
	var m monitoringArgs
	fs.StringVar(&m.webhook, "webhook", "", "operator webhook URL for fired alerts (optional)")
	fs.StringVar(&m.manifest, "backup-manifest", "", "newest backup manifest for the freshness rule (optional)")
	if err := fs.Parse(args); err != nil {
		return monitoringArgs{}, err
	}
	return m, nil
}

func runMonitoring(args []string, getenv func(string) string) error {
	if len(args) == 0 || args[0] != "eval" {
		return fmt.Errorf("unknown monitoring command %q", strings.Join(args, " "))
	}
	parsed, err := parseMonitoringArgs(args[1:])
	if err != nil {
		return err
	}
	if v := getenv("ANPFUEL_MONITORING_WEBHOOK"); parsed.webhook == "" && v != "" {
		parsed.webhook = v
	}
	if v := getenv("ANPFUEL_BACKUP_MANIFEST"); parsed.manifest == "" && v != "" {
		parsed.manifest = v
	}
	return monitoringEval(parsed, getenv)
}

// monitoringEval runs every monitoring rule once against live state and
// notifies the operator route for new firings. Exit 0 when quiet, exit
// 1 when anything fired (cron integration treats non-zero as page).
func monitoringEval(m monitoringArgs, _ func(string) string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, _, err := openPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	queue := platformjobs.NewQueue(pool.Underlying())

	var notifier alerts.Notifier = alerts.LogNotifier{Log: func(msg string, args ...any) {
		fmt.Fprintln(os.Stderr, msg, args)
	}}
	if m.webhook != "" {
		notifier = multiNotifier{
			Notifier: alerts.LogNotifier{Log: func(msg string, args ...any) {
				fmt.Fprintln(os.Stderr, msg, args)
			}},
			extra: alerts.WebhookNotifier{URL: m.webhook},
		}
	}
	rules := []alerts.Rule{
		{
			Name: "db_down", Severity: alerts.SeverityCritical,
			Check: func(ctx context.Context) (bool, string, error) {
				if err := pool.Ping(ctx); err != nil {
					return true, "code=db_unreachable", nil
				}
				return false, "", nil
			},
		},
		{
			Name: "jobs_dead", Severity: alerts.SeverityCritical,
			Check: func(ctx context.Context) (bool, string, error) {
				_, dead, _, err := queue.Backlog(ctx)
				if err != nil {
					return false, "", err
				}
				return deadRule(dead)
			},
		},
		{
			Name: "jobs_stuck", Severity: alerts.SeverityWarning,
			Check: func(ctx context.Context) (bool, string, error) {
				queued, _, oldest, err := queue.Backlog(ctx)
				if err != nil {
					return false, "", err
				}
				return stuckRule(queued, oldest, time.Now())
			},
		},
	}
	if m.manifest != "" {
		manifest := m.manifest
		rules = append(rules, alerts.Rule{
			Name: "backup_stale", Severity: alerts.SeverityCritical,
			Check: func(context.Context) (bool, string, error) {
				return backupRule(manifest, time.Now())
			},
		})
	}
	ev := &alerts.Evaluator{Rules: rules, Notifier: notifier}
	fired, err := ev.Eval(ctx)
	if err != nil {
		return err
	}
	for _, a := range fired {
		fmt.Fprintf(os.Stdout, "FIRING name=%s severity=%s detail=%s\n", a.Name, a.Severity, a.Detail)
	}
	if len(fired) > 0 {
		return fmt.Errorf("%d alert(s) firing", len(fired))
	}
	fmt.Fprintln(os.Stdout, "monitoring ok: no alerts firing")
	return nil
}

// deadRule fires when dead-lettered jobs await audited replay.
func deadRule(dead int64) (bool, string, error) {
	if dead > 0 {
		return true, fmt.Sprintf("dead=%d", dead), nil
	}
	return false, "", nil
}

// stuckRule fires when the oldest queued job exceeds the stuck budget.
// An empty queue never fires, however old the snapshot.
func stuckRule(queued int64, oldest time.Time, now time.Time) (bool, string, error) {
	if queued == 0 || oldest.IsZero() {
		return false, "", nil
	}
	if age := now.Sub(oldest); age > stuckJobThreshold {
		return true, fmt.Sprintf("oldest_queued_seconds=%d", int64(age.Seconds())), nil
	}
	return false, "", nil
}

// backupRule fires when the newest backup manifest exceeds the fresh
// window. Missing manifests fail loudly (blind freshness never counts
// as green).
func backupRule(manifest string, now time.Time) (bool, string, error) {
	info, err := os.Stat(manifest)
	if err != nil {
		return false, "", fmt.Errorf("backup manifest: %w", err)
	}
	if age := now.Sub(info.ModTime()); age > backupFreshHours*time.Hour {
		return true, fmt.Sprintf("age_hours=%d", int64(age.Hours())), nil
	}
	return false, "", nil
}

// multiNotifier fans out to the log route plus one webhook.
type multiNotifier struct {
	alerts.Notifier
	extra alerts.Notifier
}

func (m multiNotifier) Notify(ctx context.Context, alert alerts.Alert) error {
	if err := m.Notifier.Notify(ctx, alert); err != nil {
		return err
	}
	return m.extra.Notify(ctx, alert)
}
