package outputtui

import (
	"fmt"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SnapshotMsg carries an immutable Snapshot from the flusher goroutine into the
// bubbletea event loop. Only SnapshotMsg is used inside the TUI goroutine.
// Exported so tests can construct messages in the external _test package.
type SnapshotMsg struct{ S Snapshot }

// snapshotMsg is the internal alias (same type, used in output.go flush).
type snapshotMsg = SnapshotMsg

// tickMsg drives the 1-second elapsed-time counter in Update.
type tickMsg time.Time

// tuiModel is the bubbletea Model for the live dashboard. It holds an immutable
// Snapshot and a locally-maintained elapsed duration. It never accesses the
// Aggregator directly (D-04).
type tuiModel struct {
	snap    Snapshot
	started time.Time
	elapsed time.Duration
}

// TUIModelInterface is exposed so tests can call View() after type-asserting
// the return value of Update().
type TUIModelInterface interface {
	tea.Model
}

// NewTUIModel returns a new tuiModel ready for use with tea.NewProgram.
func NewTUIModel() tuiModel {
	return tuiModel{started: time.Now()}
}

// Init returns the initial tick command. This drives the elapsed-time counter
// independently of snapshot messages (D-09).
func (m tuiModel) Init() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Update processes incoming messages. It is the single entry point for all
// state changes in the TUI (Elm Architecture). Access to Aggregator is forbidden
// here — only Snapshot values are used (D-04).
func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case snapshotMsg:
		// Immutable struct copy — safe, no lock needed.
		m.snap = msg.S
		return m, nil

	case tickMsg:
		m.elapsed += time.Second
		return m, tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })

	case tea.KeyMsg:
		// 'q' and Ctrl+C quit ONLY the TUI rendering (D-07).
		// They do NOT call os.Exit — k6 continues running.
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	return m, nil
}

// View renders the full dashboard from the current Snapshot. It reads only
// m.snap — no aggregator access (D-04).
func (m tuiModel) View() string {
	return RenderDashboard(m.snap, m.elapsed)
}

// RenderDashboard builds the full panel layout from an immutable Snapshot.
// Exported for test access. Uses lipgloss adaptive defaults per D-12 (no
// hardcoded ANSI sequences). Panel order follows D-10.
func RenderDashboard(snap Snapshot, elapsed time.Duration) string {
	// Styles — adaptive, no hardcoded ANSI (D-12).
	headerStyle := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	labelStyle := lipgloss.NewStyle().Bold(true).Underline(true)

	// ── Panel 1: Header bar ──────────────────────────────────────────────────
	elapsedStr := elapsed.Round(time.Second).String()
	if elapsed == 0 {
		elapsedStr = "0s"
	}
	headerLine := headerStyle.Render(fmt.Sprintf(
		"elapsed=%-8s  vus=%-4d  reqs=%d",
		elapsedStr, snap.CurrentVUs, snap.TotalReqs,
	))

	// ── Panel 2: Latency ─────────────────────────────────────────────────────
	latPanel := labelStyle.Render("Latency (http_req_duration)") + "\n" +
		fmt.Sprintf("  p50=%.1fms  p90=%.1fms  p95=%.1fms  p99=%.1fms",
			snap.P50, snap.P90, snap.P95, snap.P99) + "\n" +
		fmt.Sprintf("  min=%.1fms  avg=%.1fms  max=%.1fms",
			snap.Min, snap.Avg, snap.Max)

	// ── Panel 3: RPS ─────────────────────────────────────────────────────────
	rpsPanel := labelStyle.Render("Requests/sec") + "\n" +
		fmt.Sprintf("  instant=%.1f  cumulative=%.1f",
			snap.InstantRPS, snap.CumulativeRPS)

	// ── Panel 4: Status codes (sorted ascending by code string) ─────────────
	codes := make([]string, 0, len(snap.StatusCodes))
	for c := range snap.StatusCodes {
		codes = append(codes, c)
	}
	sort.Strings(codes) // D-10: sorted by code key

	statusSection := labelStyle.Render("HTTP Status")
	for _, c := range codes {
		n := snap.StatusCodes[c]
		pct := float64(0)
		if snap.TotalReqs > 0 {
			pct = float64(n) / float64(snap.TotalReqs) * 100
		}
		statusSection += fmt.Sprintf("\n  %s: %d (%.1f%%)", c, n, pct)
	}

	// ── Panel 5: Error rate line ─────────────────────────────────────────────
	errLine := fmt.Sprintf("err_rate=%.2f%%  fails=%d  passes=%d",
		snap.ErrorRate*100, snap.Fails, snap.Passes)

	// Single 80-col-friendly column stack per D-10 (no side-by-side in v1).
	return lipgloss.JoinVertical(lipgloss.Left,
		headerLine, "",
		latPanel, "",
		rpsPanel, "",
		statusSection, "",
		errLine,
	)
}

// QuitKeyMsg returns a tea.KeyMsg for 'q' — exported for test use.
func QuitKeyMsg() tea.Msg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}
}
