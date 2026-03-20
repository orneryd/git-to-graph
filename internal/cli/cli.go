package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/c815719/git-to-graph/internal/indexer"
	"github.com/c815719/git-to-graph/internal/ledger"
)

func Run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("missing command")
	}

	switch args[0] {
	case "index":
		return runIndex(args[1:], stdout)
	case "asof":
		return runAsOf(args[1:], stdout)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runIndex(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	repo := fs.String("repo", ".", "Path to git repository to index")
	outDir := fs.String("out", "./.git2graph", "Output directory for ledger artifacts")
	batchSize := fs.Int("batch-size", 500, "NornicDB write batch size")
	from := fs.String("from", "", "Optional start commit (inclusive)")
	to := fs.String("to", "", "Optional end commit (inclusive)")
	parserBackend := fs.String("parser-backend", "auto", "Parser backend: auto|scip|tree-sitter|regex")
	if err := fs.Parse(args); err != nil {
		return err
	}

	absRepo, err := filepath.Abs(*repo)
	if err != nil {
		return err
	}

	cfg := indexer.Config{
		RepoPath:      absRepo,
		OutDir:        *outDir,
		BatchSize:     *batchSize,
		From:          *from,
		To:            *to,
		ParserBackend: *parserBackend,
		Stdout:        stdout,
	}

	idx := indexer.New(cfg)
	return idx.Run()
}

func runAsOf(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("asof", flag.ContinueOnError)
	ledgerPath := fs.String("ledger", "./.git2graph/ledger_versions.jsonl", "Path to ledger versions JSONL")
	at := fs.String("time", "", "RFC3339 timestamp")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *at == "" {
		return errors.New("--time is required")
	}
	ts, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		return fmt.Errorf("parse --time: %w", err)
	}

	snapshot, err := ledger.LoadSnapshotAt(*ledgerPath, ts)
	if err != nil {
		return err
	}
	return ledger.WriteSnapshot(stdout, snapshot)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "git-to-graph: canonical temporal graph ledger builder")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  index    Read git history and build canonical temporal ledger artifacts")
	fmt.Fprintln(w, "  asof     Reconstruct graph state from ledger as of a timestamp")
}
