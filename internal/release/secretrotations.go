// Package release: secret-rotations subcommands.
//
// This file ports the two bun-executed TypeScript scripts that
// formerly lived under .github/scripts/:
//
//   - check-secret-rotations.ts: walks the per-secret docs,
//     computes (today - lastRotated) for each, and opens a
//     labelled GitHub issue when a secret is within the
//     reminder window (or already overdue).
//   - record-rotation.ts: rewrites the `lastRotated:` line in
//     one per-secret file in place.
//
// Both are invoked by .github/workflows/{secret-rotation-
// reminder.yml, record-secret-rotation.yml}. Bringing them into
// the existing mdsmith-release Go CLI removes the bun runtime
// and per-script package manifest from the repo, and lets the
// pure logic ride the same `go test` suite the rest of the
// release tooling does.
package release

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// RotationsDirName is the path (relative to the repo root) that
// holds one markdown file per tracked secret. The per-secret
// files carry the canonical front matter consumed by both
// CheckSecretRotations and RecordRotation.
const RotationsDirName = "docs/development/secret-rotations"

// ReminderWindowDays is the lead time before `lastRotated +
// periodDays` at which the reminder workflow opens a labelled
// issue. Matches the value the previous TS scripts used.
const ReminderWindowDays = 30

// IssueLabel is the GitHub label every reminder issue carries so
// the `record-rotation` workflow can find and close it.
const IssueLabel = "secret-rotation"

// RotationEntry is the parsed per-secret front matter. Field
// names match the YAML keys exactly (camelCase per the existing
// docs).
type RotationEntry struct {
	Title       string
	LastRotated string
	PeriodDays  int
	Provider    string
	IssuerURL   string
	UsedBy      string
	Scope       string
}

// DueState classifies a rotation: not yet due, within the
// reminder window, or overdue.
type DueState string

// Possible DueState values returned by ComputeDueState.
const (
	DueOK      DueState = "ok"
	DueDue     DueState = "due"
	DueOverdue DueState = "overdue"
)

// DueResult pairs the classification with a signed day count.
// DaysUntilDue is positive while the rotation is still in the
// future, zero on the due date, negative once past due. Callers
// format the value differently per status: "due in N days" /
// "due today" / "OVERDUE by N days" (negate).
type DueResult struct {
	Status       DueState
	DaysUntilDue int
}

// isoDateRe pre-checks the YYYY-MM-DD shape. The full validator
// also round-trips the parsed value to reject calendar-invalid
// dates that time.Parse silently normalizes (e.g. 2026-02-31).
var isoDateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// IsISODate reports whether s is a real calendar date in
// YYYY-MM-DD form. Round-trips the parsed components so
// normalized invalid dates are rejected.
func IsISODate(s string) bool {
	if !isoDateRe.MatchString(s) {
		return false
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return false
	}
	return t.Format("2006-01-02") == s
}

// UTCToday returns today's date as UTC midnight. Cron schedules
// run in UTC, so the due-state computation must match.
func UTCToday(now time.Time) time.Time {
	u := now.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// DaysBetween returns the integer day count between two UTC
// midnights. Positive when later > earlier.
func DaysBetween(later, earlier time.Time) int {
	const day = 24 * time.Hour
	return int(later.Sub(earlier) / day)
}

// ComputeDueState returns the due-state of a rotation given the
// current UTC date, the parsed lastRotated date, and the period.
func ComputeDueState(today, lastRotated time.Time, periodDays int) DueResult {
	dueOn := lastRotated.AddDate(0, 0, periodDays)
	d := DaysBetween(dueOn, today)
	switch {
	case d < 0:
		return DueResult{Status: DueOverdue, DaysUntilDue: d}
	case d <= ReminderWindowDays:
		return DueResult{Status: DueDue, DaysUntilDue: d}
	default:
		return DueResult{Status: DueOK, DaysUntilDue: d}
	}
}

// FrontMatterError describes a malformed per-secret file. Tests
// match on the wrapped error's message; the path prefix gives
// the maintainer a clear pointer to the bad file.
type FrontMatterError struct {
	Path string
	Msg  string
}

func (e *FrontMatterError) Error() string { return fmt.Sprintf("%s: %s", e.Path, e.Msg) }

// ParseFrontMatter extracts and YAML-parses the front matter
// block of a markdown file. The block must be fenced by `---\n`
// on both sides and the root must be a YAML mapping.
func ParseFrontMatter(text, path string) (map[string]any, error) {
	if !strings.HasPrefix(text, "---\n") {
		return nil, &FrontMatterError{Path: path, Msg: "no front matter (must start with '---\\n')"}
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---\n")
	if end == -1 {
		return nil, &FrontMatterError{Path: path, Msg: "unterminated front matter"}
	}
	var parsed any
	if err := yaml.Unmarshal([]byte(rest[:end]), &parsed); err != nil {
		return nil, &FrontMatterError{Path: path, Msg: fmt.Sprintf("front matter is not valid YAML: %v", err)}
	}
	m, ok := parsed.(map[string]any)
	if !ok {
		return nil, &FrontMatterError{Path: path, Msg: "front matter is not a mapping"}
	}
	return m, nil
}

// requiredFrontMatterKeys is the set of fields every per-secret
// file must declare. Order is fixed so the "missing key" error
// surfaces the same key first regardless of YAML key ordering.
var requiredFrontMatterKeys = []string{
	"title", "lastRotated", "periodDays", "provider", "issuerUrl", "usedBy", "scope",
}

// ValidateRotationEntry projects a front-matter map into a
// RotationEntry, failing loudly on missing keys, malformed
// lastRotated, non-integer periodDays, and non-positive
// periodDays. Separated from LoadRotations so the validation is
// reachable from unit tests without filesystem fixtures.
func ValidateRotationEntry(fm map[string]any, path string) (RotationEntry, error) {
	for _, key := range requiredFrontMatterKeys {
		if _, ok := fm[key]; !ok {
			return RotationEntry{}, &FrontMatterError{Path: path, Msg: fmt.Sprintf("front matter missing `%s`", key)}
		}
	}
	last, err := asString(fm["lastRotated"])
	if err != nil {
		return RotationEntry{}, &FrontMatterError{Path: path,
			Msg: fmt.Sprintf("`lastRotated` is not a string (%v)", fm["lastRotated"])}
	}
	if !IsISODate(last) {
		return RotationEntry{}, &FrontMatterError{Path: path,
			Msg: fmt.Sprintf("`lastRotated` is not a valid ISO-8601 date (%q)", last)}
	}
	period, err := asInt(fm["periodDays"])
	if err != nil {
		return RotationEntry{}, &FrontMatterError{Path: path,
			Msg: fmt.Sprintf("`periodDays` is not an integer (%v)", fm["periodDays"])}
	}
	// A zero or negative period would compute a due date on or
	// before lastRotated, so every run would treat the secret as
	// overdue and the reminder workflow would never go quiet.
	// Reject the value at load time with a clear pointer to the
	// bad file rather than silently spamming issues.
	if period <= 0 {
		return RotationEntry{}, &FrontMatterError{Path: path,
			Msg: fmt.Sprintf("`periodDays` must be a positive integer (got %d)", period)}
	}
	title, err := asString(fm["title"])
	if err != nil {
		return RotationEntry{}, &FrontMatterError{Path: path,
			Msg: fmt.Sprintf("`title` is not a string (%v)", fm["title"])}
	}
	// `existingOpenIssue` builds a GitHub search expression by
	// surrounding the issue title with double quotes; a literal
	// `"` in the title would either truncate the search phrase
	// or shift it relative to the client-side exact-match check.
	// Reject the value at load time rather than risk silent
	// duplicate issues on every monthly reminder run.
	if strings.Contains(title, `"`) {
		return RotationEntry{}, &FrontMatterError{Path: path,
			Msg: "`title` must not contain a double-quote character"}
	}
	provider, _ := asString(fm["provider"])
	issuerURL, _ := asString(fm["issuerUrl"])
	usedBy, _ := asString(fm["usedBy"])
	scope, _ := asString(fm["scope"])
	return RotationEntry{
		Title:       title,
		LastRotated: last,
		PeriodDays:  period,
		Provider:    provider,
		IssuerURL:   issuerURL,
		UsedBy:      usedBy,
		Scope:       scope,
	}, nil
}

func asString(v any) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	return "", fmt.Errorf("not a string")
}

func asInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		if n != float64(int(n)) {
			return 0, fmt.Errorf("not an integer")
		}
		return int(n), nil
	case string:
		i, err := strconv.Atoi(n)
		if err != nil {
			return 0, err
		}
		return i, nil
	}
	return 0, fmt.Errorf("not an integer")
}

// LoadedRotation pairs a parsed entry with the base filename it
// came from. The filename feeds the issue body's "rotation
// procedure" link.
type LoadedRotation struct {
	Entry        RotationEntry
	FileBasename string
}

// Filesystem entry points are package-level vars so tests can
// swap them for failing fakes that exercise the IO error
// branches LoadRotations / FindEntry / RecordRotation guard. The
// production binary uses the os.* implementations.
var (
	fsReadDir   = os.ReadDir
	fsReadFile  = os.ReadFile
	fsWriteFile = os.WriteFile
)

// LoadRotations walks rotationsDir, parses every *.md file, and
// returns them sorted by title for deterministic iteration order.
func LoadRotations(rotationsDir string) ([]LoadedRotation, error) {
	entries, err := fsReadDir(rotationsDir)
	if err != nil {
		return nil, err
	}
	out := make([]LoadedRotation, 0, len(entries))
	for _, de := range entries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".md") {
			continue
		}
		path := filepath.Join(rotationsDir, de.Name())
		body, err := fsReadFile(path)
		if err != nil {
			return nil, err
		}
		fm, err := ParseFrontMatter(string(body), path)
		if err != nil {
			return nil, err
		}
		entry, err := ValidateRotationEntry(fm, path)
		if err != nil {
			return nil, err
		}
		out = append(out, LoadedRotation{Entry: entry, FileBasename: de.Name()})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Entry.Title < out[j].Entry.Title
	})
	return out, nil
}

// repoURL composes the absolute GitHub URL for this repository
// from the GITHUB_SERVER_URL + GITHUB_REPOSITORY env vars set by
// every GitHub Actions run. Falls back to the canonical mdsmith
// repo for local runs.
func repoURL(env Environ) string {
	server := env.Getenv("GITHUB_SERVER_URL")
	if server == "" {
		server = "https://github.com"
	}
	server = strings.TrimRight(server, "/")
	repo := env.Getenv("GITHUB_REPOSITORY")
	if repo == "" {
		repo = "jeduden/mdsmith"
	}
	return server + "/" + repo
}

// Environ abstracts os.Getenv for testability. The default
// (osEnviron) reads the process environment.
type Environ interface {
	Getenv(key string) string
}

type osEnviron struct{}

func (osEnviron) Getenv(key string) string { return os.Getenv(key) }

// MapEnviron is an in-memory Environ for tests.
type MapEnviron map[string]string

// Getenv returns the value of key in the map, or empty string.
func (m MapEnviron) Getenv(key string) string { return m[key] }

// IssueBody renders the GitHub issue body for one due/overdue
// rotation. The format mirrors the previous TS implementation
// byte-for-byte so a switch-over does not flip existing
// labelled issues' bodies on the next reminder run.
func IssueBody(entry RotationEntry, fileBasename string, due DueResult, env Environ) string {
	if env == nil {
		env = osEnviron{}
	}
	var headline string
	switch {
	case due.Status == DueOverdue:
		headline = fmt.Sprintf("`%s` is OVERDUE by %d days.", entry.Title, -due.DaysUntilDue)
	case due.DaysUntilDue == 0:
		headline = fmt.Sprintf("`%s` is due today.", entry.Title)
	default:
		headline = fmt.Sprintf("`%s` is due in %d days.", entry.Title, due.DaysUntilDue)
	}
	base := repoURL(env)
	fileURL := base + "/blob/main/docs/development/secret-rotations/" + fileBasename
	reminderURL := base + "/blob/main/.github/workflows/secret-rotation-reminder.yml"
	recordURL := base + "/actions/workflows/record-secret-rotation.yml"
	return strings.Join([]string{
		headline,
		"",
		"| Field | Value |",
		"|---|---|",
		"| Provider | " + entry.Provider + " |",
		"| Issuer URL | <" + entry.IssuerURL + "> |",
		"| Used by | " + entry.UsedBy + " |",
		"| Scope | " + entry.Scope + " |",
		"| lastRotated | " + entry.LastRotated + " |",
		fmt.Sprintf("| Period (days) | %d |", entry.PeriodDays),
		"",
		"Rotation procedure:",
		fileURL,
		"",
		"After rotating the credential at the issuer, do not",
		"hand-edit the front matter or close this issue.",
		"Instead, run the **Record Secret Rotation** workflow:",
		recordURL,
		"",
		"Pick `" + entry.Title + "` from the dropdown and click `Run workflow`.",
		"The workflow opens a PR that updates `lastRotated`",
		"and includes `Closes #` referencing this issue, so",
		"the merge both records the rotation and closes this",
		"reminder in one step.",
		"",
		"This reminder was opened automatically by " + reminderURL + ".",
		"",
	}, "\n")
}

// splitFrontMatter splits a markdown file into the literal `---\n`
// opener, the YAML block between the fences (fences NOT
// included), and the closing `\n---\n` fence plus the rest of
// the document. The three pieces round-trip back to the original
// bytes when concatenated.
type splitFrontMatterResult struct {
	Opening         string
	YAMLBlock       string
	ClosingPlusBody string
}

func splitFrontMatter(text, path string) (splitFrontMatterResult, error) {
	if !strings.HasPrefix(text, "---\n") {
		return splitFrontMatterResult{}, &FrontMatterError{Path: path, Msg: "no front matter"}
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---\n")
	if end == -1 {
		return splitFrontMatterResult{}, &FrontMatterError{Path: path, Msg: "unterminated front matter"}
	}
	return splitFrontMatterResult{
		Opening:         text[:4],
		YAMLBlock:       text[4 : 4+end],
		ClosingPlusBody: text[4+end:],
	}, nil
}

// lastRotatedRe matches a `lastRotated:` key (optionally
// indented) followed by either a double-quoted, single-quoted,
// or bare value. The value match stops at the first whitespace
// or `#`, so a trailing inline comment is preserved by the
// surrounding text outside the match. The (?m) flag enables ^
// anchoring per-line within the YAML block.
var lastRotatedRe = regexp.MustCompile(`(?m)(^[ \t]*lastRotated:[ \t]*)(?:"[^"\n]*"|'[^'\n]*'|[^\s#"'][^\s#]*)`)

// UpdateLastRotated rewrites the `lastRotated:` line in a YAML
// front-matter block to the new date. Quoting is normalized to
// double quotes regardless of source quoting. Returns the
// rewritten block, or an error if no `lastRotated:` line was
// found in the input.
func UpdateLastRotated(yamlBlock, date, path string) (string, error) {
	matched := false
	out := lastRotatedRe.ReplaceAllStringFunc(yamlBlock, func(match string) string {
		matched = true
		sub := lastRotatedRe.FindStringSubmatch(match)
		// sub[1] is the preamble (indent + key + spaces). The
		// value matched as the rest is dropped.
		return sub[1] + `"` + date + `"`
	})
	if !matched {
		return "", &FrontMatterError{Path: path, Msg: "could not locate `lastRotated:` line"}
	}
	return out, nil
}

// FindEntryResult is what FindEntry returns: the matched path
// and the sorted set of titles seen during the scan.
type FindEntryResult struct {
	Path   string
	Titles []string
}

// FindEntry locates the per-secret file whose front matter
// `title` matches entryTitle. Fails loudly on any malformed
// per-secret file and on duplicate titles across files, so a
// rewrite of `lastRotated` cannot mutate the wrong file or
// silently shadow an unrelated entry.
func FindEntry(rotationsDir, entryTitle string) (FindEntryResult, error) {
	entries, err := fsReadDir(rotationsDir)
	if err != nil {
		return FindEntryResult{}, err
	}
	titleToPath := make(map[string]string)
	for _, de := range entries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".md") {
			continue
		}
		path := filepath.Join(rotationsDir, de.Name())
		body, err := fsReadFile(path)
		if err != nil {
			return FindEntryResult{}, err
		}
		fm, err := ParseFrontMatter(string(body), path)
		if err != nil {
			return FindEntryResult{}, err
		}
		titleAny, ok := fm["title"]
		if !ok {
			return FindEntryResult{}, &FrontMatterError{Path: path, Msg: "front matter `title` is missing"}
		}
		title, ok := titleAny.(string)
		if !ok {
			return FindEntryResult{}, &FrontMatterError{Path: path, Msg: "front matter `title` is not a string"}
		}
		if prev, dup := titleToPath[title]; dup {
			return FindEntryResult{}, fmt.Errorf("duplicate title %q in %s and %s; titles must be unique",
				title, prev, path)
		}
		titleToPath[title] = path
	}
	titles := make([]string, 0, len(titleToPath))
	for t := range titleToPath {
		titles = append(titles, t)
	}
	sort.Strings(titles)
	match, ok := titleToPath[entryTitle]
	if !ok {
		return FindEntryResult{}, fmt.Errorf("unknown title %q; known: %s",
			entryTitle, strings.Join(titles, ", "))
	}
	return FindEntryResult{Path: match, Titles: titles}, nil
}

// RecordRotation updates the `lastRotated` field of the
// per-secret file whose `title` matches entryTitle. Returns
// (true, nil) if the file was rewritten, (false, nil) if the
// date was already at the requested value (no-op), or
// (false, err) on any validation failure.
func RecordRotation(repoRoot, entryTitle, date string) (changed bool, err error) {
	if !IsISODate(date) {
		return false, fmt.Errorf("invalid date %q: not a valid ISO-8601 date", date)
	}
	rotationsDir := filepath.Join(repoRoot, RotationsDirName)
	found, err := FindEntry(rotationsDir, entryTitle)
	if err != nil {
		return false, err
	}
	body, err := fsReadFile(found.Path)
	if err != nil {
		return false, err
	}
	parts, err := splitFrontMatter(string(body), found.Path)
	if err != nil {
		return false, err
	}
	updated, err := UpdateLastRotated(parts.YAMLBlock, date, found.Path)
	if err != nil {
		return false, err
	}
	if updated == parts.YAMLBlock {
		return false, nil
	}
	newText := parts.Opening + updated + parts.ClosingPlusBody
	if err := fsWriteFile(found.Path, []byte(newText), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// GHRunner is the injection point for the `gh` CLI. Production
// uses ExecGH (shells out to the `gh` binary on PATH); tests
// pass a fake function that records invocations and returns
// canned output. Returns the stdout bytes; on a non-zero exit
// the error wraps the gh stderr so workflow logs can diagnose
// the failure.
type GHRunner func(args []string) ([]byte, error)

// ExecGH is the default GHRunner: shells out to `gh` on PATH.
// On a non-zero exit, the returned error embeds the gh stderr
// text so the workflow log shows what gh actually said rather
// than the bare "exit status N" message.
func ExecGH(args []string) ([]byte, error) {
	cmd := exec.Command("gh", args...) // #nosec G204 -- gh is fixed; args are workflow-internal
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return out, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return out, err
	}
	return out, nil
}

// CheckRotationsOptions configures CheckSecretRotations. Now is
// the wall-clock time to compute due-state from; GH is the gh
// runner (overridable for tests). Env is the environment reader
// used to render issue bodies (GITHUB_SERVER_URL etc).
type CheckRotationsOptions struct {
	Now time.Time
	GH  GHRunner
	Env Environ
}

// CheckSecretRotationsResult records the per-run outcome the
// workflow log surfaces.
type CheckSecretRotationsResult struct {
	Opened  []string
	Skipped []string
}

// CheckSecretRotations is the body of the scheduled reminder
// workflow. Walks the per-secret docs, classifies each, and
// opens or skips a labelled issue per entry. The `gh` CLI is
// shelled out via opts.GH for the existing-issue lookup and
// the create/label calls — same as the prior TS implementation,
// no new go-github dependency. Tests inject a fake GHRunner so
// they never touch the real `gh` binary.
func CheckSecretRotations(repoRoot string, opts CheckRotationsOptions) (CheckSecretRotationsResult, error) {
	if opts.GH == nil {
		opts.GH = ExecGH
	}
	if opts.Env == nil {
		opts.Env = osEnviron{}
	}
	rotationsDir := filepath.Join(repoRoot, RotationsDirName)
	rotations, err := LoadRotations(rotationsDir)
	if err != nil {
		return CheckSecretRotationsResult{}, err
	}
	if len(rotations) == 0 {
		return CheckSecretRotationsResult{}, fmt.Errorf("%s: no per-secret files found", rotationsDir)
	}
	today := UTCToday(opts.Now)
	var res CheckSecretRotationsResult
	labelEnsured := false
	for _, r := range rotations {
		lastRotated, _ := time.Parse("2006-01-02", r.Entry.LastRotated)
		due := ComputeDueState(today, lastRotated, r.Entry.PeriodDays)
		if due.Status == DueOK {
			continue
		}
		title := fmt.Sprintf("Rotate %s (lastRotated %s)", r.Entry.Title, r.Entry.LastRotated)
		num, err := existingOpenIssue(opts.GH, title)
		if err != nil {
			return res, err
		}
		if num > 0 {
			res.Skipped = append(res.Skipped, r.Entry.Title)
			continue
		}
		if !labelEnsured {
			if err := ensureLabel(opts.GH); err != nil {
				return res, err
			}
			labelEnsured = true
		}
		body := IssueBody(r.Entry, r.FileBasename, due, opts.Env)
		assignee := opts.Env.Getenv("REMINDER_ASSIGNEE")
		if err := createIssue(opts.GH, title, body, IssueLabel, assignee); err != nil {
			return res, err
		}
		res.Opened = append(res.Opened, r.Entry.Title)
	}
	return res, nil
}

// existingOpenIssue narrows the candidate set server-side via
// GitHub search. `in:title "<phrase>"` matches issues whose
// title contains the quoted phrase; the exact-string check
// below catches GitHub search's tokenized/fuzzy behavior — only
// a byte-for-byte title match is treated as an existing issue.
func existingOpenIssue(gh GHRunner, title string) (int, error) {
	// `validateRotationEntry` rejects titles containing `"`, so
	// the literal-quote wrap below is safe to compose without
	// escaping: the search expression below is the same string
	// the client-side exact-match check uses.
	search := `in:title "` + title + `"`
	out, err := gh([]string{
		"issue", "list",
		"--state", "open",
		"--label", IssueLabel,
		"--search", search,
		"--json", "number,title",
		"--limit", "100",
	})
	if err != nil {
		return 0, fmt.Errorf("gh issue list: %w", err)
	}
	var issues []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
	}
	if len(out) > 0 {
		if err := json.Unmarshal(out, &issues); err != nil {
			return 0, fmt.Errorf("gh issue list: parsing JSON output: %w", err)
		}
	}
	for _, issue := range issues {
		if issue.Title == title {
			return issue.Number, nil
		}
	}
	return 0, nil
}

func ensureLabel(gh GHRunner) error {
	if _, err := gh([]string{
		"label", "create", IssueLabel,
		"--force",
		"--color", "C5DEF5",
		"--description", "Long-lived secret is due (or overdue) for rotation",
	}); err != nil {
		return fmt.Errorf("gh label create %s: %w", IssueLabel, err)
	}
	return nil
}

func createIssue(gh GHRunner, title, body, label, assignee string) error {
	args := []string{"issue", "create", "--title", title, "--body", body, "--label", label}
	if assignee != "" {
		args = append(args, "--assignee", assignee)
	}
	if _, err := gh(args); err != nil {
		return fmt.Errorf("gh issue create: %w", err)
	}
	return nil
}
