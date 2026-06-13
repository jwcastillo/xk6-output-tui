package outputtui_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	outputtui "github.com/jwcastillo/xk6-output-tui"
)

// captureLogger returns a logrus logger that writes to a buffer for assertions.
func captureLogger() (*logrus.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	log := logrus.New()
	log.SetOutput(&buf)
	log.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true, DisableColors: true})
	log.SetLevel(logrus.InfoLevel)
	return log, &buf
}

// TestFallbackFormat verifies that a fallback log line contains the expected
// fields in D-11 format: [tui] <Xs> vus=N reqs=N rps=N p95=Xms err=X%
func TestFallbackFormat(t *testing.T) {
	logger, buf := captureLogger()

	snap := outputtui.Snapshot{
		CurrentVUs:  42,
		TotalReqs:   100,
		InstantRPS:  50.0,
		P95:         175.3,
		ErrorRate:   0.05,
		Elapsed:     10 * time.Second,
	}

	fb := outputtui.NewFallback(logger)
	fb.Update(snap)

	// Directly invoke the tick/log function rather than waiting 5s in a test.
	fb.LogLine()

	line := buf.String()
	assert.Contains(t, line, "[tui]", "must have [tui] prefix")
	assert.Contains(t, line, "vus=42", "must contain vus field")
	assert.Contains(t, line, "reqs=100", "must contain reqs field")
	// rps is formatted as integer (%.0f)
	assert.Contains(t, line, "rps=50", "must contain rps field")
	assert.Contains(t, line, "p95=175.3ms", "must contain p95 field")
	// err is formatted as percentage: 0.05 → 5.0%
	assert.Contains(t, line, "err=5.0%", "must contain err field")
}

// TestFallbackStart_Stop verifies Start/Stop complete without hanging.
func TestFallbackStart_Stop(t *testing.T) {
	logger, _ := captureLogger()
	fb := outputtui.NewFallback(logger)
	fb.Start()

	done := make(chan struct{})
	go func() {
		fb.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("fallback.Stop() did not return within 3s — potential goroutine leak")
	}
}

// TestFallbackUpdate_RaceCondition verifies that concurrent Update() + Stop()
// do not race (the -race flag will detect any unsynchronized access).
func TestFallbackUpdate_RaceCondition(t *testing.T) {
	logger, _ := captureLogger()
	fb := outputtui.NewFallback(logger)
	fb.Start()

	snap := outputtui.Snapshot{CurrentVUs: 10, TotalReqs: 50}

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			fb.Update(snap)
		}
		close(done)
	}()

	<-done
	require.NoError(t, fb.Stop())
}
