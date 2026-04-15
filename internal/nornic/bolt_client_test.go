package nornic

import (
	"strings"
	"testing"
)

func TestBoltVersionQuery_UsesMonolithicHotPathShape(t *testing.T) {
	t.Parallel()

	if strings.Contains(qVersions, "WITH $rows AS rows") {
		t.Fatalf("version query should remain monolithic, not split into staged passes:\n%s", qVersions)
	}
	if !strings.Contains(qVersions, "MERGE (ck:CodeKey") || !strings.Contains(qVersions, "MERGE (cs:CodeState") || !strings.Contains(qVersions, "MERGE (c:Commit") {
		t.Fatalf("version query must upsert CodeKey, CodeState, and Commit nodes:\n%s", qVersions)
	}
	if !strings.Contains(qVersions, "MERGE (ck)-[:HAS_STATE]->(cs)") || !strings.Contains(qVersions, "MERGE (c)-[:CHANGED]->(cs)") || !strings.Contains(qVersions, "MERGE (c)-[:TOUCHED]->(ck)") {
		t.Fatalf("version query must create the expected hot-path relationships:\n%s", qVersions)
	}
}

func TestBoltEventQuery_UsesMonolithicHotPathShape(t *testing.T) {
	t.Parallel()

	if strings.Contains(qEvents, "WITH $rows AS rows") {
		t.Fatalf("event query should remain monolithic, not split into staged passes:\n%s", qEvents)
	}
	if !strings.Contains(qEvents, "MERGE (cc:CodeChange") || !strings.Contains(qEvents, "MERGE (c:Commit") {
		t.Fatalf("event query must upsert CodeChange and Commit nodes:\n%s", qEvents)
	}
	if !strings.Contains(qEvents, "MERGE (c)-[:EMITTED]->(cc)") {
		t.Fatalf("event query must create emitted relationships:\n%s", qEvents)
	}
	if !strings.Contains(qEvents, "MATCH (cs:CodeState {state_id: row.affected_state_id})") {
		t.Fatalf("event query must resolve impacts by direct state_id lookup:\n%s", qEvents)
	}
	if strings.Contains(qEvents, "OPTIONAL MATCH") || strings.Contains(qEvents, "coalesce(") {
		t.Fatalf("event query should not use optional/coalesced state lookups:\n%s", qEvents)
	}
}

func TestFallbackReusesSameMonolithicQueries(t *testing.T) {
	t.Parallel()

	if !strings.Contains(qVersions, "UNWIND $rows AS row") || !strings.Contains(qEvents, "UNWIND $rows AS row") {
		t.Fatalf("fallback should reuse the same monolithic batch query shapes")
	}
}
