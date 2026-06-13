package outputtui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.k6.io/k6/v2/metrics"
)

// --- helpers ---

// makeSample builds a metrics.Sample with the given metric name and value.
// Tags are passed as alternating key/value pairs.
func makeSample(registry *metrics.Registry, name string, value float64, tagKV ...string) metrics.Sample {
	m, err := registry.NewMetric(name, metrics.Counter)
	if err != nil {
		// Metric may already be registered; fetch it.
		m = registry.Get(name)
	}
	ts := metrics.TimeSeries{Metric: m}
	if len(tagKV) > 0 {
		tagMap := make(map[string]string, len(tagKV)/2)
		for i := 0; i+1 < len(tagKV); i += 2 {
			tagMap[tagKV[i]] = tagKV[i+1]
		}
		ts.Tags = registry.RootTagSet().WithTagsFromMap(tagMap)
	}
	return metrics.Sample{
		TimeSeries: ts,
		Time:       time.Now(),
		Value:      value,
	}
}

// makeContainer wraps a single Sample into a SampleContainer.
func makeContainer(s metrics.Sample) metrics.SampleContainer {
	return metrics.Samples{s}
}

// makeFakeHTTPDurationContainers returns n SampleContainers each holding one
// http_req_duration sample. Used by benchmarks.
func makeFakeHTTPDurationContainers(n int) []metrics.SampleContainer {
	reg := metrics.NewRegistry()
	m := reg.MustNewMetric(metrics.HTTPReqDurationName, metrics.Trend, metrics.Time)
	containers := make([]metrics.SampleContainer, n)
	now := time.Now()
	for i := 0; i < n; i++ {
		containers[i] = metrics.Samples{
			{
				TimeSeries: metrics.TimeSeries{Metric: m},
				Time:       now,
				Value:      float64(i%1000) + 1.0, // 1..1000 ms cycling
			},
		}
	}
	return containers
}

// --- tests ---

// TestAggregator_Snapshot exercises every Snapshot field with known inputs.
func TestAggregator_Snapshot(t *testing.T) {
	reg := metrics.NewRegistry()

	// Register metrics with their proper types.
	durationMetric := reg.MustNewMetric(metrics.HTTPReqDurationName, metrics.Trend, metrics.Time)
	reqsMetric := reg.MustNewMetric(metrics.HTTPReqsName, metrics.Counter)
	failedMetric := reg.MustNewMetric(metrics.HTTPReqFailedName, metrics.Rate)
	vusMetric := reg.MustNewMetric(metrics.VUsName, metrics.Gauge)

	// Build SampleContainers.
	var containers []metrics.SampleContainer

	// 10 http_req_duration samples, all 10ms
	for i := 0; i < 10; i++ {
		containers = append(containers, metrics.Samples{{
			TimeSeries: metrics.TimeSeries{Metric: durationMetric},
			Time:       time.Now(),
			Value:      10.0,
		}})
	}

	// 5 http_reqs: 3 with status "200", 2 with status "404"
	rootTagSet := reg.RootTagSet()
	for i := 0; i < 3; i++ {
		containers = append(containers, metrics.Samples{{
			TimeSeries: metrics.TimeSeries{
				Metric: reqsMetric,
				Tags:   rootTagSet.With("status", "200"),
			},
			Time:  time.Now(),
			Value: 1.0,
		}})
	}
	for i := 0; i < 2; i++ {
		containers = append(containers, metrics.Samples{{
			TimeSeries: metrics.TimeSeries{
				Metric: reqsMetric,
				Tags:   rootTagSet.With("status", "404"),
			},
			Time:  time.Now(),
			Value: 1.0,
		}})
	}

	// 3 http_req_failed: 2 fail (Value=1.0), 1 pass (Value=0.0)
	containers = append(containers, metrics.Samples{{
		TimeSeries: metrics.TimeSeries{Metric: failedMetric},
		Time:       time.Now(),
		Value:      1.0,
	}})
	containers = append(containers, metrics.Samples{{
		TimeSeries: metrics.TimeSeries{Metric: failedMetric},
		Time:       time.Now(),
		Value:      1.0,
	}})
	containers = append(containers, metrics.Samples{{
		TimeSeries: metrics.TimeSeries{Metric: failedMetric},
		Time:       time.Now(),
		Value:      0.0,
	}})

	// 1 vus sample value=42
	containers = append(containers, metrics.Samples{{
		TimeSeries: metrics.TimeSeries{Metric: vusMetric},
		Time:       time.Now(),
		Value:      42.0,
	}})

	agg := NewAggregator()
	agg.Ingest(containers)

	snap := agg.Snapshot()

	// --- Assert Snapshot fields ---
	assert.EqualValues(t, 5, snap.TotalReqs, "TotalReqs should be 5")
	assert.EqualValues(t, 3, snap.StatusCodes["200"], "status 200 count should be 3")
	assert.EqualValues(t, 2, snap.StatusCodes["404"], "status 404 count should be 2")
	assert.EqualValues(t, 42, snap.CurrentVUs, "CurrentVUs should be 42")
	assert.EqualValues(t, 2, snap.Fails, "Fails should be 2")
	assert.EqualValues(t, 1, snap.Passes, "Passes should be 1")
	assert.InDelta(t, 2.0/3.0, snap.ErrorRate, 0.001, "ErrorRate should be ~0.667")

	// Latency: all 10ms samples → avg, min, max, p50 should all be ~10ms
	assert.InDelta(t, 10.0, snap.Avg, 0.5, "Avg latency should be ~10ms")
	assert.Equal(t, 10.0, snap.Min, "Min latency should be 10ms")
	assert.Equal(t, 10.0, snap.Max, "Max latency should be 10ms")

	// p50 of all-10ms should also be ~10ms (within 2%)
	assert.InEpsilon(t, 10.0, snap.P50, 0.02, "P50 should be ~10ms")

	// Snapshot.StatusCodes must be a copy — mutating it must not affect agg
	snap.StatusCodes["999"] = 99
	snap2 := agg.Snapshot()
	_, exists := snap2.StatusCodes["999"]
	assert.False(t, exists, "mutating Snapshot.StatusCodes must not affect Aggregator state")
}

// TestAggregator_NoStatusTag verifies that http_reqs samples without a
// "status" tag do not panic (Pitfall 6).
func TestAggregator_NoStatusTag(t *testing.T) {
	reg := metrics.NewRegistry()
	reqsMetric := reg.MustNewMetric(metrics.HTTPReqsName, metrics.Counter)

	containers := []metrics.SampleContainer{
		metrics.Samples{{
			TimeSeries: metrics.TimeSeries{
				Metric: reqsMetric,
				Tags:   nil, // no status tag — must not panic
			},
			Time:  time.Now(),
			Value: 1.0,
		}},
	}

	agg := NewAggregator()
	require.NotPanics(t, func() { agg.Ingest(containers) })

	snap := agg.Snapshot()
	assert.EqualValues(t, 1, snap.TotalReqs)
}

// TestAggregator_EmptyIngest checks a fresh Aggregator returns safe zero values.
func TestAggregator_EmptyIngest(t *testing.T) {
	agg := NewAggregator()
	snap := agg.Snapshot()

	assert.Equal(t, 0.0, snap.ErrorRate, "empty ErrorRate should be 0")
	assert.Equal(t, 0.0, snap.Avg, "empty Avg should be 0")
	assert.Equal(t, 0.0, snap.P50, "empty P50 should be 0")
	assert.EqualValues(t, 0, snap.TotalReqs)
	assert.EqualValues(t, 0, snap.CurrentVUs)
	assert.EqualValues(t, 0, snap.Fails)
	assert.EqualValues(t, 0, snap.Passes)
	assert.EqualValues(t, 0, snap.CumulativeRPS, "CumulativeRPS < 1s elapsed should be 0")
}

// TestAggregator_SnapshotIsCumulative checks that Snapshot() does NOT reset
// the histogram — subsequent calls accumulate.
func TestAggregator_SnapshotIsCumulative(t *testing.T) {
	reg := metrics.NewRegistry()
	durationMetric := reg.MustNewMetric(metrics.HTTPReqDurationName, metrics.Trend, metrics.Time)

	agg := NewAggregator()

	c1 := []metrics.SampleContainer{metrics.Samples{{
		TimeSeries: metrics.TimeSeries{Metric: durationMetric},
		Time:       time.Now(),
		Value:      100.0,
	}}}
	agg.Ingest(c1)
	snap1 := agg.Snapshot()

	c2 := []metrics.SampleContainer{metrics.Samples{{
		TimeSeries: metrics.TimeSeries{Metric: durationMetric},
		Time:       time.Now(),
		Value:      200.0,
	}}}
	agg.Ingest(c2)
	snap2 := agg.Snapshot()

	// After second Ingest, max should be 200ms (cumulative)
	assert.Greater(t, snap2.Max, snap1.Max, "Max should increase after second ingest (cumulative)")
}
