// Package outputtui implements a k6 output extension that renders a live
// terminal dashboard using bubbletea and lipgloss. Activate with:
//
//	xk6 build --with github.com/jwcastillo/xk6-output-tui=.
//	k6 run --quiet --out tui script.js
package outputtui

import (
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"go.k6.io/k6/v2/output"
	"golang.org/x/term"
)

func init() {
	output.RegisterExtension("tui", New)
}

// Output is the k6 output extension that drives the live TUI dashboard.
// It embeds output.SampleBuffer so k6 can call AddMetricSamples non-blockingly.
type Output struct {
	output.SampleBuffer // embedded: AddMetricSamples promoted, handles buffering
	params   output.Params
	flusher  *output.PeriodicFlusher
	agg      *Aggregator
	fallback *fallbackOutput
	program  *tea.Program
	done     chan struct{} // closed when bubbletea program.Run() returns
}

// New constructs the Output extension. Called by k6 after xk6 build.
func New(params output.Params) (output.Output, error) {
	return &Output{
		params: params,
		agg:    NewAggregator(),
	}, nil
}

// Description returns a human-readable label shown in `k6 run` output.
func (o *Output) Description() string {
	return "tui (live terminal dashboard)"
}

// Start initializes the TUI or non-TTY fallback, then starts the PeriodicFlusher.
//
// TTY detection uses os.Stderr.Fd() directly per D-11: params.StdErr is an
// io.Writer with no Fd() method; os.Stderr is the real file descriptor.
func (o *Output) Start() error {
	// D-11: TTY detection on os.Stderr (not params.StdErr which is io.Writer).
	isTTY := term.IsTerminal(int(os.Stderr.Fd()))

	// Start the PeriodicFlusher at 100ms (D-03).
	var err error
	o.flusher, err = output.NewPeriodicFlusher(100*time.Millisecond, o.flush)
	if err != nil {
		return err
	}

	if isTTY {
		// D-08: bubbletea renders to stderr using alternate screen.
		// tea.WithInputTTY() opens /dev/tty for input so keyboard events
		// work even when stdout is redirected.
		o.done = make(chan struct{})
		o.program = tea.NewProgram(
			NewTUIModel(),
			tea.WithOutput(os.Stderr),        // D-08: render to stderr
			tea.WithInputTTY(),               // opens /dev/tty for input
			tea.WithAltScreen(),              // D-08: alternate screen buffer
			tea.WithoutSignalHandler(),       // k6 owns signals (D-06)
		)
		go func() {
			// program.Run() blocks until Quit()/Kill() — never block Start (D-06).
			defer close(o.done)
			o.program.Run() //nolint:errcheck // terminal restore errors are non-fatal
		}()
	} else {
		// D-11: non-TTY fallback — print plain-text stats lines every 5s.
		o.fallback = NewFallback(o.params.Logger)
		o.fallback.Start()
	}

	return nil
}

// flush is called by the PeriodicFlusher every 100ms (D-03). It drains the
// SampleBuffer, ingests samples into the Aggregator, builds a Snapshot, and
// dispatches it to the active display path (TUI or fallback).
//
// Single-goroutine owner of Aggregator state (D-03).
func (o *Output) flush() {
	samples := o.GetBufferedSamples()
	o.agg.Ingest(samples)
	snap := o.agg.Snapshot()

	if o.program != nil {
		// Send is goroutine-safe; safe even after program exits (D-04).
		o.program.Send(snapshotMsg{S: snap})
	} else if o.fallback != nil {
		o.fallback.Update(snap)
	}
}

// Stop flushes remaining metrics and tears down the TUI cleanly (D-06).
//
// Order:
//  1. Stop the flusher — performs a final flush (final Snapshot sent).
//  2. If TTY: request program.Quit(), wait on done channel (2s timeout).
//  3. If non-TTY: call fallback.Stop() — logs final summary line.
func (o *Output) Stop() error {
	// 1. Final flush — must happen before signalling TUI to quit (D-06).
	o.flusher.Stop()

	// 2. TTY path: ask bubbletea to quit gracefully; kill if it hangs.
	if o.program != nil {
		o.program.Quit()
		select {
		case <-o.done:
			// Clean exit.
		case <-time.After(2 * time.Second):
			// Timeout guard (D-06): force-kill restores the terminal.
			o.program.Kill()
			<-o.done
		}
	}

	// 3. Non-TTY path: log final summary and wait for goroutine.
	if o.fallback != nil {
		return o.fallback.Stop()
	}

	return nil
}
