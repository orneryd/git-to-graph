package nornic

import (
	"os"
	"strings"
)

func WriteDefaultBootstrap(path string) error {
	stmts := []string{
		"CREATE INDEX g2g_factversion_version_id IF NOT EXISTS FOR (fv:FactVersion) ON (fv.version_id);",
		"CREATE CONSTRAINT g2g_commit_hash_unique IF NOT EXISTS FOR (c:Commit) REQUIRE c.hash IS UNIQUE;",
		"CREATE CONSTRAINT g2g_mutationevent_event_id_unique IF NOT EXISTS FOR (me:MutationEvent) REQUIRE me.event_id IS UNIQUE;",
		"CREATE INDEX g2g_factkey_subject IF NOT EXISTS FOR (fk:FactKey) ON (fk.subject_entity_id);",
		"CREATE INDEX g2g_factkey_predicate IF NOT EXISTS FOR (fk:FactKey) ON (fk.predicate);",
		"CREATE INDEX g2g_factversion_fact_key IF NOT EXISTS FOR (fv:FactVersion) ON (fv.fact_key);",
		"CREATE INDEX g2g_factversion_commit_hash IF NOT EXISTS FOR (fv:FactVersion) ON (fv.commit_hash);",
		"CREATE INDEX g2g_factversion_valid_from IF NOT EXISTS FOR (fv:FactVersion) ON (fv.valid_from);",
		"CREATE INDEX g2g_mutationevent_commit_hash IF NOT EXISTS FOR (me:MutationEvent) ON (me.commit_hash);",
		"CREATE INDEX g2g_mutationevent_timestamp IF NOT EXISTS FOR (me:MutationEvent) ON (me.timestamp);",
	}
	content := strings.Join(stmts, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}
