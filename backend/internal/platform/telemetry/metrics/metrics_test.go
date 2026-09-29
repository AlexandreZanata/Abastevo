package metrics

import (
	"strings"
	"sync"
	"testing"
)

func TestNameValidation(t *testing.T) {
	reg := NewRegistry()
	for _, bad := range []string{"", "has space", "9starts-digit", "with-dash", "metrics_dropped_total"} {
		if _, err := reg.Counter(bad, "h"); err == nil {
			t.Errorf("bad counter name accepted: %q", bad)
		}
		if _, err := reg.Gauge(bad, "h"); err == nil {
			t.Errorf("bad gauge name accepted: %q", bad)
		}
	}
	if _, err := reg.Counter("ok_name", "h", "bad-label"); err == nil {
		t.Error("bad label name accepted")
	}
	if _, err := reg.Counter("ok_name", "h", "dup", "dup"); err == nil {
		t.Error("duplicate label names accepted")
	}
	c, err := reg.Counter("http_requests_total", "h", "method")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Inc(reg, map[string]string{"method": "GET", "extra": "x"}); err == nil {
		t.Error("label arity mismatch accepted")
	}
	if err := c.Inc(reg, map[string]string{"method": "a\nb"}); err == nil {
		t.Error("newline label value accepted")
	}
}

func TestCardinalityCap(t *testing.T) {
	reg := NewRegistry()
	c, err := reg.Counter("capped_total", "h", "route")
	if err != nil {
		t.Fatal(err)
	}
	refused := 0
	for i := 0; i < MaxSeriesPerMetric+10; i++ {
		if err := c.Inc(reg, map[string]string{"route": strings.Repeat("r", 3) + string(rune('0'+i/100)) + string(rune('0'+(i/10)%10)) + string(rune('0'+i%10))}); err != nil {
			refused++
		}
	}
	if refused != 10 {
		t.Errorf("refused = %d, want 10", refused)
	}
	if reg.Dropped() != 10 {
		t.Errorf("dropped = %d, want 10", reg.Dropped())
	}
	if !strings.Contains(reg.Expose(), "metrics_dropped_total 10") {
		t.Error("dropped counter missing from exposition")
	}
}

func TestExpositionFormat(t *testing.T) {
	reg := NewRegistry()
	c, _ := reg.Counter("http_requests_total", "Requests.", "method", "route", "class")
	_ = c.Inc(reg, map[string]string{"method": "GET", "route": "/v1/stations/{id}", "class": "2xx"})
	g, _ := reg.Gauge("jobs_queued", "Depth.", "kind")
	_ = g.Set(reg, 3, map[string]string{"kind": "privacy-erasure"})
	_, _ = reg.GaugeFunc("db_pool_acquired", "Acquired.", func() float64 { return 2 })
	out := reg.Expose()
	for _, want := range []string{
		"# TYPE http_requests_total counter",
		`http_requests_total{class="2xx",method="GET",route="/v1/stations/{id}"} 1`,
		"# TYPE jobs_queued gauge",
		`jobs_queued{kind="privacy-erasure"} 3`,
		"db_pool_acquired 2",
		"metrics_dropped_total 0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("exposition missing %q\n%s", want, out)
		}
	}
	// Contributor, station and observation IDs must never appear as
	// label values in this package's vocabulary: route templates and
	// classes only.
	if strings.Contains(out, "tok-") || strings.Contains(out, "45d0e8bc") {
		t.Error("identity leaked into exposition")
	}
}

func TestConcurrentSafety(t *testing.T) {
	reg := NewRegistry()
	c, _ := reg.Counter("race_total", "h", "w")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = c.Inc(reg, map[string]string{"w": "x"})
			}
		}()
	}
	wg.Wait()
	if !strings.Contains(reg.Expose(), "race_total{w=\"x\"} 800") {
		t.Error("lost increments under race")
	}
}
