// Package externallink implements MDS071, an opt-in rule that
// validates external `http://` and `https://` URLs by making an HTTP
// HEAD request (falling back to GET on 405) and reporting any URL that
// returns a transport error, a 4xx, or a 5xx response. Results are
// cached per URL for the run so a URL referenced in many files costs at
// most one request. See plan/2606280208_external-link-check.md and
// issue #47 for the design.
package externallink

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"

	goldast "github.com/jeduden/mdsmith/pkg/goldmark/ast"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

func init() {
	rule.Register(&Rule{})
}

const (
	defaultTimeout   = 5 * time.Second
	defaultRateLimit = 10
)

// externalLinkConfig holds the keys MDS071 reads from the shared
// `links:` block. Skip is the list of regex patterns from
// `external-skip`; Timeout is `external-timeout` (default 5s);
// RateLimit is `external-rate-limit` (default 10, min 1). RateLimit's
// zero value doubles as the "ApplySettings never ran" sentinel that
// Check uses for its no-network early return.
type externalLinkConfig struct {
	Skip      []string
	Timeout   time.Duration
	RateLimit int
}

// Rule validates external URLs over HTTP.
type Rule struct {
	links    externalLinkConfig
	skipRegs []*regexp.Regexp

	// initOnce guards lazy construction of the semaphore and HTTP
	// client. The engine shares one Rule singleton across a worker
	// pool, so the first Check across all files fixes the run's
	// effective rate limit and timeout.
	initOnce   sync.Once
	semaphore  chan struct{}
	httpClient *http.Client
}

// urlCache is process-global so a URL referenced across many files is
// fetched once per run. The CLI is short-lived, so a per-process cache
// needs no eviction. Keys are raw URL strings; values are urlResult.
var urlCache sync.Map

// urlResult is one cached probe outcome. statusCode is the final HTTP
// status (0 when err is non-nil); err is the transport error, if any.
type urlResult struct {
	statusCode int
	err        error
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS071" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "external-link-check" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "link" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule. It collects every external http/https
// URL in f (inline links, reference-resolved destinations via the AST,
// and autolinks), probes each one, and emits a diagnostic for failures.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	// Unconfigured: ApplySettings was never called (RateLimit zero
	// value). Return nil with no allocations and, crucially, no
	// network I/O so the alloc-budget gate can run this rule.
	if r.links.RateLimit == 0 {
		return nil
	}
	if f == nil || f.AST == nil {
		return nil
	}
	r.initOnce.Do(r.init)

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
			line, col := r.position(f, n)
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
// (link or autolink) when it carries one, or ok=false otherwise.
// Non-http(s) schemes (mailto:, data:) and local destinations are
// rejected here so the caller never probes them.
func externalURL(n goldast.Node, source []byte) (string, bool) {
	var raw string
	switch node := n.(type) {
	case *goldast.Link:
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
// node by locating its first descendant text segment. AutoLink stores
// its text in a private value node, so the walk falls back to the
// node's own first text child via Lines when present.
func (r *Rule) position(f *lint.File, n goldast.Node) (int, int) {
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
	if offset < 0 {
		return 1, 1
	}
	return f.LineOfOffset(offset), f.ColumnOfOffset(offset)
}

// failureMessage returns the diagnostic message for a probe result, or
// "" when the URL is healthy (a 2xx or 3xx response).
func failureMessage(raw string, res urlResult) string {
	if res.err != nil {
		return fmt.Sprintf("external URL unreachable: %s (%v)", raw, res.err)
	}
	if res.statusCode >= 400 {
		return fmt.Sprintf("external URL returned HTTP %d: %s", res.statusCode, raw)
	}
	return ""
}

// init lazily builds the rate-limit semaphore and HTTP client from the
// configured settings. Called once via initOnce on the first Check.
func (r *Rule) init() {
	r.semaphore = make(chan struct{}, r.links.RateLimit)
	r.httpClient = &http.Client{Timeout: r.links.Timeout}
}

// checkURL probes raw once and caches the result. A cache hit (this run
// or a sibling file) returns immediately with no network I/O. On a
// miss it acquires a rate-limit slot, issues a HEAD (retrying with GET
// on 405), and stores the outcome.
func (r *Rule) checkURL(raw string) urlResult {
	if v, ok := urlCache.Load(raw); ok {
		return v.(urlResult)
	}

	r.semaphore <- struct{}{}
	res := r.probe(raw)
	<-r.semaphore

	// LoadOrStore so concurrent probes of the same URL converge on one
	// stored result rather than racing the map.
	actual, _ := urlCache.LoadOrStore(raw, res)
	return actual.(urlResult)
}

// probe issues the HEAD (then GET on 405) request and maps the outcome
// to a urlResult.
func (r *Rule) probe(raw string) urlResult {
	resp, err := r.do(http.MethodHead, raw)
	if err != nil {
		return urlResult{err: err}
	}
	if resp.StatusCode == http.StatusMethodNotAllowed {
		resp, err = r.do(http.MethodGet, raw)
		if err != nil {
			return urlResult{err: err}
		}
	}
	return urlResult{statusCode: resp.StatusCode}
}

// do performs one request with the given method and drains/closes the
// body so the connection can be reused.
func (r *Rule) do(method, raw string) (*http.Response, error) {
	req, err := http.NewRequest(method, raw, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	// The status code is all we need; close the body immediately.
	_ = resp.Body.Close()
	return resp, nil
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	// Defaults take effect whenever ApplySettings runs, so an enabled
	// rule with no `links:` block still probes with a 5s timeout and a
	// concurrency of 10 — and RateLimit is non-zero so Check does not
	// take its unconfigured early return.
	r.links.Timeout = defaultTimeout
	r.links.RateLimit = defaultRateLimit

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
			"external-skip":       []string{},
			"external-timeout":    "5s",
			"external-rate-limit": defaultRateLimit,
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
