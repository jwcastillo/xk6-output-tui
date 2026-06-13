package outputtui

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// fallbackOutput prints periodic plain-text stats lines to the logger when the
// output is running in a non-TTY environment (D-11). It uses no bubbletea.
//
// Format per D-11:
//
//	[tui] 12.3s vus=50 reqs=15234 rps=1240 p95=180ms err=0.4%
type fallbackOutput struct {
	logger   logrus.FieldLogger
	stop     chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
	lastSnap Snapshot
}

// NewFallback constructs a fallbackOutput. Call Start() to begin periodic logging.
//
//nolint:revive // type intentionally unexported; constructor exported for the _test package
func NewFallback(logger logrus.FieldLogger) *fallbackOutput {
	return &fallbackOutput{
		logger: logger,
		stop:   make(chan struct{}),
	}
}

// Start launches the periodic logging goroutine. Logs one line every 5 seconds
// (if at least one request has been observed). Safe to call once.
func (f *fallbackOutput) Start() {
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-f.stop:
				return
			case <-ticker.C:
				f.LogLine()
			}
		}
	}()
}

// LogLine emits a single stats line to the logger. Exported so tests can
// drive the log output without waiting 5 seconds.
func (f *fallbackOutput) LogLine() {
	f.mu.Lock()
	snap := f.lastSnap
	f.mu.Unlock()

	// Only emit if we have seen at least one request (avoids spammy zero lines).
	if snap.TotalReqs == 0 {
		return
	}

	f.logger.Infof("[tui] %.1fs vus=%d reqs=%d rps=%.0f p95=%.1fms err=%.1f%%",
		snap.Elapsed.Seconds(),
		snap.CurrentVUs,
		snap.TotalReqs,
		snap.InstantRPS,
		snap.P95,
		snap.ErrorRate*100,
	)
}

// Update atomically replaces the stored Snapshot. Called by the flusher
// goroutine; LogLine reads it from the ticker goroutine — mutex protects both.
func (f *fallbackOutput) Update(snap Snapshot) {
	f.mu.Lock()
	f.lastSnap = snap
	f.mu.Unlock()
}

// Stop signals the logging goroutine to exit, waits for it, then logs the final
// summary line.
func (f *fallbackOutput) Stop() error {
	close(f.stop)
	f.wg.Wait()

	// Final summary line — always log even if TotalReqs == 0.
	f.mu.Lock()
	snap := f.lastSnap
	f.mu.Unlock()

	f.logger.Infof("[tui] final: %.1fs vus=%d reqs=%d rps=%.0f p95=%.1fms err=%.1f%%",
		snap.Elapsed.Seconds(),
		snap.CurrentVUs,
		snap.TotalReqs,
		snap.InstantRPS,
		snap.P95,
		snap.ErrorRate*100,
	)
	return nil
}
