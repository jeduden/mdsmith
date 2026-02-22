package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jeduden/mdsmith/internal/corpus"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "corpusctl: %v\n", err)
		if isUsageError(err) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError("usage: corpusctl <build|measure|qa|drift> [flags]")
	}

	switch args[0] {
	case "build":
		return runBuild(args[1:])
	case "measure":
		return runMeasure(args[1:])
	case "qa":
		return runQA(args[1:])
	case "drift":
		return runDrift(args[1:])
	default:
		return usageError("usage: corpusctl <build|measure|qa|drift> [flags]")
	}
}

func runBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to corpus config yaml")
	outDir := fs.String("out", "", "output directory")
	cacheDir := fs.String("cache", defaultCacheDir(), "cache directory for source clones")
	if err := fs.Parse(args); err != nil {
		return usageError(err.Error())
	}
	if *configPath == "" || *outDir == "" {
		return usageError("build requires -config and -out")
	}

	statusf("build: loading config %s", *configPath)
	cfg, err := corpus.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	cfg.Progress = func(message string) {
		statusf("build: %s", message)
	}
	statusf("build: collecting and building corpus")
	result, err := corpus.Build(cfg, *cacheDir)
	if err != nil {
		return err
	}

	manifestPath := filepath.Join(*outDir, "manifest.jsonl")
	reportPath := filepath.Join(*outDir, "report.json")
	samplePath := filepath.Join(*outDir, "qa-sample.jsonl")

	statusf("build: writing %s", manifestPath)
	if err := corpus.WriteManifest(manifestPath, result.Manifest); err != nil {
		return err
	}
	statusf("build: writing %s", reportPath)
	if err := corpus.WriteJSON(reportPath, result.Report); err != nil {
		return err
	}
	statusf("build: writing %s", samplePath)
	if err := corpus.WriteQASample(samplePath, result.QASample); err != nil {
		return err
	}

	fmt.Println(manifestPath)
	fmt.Println(reportPath)
	fmt.Println(samplePath)
	return nil
}

func runQA(args []string) error {
	fs := flag.NewFlagSet("qa", flag.ContinueOnError)
	samplePath := fs.String("sample", "", "path to qa-sample.jsonl")
	annotationsPath := fs.String("annotations", "", "path to annotations csv")
	outPath := fs.String("out", "", "path to write qa report")
	if err := fs.Parse(args); err != nil {
		return usageError(err.Error())
	}
	if *samplePath == "" || *annotationsPath == "" || *outPath == "" {
		return usageError("qa requires -sample, -annotations, and -out")
	}

	statusf("qa: reading sample %s", *samplePath)
	sample, err := corpus.ReadQASample(*samplePath)
	if err != nil {
		return err
	}
	statusf("qa: reading annotations %s", *annotationsPath)
	annotations, err := corpus.ReadQAAnnotationsCSV(*annotationsPath)
	if err != nil {
		return err
	}
	statusf("qa: evaluating %d sample rows", len(sample))
	report, err := corpus.EvaluateQA(sample, annotations)
	if err != nil {
		return err
	}
	statusf("qa: writing %s", *outPath)
	if err := corpus.WriteJSON(*outPath, report); err != nil {
		return err
	}

	fmt.Println(*outPath)
	return nil
}

func runDrift(args []string) error {
	fs := flag.NewFlagSet("drift", flag.ContinueOnError)
	baselinePath := fs.String("baseline", "", "path to baseline report.json")
	candidatePath := fs.String("candidate", "", "path to candidate report.json")
	outPath := fs.String("out", "", "path to write drift report")
	if err := fs.Parse(args); err != nil {
		return usageError(err.Error())
	}
	if *baselinePath == "" || *candidatePath == "" || *outPath == "" {
		return usageError("drift requires -baseline, -candidate, and -out")
	}

	statusf("drift: reading baseline %s", *baselinePath)
	baseline, err := corpus.ReadBuildReport(*baselinePath)
	if err != nil {
		return err
	}
	statusf("drift: reading candidate %s", *candidatePath)
	candidate, err := corpus.ReadBuildReport(*candidatePath)
	if err != nil {
		return err
	}
	statusf("drift: comparing reports")
	drift := corpus.CompareReports(baseline, candidate)
	statusf("drift: writing %s", *outPath)
	if err := corpus.WriteJSON(*outPath, drift); err != nil {
		return err
	}

	fmt.Println(*outPath)
	return nil
}

type usageErr struct {
	msg string
}

func (e usageErr) Error() string {
	return e.msg
}

func usageError(msg string) error {
	return usageErr{msg: msg}
}

func isUsageError(err error) bool {
	var target usageErr
	return errors.As(err, &target)
}

func defaultCacheDir() string {
	userCacheDir, err := os.UserCacheDir()
	if err != nil || userCacheDir == "" {
		return filepath.Join(os.TempDir(), "corpusctl")
	}
	return filepath.Join(userCacheDir, "corpusctl")
}

func statusf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "corpusctl: "+format+"\n", args...)
}
