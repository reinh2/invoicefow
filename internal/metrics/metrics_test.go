package metrics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func render(t *testing.T, registry *Registry) string {
	t.Helper()
	var builder strings.Builder
	if err := registry.WriteText(context.Background(), &builder); err != nil {
		t.Fatalf("write: %v", err)
	}
	return builder.String()
}

func TestCounterRendersSortedLabelsAndAccumulates(t *testing.T) {
	registry := NewRegistry()
	counter := registry.Counter("invoiceflow_jobs_total", "Jobs.", "outcome")
	counter.Inc("success")
	counter.Inc("success")
	counter.Inc("dead_letter")
	// A negative delta is a caller bug; a counter must never go backwards.
	counter.Add("success", -5)

	if got := counter.Value("success"); got != 2 {
		t.Fatalf("success counter = %d, want 2", got)
	}
	out := render(t, registry)
	want := "# HELP invoiceflow_jobs_total Jobs.\n# TYPE invoiceflow_jobs_total counter\n" +
		"invoiceflow_jobs_total{outcome=\"dead_letter\"} 1\n" +
		"invoiceflow_jobs_total{outcome=\"success\"} 2\n"
	if out != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestHistogramBucketsAreCumulative(t *testing.T) {
	registry := NewRegistry()
	histogram := registry.Histogram("d_seconds", "Duration.", []float64{1, 5})
	histogram.Observe(0.5)
	histogram.Observe(3)
	histogram.Observe(120) // Above every bound: counted only in +Inf.

	if histogram.Count() != 3 {
		t.Fatalf("count = %d, want 3", histogram.Count())
	}
	out := render(t, registry)
	for _, line := range []string{
		`d_seconds_bucket{le="1"} 1`,
		`d_seconds_bucket{le="5"} 2`,
		`d_seconds_bucket{le="+Inf"} 3`,
		`d_seconds_count 3`,
	} {
		if !strings.Contains(out, line) {
			t.Fatalf("missing %q in:\n%s", line, out)
		}
	}
}

// One failing gauge must not fail the whole scrape: the remaining instruments
// still have to render, or a single database hiccup blinds every metric.
func TestGaugeCollectionFailureSuppressesOnlyThatGauge(t *testing.T) {
	registry := NewRegistry()
	registry.Counter("kept_total", "Kept.", "outcome").Inc("success")
	registry.GaugeFunc("broken", "Broken.", "status", func(context.Context) ([]LabeledValue, error) {
		return nil, errors.New("database is down")
	})

	out := render(t, registry)
	if !strings.Contains(out, `kept_total{outcome="success"} 1`) {
		t.Fatalf("counter was lost:\n%s", out)
	}
	if !strings.Contains(out, "# broken collection failed") {
		t.Fatalf("failure was not reported:\n%s", out)
	}
	// The driver's own message must not reach the exposition.
	if strings.Contains(out, "database is down") {
		t.Fatalf("collector error text leaked:\n%s", out)
	}
}

func TestGaugeFuncRendersSortedSamples(t *testing.T) {
	registry := NewRegistry()
	registry.GaugeFunc("invoiceflow_documents", "Documents.", "status", func(context.Context) ([]LabeledValue, error) {
		return []LabeledValue{{Label: "queued", Value: 3}, {Label: "approved", Value: 1}}, nil
	})
	out := render(t, registry)
	approved := strings.Index(out, `status="approved"`)
	queued := strings.Index(out, `status="queued"`)
	if approved < 0 || queued < 0 || approved > queued {
		t.Fatalf("samples are not sorted:\n%s", out)
	}
}

func TestLabelValueEscaping(t *testing.T) {
	if got := escapeLabelValue("a\"b\\c\nd"); got != `a\"b\\c\nd` {
		t.Fatalf("got %q", got)
	}
}

func TestHandlerServesOnlyMetricsAndSetsSafeHeaders(t *testing.T) {
	registry := NewRegistry()
	registry.Counter("kept_total", "Kept.", "outcome").Inc("success")
	handler := registry.Handler()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("content type %q", got)
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff")
	}

	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/", nil),
		httptest.NewRequest(http.MethodPost, "/metrics", nil),
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code == http.StatusOK {
			t.Fatalf("%s %s was served", request.Method, request.URL.Path)
		}
	}
}

type stubSource struct {
	counts StatusCounts
	calls  int
}

func (s *stubSource) StatusCounts(context.Context) (StatusCounts, error) {
	s.calls++
	return s.counts, nil
}

func TestWorkerInstrumentsExposeJobOutcomesAndStatusGauges(t *testing.T) {
	source := &stubSource{counts: StatusCounts{
		DocumentsByStatus: []LabeledValue{{Label: "needs_review", Value: 2}},
		JobsByStatus:      []LabeledValue{{Label: "ready", Value: 1}},
	}}
	worker := NewWorker(source)
	worker.ProcessJobs.Inc("success")
	worker.ProcessJobs.Inc("dead_letter")
	worker.ExportJobs.Inc("retry")
	worker.ExtractionDuration.Observe(0.4)

	out := render(t, worker.Registry)
	for _, line := range []string{
		`invoiceflow_process_jobs_total{outcome="success"} 1`,
		`invoiceflow_process_jobs_total{outcome="dead_letter"} 1`,
		`invoiceflow_export_jobs_total{outcome="retry"} 1`,
		`invoiceflow_extraction_duration_seconds_count 1`,
		`invoiceflow_documents{status="needs_review"} 2`,
		`invoiceflow_jobs{status="ready"} 1`,
	} {
		if !strings.Contains(out, line) {
			t.Fatalf("missing %q in:\n%s", line, out)
		}
	}
}

// Without a status source the database-backed gauges must be absent rather than
// reporting zeroes, which would read as an empty, healthy system.
func TestWorkerWithoutSourceOmitsDatabaseGauges(t *testing.T) {
	out := render(t, NewWorker(nil).Registry)
	if strings.Contains(out, "invoiceflow_documents") || strings.Contains(out, "invoiceflow_jobs{") {
		t.Fatalf("database gauges were registered without a source:\n%s", out)
	}
}
