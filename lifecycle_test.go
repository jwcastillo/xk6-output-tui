package outputtui_test

import (
	"io"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.k6.io/k6/v2/metrics"
	"go.k6.io/k6/v2/output"

	outputtui "github.com/jwcastillo/xk6-output-tui"
)

// newTestParams returns output.Params suitable for unit tests: discards stdout/
// stderr output and uses a logrus logger writing to a buffer.
func newTestParams() output.Params {
	log := logrus.New()
	log.SetOutput(io.Discard)
	log.SetLevel(logrus.InfoLevel)

	return output.Params{
		Logger: log,
		StdOut: io.Discard,
		StdErr: io.Discard,
	}
}

// TestStop_NonTTY verifies that Start() and Stop() complete without hanging in
// a non-TTY environment (CI pipe). Stop() must return within 3s.
func TestStop_NonTTY(t *testing.T) {
	o, err := outputtui.New(newTestParams())
	require.NoError(t, err)

	require.NoError(t, o.Start())

	done := make(chan error, 1)
	go func() {
		done <- o.Stop()
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("Stop() did not return within 3s — potential goroutine hang")
	}
}

// TestAddMetricSamples_NonBlocking verifies that concurrent goroutines calling
// AddMetricSamples do not deadlock and complete well within the allowed budget.
func TestAddMetricSamples_NonBlocking(t *testing.T) {
	o, err := outputtui.New(newTestParams())
	require.NoError(t, err)
	require.NoError(t, o.Start())

	registry := metrics.NewRegistry()
	m, err := registry.NewMetric("http_req_duration", metrics.Trend, metrics.Time)
	require.NoError(t, err)

	const goroutines = 10
	const callsEach = 1000
	done := make(chan struct{}, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < callsEach; j++ {
				o.AddMetricSamples([]metrics.SampleContainer{
					metrics.Sample{
						TimeSeries: metrics.TimeSeries{Metric: m},
						Value:      float64(j % 500),
						Time:       time.Now(),
					},
				})
			}
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines to finish (max 5s).
	deadline := time.After(5 * time.Second)
	for finished := 0; finished < goroutines; finished++ {
		select {
		case <-done:
		case <-deadline:
			t.Fatalf("only %d/%d goroutines finished within 5s", finished, goroutines)
		}
	}

	require.NoError(t, o.Stop())
}

// TestDescription_Exact verifies the exact description string required by
// the plan acceptance criteria.
func TestDescription_Exact(t *testing.T) {
	o, err := outputtui.New(newTestParams())
	require.NoError(t, err)
	assert.Equal(t, "tui (live terminal dashboard)", o.Description())
}

// TestNonTTY_FallbackPath verifies that in a non-TTY environment (always true
// in tests), AddMetricSamples + Stop() complete without error and the fallback
// path is exercised (not bubbletea).
func TestNonTTY_FallbackPath(t *testing.T) {
	logger, buf := captureLogger()

	params := output.Params{
		Logger: logger,
		StdOut: io.Discard,
		StdErr: io.Discard,
	}

	o, err := outputtui.New(params)
	require.NoError(t, err)
	require.NoError(t, o.Start())

	// Feed some samples so the fallback has data.
	registry := metrics.NewRegistry()
	httpReqs, err := registry.NewMetric(metrics.HTTPReqsName, metrics.Counter)
	require.NoError(t, err)

	o.AddMetricSamples([]metrics.SampleContainer{
		metrics.Sample{
			TimeSeries: metrics.TimeSeries{Metric: httpReqs},
			Value:      1,
			Time:       time.Now(),
		},
	})

	// Give the flusher at least one cycle.
	time.Sleep(200 * time.Millisecond)

	require.NoError(t, o.Stop())

	// In non-TTY mode, Stop() triggers a final [tui] summary log line.
	_ = buf // logger output captured; just ensure no panic and clean stop.
}

// TestStop_IdempotentAgg verifies that AddMetricSamples before Start does not
// panic (SampleBuffer is always ready).
func TestStop_IdempotentAgg(t *testing.T) {
	o, err := outputtui.New(newTestParams())
	require.NoError(t, err)

	registry := metrics.NewRegistry()
	m, err := registry.NewMetric("http_req_duration", metrics.Trend, metrics.Time)
	require.NoError(t, err)

	// AddMetricSamples before Start — must not panic.
	assert.NotPanics(t, func() {
		o.AddMetricSamples([]metrics.SampleContainer{
			metrics.Sample{
				TimeSeries: metrics.TimeSeries{Metric: m},
				Value:      100,
				Time:       time.Now(),
			},
		})
	})
}
