package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jeduden/mdsmith/internal/rule"

	// Import the production rule set so the latency benchmark
	// reflects what an editor actually triggers, not just the
	// two-rule subset the unit tests register.
	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// BenchmarkLatency1kLines measures end-to-end didChange →
// publishDiagnostics latency on a 1 000-line synthetic Markdown
// document. Plan 121 sets a p95 budget of 150 ms; the benchmark
// guards that budget now that `onType` is the default `mdsmith.run`.
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

	// newBenchHarness already registers `b.Cleanup(h.close)`; do
	// not also `defer h.close()` — the second close would block on
	// the already-drained srvDone channel and add a 2-second
	// timeout to every benchmark run.
	h := newBenchHarness(b)

	// Force `mdsmith.run = onType` so the benchmark stays independent
	// of the production default (also onType) and always drives the
	// didChange events that onSave would otherwise filter.
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOnType
	h.srv.settingsMu.Unlock()

	uri := "file:///bench/sample.md"
	// Two same-sized buffers swapped each iteration so the document
	// length stays constant across the run. With Go's time-based
	// benchtime b.N can grow unbounded; an `append`-style mutation
	// would balloon the document and skew later samples toward
	// larger files than earlier ones.
	bufA := buildSyntheticMarkdown(lines)
	bufB := flipFirstParagraph(bufA)
	if len(bufA) != len(bufB) {
		b.Fatalf("benchmark buffers must match in length: %d vs %d", len(bufA), len(bufB))
	}
	h.notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": uri, "languageId": "markdown", "version": 1, "text": bufA,
		},
	})
	h.awaitDiagnostics(b, uri, 5*time.Second)

	samples := make([]time.Duration, 0, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		text := bufA
		if i%2 == 1 {
			text = bufB
		}
		start := time.Now()
		h.notify("textDocument/didChange", map[string]any{
			"textDocument":   map[string]any{"uri": uri, "version": i + 2},
			"contentChanges": []map[string]any{{"text": text}},
		})
		h.awaitDiagnostics(b, uri, 5*time.Second)
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
		if i%50 == 0 {
			b.WriteString("This paragraph mentions https://example.com inline.\n")
		} else {
			b.WriteString("Synthetic line content for benchmarking purposes.\n")
		}
	}
	return b.String()
}

// flipFirstParagraph returns a same-length copy of src with its
// first content paragraph rewritten. Used by benchLatency to swap
// between two stable-size buffers so the benchmark drives didChange
// without growing the document over time. The replacement string
// has identical byte length to the original "Synthetic line ..."
// line so the resulting buffer's length matches the input exactly.
func flipFirstParagraph(src string) string {
	const original = "Synthetic line content for benchmarking purposes."
	const swapped = "Alternate line content for benchmarking purposes."
	if len(original) != len(swapped) {
		panic("flipFirstParagraph requires same-length strings")
	}
	return strings.Replace(src, original, swapped, 1)
}

// benchHarness wraps a Server and the in-memory pipes it talks to.
// It does not depend on testing.T (Cleanup/Helper are not safe with
// a manually allocated *testing.T per the docs).
type benchHarness struct {
	srv          *Server
	cancel       context.CancelFunc
	clientWriter io.WriteCloser
	clientReader *bufio.Reader
	srvDone      chan error
	mu           sync.Mutex
	nextID       int
}

func newBenchHarness(b *testing.B) *benchHarness {
	b.Helper()
	srvIn, clientWriter := io.Pipe()
	clientRawReader, srvOut := io.Pipe()
	srv := New(Options{
		Rules:    rule.All(),
		Reader:   srvIn,
		Writer:   srvOut,
		Debounce: -1,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx)
		_ = srvOut.Close()
	}()
	h := &benchHarness{
		srv:          srv,
		cancel:       cancel,
		clientWriter: clientWriter,
		clientReader: bufio.NewReader(clientRawReader),
		srvDone:      done,
	}
	b.Cleanup(h.close)

	// Initialize so the server reaches a steady state.
	resultRaw, errResp := h.request(b, "initialize", map[string]any{"capabilities": map[string]any{}})
	if errResp != nil || resultRaw == nil {
		b.Fatalf("initialize failed: %v", errResp)
	}
	return h
}

func (h *benchHarness) close() {
	if h.cancel != nil {
		h.cancel()
	}
	_ = h.clientWriter.Close()
	select {
	case <-h.srvDone:
	case <-time.After(2 * time.Second):
	}
}

func (h *benchHarness) write(v any) {
	body, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	if _, err := fmt.Fprintf(h.clientWriter, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		panic(err)
	}
	if _, err := h.clientWriter.Write(body); err != nil {
		panic(err)
	}
}

func (h *benchHarness) read() []byte {
	tp := textproto.NewReader(h.clientReader)
	hdr, err := tp.ReadMIMEHeader()
	if err != nil {
		panic(err)
	}
	cl := hdr.Get("Content-Length")
	n, err := strconv.Atoi(cl)
	if err != nil {
		panic(err)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(h.clientReader, body); err != nil {
		panic(err)
	}
	return body
}

func (h *benchHarness) request(b *testing.B, method string, params any) (json.RawMessage, *responseError) {
	b.Helper()
	h.mu.Lock()
	h.nextID++
	id := h.nextID
	h.mu.Unlock()
	idJSON, _ := json.Marshal(id)
	h.write(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  any             `json:"params,omitempty"`
	}{JSONRPC: "2.0", ID: idJSON, Method: method, Params: params})
	for {
		raw := h.read()
		var probe struct {
			ID     json.RawMessage `json:"id,omitempty"`
			Method string          `json:"method,omitempty"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		if probe.Method != "" {
			// Server-initiated request: ack with empty result.
			if len(probe.ID) > 0 {
				h.write(struct {
					JSONRPC string          `json:"jsonrpc"`
					ID      json.RawMessage `json:"id"`
					Result  any             `json:"result"`
				}{JSONRPC: "2.0", ID: probe.ID, Result: nil})
			}
			continue
		}
		var resp responseMessage
		if err := json.Unmarshal(raw, &resp); err != nil {
			continue
		}
		if string(resp.ID) != string(idJSON) {
			continue
		}
		return resp.Result, resp.Error
	}
}

func (h *benchHarness) notify(method string, params any) {
	h.write(notificationMessage{JSONRPC: "2.0", Method: method, Params: params})
}

// awaitDiagnostics reads frames until publishDiagnostics arrives for
// uri. Fails the benchmark on timeout so a stuck server doesn't
// silently produce meaningless samples.
func (h *benchHarness) awaitDiagnostics(b *testing.B, uri string, timeout time.Duration) {
	b.Helper()
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
		// Server-side request: ack with empty result.
		if len(probe.ID) > 0 {
			h.write(struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      json.RawMessage `json:"id"`
				Result  any             `json:"result"`
			}{JSONRPC: "2.0", ID: probe.ID, Result: nil})
		}
	}
	b.Fatalf("timeout waiting for publishDiagnostics on %s", uri)
}
