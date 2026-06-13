// smoke.js — minimal k6 script for CI non-TTY smoke test of --out tui
//
// Empirically verified xk6 build command (run from repo root; output-tui is
// its own Go module — xk6 requires module-path == go.mod module path):
//   xk6 build \
//     --with github.com/jwcastillo/xk6-output-tui=. \
//     --output /tmp/k6-tui-smoke
//
// Run smoke test:
//   /tmp/k6-tui-smoke run --quiet --out tui output-tui/testdata/smoke.js
import http from 'k6/http';

export const options = {
  vus: 2,
  duration: '2s',
};

export default function () {
  http.get('https://httpbin.test.k6.io/get');
}
