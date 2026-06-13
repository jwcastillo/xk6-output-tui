package outputtui

import "math"

// Histogram is an HDR-style fixed-array histogram for measuring latency in
// milliseconds. It uses 512 exponentially-spaced buckets with a growth factor
// of 2^(1/16) ≈ 1.0443, giving ~16 buckets per octave and ~0.5% relative error.
//
// Covered range: [0.001ms, 120000ms]. Values outside this range are clamped.
// The last bucket's upper boundary is ~123078ms, safely covering 120000ms.
// The zero value is NOT valid — use NewHistogram().
//
// All fields are intentionally unexported; package tests access them directly.
// Histogram is not safe for concurrent use; the owning Aggregator is
// single-threaded (D-03).
type Histogram struct {
	buckets [numBuckets]int64
	min     float64
	max     float64
	sum     float64
	count   int64
}

const (
	// numBuckets must be large enough to cover [histMin, histMax].
	// With 2^(1/16) growth: ceil(log(120000/0.001) / log(2^(1/16))) ≈ 429 buckets.
	// We use 512 for a comfortable margin and ~0.5% relative error.
	numBuckets = 512

	// histMin is the lower clamp bound (ms).
	histMin = 0.001
	// histMax is the upper clamp bound (ms).
	histMax = 120_000.0

	// logGrowth = log(2^(1/16)) — precomputed for bucket-index conversion.
	// Growth factor g = 2^(1/16) ≈ 1.0443, ~16 buckets per octave, ~0.5% relative error.
	// bucket index = floor(log(v/histMin) / logGrowth)
	logGrowth = 0.04332169878499654 // math.Log(math.Pow(2, 1.0/16))
)

// NewHistogram returns a properly initialized Histogram.
// min is set to MaxFloat64 so the first Add() beats it; max to -MaxFloat64.
func NewHistogram() Histogram {
	return Histogram{
		min: math.MaxFloat64,
		max: -math.MaxFloat64,
	}
}

// bucketIndex returns the bucket index for value v (already clamped).
// index = clamp(floor(log(v/histMin) / logGrowth), 0, numBuckets-1)
func bucketIndex(v float64) int {
	if v <= histMin {
		return 0
	}
	idx := int(math.Log(v/histMin) / logGrowth)
	if idx >= numBuckets {
		return numBuckets - 1
	}
	return idx
}

// bucketLower returns the lower boundary of bucket i.
// lower(i) = histMin * g^i   where g = 2^(1/8)
func bucketLower(i int) float64 {
	return histMin * math.Exp(float64(i)*logGrowth)
}

// bucketUpper returns the upper boundary of bucket i.
// upper(i) = lower(i+1)
func bucketUpper(i int) float64 {
	return histMin * math.Exp(float64(i+1)*logGrowth)
}

// Add records a single latency sample v (in milliseconds).
// Values are clamped to [histMin, histMax] before bucketing.
// Add never allocates — it increments a fixed-array slot and updates
// min/max/sum/count scalars.
func (h *Histogram) Add(v float64) {
	// Clamp
	if v < histMin {
		v = histMin
	} else if v > histMax {
		v = histMax
	}

	h.buckets[bucketIndex(v)]++
	h.count++
	h.sum += v
	if v < h.min {
		h.min = v
	}
	if v > h.max {
		h.max = v
	}
}

// Percentile returns the q-th percentile (q in [0,1]) using linear
// interpolation within the bucket that contains the target rank.
// Returns 0 if the histogram is empty.
func (h *Histogram) Percentile(q float64) float64 {
	if h.count == 0 {
		return 0
	}
	// Target rank: the 1-indexed sample position we want.
	// For p50 with 1000 samples: target = 0.50 * 1000 = 500.0
	target := q * float64(h.count)

	var cum int64
	for i := 0; i < numBuckets; i++ {
		cum += h.buckets[i]
		if float64(cum) >= target {
			// Linear interpolation within [lower, upper) of bucket i.
			lower := bucketLower(i)
			upper := bucketUpper(i)

			// Fraction of the bucket we need:
			// How many samples are in this bucket? h.buckets[i]
			// We need (target - cumBefore) out of h.buckets[i] samples.
			cumBefore := float64(cum - h.buckets[i])
			frac := (target - cumBefore) / float64(h.buckets[i])
			return lower + frac*(upper-lower)
		}
	}
	// Should not reach here, but guard:
	return h.max
}

// Avg returns the arithmetic mean of all recorded samples, or 0 if empty.
func (h *Histogram) Avg() float64 {
	if h.count == 0 {
		return 0
	}
	return h.sum / float64(h.count)
}

// Reset zeroes all histogram state. Equivalent to replacing with NewHistogram().
func (h *Histogram) Reset() {
	*h = NewHistogram()
}
