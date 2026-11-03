package httpserver

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry"
)

// The child uses the real signal lifecycle and real TCP sockets, including
// an upload body that stays in flight past endpoint withdrawal.
func TestDrainSignalChild(t *testing.T) {
	if os.Getenv("ABASTEVO_DRAIN_TEST_CHILD") != "1" {
		return
	}
	logger, _ := telemetry.New(io.Discard, "info", telemetry.RoleAPI)
	var started atomic.Bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/upload" {
			var first [1]byte
			if _, err := io.ReadFull(r.Body, first[:]); err != nil {
				t.Error(err)
				return
			}
			started.Store(true)
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				t.Error(err)
				return
			}
		}
		if r.URL.Path == "/started" && !started.Load() {
			w.WriteHeader(409)
			return
		}
		w.WriteHeader(200)
	})
	s, err := New(Options{Addr: "127.0.0.1:0", ReadHeaderTimeout: time.Second,
		ShutdownTimeout: 3 * time.Second, ShutdownDrainDelay: time.Second,
		MaxBodyBytes: 1 << 20, Logger: logger}, h)
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	fmt.Println(s.Addr())
	if err := s.Run(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSIGTERMWithdrawsReadinessBeforeClosingListener(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestDrainSignalChild$")
	cmd.Env = append(os.Environ(), "ABASTEVO_DRAIN_TEST_CHILD=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		t.Fatal("child did not publish its listener")
	}
	base := "http://" + scanner.Text()
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	t.Cleanup(func() { client.CloseIdleConnections() })
	status := func(path string) int {
		r, err := client.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		_, _ = io.Copy(io.Discard, r.Body)
		return r.StatusCode
	}
	if status("/health/ready") != 200 {
		t.Fatal("initial readiness")
	}
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close(); _ = reader.Close() })
	uploadDone := make(chan error, 1)
	go func() {
		r, err := client.Post(base+"/upload", "application/octet-stream", reader)
		if err == nil {
			_, _ = io.Copy(io.Discard, r.Body)
			_ = r.Body.Close()
			if r.StatusCode != 200 {
				err = fmt.Errorf("upload: %d", r.StatusCode)
			}
		}
		uploadDone <- err
	}()
	if _, err := writer.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for status("/started") != 200 {
		if time.Now().After(deadline) {
			t.Fatal("upload not started")
		}
		time.Sleep(time.Millisecond)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(500 * time.Millisecond)
	for status("/health/ready") != 503 {
		if time.Now().After(deadline) {
			t.Fatal("readiness did not withdraw")
		}
		time.Sleep(time.Millisecond)
	}
	for i := 0; i < 5; i++ {
		if status("/normal") != 200 || status("/health/live") != 200 {
			t.Fatal("traffic refused during drain")
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Keep the upload in flight after the listener enters http.Shutdown.
	time.Sleep(700 * time.Millisecond)
	if _, err := io.Copy(writer, strings.NewReader(strings.Repeat("x", 64*1024))); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-uploadDone; err != nil {
		t.Fatal(err)
	}
	for scanner.Scan() {
	} // Drain test-process output before waiting.
	if err := cmd.Wait(); err != nil {
		t.Fatalf("unclean signal exit: %v", err)
	}
}
