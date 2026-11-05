// station-sync is a short-lived, daily official registry refresh. Kubernetes
// owns cadence/restarts; the existing preparer and loader own their contracts.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	syncer "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registrysync"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func event(code string, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["event"] = code
	fields["at"] = time.Now().UTC().Format(time.RFC3339Nano)
	_ = json.NewEncoder(os.Stdout).Encode(fields)
}
func main() {
	if err := run(); err != nil {
		event("refresh_failed", map[string]any{"reason": err.Error()})
		os.Exit(1)
	}
}
func run() error {
	referenceDir := flag.String("reference-dir", "", "write aliases.json from a local ibge.json and exit")
	root := flag.String("root", "/state", "private persistent state directory")
	preparer := flag.String("prepare", "/tools/prepare_registry", "trusted Rust executable")
	loader := flag.String("loader", "/tools/station-load", "trusted Go loader executable")
	nameInputs := flag.String("name-inputs", "", "optional pinned local names manifest; defaults to ROOT/name-inputs/manifest.json when present")
	opencnpj := flag.String("opencnpj", "/tools/opencnpj", "trusted offline OpenCNPJ executable")
	opencnpjRoot := flag.String("opencnpj-root", "", "private published OpenCNPJ source directory")
	rfbUpdate := flag.Bool("rfb-update", false, "refresh and verify the monthly RFB snapshot and names projection; never accesses the application DB")
	namesDir := flag.String("names-dir", "", "prepare minimized names for registry.csv in this directory, then exit without DB access")
	timeout := flag.Duration("timeout", 18*time.Minute, "overall bounded refresh duration")
	budgetWait := flag.Duration("budget-wait", 6*time.Minute, "bounded wait for the host safety budget before starting (inside --timeout)")
	flag.Parse()
	if *referenceDir != "" {
		return syncer.WriteAliases(*referenceDir)
	}
	maximum := 20 * time.Minute
	if *rfbUpdate {
		maximum = 2 * time.Hour
	}
	needsDB := !(*rfbUpdate || *namesDir != "")
	if *timeout <= 0 || *timeout > maximum || *budgetWait < 0 || *budgetWait >= *timeout/2 || (needsDB && os.Getenv("ANPFUEL_DATABASE_URL") == "") || !filepath.IsAbs(*root) || !filepath.IsAbs(*preparer) || !filepath.IsAbs(*loader) || (*opencnpjRoot != "" && (!filepath.IsAbs(*opencnpjRoot) || !filepath.IsAbs(*opencnpj))) || (*rfbUpdate && (*opencnpjRoot != *root || *namesDir != "")) {
		return fmt.Errorf("invalid bounded refresh configuration")
	}
	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(signalContext, *timeout)
	defer cancel()
	provider := syncer.OpenCNPJNames{Binary: *opencnpj, DataRoot: *opencnpjRoot, CacheRoot: filepath.Join(*root, "name-cache"), RequireSeed: needsDB, Now: time.Now}
	if *namesDir != "" {
		if *opencnpjRoot == "" || !filepath.IsAbs(*namesDir) {
			return fmt.Errorf("offline name preparation configuration missing")
		}
		path, err := provider.Generate(ctx, *namesDir)
		if err != nil {
			return err
		}
		event("names_prepared", map[string]any{"manifest": path})
		return nil
	}
	safetyClient := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("readiness redirect refused") }}
	check := func(ctx context.Context) error {
		if err := safety(ctx, *root, safetyClient); err != nil {
			return err
		}
		if *rfbUpdate {
			return syncer.CheckOpenCNPJDisk(*root)
		}
		return nil
	}
	if err := waitForBudget(ctx, check, *budgetWait, 15*time.Second, func(err error) {
		event("budget_deferred", map[string]any{"reason": err.Error()})
	}); err != nil {
		return err
	}
	done := make(chan struct{})
	guardError := make(chan error, 1)
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := check(ctx); err != nil {
					guardError <- err
					cancel()
					return
				}
			}
		}
	}()
	if *rfbUpdate {
		updater := syncer.SourceUpdater{Root: *root, Binary: *opencnpj, Now: time.Now, Info: provider.Info, Invoke: func(ctx context.Context, stage, binary string, args []string) error {
			return child(ctx, stage, binary, args, false)
		}}
		err := updater.Update(ctx)
		if err == nil {
			err = provider.PrepareMonthly(ctx, &http.Client{Timeout: 30 * time.Second})
		}
		cancel()
		<-done
		select {
		case failure := <-guardError:
			return failure
		default:
		}
		if err != nil {
			return err
		}
		event("rfb_refresh_complete", nil)
		return nil
	}
	r := syncer.Runner{Root: *root, Now: time.Now, Client: &http.Client{Timeout: 30 * time.Second}, Observe: func(code string) { event(code, nil) }, ValidatePrepared: syncer.ValidateRegistryAdmission}
	r.Analyze = func(previous, current string) error {
		report, err := syncer.AnalyzeRegistryEdition(previous, current)
		event("registry_edition", map[string]any{"admitted": err == nil, "source_sha256": report.SourceSHA256, "previous_sha256": report.PreviousSHA256, "rows": report.Rows, "added": report.Added, "changed": report.Changed, "missing": report.Missing, "unchanged": report.Unchanged})
		return err
	}
	r.NameInputs = *nameInputs
	if *opencnpjRoot != "" {
		r.GenerateNames = provider.Generate
	}
	if r.NameInputs == "" && r.GenerateNames == nil {
		candidate := filepath.Join(*root, "name-inputs", "manifest.json")
		if _, err := os.Lstat(candidate); err == nil {
			r.NameInputs = candidate
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("name input configuration unavailable")
		}
	}
	r.Prepare = func(ctx context.Context, dir string, s syncer.Snapshot) error {
		args := []string{filepath.Join(dir, "registry.csv"), filepath.Join(dir, "aliases.json"), filepath.Join(dir, "prepared"), syncer.ANPURL, "content-sha256:" + s.SourceHash, s.RunID, s.FetchedAt}
		if s.NameInputsHash != "" {
			args = append(args, "--name-inputs", filepath.Join(dir, "names", "manifest.json"))
		}
		return child(ctx, "prepare", *preparer, args, false)
	}
	r.Apply = func(ctx context.Context, dir string) error {
		return child(ctx, "load_publish", *loader, []string{"--emit-dir", dir, "--publish", "--attempts", "3", "--timeout", "14m"}, true)
	}
	r.Verify = func(ctx context.Context, dir string) error {
		return child(ctx, "verify", *loader, []string{"--emit-dir", dir, "--verify", "--timeout", "1m"}, true)
	}
	started := time.Now()
	err := r.Run(ctx)
	cancel()
	<-done
	select {
	case failure := <-guardError:
		return failure
	default:
	}
	if err != nil {
		return err
	}
	event("refresh_complete", map[string]any{"seconds": time.Since(started).Seconds(), "container_memory_peak_bytes": memoryPeak()})
	return nil
}
func child(ctx context.Context, stage, binary string, args []string, db bool) error {
	started := time.Now()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = []string{"LANG=C.UTF-8", "TZ=UTC", "GOMAXPROCS=1", "GOMEMLIMIT=64MiB"}
	if db {
		cmd.Env = append(cmd.Env, "ANPFUEL_DATABASE_URL="+os.Getenv("ANPFUEL_DATABASE_URL"))
	}
	// Source/library errors can contain input fragments; logs expose only stages,
	// exit status and aggregate timing/resource counters, never child output.
	var output countsBuffer
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	err := cmd.Run()
	fields := map[string]any{"stage": stage, "seconds": time.Since(started).Seconds(), "ok": err == nil}
	if cmd.ProcessState != nil {
		usage := cmd.ProcessState.SysUsage().(*syscall.Rusage)
		fields["child_max_rss_kib"] = usage.Maxrss
		fields["child_cpu_seconds"] = cmd.ProcessState.UserTime().Seconds() + cmd.ProcessState.SystemTime().Seconds()
	}
	fields["counts"] = output.counts()
	event("stage_complete", fields)
	if err != nil {
		return fmt.Errorf("refresh stage %s failed", stage)
	}
	return nil
}

// At most4KiB of known numeric CLI counters are retained. Other child output
// is discarded, including source fragments and persistence error strings.
type countsBuffer struct{ bytes.Buffer }

func (b *countsBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < 4096 {
		keep := 4096 - b.Len()
		if len(p) > keep {
			p = p[:keep]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}
func (b *countsBuffer) counts() map[string]uint64 {
	result := map[string]uint64{}
	allowed := map[string]bool{"input": true, "accepted": true, "duplicates": true, "quarantined": true, "rejected": true, "registry_assertions": true, "preserved_curated": true}
	for _, token := range strings.Fields(b.String()) {
		key, value, ok := strings.Cut(token, "=")
		if ok && allowed[key] {
			if count, err := strconv.ParseUint(value, 10, 64); err == nil {
				result[key] = count
			}
		}
	}
	return result
}
