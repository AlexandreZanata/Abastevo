package main

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func budget(availableKiB uint64, load float64, cpus int, freeBytes uint64) error {
	if availableKiB < 12*1024*1024 {
		return fmt.Errorf("host available RAM below 12GiB reserve")
	}
	if math.IsNaN(load) || math.IsInf(load, 0) || load < 0 || cpus < 1 || load > float64(cpus)*0.65 {
		return fmt.Errorf("host load1 exceeds conservative 0.65 x CPU-count reserve")
	}
	if freeBytes < 50*1024*1024*1024 {
		return fmt.Errorf("host disk below 50GiB reserve")
	}
	return nil
}
func safety(ctx context.Context, root string, client *http.Client) error {
	memory, err := os.Open("/proc/meminfo")
	if err != nil {
		return fmt.Errorf("host memory telemetry unavailable")
	}
	defer memory.Close()
	var available uint64
	scan := bufio.NewScanner(memory)
	for scan.Scan() {
		fields := strings.Fields(scan.Text())
		if len(fields) == 3 && fields[0] == "MemAvailable:" && fields[2] == "kB" {
			available, err = strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return fmt.Errorf("host memory telemetry invalid")
			}
		}
	}
	if scan.Err() != nil || available == 0 {
		return fmt.Errorf("host memory telemetry missing")
	}
	raw, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return fmt.Errorf("host load telemetry unavailable")
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return fmt.Errorf("host load telemetry missing")
	}
	load, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return fmt.Errorf("host load telemetry invalid")
	}
	var disk syscall.Statfs_t
	if err = syscall.Statfs(root, &disk); err != nil {
		return fmt.Errorf("host disk telemetry unavailable")
	}
	if err = budget(available, load, runtime.NumCPU(), disk.Bavail*uint64(disk.Bsize)); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://teste.abastevo.com.br/health/ready", nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "abastevo-rst-qualified/1")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("abastevo HTTPS readiness unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("abastevo HTTPS readiness refused")
	}
	return nil
}

// waitForBudget re-runs the host safety check until it passes or the bounded
// wait expires. A transient spike at the scheduled minute defers the refresh
// instead of failing it; a persistent refusal still fails closed with the
// last reason. The in-run guard keeps aborting on any later breach.
func waitForBudget(ctx context.Context, check func(context.Context) error, wait, interval time.Duration, observe func(error)) error {
	deadline := time.Now().Add(wait)
	for {
		err := check(ctx)
		if err == nil {
			return nil
		}
		if !time.Now().Add(interval).Before(deadline) {
			return err
		}
		observe(err)
		select {
		case <-ctx.Done():
			return err
		case <-time.After(interval):
		}
	}
}

func memoryPeak() any {
	raw, err := os.ReadFile("/sys/fs/cgroup/memory.peak")
	if err != nil {
		return nil
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return nil
	}
	return value
}
