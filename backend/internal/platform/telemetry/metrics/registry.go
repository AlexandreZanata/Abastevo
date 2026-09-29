package metrics

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// MaxSeriesPerMetric caps the series one metric may hold: a mislabeled
// caller fails loudly instead of growing memory with request data.
const MaxSeriesPerMetric = 512

var (
	ErrBadName       = errors.New("metrics: invalid metric or label name")
	ErrCardinality   = errors.New("metrics: series cap exceeded")
	namePattern      = regexp.MustCompile(`^[a-z_:][a-z0-9_:]*$`)
	reservedCounters = map[string]bool{"metrics_dropped_total": true}
)

func validName(s string) bool { return namePattern.MatchString(s) }

// seriesKey joins sorted label pairs into a stable map key.
func seriesKey(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte(0)
		b.WriteString(labels[k])
		b.WriteByte(0)
	}
	return b.String()
}

// Registry owns every metric. The zero value is unusable; build with
// NewRegistry.
type Registry struct {
	mu       sync.Mutex
	counters map[string]*Counter
	gauges   map[string]*Gauge
	funcs    map[string]*GaugeFunc
	dropped  int64
}

// NewRegistry builds an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		counters: map[string]*Counter{},
		gauges:   map[string]*Gauge{},
		funcs:    map[string]*GaugeFunc{},
	}
}

// Counter is a monotonically increasing total partitioned by labels.
type Counter struct {
	name   string
	help   string
	labels []string
	mu     sync.Mutex
	series map[string]float64
	order  []string
	values map[string]map[string]string
}

// Gauge is a point-in-time value partitioned by labels.
type Gauge struct {
	name   string
	help   string
	labels []string
	mu     sync.Mutex
	series map[string]float64
	order  []string
	values map[string]map[string]string
}

// GaugeFunc evaluates once per scrape: cheap readers only (pool stats,
// queue depth), never blocking I/O inside the scrape path beyond a
// bounded query.
type GaugeFunc struct {
	name string
	help string
	fn   func() float64
}

func checkName(name string, labels []string) error {
	if !validName(name) || reservedCounters[name] {
		return fmt.Errorf("%w: %q", ErrBadName, name)
	}
	seen := map[string]bool{}
	for _, l := range labels {
		if !validName(l) || seen[l] {
			return fmt.Errorf("%w: %q", ErrBadName, l)
		}
		seen[l] = true
	}
	return nil
}

func checkValues(names []string, values map[string]string) (map[string]string, error) {
	if len(values) != len(names) {
		return nil, fmt.Errorf("%w: label arity", ErrBadName)
	}
	out := make(map[string]string, len(values))
	for _, n := range names {
		v, ok := values[n]
		if !ok || strings.ContainsAny(v, "\n\"\\") {
			return nil, fmt.Errorf("%w: label value", ErrBadName)
		}
		out[n] = v
	}
	return out, nil
}

// Counter registers (or returns) a counter with fixed label names.
func (r *Registry) Counter(name, help string, labels ...string) (*Counter, error) {
	if err := checkName(name, labels); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.counters[name]; ok {
		if strings.Join(c.labels, ",") != strings.Join(labels, ",") {
			return nil, fmt.Errorf("%w: redefined labels", ErrBadName)
		}
		return c, nil
	}
	c := &Counter{name: name, help: help, labels: append([]string{}, labels...), series: map[string]float64{}, values: map[string]map[string]string{}}
	r.counters[name] = c
	return c, nil
}

// Gauge registers (or returns) a gauge with fixed label names.
func (r *Registry) Gauge(name, help string, labels ...string) (*Gauge, error) {
	if err := checkName(name, labels); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok := r.gauges[name]; ok {
		if strings.Join(g.labels, ",") != strings.Join(labels, ",") {
			return nil, fmt.Errorf("%w: redefined labels", ErrBadName)
		}
		return g, nil
	}
	g := &Gauge{name: name, help: help, labels: append([]string{}, labels...), series: map[string]float64{}, values: map[string]map[string]string{}}
	r.gauges[name] = g
	return g, nil
}

// GaugeFunc registers a scrape-time gauge.
func (r *Registry) GaugeFunc(name, help string, fn func() float64) (*GaugeFunc, error) {
	if err := checkName(name, nil); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok := r.funcs[name]; ok {
		return g, nil
	}
	g := &GaugeFunc{name: name, help: help, fn: fn}
	r.funcs[name] = g
	return g, nil
}

// Dropped counts series refused by the cardinality cap.
func (r *Registry) Dropped() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dropped
}

func (r *Registry) refuse() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dropped++
}

// Inc adds one to the named series, dropping (and counting) series
// past the cap instead of growing memory.
func (c *Counter) Inc(r *Registry, values map[string]string) error {
	checked, err := checkValues(c.labels, values)
	if err != nil {
		return err
	}
	key := seriesKey(checked)
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.series[key]; !ok {
		if len(c.series) >= MaxSeriesPerMetric {
			r.refuse()
			return ErrCardinality
		}
		c.order = append(c.order, key)
		c.values[key] = checked
	}
	c.series[key]++
	return nil
}

// Add adds n to the named series.
func (c *Counter) Add(r *Registry, n float64, values map[string]string) error {
	checked, err := checkValues(c.labels, values)
	if err != nil {
		return err
	}
	key := seriesKey(checked)
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.series[key]; !ok {
		if len(c.series) >= MaxSeriesPerMetric {
			r.refuse()
			return ErrCardinality
		}
		c.order = append(c.order, key)
		c.values[key] = checked
	}
	c.series[key] += n
	return nil
}

// Set replaces the named series value.
func (g *Gauge) Set(r *Registry, v float64, values map[string]string) error {
	checked, err := checkValues(g.labels, values)
	if err != nil {
		return err
	}
	key := seriesKey(checked)
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.series[key]; !ok {
		if len(g.series) >= MaxSeriesPerMetric {
			r.refuse()
			return ErrCardinality
		}
		g.order = append(g.order, key)
		g.values[key] = checked
	}
	g.series[key] = v
	return nil
}
