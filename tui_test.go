package outputtui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	outputtui "github.com/jwcastillo/xk6-output-tui"
)

// TestTUIModel_Init verifies Init() returns a non-nil Cmd (the initial tick).
func TestTUIModel_Init(t *testing.T) {
	m := outputtui.NewTUIModel()
	cmd := m.Init()
	// Init must return a non-nil Cmd (tea.Tick)
	require.NotNil(t, cmd)
}

// TestTUIModel_View_Empty verifies View() returns a non-empty string even
// on an empty (zero-value) snapshot — the dashboard always renders something.
func TestTUIModel_View_Empty(t *testing.T) {
	m := outputtui.NewTUIModel()
	v := m.View()
	require.NotEmpty(t, v)
}

// TestTUIModel_View_WithSnapshot verifies View() shows snapshot values after
// an Update with a snapshotMsg.
func TestTUIModel_View_WithSnapshot(t *testing.T) {
	m := outputtui.NewTUIModel()

	snap := outputtui.Snapshot{
		P95:           180.5,
		TotalReqs:     1234,
		CurrentVUs:    50,
		InstantRPS:    420.0,
		CumulativeRPS: 380.0,
		Fails:         3,
		Passes:        1231,
		ErrorRate:     0.0024,
		Elapsed:       12 * time.Second,
		StatusCodes:   map[string]int64{"200": 1231, "500": 3},
	}

	newModel, _ := m.Update(outputtui.SnapshotMsg{S: snap})
	view := newModel.(outputtui.TUIModelInterface).View()

	// Header fields
	assert.Contains(t, view, "50", "should contain VU count")
	assert.Contains(t, view, "1234", "should contain total reqs")
	// Latency panel: p95
	assert.Contains(t, view, "180.5", "should contain p95 value")
	// Status codes (sorted) — both codes should appear
	assert.Contains(t, view, "200", "should contain status 200")
	assert.Contains(t, view, "500", "should contain status 500")
}

// TestTUIModel_QuitKey verifies pressing 'q' produces tea.Quit command.
func TestTUIModel_QuitKey(t *testing.T) {
	m := outputtui.NewTUIModel()
	_, cmd := m.Update(outputtui.QuitKeyMsg())
	// Cmd should be tea.Quit (non-nil)
	require.NotNil(t, cmd)
}

// TestRenderDashboard verifies the panel structure from renderDashboard.
func TestRenderDashboard(t *testing.T) {
	snap := outputtui.Snapshot{
		P50: 42.1, P90: 95.3, P95: 120.7, P99: 200.0,
		Min: 10.0, Avg: 65.0, Max: 350.0,
		TotalReqs:     500,
		InstantRPS:    100.0,
		CumulativeRPS: 90.0,
		CurrentVUs:    10,
		StatusCodes:   map[string]int64{"200": 490, "404": 10},
		Fails:         10,
		Passes:        490,
		ErrorRate:     0.02,
		Elapsed:       5 * time.Second,
	}

	out := outputtui.RenderDashboard(snap, 5*time.Second)
	require.NotEmpty(t, out)

	// Panel 1 — header
	assert.Contains(t, out, "500", "header must show total reqs")

	// Panel 2 — latency
	assert.Contains(t, out, "Latency", "latency panel label")
	assert.Contains(t, out, "42.1", "p50 value")
	assert.Contains(t, out, "200.0", "p99 value")

	// Panel 3 — RPS
	assert.Contains(t, out, "Requests", "RPS panel label")
	assert.Contains(t, out, "100.0", "instant RPS")

	// Panel 4 — status codes (sorted)
	assert.Contains(t, out, "200", "status 200")
	assert.Contains(t, out, "404", "status 404")
	// 200 must appear before 404 (sorted)
	pos200 := strings.Index(out, "200")
	pos404 := strings.Index(out, "404")
	assert.Less(t, pos200, pos404, "status codes must be sorted ascending")

	// Panel 5 — error rate
	assert.Contains(t, out, "2.00", "error rate %")
}
