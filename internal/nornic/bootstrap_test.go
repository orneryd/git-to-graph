package nornic

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteDefaultBootstrapUsesUniqueConstraintsForIdentityKeys(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "bootstrap.cypher")

	if err := WriteDefaultBootstrap(path); err != nil {
		t.Fatalf("WriteDefaultBootstrap() error = %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	schema := string(contents)

	if !strings.Contains(schema, "CREATE CONSTRAINT g2g_codestate_state_id_unique IF NOT EXISTS FOR (cs:CodeState) REQUIRE cs.state_id IS UNIQUE;") {
		t.Fatalf("bootstrap missing unique constraint for CodeState.state_id:\n%s", schema)
	}

	if !strings.Contains(schema, "CREATE CONSTRAINT g2g_codekey_entity_relation_unique IF NOT EXISTS FOR (ck:CodeKey) REQUIRE (ck.entity_id, ck.relation_type) IS UNIQUE;") {
		t.Fatalf("bootstrap missing unique constraint for CodeKey identity:\n%s", schema)
	}

	if strings.Contains(schema, "CREATE INDEX g2g_codestate_state_id IF NOT EXISTS FOR (cs:CodeState) ON (cs.state_id);") {
		t.Fatalf("bootstrap still defines legacy non-unique index for CodeState.state_id:\n%s", schema)
	}

	if strings.Contains(schema, "CREATE INDEX g2g_codekey_entity_relation IF NOT EXISTS FOR (ck:CodeKey) ON (ck.entity_id, ck.relation_type);") {
		t.Fatalf("bootstrap still defines legacy non-unique index for CodeKey identity:\n%s", schema)
	}
}
