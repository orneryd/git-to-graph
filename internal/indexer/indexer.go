package indexer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/c815719/git-to-graph/internal/gitreader"
	"github.com/c815719/git-to-graph/internal/graph"
	"github.com/c815719/git-to-graph/internal/ledger"
	"github.com/c815719/git-to-graph/internal/model"
	"github.com/c815719/git-to-graph/internal/nornic"
	"github.com/c815719/git-to-graph/internal/parser"
	"github.com/c815719/git-to-graph/internal/progress"
)

type Config struct {
	RepoPath          string
	OutDir            string
	BatchSize         int
	From              string
	To                string
	ParserBackend     string
	ApplyToDB         bool
	DBTransport       string
	BoltURI           string
	DBURL             string
	DBUser            string
	DBPassword        string
	DBToken           string
	DBDatabase        string
	BootstrapCypher   string
	ContinueOnDBError bool
	Stdout            io.Writer
}

type Indexer struct {
	cfg Config
}

func New(cfg Config) *Indexer {
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 500
	}
	return &Indexer{cfg: cfg}
}

func (i *Indexer) Run() error {
	reporter := progress.New(i.cfg.Stdout)
	reporter.Info("Starting canonical temporal graph ledger index")
	parser.SetBackendMode(i.cfg.ParserBackend)

	gr := gitreader.New(i.cfg.RepoPath)
	if err := gr.ValidateRepo(); err != nil {
		return err
	}
	if dirty, err := gr.IsDirty(); err == nil && dirty {
		reporter.Info("Warning: repository has uncommitted changes; index includes committed history only")
	}

	commits, err := gr.CommitList(i.cfg.From, i.cfg.To)
	if err != nil {
		return err
	}
	reporter.StartPhase("pre_scan", len(commits))
	for _, c := range commits {
		reporter.Tick(short(c))
	}
	reporter.Complete(fmt.Sprintf("discovered %d commits", len(commits)))

	state := map[string]model.FileGraph{}
	led := ledger.New()

	reporter.StartPhase("index_commits", len(commits))
	unresolvedCalls := 0
	for _, hash := range commits {
		meta, err := gr.CommitMeta(hash)
		if err != nil {
			return err
		}
		changes, err := gr.ChangedFiles(hash)
		if err != nil {
			return err
		}

		for _, ch := range changes {
			switch ch.Status {
			case "D":
				delete(state, ch.Path)
			case "R":
				delete(state, ch.OldPath)
				fallthrough
			default:
				if !isCodeFile(ch.Path) {
					continue
				}
				ignored, err := gr.IsIgnored(ch.Path)
				if err == nil && ignored {
					continue
				}
				content, err := gr.FileAtCommit(hash, ch.Path)
				if err != nil {
					continue
				}
				state[ch.Path] = parser.Parse(ch.Path, content)
			}
		}

		nameIdx := graph.BuildNameIndex(state)
		facts := graph.BuildFacts(i.repoID(), state, nameIdx)
		for _, fg := range state {
			for _, call := range fg.Calls {
				if _, ok := nameIdx[call.Callee]; !ok {
					unresolvedCalls++
				}
			}
		}
		_ = led.ApplySnapshot(facts, meta)
		reporter.Tick(fmt.Sprintf("%s files=%d facts=%d", short(hash), len(state), len(facts)))
	}
	reporter.Complete("commit replay complete")

	artifactDir := i.cfg.OutDir
	cleanupArtifacts := false
	if i.cfg.ApplyToDB {
		tmpDir, err := os.MkdirTemp("", "g2g-artifacts-")
		if err != nil {
			return err
		}
		artifactDir = tmpDir
		cleanupArtifacts = true
	} else {
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			return err
		}
	}
	if cleanupArtifacts {
		defer os.RemoveAll(artifactDir)
	}

	ledgerVersionsPath := filepath.Join(artifactDir, "ledger_versions.jsonl")
	ledgerEventsPath := filepath.Join(artifactDir, "mutation_events.jsonl")

	reporter.StartPhase("write_ledger", 2)
	if err := ledger.WriteJSONL(ledgerVersionsPath, led.CodeStates()); err != nil {
		return err
	}
	reporter.Tick(filepath.Base(ledgerVersionsPath))
	if err := ledger.WriteJSONL(ledgerEventsPath, led.CodeChanges()); err != nil {
		return err
	}
	reporter.Tick(filepath.Base(ledgerEventsPath))
	reporter.Complete("jsonl artifacts complete")

	reporter.StartPhase("export_nornic", 1)
	ex := nornic.Exporter{OutDir: artifactDir, BatchSize: i.cfg.BatchSize}
	if err := ex.Write(led.CodeStates(), led.CodeChanges()); err != nil {
		return err
	}
	reporter.Tick("nornic cypher")
	reporter.Complete("nornic export complete")

	if i.cfg.ApplyToDB {
		bootstrapFiles := make([]string, 0, 2)
		autoBootstrapPath := filepath.Join(artifactDir, "g2g-bootstrap.cypher")
		if err := nornic.WriteDefaultBootstrap(autoBootstrapPath); err != nil {
			return err
		}
		bootstrapFiles = append(bootstrapFiles, autoBootstrapPath)
		if strings.TrimSpace(i.cfg.BootstrapCypher) != "" {
			bootstrapFiles = append(bootstrapFiles, i.cfg.BootstrapCypher)
		}
		cypherFiles := make([]string, 0, 4)
		cypherFiles = append(cypherFiles, bootstrapFiles...)
		cypherFiles = append(cypherFiles,
			filepath.Join(artifactDir, "nornic_versions.cypher"),
			filepath.Join(artifactDir, "nornic_events.cypher"),
		)
		stmtCount, err := nornic.CountStatements(cypherFiles)
		if err != nil || stmtCount <= 0 {
			stmtCount = 1
		}
		transport := strings.ToLower(strings.TrimSpace(i.cfg.DBTransport))
		if transport == "" {
			transport = "bolt"
		}
		var count int
		if transport == "graphql" {
			reporter.StartPhase("apply_nornic", stmtCount)
			client := nornic.NewGraphQLClient(nornic.GraphQLConfig{
				URL:             i.cfg.DBURL,
				User:            i.cfg.DBUser,
				Password:        i.cfg.DBPassword,
				Token:           i.cfg.DBToken,
				Database:        i.cfg.DBDatabase,
				ContinueOnError: i.cfg.ContinueOnDBError,
			})
			count, err = nornic.ApplyCypherFiles(context.Background(), client, cypherFiles, func(done, total int, detail string) {
				if total > 0 {
					if strings.TrimSpace(detail) == "" {
						detail = fmt.Sprintf("%d/%d", done, total)
					}
					reporter.Progress(done, detail)
				}
			})
		} else {
			bootstrapStmtCount, berr := nornic.CountStatements(bootstrapFiles)
			if berr != nil || bootstrapStmtCount <= 0 {
				bootstrapStmtCount = 1
			}
			reporter.StartPhase("bootstrap_nornic", bootstrapStmtCount)
			bootstrapCount, berr := nornic.ApplyCypherFilesBolt(context.Background(), nornic.BoltConfig{
				URI:             i.cfg.BoltURI,
				User:            i.cfg.DBUser,
				Password:        i.cfg.DBPassword,
				Token:           i.cfg.DBToken,
				Database:        i.cfg.DBDatabase,
				ContinueOnError: i.cfg.ContinueOnDBError,
			}, bootstrapFiles, func(done, total int, _ string) {
				if total > 0 {
					reporter.Progress(done, fmt.Sprintf("%d/%d", done, total))
				}
			})
			if berr != nil {
				return berr
			}
			reporter.Complete(fmt.Sprintf("nornic bootstrap complete (statements=%d)", bootstrapCount))

			boltTotal := len(led.CodeStates()) + len(led.CodeChanges())
			if boltTotal <= 0 {
				boltTotal = 1
			}
			reporter.StartPhase("apply_nornic", boltTotal)
			count, err = nornic.ApplyLedgerBolt(context.Background(), nornic.BoltConfig{
				URI:             i.cfg.BoltURI,
				User:            i.cfg.DBUser,
				Password:        i.cfg.DBPassword,
				Token:           i.cfg.DBToken,
				Database:        i.cfg.DBDatabase,
				ContinueOnError: i.cfg.ContinueOnDBError,
			}, led.CodeStates(), led.CodeChanges(), func(done, total int, detail string) {
				if total > 0 {
					if strings.TrimSpace(detail) == "" {
						detail = fmt.Sprintf("%d/%d", done, total)
					}
					reporter.Progress(done, detail)
				}
			})
		}
		if err != nil {
			return err
		}
		reporter.Complete(fmt.Sprintf("nornic apply complete (statements=%d)", count))
	}

	reporter.Info("Summary:")
	reporter.Info(fmt.Sprintf("  commits: %d", len(commits)))
	reporter.Info(fmt.Sprintf("  active files: %d", len(state)))
	reporter.Info(fmt.Sprintf("  code states: %d", len(led.CodeStates())))
	reporter.Info(fmt.Sprintf("  code changes: %d", len(led.CodeChanges())))
	reporter.Info(fmt.Sprintf("  unresolved calls: %d", unresolvedCalls))
	if !i.cfg.ApplyToDB {
		reporter.Info(fmt.Sprintf("  output: %s", i.cfg.OutDir))
	}
	if i.cfg.ApplyToDB {
		target := i.cfg.DBURL
		if strings.ToLower(strings.TrimSpace(i.cfg.DBTransport)) != "graphql" {
			target = i.cfg.BoltURI
		}
		reporter.Info(fmt.Sprintf("  applied to db: %s", target))
	}
	return nil
}

func (i *Indexer) repoID() string {
	base := filepath.Base(i.cfg.RepoPath)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "repo"
	}
	return base
}

func short(v string) string {
	if len(v) > 8 {
		return v[:8]
	}
	return v
}

func isCodeFile(path string) bool {
	p := strings.ToLower(path)
	for _, ext := range []string{".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".java"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}
