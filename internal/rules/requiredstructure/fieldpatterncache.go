package requiredstructure

import (
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/fieldinterp"
)

// fieldPatternCache memoizes buildFieldPattern's compiled regexes by
// body text, scoped to a single parseSchemaWithRootFS call: within that
// one call, buildSchemaHeading and collectBodySyncPoints can both hit
// the same {field} template text (e.g. a repeated table-row pattern),
// so a call-scoped cache avoids rebuilding an identical NFA.
//
// It is deliberately not a package-level var: bodyText is schema-
// authored Markdown, which churns during interactive editing in
// mdsmith lsp, unlike the bounded, static config patterns
// compiledPatterns caches in maxsectionlength and requiredtextpatterns
// — a process-lifetime cache here would grow with edit history instead
// of workspace size. parseSchemaWithCache already caches and
// invalidates the *parsedSchema this produces, so a fresh per-call
// cache costs nothing extra on the (already cached) common path.
//
// The zero value is ready to use and allocates its backing map lazily
// on the first miss, so a schema with no {field} text costs nothing.
// A nil *fieldPatternCache is also valid (get/put are no-ops), so
// callers that don't want caching can pass nil.
type fieldPatternCache struct {
	m map[string]*regexp.Regexp
}

func (c *fieldPatternCache) get(bodyText string) (*regexp.Regexp, bool) {
	if c == nil {
		return nil, false
	}
	re, ok := c.m[bodyText]
	return re, ok
}

func (c *fieldPatternCache) put(bodyText string, re *regexp.Regexp) {
	if c == nil {
		return
	}
	if c.m == nil {
		c.m = make(map[string]*regexp.Regexp)
	}
	c.m[bodyText] = re
}

// buildFieldPattern compiles a regex that matches a body line whose
// {field} placeholders have been replaced by any non-empty run. The
// pattern is always valid: each part is regexp.QuoteMeta'd and joined
// with ".+", so regexp.MustCompile never panics here. See
// fieldPatternCache's doc comment for cache's scope and lifetime.
func buildFieldPattern(bodyText string, cache *fieldPatternCache) *regexp.Regexp {
	if re, ok := cache.get(bodyText); ok {
		return re
	}
	parts := fieldinterp.SplitOnFields(bodyText)
	var patBuf strings.Builder
	patBuf.WriteString("^")
	for i, part := range parts {
		patBuf.WriteString(regexp.QuoteMeta(part))
		if i < len(parts)-1 {
			patBuf.WriteString(".+")
		}
	}
	patBuf.WriteString("$")
	compiled := regexp.MustCompile(patBuf.String())
	cache.put(bodyText, compiled)
	return compiled
}
