// Package outputtui implements a k6 output extension that renders a live
// terminal dashboard using bubbletea and lipgloss. Activate with:
//
//	xk6 build --with github.com/jwcastillo/xk6-output-tui=.
//	k6 run --quiet --out tui script.js
//
// This package is its own Go module: xk6 requires the --with extension path
// to exactly match the module path declared in go.mod (empirically verified;
// a package inside a parent module cannot be built by xk6).
package outputtui
