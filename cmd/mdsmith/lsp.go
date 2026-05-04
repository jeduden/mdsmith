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

	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage: mdsmith lsp\n\n"+
			"Run the mdsmith Language Server Protocol server over stdio.\n"+
			"Designed to be spawned by an LSP client (VS Code, Neovim,\n"+
			"Helix, JetBrains LSP plugin). Reads JSON-RPC frames on\n"+
			"stdin and writes responses and notifications on stdout.\n\n"+
			"The server reuses the same lint and fix pipelines as\n"+
			"`mdsmith check` and `mdsmith fix`. See\n"+
			"docs/guides/editors/vscode.md for client setup.\n")
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "mdsmith: lsp takes no positional arguments\n")
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv := lsp.New(lsp.Options{
		Rules:  rule.All(),
		Reader: stdin,
		Writer: stdout,
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
