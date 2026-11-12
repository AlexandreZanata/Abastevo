package adapters

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
)

// Concurrent transfers share one full-decode slot, so the worst-case decode
// working set is paid once per process; a cancelled wait never decodes.
func TestFullDecodesAreSerializedAndWaitHonoursCancellation(t *testing.T) {
	raw, err := os.ReadFile("../../evidence/adapters/media/testdata/valid-4x4.jpg")
	if err != nil {
		t.Fatal(err)
	}
	var running, peak, calls atomic.Int32
	original := decodeImage
	t.Cleanup(func() { decodeImage = original })
	release := make(chan struct{})
	decodeImage = func(raw []byte) error {
		calls.Add(1)
		now := running.Add(1)
		for old := peak.Load(); now > old && !peak.CompareAndSwap(old, now); old = peak.Load() {
		}
		<-release
		running.Add(-1)
		return original(raw)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if mime, err := (JPEGInspector{}).Inspect(t.Context(), raw); err != nil || mime != "image/jpeg" {
				t.Error(mime, err)
			}
		})
	}
	deadline := time.Now().Add(5 * time.Second)
	for running.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithCancel(t.Context())
	waited := make(chan error, 1)
	go func() { _, err := (JPEGInspector{}).Inspect(ctx, raw); waited <- err }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-waited; !errors.Is(err, domain.ErrUnavailable) {
		t.Error("cancelled wait decoded", err)
	}
	close(release)
	wg.Wait()
	if peak.Load() != 1 || calls.Load() != 4 {
		t.Fatal("concurrent or cancelled full decodes", peak.Load(), calls.Load())
	}
}
