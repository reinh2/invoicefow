// Package metrics is a small, dependency-free instrument registry that renders
// the Prometheus text exposition format.
//
// It is deliberately minimal: counters and gauges carry at most one label
// dimension, and every label value in this project is server-owned (a job
// outcome or a database status), never client input. That keeps the exposition
// free of unbounded cardinality without needing a full client library.
//
// The registry exposes operational volume, so its endpoint is not part of the
// public API and is served on its own listener (see ADR-017).
package metrics

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// LabeledValue is one gauge sample: a label value and its current count.
type LabeledValue struct {
	Label string
	Value int64
}

type collector interface {
	write(ctx context.Context, w io.Writer) error
}

// Registry owns a set of instruments and renders them on demand.
type Registry struct {
	mu         sync.Mutex
	collectors []collector
	names      map[string]bool
}

func NewRegistry() *Registry { return &Registry{names: map[string]bool{}} }

func (r *Registry) add(name string, c collector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.names[name] {
		panic("metrics: duplicate instrument " + name)
	}
	r.names[name] = true
	r.collectors = append(r.collectors, c)
}

// Counter registers a monotonically increasing counter with one label
// dimension. Label values must be server-owned constants.
func (r *Registry) Counter(name, help, label string) *Counter {
	c := &Counter{name: name, help: help, label: label, values: map[string]int64{}}
	r.add(name, c)
	return c
}

// Histogram registers a cumulative histogram over the given upper bounds, in
// the metric's own unit. Bounds must be sorted ascending.
func (r *Registry) Histogram(name, help string, bounds []float64) *Histogram {
	h := &Histogram{name: name, help: help, bounds: append([]float64(nil), bounds...), counts: make([]int64, len(bounds))}
	r.add(name, h)
	return h
}

// GaugeFunc registers a gauge whose samples are collected at scrape time. It is
// used for values that live in the database (queue depth, documents by status)
// rather than in process memory, so a restarted process reports the truth
// immediately instead of counting up from zero.
//
// A collection error is rendered as a comment and suppresses only that gauge:
// one unavailable query must not fail the whole scrape.
func (r *Registry) GaugeFunc(name, help, label string, collect func(context.Context) ([]LabeledValue, error)) {
	r.add(name, &gaugeFunc{name: name, help: help, label: label, collect: collect})
}

// WriteText renders every instrument in registration order.
func (r *Registry) WriteText(ctx context.Context, w io.Writer) error {
	r.mu.Lock()
	collectors := append([]collector(nil), r.collectors...)
	r.mu.Unlock()
	for _, c := range collectors {
		if err := c.write(ctx, w); err != nil {
			return err
		}
	}
	return nil
}

// Counter is a labeled monotonic counter.
type Counter struct {
	name, help, label string
	mu                sync.Mutex
	values            map[string]int64
}

func (c *Counter) Inc(labelValue string) { c.Add(labelValue, 1) }

func (c *Counter) Add(labelValue string, delta int64) {
	if delta < 0 {
		return // A counter never decreases; a negative delta is a caller bug, not data.
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[labelValue] += delta
}

// Value reports the current count, for tests.
func (c *Counter) Value(labelValue string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.values[labelValue]
}

func (c *Counter) write(_ context.Context, w io.Writer) error {
	c.mu.Lock()
	labels := make([]string, 0, len(c.values))
	for label := range c.values {
		labels = append(labels, label)
	}
	values := make(map[string]int64, len(c.values))
	for label, value := range c.values {
		values[label] = value
	}
	c.mu.Unlock()
	sort.Strings(labels) // Stable output makes a scrape diffable.

	if err := writeHeader(w, c.name, c.help, "counter"); err != nil {
		return err
	}
	for _, label := range labels {
		if _, err := fmt.Fprintf(w, "%s{%s=\"%s\"} %d\n", c.name, c.label, escapeLabelValue(label), values[label]); err != nil {
			return err
		}
	}
	return nil
}

// Histogram is a cumulative histogram with a fixed bound set.
type Histogram struct {
	name, help string
	bounds     []float64
	mu         sync.Mutex
	counts     []int64
	count      int64
	sum        float64
}

// Observe records one sample. Values above the last bound land only in +Inf.
func (h *Histogram) Observe(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.count++
	h.sum += value
	for index, bound := range h.bounds {
		if value <= bound {
			h.counts[index]++
		}
	}
}

// Count reports how many samples were observed, for tests.
func (h *Histogram) Count() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count
}

func (h *Histogram) write(_ context.Context, w io.Writer) error {
	h.mu.Lock()
	counts := append([]int64(nil), h.counts...)
	count, sum := h.count, h.sum
	h.mu.Unlock()

	if err := writeHeader(w, h.name, h.help, "histogram"); err != nil {
		return err
	}
	for index, bound := range h.bounds {
		if _, err := fmt.Fprintf(w, "%s_bucket{le=\"%s\"} %d\n", h.name, strconv.FormatFloat(bound, 'g', -1, 64), counts[index]); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "%s_bucket{le=\"+Inf\"} %d\n", h.name, count); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s_sum %s\n", h.name, strconv.FormatFloat(sum, 'g', -1, 64)); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "%s_count %d\n", h.name, count)
	return err
}

type gaugeFunc struct {
	name, help, label string
	collect           func(context.Context) ([]LabeledValue, error)
}

func (g *gaugeFunc) write(ctx context.Context, w io.Writer) error {
	samples, err := g.collect(ctx)
	if err != nil {
		// The message is server-owned; the collector must not surface driver
		// detail here. Suppressing one gauge keeps the rest of the scrape usable.
		_, writeErr := fmt.Fprintf(w, "# %s collection failed\n", g.name)
		return writeErr
	}
	if err := writeHeader(w, g.name, g.help, "gauge"); err != nil {
		return err
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].Label < samples[j].Label })
	for _, sample := range samples {
		if _, err := fmt.Fprintf(w, "%s{%s=\"%s\"} %d\n", g.name, g.label, escapeLabelValue(sample.Label), sample.Value); err != nil {
			return err
		}
	}
	return nil
}

func writeHeader(w io.Writer, name, help, kind string) error {
	_, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
	return err
}

// escapeLabelValue applies the exposition-format escaping rules. Every label
// value in this project is a server-owned constant, but escaping is applied
// regardless so a future caller cannot break the format.
func escapeLabelValue(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return replacer.Replace(value)
}
