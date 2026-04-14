package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
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
	// Reorder args so positional arguments come after flags.
	// Go's flag package stops parsing at the first non-flag argument,
	// so "g2g index . --from HEAD" would treat --from as unparsed.
	args = movePositionalToEnd(args)

	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	outDir := fs.String("out", "./.git2graph", "Output directory for ledger artifacts")
	batchSize := fs.Int("batch-size", 500, "NornicDB write batch size")
	from := fs.String("from", "", "Optional start commit (inclusive)")
	to := fs.String("to", "", "Optional end commit (inclusive)")
	parserBackend := fs.String("parser-backend", "auto", "Parser backend: auto|scip|tree-sitter|regex")
	dbURI := fs.String("db-uri", "bolt://localhost:7687", "DB URI. Examples: bolt://localhost:7687 or http://localhost:7474/graphql")
	dbUser := fs.String("db-user", "admin", "NornicDB username for basic auth")
	dbPassword := fs.String("db-password", "password", "NornicDB password for basic auth")
	dbToken := fs.String("db-token", "", "NornicDB bearer token")
	dbDatabase := fs.String("db-database", "", "Optional NornicDB database name")
	bootstrapCypher := fs.String("bootstrap-cypher", "", "Optional bootstrap Cypher file to run before inserts")
	continueOnDBError := fs.Bool("continue-on-db-error", false, "Continue applying remaining statements when one fails")
	if err := fs.Parse(args); err != nil {
		return err
	}

	repoArg := "."
	if rest := fs.Args(); len(rest) > 0 {
		repoArg = rest[0]
	}
	absRepo, err := filepath.Abs(repoArg)
	if err != nil {
		return err
	}

	transport := ""
	applyToDB := false
	bURI := ""
	gqlURL := ""
	if v := *dbURI; v != "" {
		applyToDB = true
		if isBoltURI(v) {
			transport = "bolt"
			bURI = v
		} else {
			transport = "graphql"
			gqlURL = v
		}
	}

	cfg := indexer.Config{
		RepoPath:          absRepo,
		OutDir:            *outDir,
		BatchSize:         *batchSize,
		From:              *from,
		To:                *to,
		ParserBackend:     *parserBackend,
		ApplyToDB:         applyToDB,
		DBTransport:       transport,
		BoltURI:           bURI,
		DBURL:             gqlURL,
		DBUser:            *dbUser,
		DBPassword:        *dbPassword,
		DBToken:           *dbToken,
		DBDatabase:        *dbDatabase,
		BootstrapCypher:   *bootstrapCypher,
		ContinueOnDBError: *continueOnDBError,
		Stdout:            stdout,
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
	fmt.Fprintln(w, "g2g: canonical temporal graph ledger builder")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  index    Read git history and build canonical temporal ledger artifacts")
	fmt.Fprintln(w, "  asof     Reconstruct graph state from ledger as of a timestamp")
}

// movePositionalToEnd moves any leading non-flag arguments to the end
// of the arg list. Go's flag package stops parsing at the first non-flag
// argument, so "g2g index . --from HEAD" would leave --from unparsed.
// This reorders it to "--from HEAD ." so flags are parsed correctly.
func movePositionalToEnd(args []string) []string {
	i := 0
	for i < len(args) && !strings.HasPrefix(args[i], "-") {
		i++
	}
	if i == 0 || i == len(args) {
		return args
	}
	reordered := make([]string, 0, len(args))
	reordered = append(reordered, args[i:]...)
	reordered = append(reordered, args[:i]...)
	return reordered
}

func isBoltURI(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return strings.HasPrefix(v, "bolt://") || strings.HasPrefix(v, "neo4j://")
}
