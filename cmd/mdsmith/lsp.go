package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/lsp"
	"github.com/jeduden/mdsmith/internal/rule"
)

// runLSP implements the "lsp" subcommand: speak Language Server
// Protocol over stdio. The server lives in internal/lsp; this file is
// only the CLI entry point.
func runLSP(args []string) int {
	return runLSPWith(args, os.Stdin, os.Stdout, os.Stderr)
}

// runLSPWith is the testable variant. The ctor is split so unit tests
// can drive the server with in-memory pipes; production goes through
// runLSP and uses the process's actual stdio.
func runLSPWith(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("lsp", flag.ContinueOnError)
	fs.SetOutput(stderr)

	// --stdio is a no-op accepted for client compatibility.
	// vscode-languageclient appends `--stdio` whenever
	// `TransportKind.stdio` is selected (and other LSP servers like
	// rust-analyzer / typescript-language-server document the flag
	// the same way). The transport is always stdio for `mdsmith lsp`,
	// so we just accept and ignore the flag — without it,
	// fs.Parse would return "unknown flag: --stdio" and the server
	// would exit 2 on every VS Code launch.
	var stdioFlag bool
	fs.BoolVar(&stdioFlag, "stdio", false,
		"Use stdio transport (always on; accepted for LSP-client compatibility)")

	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage: mdsmith lsp [--stdio]\n\n"+
			"Run the mdsmith Language Server Protocol server over stdio.\n"+
			"Designed to be spawned by an LSP client (VS Code, Neovim,\n"+
			"Helix, JetBrains LSP plugin). Reads JSON-RPC frames on\n"+
			"stdin and writes responses and notifications on stdout.\n\n"+
			"The server reuses the same lint and fix pipelines as\n"+
			"`mdsmith check` and `mdsmith fix`. See\n"+
			"docs/guides/editors/vscode.md for client setup.\n")
	}

	if err := fs.Parse(args); err != nil {
		// pflag returns ErrHelp for -h/--help; match the rest of the
		// CLI by treating help as a successful exit.
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		// Surface the parse error so the LSP client (or anyone
		// reading stderr) can see WHY the server bailed instead of
		// looking at a silent exit 2. pflag's own ContinueOnError
		// path does not write to fs.Output() reliably.
		_, _ = fmt.Fprintf(stderr, "mdsmith: lsp: %v\n", err)
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "mdsmith: lsp takes no positional arguments\n")
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv := lsp.New(lsp.Options{
		Rules:          rule.All(),
		Reader:         stdin,
		Writer:         stdout,
		OnConfigReload: installIncludeExtractProjector,
		// Reap an orphaned server left behind by a leaked editor host:
		// when a reload spawns a fresh server for the same workspace,
		// the older one steps aside so exactly one stays live.
		EnableWorkspaceSingleton: true,
	})
	// SIGINT/SIGTERM cancel ctx, so srv.Run returns context.Canceled.
	// That's a clean shutdown (the user or the OS asked us to exit), not
	// a runtime error — return 0 and stay silent on stderr.
	if err := srv.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		_, _ = fmt.Fprintf(stderr, "mdsmith: lsp: %v\n", err)
		return 2
	}
	return 0
}
