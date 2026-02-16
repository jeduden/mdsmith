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
		os.Exit(2)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}

	switch args[0] {
	case "build":
		return runBuild(args[1:])
	case "qa":
		return runQA(args[1:])
	case "drift":
		return runDrift(args[1:])
	default:
		return usageError()
	}
}

func runBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to corpus build config yaml")
	outDir := fs.String("out", "", "output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" || *outDir == "" {
		return errors.New("build requires -config and -out")
	}

	cfg, err := corpus.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	result, err := corpus.Build(cfg)
	if err != nil {
		return err
	}

	manifestPath := filepath.Join(*outDir, "manifest.jsonl")
	reportPath := filepath.Join(*outDir, "report.json")
	samplePath := filepath.Join(*outDir, "qa-sample.jsonl")

	if err := corpus.WriteManifest(manifestPath, result.Manifest); err != nil {
		return err
	}
	if err := corpus.WriteJSON(reportPath, result.Report); err != nil {
		return err
	}
	if err := corpus.WriteQASample(samplePath, result.QASample); err != nil {
		return err
	}

	fmt.Printf("manifest: %s\n", manifestPath)
	fmt.Printf("report:   %s\n", reportPath)
	fmt.Printf("qa:       %s\n", samplePath)
	return nil
}

func runQA(args []string) error {
	fs := flag.NewFlagSet("qa", flag.ContinueOnError)
	samplePath := fs.String("sample", "", "path to qa-sample.jsonl")
	annotationsPath := fs.String("annotations", "", "path to manual annotations csv")
	outPath := fs.String("out", "", "path to write qa report json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *samplePath == "" || *annotationsPath == "" || *outPath == "" {
		return errors.New("qa requires -sample, -annotations, and -out")
	}

	sample, err := corpus.ReadQASample(*samplePath)
	if err != nil {
		return err
	}
	annotations, err := corpus.ReadQAAnnotationsCSV(*annotationsPath)
	if err != nil {
		return err
	}
	report, err := corpus.EvaluateQA(sample, annotations)
	if err != nil {
		return err
	}
	if err := corpus.WriteJSON(*outPath, report); err != nil {
		return err
	}
	fmt.Printf("qa report: %s\n", *outPath)
	return nil
}

func runDrift(args []string) error {
	fs := flag.NewFlagSet("drift", flag.ContinueOnError)
	baselinePath := fs.String("baseline", "", "path to baseline report.json")
	candidatePath := fs.String("candidate", "", "path to candidate report.json")
	outPath := fs.String("out", "", "path to write drift report json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *baselinePath == "" || *candidatePath == "" || *outPath == "" {
		return errors.New("drift requires -baseline, -candidate, and -out")
	}

	baseline, err := corpus.ReadBuildReport(*baselinePath)
	if err != nil {
		return err
	}
	candidate, err := corpus.ReadBuildReport(*candidatePath)
	if err != nil {
		return err
	}
	drift := corpus.CompareReports(baseline, candidate)
	if err := corpus.WriteJSON(*outPath, drift); err != nil {
		return err
	}
	fmt.Printf("drift report: %s\n", *outPath)
	return nil
}

func usageError() error {
	return errors.New("usage: corpusctl <build|qa|drift> [flags]")
}
