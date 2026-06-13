package outputtui

import (
	"time"

	"go.k6.io/k6/v2/metrics"
)

// Snapshot is an immutable value struct carrying the aggregated state of all
// metrics observed so far. Built by Aggregator.Snapshot() and consumed by the
// TUI render path (Wave 3 — D-04).
//
// All latency values are in milliseconds.
type Snapshot struct {
	// Latency percentiles (ms), from the cumulative http_req_duration histogram.
	P50, P90, P95, P99 float64

	// Latency summary (ms).
	Min, Avg, Max float64

	// Request counts.
	TotalReqs int64

	// RPS metrics.
	InstantRPS    float64 // requests in last 1-second window
	CumulativeRPS float64 // totalReqs / elapsed seconds; 0 if elapsed < 1s

	// Virtual users.
	CurrentVUs int64

	// Status code distribution. This is a COPY of aggregator state; safe to mutate.
	StatusCodes map[string]int64

	// Error tracking.
	Fails, Passes int64
	ErrorRate     float64 // fails/(fails+passes); 0 if both zero

	// Elapsed time since aggregator start.
	Elapsed time.Duration
}

// rpsRing is a 1-second-windowed ring buffer tracking request counts per second.
// It holds [rpsWindowSize] slots, each corresponding to one second of wall time.
// Single-threaded; no locks needed (owned by Aggregator's flush goroutine, D-03).
const rpsWindowSize = 10

type rpsRing struct {
	counts [rpsWindowSize]int64
	epochs [rpsWindowSize]int64 // Unix second each slot covers
	index  int                  // next write slot
}

// add records count new requests arriving at time t.
func (r *rpsRing) add(t time.Time, count int64) {
	epoch := t.Unix()
	// Find an existing slot for this epoch, or overwrite the oldest.
	for i := 0; i < rpsWindowSize; i++ {
		if r.epochs[i] == epoch {
			r.counts[i] += count
			return
		}
	}
	// Overwrite the slot at r.index (oldest in a simple circular overwrite).
	r.counts[r.index] = count
	r.epochs[r.index] = epoch
	r.index = (r.index + 1) % rpsWindowSize
}

// instantRPS returns requests per second in the most recent 1-second window.
func (r *rpsRing) instantRPS(now time.Time) float64 {
	nowEpoch := now.Unix()
	// Sum all slots whose epoch == nowEpoch-1 (the last completed second) or
	// nowEpoch (the current in-flight second). We take the max of both to give
	// a stable, non-zero reading.
	var lastSec, thisSec int64
	for i := 0; i < rpsWindowSize; i++ {
		switch r.epochs[i] {
		case nowEpoch - 1:
			lastSec += r.counts[i]
		case nowEpoch:
			thisSec += r.counts[i]
		}
	}
	if lastSec > thisSec {
		return float64(lastSec)
	}
	return float64(thisSec)
}

// Aggregator accumulates raw k6 SampleContainers into running statistics.
// It is single-threaded: the PeriodicFlusher goroutine is the sole writer;
// the TUI render path reads only Snapshot values (D-03, D-04).
type Aggregator struct {
	histogram   Histogram
	totalReqs   int64
	fails       int64
	passes      int64
	statusCodes map[string]int64
	currentVUs  int64
	startTime   time.Time
	ring        rpsRing
}

// NewAggregator returns a ready-to-use Aggregator with all fields initialized.
func NewAggregator() *Aggregator {
	return &Aggregator{
		histogram:   NewHistogram(),
		statusCodes: make(map[string]int64),
		startTime:   time.Now(),
	}
}

// Ingest processes a slice of SampleContainers. It is designed to run on a
// single goroutine (no locks). Called by the PeriodicFlusher at 100ms intervals.
//
// Metric routing (D-02):
//   - http_req_duration → histogram.Add (cumulative)
//   - http_reqs → totalReqs++; rpsRing.add; optional statusCodes[code]++
//   - http_req_failed → fails++ or passes++ based on Value
//   - vus → currentVUs = int64(Value)
func (a *Aggregator) Ingest(containers []metrics.SampleContainer) {
	for _, sc := range containers {
		for _, s := range sc.GetSamples() {
			if s.Metric == nil {
				continue
			}
			switch s.Metric.Name {
			case metrics.HTTPReqDurationName:
				a.histogram.Add(s.Value)

			case metrics.HTTPReqsName:
				a.totalReqs++
				a.ring.add(s.Time, 1)
				// Guard for Pitfall 6: status tag may be absent on non-HTTP samples.
				if s.Tags != nil {
					if code, ok := s.Tags.Get("status"); ok {
						a.statusCodes[code]++
					}
				}

			case metrics.HTTPReqFailedName:
				if s.Value == 1.0 {
					a.fails++
				} else {
					a.passes++
				}

			case metrics.VUsName:
				a.currentVUs = int64(s.Value)
			}
		}
	}
}

// copyStatusCodes returns a defensive copy of the status code map so that
// callers who mutate Snapshot.StatusCodes cannot corrupt aggregator state.
func (a *Aggregator) copyStatusCodes() map[string]int64 {
	sc := make(map[string]int64, len(a.statusCodes))
	for k, v := range a.statusCodes {
		sc[k] = v
	}
	return sc
}

// histMax returns the histogram max, or 0 if no samples have been recorded.
func (a *Aggregator) histMax() float64 {
	if a.histogram.count > 0 {
		return a.histogram.max
	}
	return 0
}

// histMin returns the histogram min, or 0 if no samples have been recorded.
func (a *Aggregator) histMinVal() float64 {
	if a.histogram.count > 0 {
		return a.histogram.min
	}
	return 0
}

// Snapshot builds and returns an immutable Snapshot of current aggregated state.
// It does NOT reset the histogram — state is cumulative across flushes (D-02).
//
// StatusCodes in the returned Snapshot is a copy; callers may safely mutate it.
func (a *Aggregator) Snapshot() Snapshot {
	now := time.Now()
	elapsed := now.Sub(a.startTime)

	var cumRPS float64
	if elapsed.Seconds() >= 1.0 {
		cumRPS = float64(a.totalReqs) / elapsed.Seconds()
	}

	var errRate float64
	if total := a.fails + a.passes; total > 0 {
		errRate = float64(a.fails) / float64(total)
	}

	return Snapshot{
		P50: a.histogram.Percentile(0.50),
		P90: a.histogram.Percentile(0.90),
		P95: a.histogram.Percentile(0.95),
		P99: a.histogram.Percentile(0.99),

		Min: a.histMinVal(),
		Avg: a.histogram.Avg(),
		Max: a.histMax(),

		TotalReqs:     a.totalReqs,
		InstantRPS:    a.ring.instantRPS(now),
		CumulativeRPS: cumRPS,

		CurrentVUs:  a.currentVUs,
		StatusCodes: a.copyStatusCodes(),

		Fails:     a.fails,
		Passes:    a.passes,
		ErrorRate: errRate,

		Elapsed: elapsed,
	}
}
