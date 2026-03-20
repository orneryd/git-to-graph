package nornic

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c815719/git-to-graph/internal/ledger"
)

func TestWriteVersionBatches_UsesParserSafeShapes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "nornic_versions.cypher")
	now := time.Date(2026, 3, 20, 20, 22, 20, 0, time.UTC)

	versions := []ledger.FactVersion{
		{
			FactKey:    "repo_fact|calls|file::a.go->symbol::b.go::fn::C",
			ValueJSON:  `{"repo":"x","text":"Bob's {call} shape"}`,
			ValidFrom:  now,
			ValidTo:    nil,
			AssertedAt: now,
			AssertedBy: "TJ Sweet",
			TxID:       "tx-1",
			CommitHash: "abc123",
		},
	}

	if err := writeVersionBatches(path, versions, 500); err != nil {
		t.Fatalf("writeVersionBatches failed: %v", err)
	}

	buf, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	content := string(buf)

	if strings.Contains(content, "UNWIND ") {
		t.Fatalf("unexpected UNWIND shape in exporter output:\n%s", content)
	}
	if strings.Contains(content, "split(") {
		t.Fatalf("unexpected split(...) expression in exporter output:\n%s", content)
	}
	if strings.Contains(content, "MERGE (:FactVersion {fact_key:") {
		t.Fatalf("FactVersion MERGE should key on version_id only for parser compatibility:\n%s", content)
	}
	if !strings.Contains(content, "MERGE (:FactVersion:") || !strings.Contains(content, "{version_id: 'fv-") {
		t.Fatalf("missing version_id-based MERGE:\n%s", content)
	}
	if !strings.Contains(content, "MATCH (fk:FactKey") || !strings.Contains(content, "MATCH (fv:FactVersion {version_id:") {
		t.Fatalf("expected both MATCH bindings before relationship MERGE:\n%s", content)
	}
}

func TestFactVersionID_Deterministic(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 20, 22, 20, 0, time.UTC)
	v := ledger.FactVersion{
		FactKey:    "repo_fact|calls|x",
		ValidFrom:  now,
		TxID:       "tx-1",
		CommitHash: "abc",
	}

	a := factVersionID(v)
	b := factVersionID(v)
	if a != b {
		t.Fatalf("factVersionID is not deterministic: %q != %q", a, b)
	}
	if !strings.HasPrefix(a, "fv-") {
		t.Fatalf("expected fv- prefix, got %q", a)
	}
}
