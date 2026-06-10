// mdsmith-secreview is the review-time CLI for the
// mdsmith-security-review skill. It is intentionally NOT part of the
// user-facing `mdsmith` binary — its commands turn a findings.json into
// the three review outputs and grade a findings.json against a case's
// machine-checkable rubric, both only useful while running the skill.
//
// Usage:
//
//	mdsmith-secreview render <findings.json> [--out-dir DIR]
//	mdsmith-secreview grade --findings F --cases C --case ID
//	mdsmith-secreview grade --findings F --forbid-severity S... \
//	    --require-min-severity S --require-location-file F
//
// render exits non-zero on error. grade exits 0 when every constraint
// holds, 1 when a constraint fails, and 2 on an input error (a bad
// findings.json or a bad/vacuous rubric) so a model's bad output is
// distinguishable from a real failure.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/secreview"
)

const usageText = `Usage: mdsmith-secreview <command> [args]

Commands:
  render <findings.json> [--out-dir DIR]
                          Render findings.sarif, report.md, and
                          inline-annotations.json into DIR (default ".").
                          Point DIR at the per-audit directory
                          docs/security/<YYYY-MM-DD-slug>/.
  grade --findings F (--cases C --case ID | --forbid-severity S...
                      --require-min-severity S --require-location-file F)
                          Grade a findings.json against a case rubric (via
                          --cases/--case) or against constraints passed as
                          flags. Exit 0 pass, 1 fail, 2 input error.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches one subcommand and returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usageText)
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, usageText)
		return 0
	case "render":
		return runRender(rest, stdout, stderr)
	case "grade":
		return runGrade(rest, stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "mdsmith-secreview: unknown command %q\n%s", cmd, usageText)
		return 2
	}
}

// runRender implements `render <findings.json> [--out-dir DIR]`.
func runRender(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outDir := fs.String("out-dir", ".", "per-audit directory to write the three outputs into")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "Usage: mdsmith-secreview render <findings.json> [--out-dir DIR]\n")
	}
	if code := parseFlags(fs, args, stderr, "render"); code >= 0 {
		return code
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	report, err := secreview.LoadReport(fs.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "mdsmith-secreview: %v\n", err)
		return 1
	}
	if err := secreview.Render(report, *outDir); err != nil {
		_, _ = fmt.Fprintf(stderr, "mdsmith-secreview: %v\n", err)
		return 1
	}
	names := secreview.RenderFileNames()
	_, _ = fmt.Fprintf(stdout, "rendered %d finding(s) -> %s\n",
		len(report.Findings), strings.Join(names, ", "))
	return 0
}

// gradeFlags collects the grade subcommand's flag values.
type gradeFlags struct {
	findings    string
	cases       string
	caseID      string
	forbid      []string
	requireMin  string
	requireFile string
}

// runGrade implements the grade subcommand. Exit codes: 0 pass, 1 fail,
// 2 input error (bad findings.json or bad/vacuous rubric).
func runGrade(args []string, stdout, stderr io.Writer) int {
	gf, code := parseGradeFlags(args, stderr)
	if code >= 0 {
		return code
	}
	con, label, code := gradeConstraints(gf, stderr)
	if code >= 0 {
		return code
	}
	report, err := secreview.LoadReport(gf.findings)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "mdsmith-secreview: %v\n", err)
		return 2
	}
	failures := secreview.Grade(report.Findings, con)
	if len(failures) > 0 {
		_, _ = fmt.Fprintf(stderr, "FAIL %s\n", label)
		for _, f := range failures {
			_, _ = fmt.Fprintf(stderr, "  - %s\n", f)
		}
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "PASS %s\n", label)
	return 0
}

// parseGradeFlags parses the grade subcommand flags. It returns a code
// >= 0 when parsing or a required-flag check already decided the outcome.
func parseGradeFlags(args []string, stderr io.Writer) (gradeFlags, int) {
	fs := flag.NewFlagSet("grade", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var gf gradeFlags
	fs.StringVar(&gf.findings, "findings", "", "findings.json to grade (required)")
	fs.StringVar(&gf.cases, "cases", "", "cases.yaml to read the rubric from")
	fs.StringVar(&gf.caseID, "case", "", "case id within --cases")
	fs.StringArrayVar(&gf.forbid, "forbid-severity", nil, "severity that must not appear (repeatable)")
	fs.StringVar(&gf.requireMin, "require-min-severity", "", "minimum severity of a required finding")
	fs.StringVar(&gf.requireFile, "require-location-file", "", "primary-location file of the required finding")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, gradeUsage) }
	if code := parseFlags(fs, args, stderr, "grade"); code >= 0 {
		return gf, code
	}
	if gf.findings == "" {
		_, _ = fmt.Fprint(stderr, "mdsmith-secreview: grade requires --findings\n")
		return gf, 2
	}
	if gf.caseID != "" && gf.cases == "" {
		_, _ = fmt.Fprint(stderr, "mdsmith-secreview: --case requires --cases\n")
		return gf, 2
	}
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr,
			"mdsmith-secreview: grade takes no positional args, got %v\n", fs.Args())
		fs.Usage()
		return gf, 2
	}
	return gf, -1
}

// gradeConstraints builds the Constraints for the grade run from a case
// (when --case is set) or from the flag-supplied constraints. The returned
// label names what is being graded. A code >= 0 signals an input error.
func gradeConstraints(gf gradeFlags, stderr io.Writer) (secreview.Constraints, string, int) {
	if gf.caseID != "" {
		spec, err := secreview.LoadSpec(gf.cases)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "mdsmith-secreview: %v\n", err)
			return secreview.Constraints{}, "", 2
		}
		if err := spec.Validate(); err != nil {
			_, _ = fmt.Fprintf(stderr, "mdsmith-secreview: %v\n", err)
			return secreview.Constraints{}, "", 2
		}
		con, err := constraintsForID(spec, gf.caseID)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "mdsmith-secreview: %v\n", err)
			return secreview.Constraints{}, "", 2
		}
		return con, gf.caseID, -1
	}
	con, err := secreview.BuildConstraints(gf.forbid, gf.requireMin, gf.requireFile)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "mdsmith-secreview: %v\n", err)
		return secreview.Constraints{}, "", 2
	}
	return con, gf.findings, -1
}

// constraintsForID finds the named case in spec and returns its
// constraints, or an error when the case is absent or its rubric is bad.
func constraintsForID(spec *secreview.Spec, id string) (secreview.Constraints, error) {
	for _, c := range spec.Cases {
		if c.ID == id {
			return secreview.ConstraintsForCase(c)
		}
	}
	return secreview.Constraints{}, fmt.Errorf("case %q not found", id)
}

// gradeUsage is the grade subcommand's usage text.
const gradeUsage = `Usage: mdsmith-secreview grade --findings F \
    (--cases C --case ID | --forbid-severity S... \
     --require-min-severity S --require-location-file F)
`

// parseFlags parses fs and maps a flag error to an exit code: 0 for
// --help, 2 for a real parse error, and -1 to continue.
func parseFlags(fs *flag.FlagSet, args []string, stderr io.Writer, name string) int {
	err := fs.Parse(args)
	if err == nil {
		return -1
	}
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	_, _ = fmt.Fprintf(stderr, "mdsmith-secreview: %s: %v\n", name, err)
	return 2
}
