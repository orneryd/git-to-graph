package nornic

import (
	"os"
	"strings"
)

func WriteDefaultBootstrap(path string) error {
	stmts := []string{
		"CREATE INDEX g2g_codestate_state_id IF NOT EXISTS FOR (cs:CodeState) ON (cs.state_id);",
		"CREATE CONSTRAINT g2g_commit_hash_unique IF NOT EXISTS FOR (c:Commit) REQUIRE c.hash IS UNIQUE;",
		"CREATE CONSTRAINT g2g_codechange_change_id_unique IF NOT EXISTS FOR (cc:CodeChange) REQUIRE cc.change_id IS UNIQUE;",
		"CREATE INDEX g2g_codekey_entity_relation IF NOT EXISTS FOR (ck:CodeKey) ON (ck.entity_id, ck.relation_type);",
		"CREATE INDEX g2g_codekey_relation_entity IF NOT EXISTS FOR (ck:CodeKey) ON (ck.relation_type, ck.entity_id);",
		"CREATE INDEX g2g_codestate_code_key_tx_id IF NOT EXISTS FOR (cs:CodeState) ON (cs.code_key, cs.tx_id);",
		"CREATE INDEX g2g_codekey_entity IF NOT EXISTS FOR (ck:CodeKey) ON (ck.entity_id);",
		"CREATE INDEX g2g_codekey_relation_type IF NOT EXISTS FOR (ck:CodeKey) ON (ck.relation_type);",
		"CREATE INDEX g2g_codestate_code_key IF NOT EXISTS FOR (cs:CodeState) ON (cs.code_key);",
		"CREATE INDEX g2g_codestate_semantic_type IF NOT EXISTS FOR (cs:CodeState) ON (cs.semantic_type);",
		"CREATE INDEX g2g_codestate_tx_id IF NOT EXISTS FOR (cs:CodeState) ON (cs.tx_id);",
		"CREATE INDEX g2g_codestate_commit_hash IF NOT EXISTS FOR (cs:CodeState) ON (cs.commit_hash);",
		"CREATE INDEX g2g_codestate_valid_from IF NOT EXISTS FOR (cs:CodeState) ON (cs.valid_from);",
		"CREATE INDEX g2g_commit_tx_id IF NOT EXISTS FOR (c:Commit) ON (c.tx_id);",
		"CREATE INDEX g2g_codechange_tx_id IF NOT EXISTS FOR (cc:CodeChange) ON (cc.tx_id);",
		"CREATE INDEX g2g_codechange_commit_hash IF NOT EXISTS FOR (cc:CodeChange) ON (cc.commit_hash);",
		"CREATE INDEX g2g_codechange_timestamp IF NOT EXISTS FOR (cc:CodeChange) ON (cc.timestamp);",
	}
	content := strings.Join(stmts, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}
