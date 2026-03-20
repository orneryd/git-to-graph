package indexer

import (
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
	RepoPath      string
	OutDir        string
	BatchSize     int
	From          string
	To            string
	ParserBackend string
	Stdout        io.Writer
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

	if err := os.MkdirAll(i.cfg.OutDir, 0o755); err != nil {
		return err
	}
	ledgerVersionsPath := filepath.Join(i.cfg.OutDir, "ledger_versions.jsonl")
	ledgerEventsPath := filepath.Join(i.cfg.OutDir, "mutation_events.jsonl")

	reporter.StartPhase("write_ledger", 2)
	if err := ledger.WriteJSONL(ledgerVersionsPath, led.Versions()); err != nil {
		return err
	}
	reporter.Tick(filepath.Base(ledgerVersionsPath))
	if err := ledger.WriteJSONL(ledgerEventsPath, led.Events()); err != nil {
		return err
	}
	reporter.Tick(filepath.Base(ledgerEventsPath))
	reporter.Complete("jsonl artifacts complete")

	reporter.StartPhase("export_nornic", 1)
	ex := nornic.Exporter{OutDir: i.cfg.OutDir, BatchSize: i.cfg.BatchSize}
	if err := ex.Write(led.Versions(), led.Events()); err != nil {
		return err
	}
	reporter.Tick("nornic cypher")
	reporter.Complete("nornic export complete")

	reporter.Info("Summary:")
	reporter.Info(fmt.Sprintf("  commits: %d", len(commits)))
	reporter.Info(fmt.Sprintf("  active files: %d", len(state)))
	reporter.Info(fmt.Sprintf("  fact versions: %d", len(led.Versions())))
	reporter.Info(fmt.Sprintf("  mutation events: %d", len(led.Events())))
	reporter.Info(fmt.Sprintf("  unresolved calls: %d", unresolvedCalls))
	reporter.Info(fmt.Sprintf("  output: %s", i.cfg.OutDir))
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
