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

	versions := []ledger.CodeState{
		{
			CodeKey:    "repo_fact|calls|file::a.go->symbol::b.go::fn::C",
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
	if strings.Contains(content, "MERGE (:CodeState {code_key:") {
		t.Fatalf("CodeState MERGE should key on state_id only for parser compatibility:\n%s", content)
	}
	if !strings.Contains(content, "MERGE (:CodeState:") || !strings.Contains(content, "{state_id: 'cs-") {
		t.Fatalf("missing state_id-based MERGE:\n%s", content)
	}
	if !strings.Contains(content, "MATCH (ck:CodeKey") || !strings.Contains(content, "MATCH (cs:CodeState {state_id:") {
		t.Fatalf("expected both MATCH bindings before relationship MERGE:\n%s", content)
	}
}

func TestCodeStateID_Deterministic(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 20, 22, 20, 0, time.UTC)
	v := ledger.CodeState{
		CodeKey:    "repo_fact|calls|x",
		ValidFrom:  now,
		TxID:       "tx-1",
		CommitHash: "abc",
	}

	a := v.StateID()
	b := v.StateID()
	if a != b {
		t.Fatalf("codeStateID is not deterministic: %q != %q", a, b)
	}
	if !strings.HasPrefix(a, "cs-") {
		t.Fatalf("expected cs- prefix, got %q", a)
	}
}
