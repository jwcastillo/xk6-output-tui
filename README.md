# xk6-output-tui

Live terminal dashboard for k6 load tests.

Attach a real-time TUI showing latency percentiles, RPS, VU count, status-code distribution,
and error rate — updated every 100 ms while k6 runs.

---

## Build

Install [xk6](https://github.com/grafana/xk6) v1.4.6 and build a custom k6 binary:

```bash
go install go.k6.io/xk6/cmd/xk6@v1.4.6

# Local clone (development / CI):
xk6 build \
  --with github.com/jwcastillo/xk6-output-tui=. \
  --output ./k6

# From the published module (no clone needed):
xk6 build --with github.com/jwcastillo/xk6-output-tui@latest --output ./k6
```

This produces a `./k6` binary with the `tui` output extension built in.

---

## Usage

```bash
# Recommended: suppress k6's own progress bars so the TUI owns the terminal
./k6 run --quiet --out tui your_script.js
```

Without `--quiet`, k6's built-in progress text and the TUI will both write to stderr
and interleave. Use `--quiet` for a clean dashboard.

---

## Keybindings

| Key      | Action                                              |
|----------|-----------------------------------------------------|
| `q`      | Quit the TUI rendering (k6 load test continues)    |
| `Ctrl+C` | Quit the TUI rendering (k6 load test continues)    |

**Note:** Pressing `q` or `Ctrl+C` inside the TUI quits only the live dashboard — the
k6 output extension continues aggregating metrics silently and k6 finishes the test normally.
To abort the entire k6 run, press `Ctrl+C` at the shell level (outside the TUI).

---

## Non-TTY / CI behavior

When stderr is not a TTY (piped output, CI runners, `--out tui 2>/dev/null`), bubbletea
is never initialized. Instead, the extension prints one plain-text stats line to stderr
every 5 seconds:

```
[tui] 12.3s vus=50 reqs=15234 rps=1240 p95=180.0ms err=0.4%
```

And a final summary line on `Stop()`:

```
[tui] final: 2.0s vus=2 reqs=24 rps=12 p95=394.0ms err=0.0%
```

This ensures the extension never hangs in CI and provides machine-parseable progress without
requiring a PTY.

---

## Dashboard (text rendering example)

```
 elapsed=12s        vus=50    reqs=15234

Latency (http_req_duration)
  p50=80.0ms  p90=160.0ms  p95=210.0ms  p99=350.0ms
  min=4.5ms  avg=90.0ms  max=520.0ms

Requests/sec
  instant=1240.0  cumulative=1200.0

HTTP Status
  200: 15170 (99.6%)
  500: 64 (0.4%)

err_rate=0.42%  fails=64  passes=15170
```

<!-- Screenshot: insert terminal PNG here -->

Panel order (top to bottom): header bar, latency percentiles, RPS, HTTP status codes,
error rate. All panels refresh every 100 ms from the metrics aggregator.

---

## CI

Built and smoke-tested by the `xk6-build` job in GitHub Actions CI on every push to
`main` and on every pull request. The smoke run uses a non-TTY environment, which
exercises the fallback path described above and proves `--out tui` never blocks CI.

---

## Running the Tests

```bash
go test -race ./...
```

The suite covers the histogram (accuracy and zero-allocation ingest), the
aggregator (per-metric tracking, status-code tags, instantaneous RPS window),
snapshot immutability, the bubbletea model, the non-TTY fallback formatting,
and the full output lifecycle (Start/flush/Stop with shutdown-timeout and
no-hang guarantees). An ingest microbenchmark guards the performance budget:

```bash
go test -bench BenchmarkIngest -benchmem -run '^$' .
```

## License

Dual-licensed under [MIT](LICENSE-MIT) or [Apache-2.0](LICENSE-APACHE), at
your option.
