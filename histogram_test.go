package outputtui

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHistogram_Percentile checks that a uniform distribution 1..1000ms
// yields percentiles within 2% relative error of the theoretical values.
func TestHistogram_Percentile(t *testing.T) {
	h := NewHistogram()

	// Feed 1000 samples: 1ms, 2ms, ..., 1000ms
	for i := 1; i <= 1000; i++ {
		h.Add(float64(i))
	}

	assert.EqualValues(t, 1000, h.count, "count should be 1000")

	// p50 theoretical = 500ms (median of 1..1000)
	p50 := h.Percentile(0.50)
	assert.InEpsilon(t, 500.0, p50, 0.02,
		"p50 should be within 2%% of 500ms, got %.2fms", p50)

	// p99 theoretical = 990ms (99th of 1..1000)
	p99 := h.Percentile(0.99)
	assert.InEpsilon(t, 990.0, p99, 0.02,
		"p99 should be within 2%% of 990ms, got %.2fms", p99)

	// p90 theoretical = 900ms
	p90 := h.Percentile(0.90)
	assert.InEpsilon(t, 900.0, p90, 0.02,
		"p90 should be within 2%% of 900ms, got %.2fms", p90)
}

// TestHistogram_Avg verifies sum/count arithmetic.
func TestHistogram_Avg(t *testing.T) {
	h := NewHistogram()
	h.Add(10.0)
	h.Add(20.0)
	h.Add(30.0)

	avg := h.Avg()
	// sum = 60, count = 3, avg = 20
	assert.InDelta(t, 20.0, avg, 1e-9, "avg should be 20ms")
}

// TestHistogram_AvgEmpty verifies zero return when no samples added.
func TestHistogram_AvgEmpty(t *testing.T) {
	h := NewHistogram()
	assert.Equal(t, 0.0, h.Avg(), "empty histogram avg should be 0")
	assert.Equal(t, 0.0, h.Percentile(0.50), "empty histogram p50 should be 0")
}

// TestHistogram_MinMax verifies min and max tracking.
func TestHistogram_MinMax(t *testing.T) {
	h := NewHistogram()
	h.Add(5.0)
	h.Add(100.0)
	h.Add(50.0)

	assert.Equal(t, 5.0, h.min, "min should be 5.0")
	assert.Equal(t, 100.0, h.max, "max should be 100.0")
}

// TestHistogram_Clamp checks that values outside [0.001, 120000] are clamped
// without panicking.
func TestHistogram_Clamp(t *testing.T) {
	h := NewHistogram()
	h.Add(-1.0)     // below min → clamped to 0.001
	h.Add(200000.0) // above max → clamped to 120000
	h.Add(0.0)      // zero → clamped to 0.001

	require.EqualValues(t, 3, h.count, "all three samples should be counted despite clamping")
}

// TestHistogram_Reset verifies that Reset zeroes all fields.
func TestHistogram_Reset(t *testing.T) {
	h := NewHistogram()
	h.Add(100.0)
	h.Add(200.0)

	h.Reset()

	assert.EqualValues(t, 0, h.count, "count should be 0 after Reset")
	assert.Equal(t, 0.0, h.sum, "sum should be 0 after Reset")
	assert.Equal(t, math.MaxFloat64, h.min, "min should be MaxFloat64 after Reset")
	assert.Equal(t, -math.MaxFloat64, h.max, "max should be -MaxFloat64 after Reset")
	assert.Equal(t, 0.0, h.Avg(), "Avg() should be 0 after Reset")
	assert.Equal(t, 0.0, h.Percentile(0.50), "Percentile should be 0 after Reset")
}

// TestHistogram_SingleSample checks boundary: one sample, p50 = that value.
func TestHistogram_SingleSample(t *testing.T) {
	h := NewHistogram()
	h.Add(42.0)

	p50 := h.Percentile(0.50)
	// With a single sample, any percentile should return approximately 42ms
	assert.InEpsilon(t, 42.0, p50, 0.02, "p50 of single 42ms sample should be ~42ms")
}

// BenchmarkHistogramAdd measures the cost of Add() to verify allocation-free
// hot path. Run with: go test -bench BenchmarkHistogramAdd -benchmem ./...
func BenchmarkHistogramAdd(b *testing.B) {
	h := NewHistogram()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Add(float64(i%1000) + 1.0)
	}
}

// BenchmarkIngest measures the cost of ingesting 100k http_req_duration
// samples via the Aggregator to provide D-15 throughput evidence (TUI-06).
// The total wall-time for 100k samples must be well under 50ms.
func BenchmarkIngest(b *testing.B) {
	b.ReportAllocs()

	// Build 100k fake sample containers once, outside the timer.
	const sampleCount = 100_000
	containers := makeFakeHTTPDurationContainers(sampleCount)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agg := NewAggregator()
		agg.Ingest(containers)
	}
}

// TestIngestBudget ingests 100k samples once and asserts wall-time < 50ms (TUI-06 budget).
func TestIngestBudget(t *testing.T) {
	t.Parallel()
	const sampleCount = 100_000
	containers := makeFakeHTTPDurationContainers(sampleCount)

	agg := NewAggregator()
	start := time.Now()
	agg.Ingest(containers)
	elapsed := time.Since(start)
	if elapsed > 50*time.Millisecond {
		t.Fatalf("Ingest of %d samples took %v, budget is 50ms", sampleCount, elapsed)
	}
}
