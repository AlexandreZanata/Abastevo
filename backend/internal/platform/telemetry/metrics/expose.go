package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// quote escapes one label value for the text exposition format.
func quote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

func formatLabels(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+`="`+quote(values[k])+`"`)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// Expose renders the registry in Prometheus text format: HELP/TYPE
// headers plus one line per series. Scrape-time gauges evaluate here,
// so their functions must stay cheap and non-blocking.
func (r *Registry) Expose() string {
	r.mu.Lock()
	counters := make([]*Counter, 0, len(r.counters))
	for _, c := range r.counters {
		counters = append(counters, c)
	}
	gauges := make([]*Gauge, 0, len(r.gauges))
	for _, g := range r.gauges {
		gauges = append(gauges, g)
	}
	funcs := make([]*GaugeFunc, 0, len(r.funcs))
	for _, f := range r.funcs {
		funcs = append(funcs, f)
	}
	dropped := r.dropped
	r.mu.Unlock()

	sort.Slice(counters, func(i, j int) bool { return counters[i].name < counters[j].name })
	sort.Slice(gauges, func(i, j int) bool { return gauges[i].name < gauges[j].name })
	sort.Slice(funcs, func(i, j int) bool { return funcs[i].name < funcs[j].name })

	var b strings.Builder
	for _, c := range counters {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s counter\n", c.name, c.help, c.name)
		c.mu.Lock()
		for _, key := range c.order {
			fmt.Fprintf(&b, "%s%s %v\n", c.name, formatLabels(c.values[key]), c.series[key])
		}
		c.mu.Unlock()
	}
	for _, g := range gauges {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n", g.name, g.help, g.name)
		g.mu.Lock()
		for _, key := range g.order {
			fmt.Fprintf(&b, "%s%s %v\n", g.name, formatLabels(g.values[key]), g.series[key])
		}
		g.mu.Unlock()
	}
	for _, f := range funcs {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n%s %v\n", f.name, f.help, f.name, f.name, f.fn())
	}
	fmt.Fprintf(&b, "# HELP metrics_dropped_total Refused series past the cardinality cap.\n# TYPE metrics_dropped_total counter\nmetrics_dropped_total %d\n", dropped)
	return b.String()
}

// Handler serves the exposition with a no-store directive: metrics
// stay out of shared caches even on the private listener.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(r.Expose()))
	})
}
