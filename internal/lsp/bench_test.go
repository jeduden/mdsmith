package lsp

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jeduden/mdsmith/internal/rule"

	_ "github.com/jeduden/mdsmith/internal/rules/linelength"
	_ "github.com/jeduden/mdsmith/internal/rules/notrailingspaces"
)

// BenchmarkLatency1kLines measures end-to-end didChange →
// publishDiagnostics latency on a 1 000-line synthetic Markdown
// document. Plan 121 sets a p95 budget of 150 ms; missing it blocks
// the default `mdsmith.run` from flipping to `onType`.
func BenchmarkLatency1kLines(b *testing.B) {
	benchLatency(b, 1000, 150*time.Millisecond)
}

// BenchmarkLatency5kLines measures the same path on a 5 000-line
// synthetic document. Plan 121 sets a p95 budget of 500 ms.
func BenchmarkLatency5kLines(b *testing.B) {
	benchLatency(b, 5000, 500*time.Millisecond)
}

func benchLatency(b *testing.B, lines int, budget time.Duration) {
	b.Helper()
	if testing.Short() {
		b.Skip("benchmark skipped in -short mode")
	}

	h := newBenchHarness(b)
	defer h.close()

	uri := "file:///bench/sample.md"
	source := buildSyntheticMarkdown(lines)
	h.notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": uri, "languageId": "markdown", "version": 1, "text": source,
		},
	})
	h.awaitDiagnostics(uri, 5*time.Second)

	samples := make([]time.Duration, 0, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Mutate one line so didChange triggers a re-lint.
		mutated := source + "\n<!-- iter " + itoa(i) + " -->\n"
		start := time.Now()
		h.notify("textDocument/didChange", map[string]any{
			"textDocument":   map[string]any{"uri": uri, "version": i + 2},
			"contentChanges": []map[string]any{{"text": mutated}},
		})
		h.awaitDiagnostics(uri, 5*time.Second)
		samples = append(samples, time.Since(start))
	}
	b.StopTimer()

	if len(samples) == 0 {
		b.Skip("no samples — benchmark needs more iterations")
	}
	p95 := percentile(samples, 0.95)
	b.ReportMetric(float64(p95.Milliseconds()), "p95_ms")
	if p95 > budget {
		b.Fatalf("p95 latency %v exceeds budget %v on %d-line doc", p95, budget, lines)
	}
}

func percentile(samples []time.Duration, q float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	cp := append([]time.Duration(nil), samples...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	idx := int(float64(len(cp)-1) * q)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}

func buildSyntheticMarkdown(lines int) string {
	var b strings.Builder
	b.WriteString("# Synthetic Document\n\n")
	for i := 0; i < lines; i++ {
		// Mix of plain text and one bare URL line every 50 lines so the
		// linter has work to do; avoid pathological line lengths.
		if i%50 == 0 {
			b.WriteString("This paragraph mentions https://example.com inline.\n")
		} else {
			b.WriteString("Synthetic line content for benchmarking purposes.\n")
		}
	}
	return b.String()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// benchHarness is a thin wrapper around testHarness for benchmarks.
// It re-implements the bits we need without duplicating testing.T
// dependencies.
type benchHarness struct {
	*testHarness
}

func newBenchHarness(b *testing.B) *benchHarness {
	t := &testing.T{}
	h := newHarness(t)
	// Initialize the server.
	resultRaw, errResp := h.request("initialize", initializeParams{
		Capabilities: clientCapabilities{Workspace: &workspaceClientCapabilities{Configuration: true}},
	})
	if errResp != nil || resultRaw == nil {
		b.Fatalf("initialize failed: %v", errResp)
	}
	// Configure rule set explicitly so the registry pulled in by the
	// blank imports is the production set.
	_ = rule.All()
	return &benchHarness{testHarness: h}
}

func (h *benchHarness) close() {
	if h.cancel != nil {
		h.cancel()
	}
}

// awaitDiagnostics reads frames until publishDiagnostics arrives for
// the given URI.
func (h *benchHarness) awaitDiagnostics(uri string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		raw := h.read()
		var probe struct {
			ID     json.RawMessage `json:"id,omitempty"`
			Method string          `json:"method,omitempty"`
			Params json.RawMessage `json:"params,omitempty"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		if probe.Method == "" {
			continue
		}
		if probe.Method == "textDocument/publishDiagnostics" {
			var p publishDiagnosticsParams
			if err := json.Unmarshal(probe.Params, &p); err == nil && p.URI == uri {
				return
			}
			continue
		}
		// Server-side request: ack with empty result so it can proceed.
		if len(probe.ID) > 0 {
			h.write(struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      json.RawMessage `json:"id"`
				Result  any             `json:"result"`
			}{JSONRPC: "2.0", ID: probe.ID, Result: nil})
		}
	}
}
