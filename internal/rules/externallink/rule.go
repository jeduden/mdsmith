// Package externallink implements MDS072, an opt-in rule that
// validates external `http://` and `https://` URLs by making an HTTP
// HEAD request (falling back to GET on 405) and reporting any URL that
// returns a transport error, a 4xx, or a 5xx response. Results are
// cached per URL for the run so a URL referenced in many files costs at
// most one request. See plan/2606280208_external-link-check.md and
// issue #47 for the design.
//
// The HTTP probing lives in probe_net.go, behind a `!(js && wasm)`
// build tag. The js/wasm build (probe_wasm.go) cannot reach the network,
// so it returns a not-probed result (probed=false) that yields no
// diagnostic — the rule reports a URL as neither broken nor healthy
// rather than faking a 200. That keeps net/http out of the WebAssembly
// artifact (a ~6 MB cost) while staying honest: a future host bridge
// (plan 2607170527) will let a wasm host supply real probed results.
// Config parsing and AST traversal are shared, so `.mdsmith.yml`
// validates identically on every platform.
//
// Concurrency: the engine clones one Rule per worker, so the rate-limit
// semaphore and the per-URL result cache are PACKAGE-LEVEL (not Rule
// fields) — otherwise each worker's clone would hold its own semaphore
// and the configured concurrency cap would bound nothing. A
// singleflight group collapses concurrent probes of the same URL onto a
// single request, so the "one request per URL per run" guarantee holds
// even when two workers hit the same URL at once.
//
// Cache lifetime: the result cache lives for the process. That fits the
// short-lived CLI, but a long-lived native `mdsmith lsp` session keeps a
// probed URL's result for the editor's lifetime and never re-checks it;
// eviction is a documented follow-up, not implemented here.
package externallink

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	goldast "github.com/jeduden/mdsmith/pkg/goldmark/ast"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

func init() {
	rule.Register(newRule())
}

// newRule builds a Rule with the built-in defaults already applied, so
// an instance that is enabled with the bare `external-link-check: true`
// form — which never calls ApplySettings (checker.ConfigureRule returns
// the rule unchanged when cfg.Settings is nil) — still probes with a 5s
// timeout and a concurrency of 10. CloneInstance copies these fields, so
// every per-worker clone inherits them too.
func newRule() *Rule {
	return &Rule{links: externalLinkConfig{
		Timeout:   defaultTimeout,
		RateLimit: defaultRateLimit,
		MaxProbes: defaultMaxProbes,
	}}
}

const (
	defaultTimeout   = 5 * time.Second
	defaultRateLimit = 10
	defaultMaxProbes = 1000
)

// externalLinkConfig holds the keys MDS072 reads from the shared
// `links:` block.
//
//   - Skip: regex patterns from `external-skip`
//   - Timeout: `external-timeout` (default 5s)
//   - RateLimit: `external-rate-limit` (default 10, min 1)
//   - AllowInternal: `external-allow-internal` (default false); when false the
//     SSRF guard blocks loopback, private, link-local, ULA, and metadata IPs
//   - MaxProbes: `external-max-probes` (default 1000); bounds total distinct
//     URLs probed per run; 0 means unlimited
type externalLinkConfig struct {
	Skip          []string
	Timeout       time.Duration
	RateLimit     int
	AllowInternal bool
	MaxProbes     int
}

// Rule validates external URLs over HTTP.
type Rule struct {
	links    externalLinkConfig
	skipRegs []*regexp.Regexp
}

// Package-level probe state, shared across every per-worker Rule clone.
//
//   - urlCache stores each URL's outcome so a URL referenced across many
//     files is fetched once per run.
//   - probeGroup collapses concurrent probes of the same URL onto one
//     request (the cache alone is check-then-act and would let two
//     workers double-probe).
//   - semaphore is the global in-flight cap. It is sized once, on the
//     first probe, from that Rule's RateLimit; one config per run makes
//     that deterministic.
//   - probeCount is the number of distinct URLs actually probed this run.
//     It is incremented inside the singleflight fn (once per unique URL)
//     and compared against probeMax to enforce external-max-probes.
//   - probeMax is set once (probeMaxOnce) from the first Rule's MaxProbes.
var (
	urlCache     sync.Map
	probeGroup   singleflight.Group
	semaphore    chan struct{}
	semOnce      sync.Once
	probeCount   atomic.Int64
	probeMax     int
	probeMaxOnce sync.Once
)

// urlResult is one cached probe outcome. probed reports whether the URL
// was actually reached: a prober that cannot make network requests (the
// wasm build with no host bridge) returns probed=false, and a not-probed
// URL never produces a diagnostic — neither a false failure nor a false
// pass. When probed is true, statusCode is the final HTTP status (0 when
// err is non-nil) and err is the transport error, if any.
//
// capped is set when the URL was skipped because the per-run probe ceiling
// (external-max-probes) was reached. A capped URL is neither probed=true
// nor probed=false in the normal sense: it was not attempted at all, and
// failureMessage surfaces it as "not probed: per-run limit reached" so the
// user knows their coverage was truncated.
type urlResult struct {
	probed     bool
	statusCode int
	err        error
	capped     bool
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS072" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "external-link-check" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "link" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule. It collects every external http/https URL
// in f (inline links, autolinks, and images; the AST walk resolves the
// same destinations linkgraph would, plus autolinks linkgraph drops),
// probes each one, and emits a diagnostic for failures.
//
// The engine only calls Check on an enabled rule, so there is no
// "unconfigured" guard here: a registered or cloned Rule always carries
// the built-in defaults (see newRule), so RateLimit is never zero on a
// real run. The network-bound alloc/bench gates skip this rule rather
// than measure a rule that would otherwise hit the network.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f == nil || f.AST == nil {
		return nil
	}

	var diags []lint.Diagnostic
	_ = goldast.Walk(f.AST, func(n goldast.Node, entering bool) (goldast.WalkStatus, error) {
		if !entering {
			return goldast.WalkContinue, nil
		}
		raw, ok := externalURL(n, f.Source)
		if !ok {
			return goldast.WalkContinue, nil
		}
		if r.skip(raw) {
			return goldast.WalkContinue, nil
		}
		res := r.checkURL(raw)
		if msg := failureMessage(raw, res); msg != "" {
			line, col := r.position(f, n, raw)
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   col,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  msg,
			})
		}
		return goldast.WalkContinue, nil
	})
	return diags
}

// externalURL returns the raw http/https destination of an AST node
// (link, autolink, or image) when it carries one, or ok=false otherwise.
// Non-http(s) schemes (mailto:, data:) and local destinations are
// rejected here so the caller never probes them.
func externalURL(n goldast.Node, source []byte) (string, bool) {
	var raw string
	switch node := n.(type) {
	case *goldast.Link:
		raw = string(node.Destination)
	case *goldast.Image:
		raw = string(node.Destination)
	case *goldast.AutoLink:
		raw = string(node.URL(source))
	default:
		return "", false
	}
	if !isExternalHTTP(raw) {
		return "", false
	}
	return raw, true
}

// isExternalHTTP reports whether raw parses as an absolute http or
// https URL.
func isExternalHTTP(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// skip reports whether raw matches any compiled external-skip pattern.
func (r *Rule) skip(raw string) bool {
	for _, re := range r.skipRegs {
		if re.MatchString(raw) {
			return true
		}
	}
	return false
}

// position returns the 1-based body-relative line and column of an AST
// node. A Link or Image carries its text in a descendant *Text segment,
// so the first-text-offset walk locates it. An AutoLink stores its text
// in a private value node with no walkable child, so the walk would find
// nothing and collapse every autolink diagnostic onto (1, 1); for those
// the literal `<url>` is located in the enclosing block's source
// instead.
func (r *Rule) position(f *lint.File, n goldast.Node, raw string) (int, int) {
	if _, ok := n.(*goldast.AutoLink); ok {
		return autolinkPosition(f, n, raw)
	}
	offset := firstTextOffset(n)
	if offset < 0 {
		return 1, 1
	}
	return f.LineOfOffset(offset), f.ColumnOfOffset(offset)
}

// firstTextOffset returns the source offset of n's earliest descendant
// text segment, or -1 when n has none.
func firstTextOffset(n goldast.Node) int {
	offset := -1
	_ = goldast.Walk(n, func(cur goldast.Node, entering bool) (goldast.WalkStatus, error) {
		if !entering {
			return goldast.WalkContinue, nil
		}
		if t, ok := cur.(*goldast.Text); ok {
			if offset == -1 || t.Segment.Start < offset {
				offset = t.Segment.Start
			}
		}
		return goldast.WalkContinue, nil
	})
	return offset
}

// autolinkPosition locates the literal `<url>` in the source of the
// autolink's nearest block ancestor and returns its line/column. Lines()
// panics on inline nodes, so only block ancestors are searched; a URL
// that is not found (e.g. a synthesized node) falls back to (1, 1).
//
// It reports the FIRST `<url>` occurrence in the block, so two identical
// autolinks in one block both anchor to the first — a minor column
// inaccuracy on the duplicate (the URL is still correctly flagged and
// deduped by the cache). This matches linkstyle.autolinkPosition; a
// rare case not worth per-occurrence bookkeeping.
func autolinkPosition(f *lint.File, n goldast.Node, rawURL string) (int, int) {
	if rawURL == "" {
		return 1, 1
	}
	pat := make([]byte, 0, len(rawURL)+2)
	pat = append(pat, '<')
	pat = append(pat, rawURL...)
	pat = append(pat, '>')
	for p := n.Parent(); p != nil; p = p.Parent() {
		if p.Type() != goldast.TypeBlock {
			continue
		}
		lines := p.Lines()
		for i := range lines.Len() {
			seg := lines.At(i)
			if idx := bytes.Index(f.Source[seg.Start:seg.Stop], pat); idx >= 0 {
				off := seg.Start + idx
				return f.LineOfOffset(off), f.ColumnOfOffset(off)
			}
		}
		break
	}
	return 1, 1
}

// failureMessage returns the diagnostic message for a probe result, or
// "" when the URL is healthy (a 2xx or 3xx response) or was not probed.
// A not-probed URL (probed=false, capped=false) is never reported: a
// host that cannot reach the network must not flag every link, nor pass
// every link.
//
// A capped URL (capped=true) yields a distinct "not probed: per-run
// limit reached" message so the user knows their coverage was truncated
// rather than assuming all un-flagged URLs were verified healthy.
func failureMessage(raw string, res urlResult) string {
	if res.capped {
		return "external URL not probed: per-run limit reached (links.external-max-probes): " + raw
	}
	if !res.probed {
		return ""
	}
	if res.err != nil {
		return fmt.Sprintf("external URL unreachable: %s (%v)", raw, res.err)
	}
	if res.statusCode >= 400 {
		return fmt.Sprintf("external URL returned HTTP %d: %s", res.statusCode, raw)
	}
	return ""
}

// checkURL probes raw once and caches the result. A cache hit (this run
// or a sibling file) returns immediately with no network I/O. On a miss
// the probe runs inside a singleflight group keyed by the URL, so
// concurrent workers that miss the same URL share one request rather
// than each issuing their own; the probe itself acquires a global
// rate-limit slot. Once the first probe stores its result, every later
// caller takes the outer cache-hit path, so no second request is issued.
//
// When the per-run probe ceiling (external-max-probes) is reached, the
// fn returns a capped result without probing. The ceiling is set once
// (probeMaxOnce) from this Rule's MaxProbes; probeCount tracks distinct
// URLs probed so far.
func (r *Rule) checkURL(raw string) urlResult {
	if v, ok := urlCache.Load(raw); ok {
		return v.(urlResult)
	}
	v, _, _ := probeGroup.Do(raw, func() (any, error) {
		if r.links.MaxProbes > 0 {
			probeMaxOnce.Do(func() { probeMax = r.links.MaxProbes })
			// Atomically claim a probe slot. If the new count exceeds the
			// ceiling, give back the slot and cache the capped result so
			// subsequent calls for the same URL take the fast cache-hit path.
			if probeCount.Add(1) > int64(probeMax) {
				probeCount.Add(-1)
				res := urlResult{capped: true}
				urlCache.Store(raw, res)
				return res, nil
			}
		}
		r.acquire()
		defer r.release()
		res := probe(raw, r.links.Timeout, r.links.AllowInternal)
		urlCache.Store(raw, res)
		return res, nil
	})
	return v.(urlResult)
}

// acquire takes a global rate-limit slot, sizing the semaphore on first
// use from this Rule's RateLimit (min 1). release returns the slot.
func (r *Rule) acquire() {
	semOnce.Do(func() {
		n := r.links.RateLimit
		if n < 1 {
			n = 1
		}
		semaphore = make(chan struct{}, n)
	})
	semaphore <- struct{}{}
}

func (r *Rule) release() { <-semaphore }

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	// Defaults take effect whenever ApplySettings runs, so a rule
	// configured with a partial `links:` block still probes with a 5s
	// timeout and a concurrency of 10. AllowInternal resets to false
	// (SSRF guard on) and MaxProbes resets to defaultMaxProbes so a
	// partial override never silently disables the guard or the ceiling.
	// Skip resets to nil so that removing external-skip from the config
	// takes effect without a process restart.
	r.links.Timeout = defaultTimeout
	r.links.RateLimit = defaultRateLimit
	r.links.AllowInternal = false
	r.links.MaxProbes = defaultMaxProbes
	r.links.Skip = nil

	for k, v := range settings {
		switch k {
		case "links":
			m, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("external-link-check: links must be a map, got %T", v)
			}
			if err := r.applyLinks(m); err != nil {
				return err
			}
		default:
			return fmt.Errorf("external-link-check: unknown setting %q", k)
		}
	}
	return r.compileSkip()
}

func (r *Rule) applyLinks(m map[string]any) error {
	for k, v := range m {
		switch k {
		case "external-skip":
			list, ok := toStringSlice(v)
			if !ok {
				return fmt.Errorf(
					"external-link-check: links.external-skip must be a list of strings, got %T", v)
			}
			r.links.Skip = list
		case "external-timeout":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf(
					"external-link-check: links.external-timeout must be a duration string, got %T", v)
			}
			d, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf(
					"external-link-check: links.external-timeout %q: %w", s, err)
			}
			if d <= 0 {
				d = defaultTimeout
			}
			r.links.Timeout = d
		case "external-rate-limit":
			n, ok := toInt(v)
			if !ok {
				return fmt.Errorf(
					"external-link-check: links.external-rate-limit must be an integer, got %T", v)
			}
			if n < 1 {
				n = 1
			}
			r.links.RateLimit = n
		case "external-allow-internal":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf(
					"external-link-check: links.external-allow-internal must be a bool, got %T", v)
			}
			r.links.AllowInternal = b
		case "external-max-probes":
			n, ok := toInt(v)
			if !ok {
				return fmt.Errorf(
					"external-link-check: links.external-max-probes must be an integer, got %T", v)
			}
			if n < 0 {
				n = 0
			}
			r.links.MaxProbes = n
		// Keys owned by MDS027 and MDS068; tolerated so one shared
		// links: block configures every link rule. No-ops here.
		case "style", "site-root", "validate-images", "validate-reference-style":
			// no-op for external-link-check
		default:
			return fmt.Errorf("external-link-check: unknown links setting %q", k)
		}
	}
	return nil
}

// compileSkip compiles the external-skip patterns once, after settings
// are parsed, so Check pays only a MatchString per URL.
func (r *Rule) compileSkip() error {
	r.skipRegs = r.skipRegs[:0]
	for _, pat := range r.links.Skip {
		re, err := regexp.Compile(pat)
		if err != nil {
			return fmt.Errorf(
				"external-link-check: links.external-skip pattern %q: %w", pat, err)
		}
		r.skipRegs = append(r.skipRegs, re)
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"links": map[string]any{
			"external-skip":           []string{},
			"external-timeout":        "5s",
			"external-rate-limit":     defaultRateLimit,
			"external-allow-internal": false,
			"external-max-probes":     defaultMaxProbes,
		},
	}
}

func toStringSlice(v any) ([]string, bool) {
	switch list := v.(type) {
	case []string:
		out := make([]string, len(list))
		copy(out, list)
		return out, true
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	return nil, false
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
