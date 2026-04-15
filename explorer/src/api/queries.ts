export const listRepositoriesQuery = () => `
MATCH (ck:CodeKey)-[:HAS_STATE]->(cs:CodeState)
WHERE ck.relation_type = 'repository'
WITH ck.entity_id AS repo_id,
  collect(CASE WHEN cs.valid_to IS NULL AND cs.value_json IS NOT NULL THEN cs.value_json END) AS activeInfos,
  collect(CASE WHEN cs.value_json IS NOT NULL THEN cs.value_json END) AS infos
RETURN repo_id, coalesce(head(activeInfos), head(infos), '{}') AS info
ORDER BY repo_id ASC
`;

export const listCommitsQuery = () => `
MATCH (c:Commit)
RETURN c.hash AS hash, c.timestamp AS timestamp, c.actor AS actor
ORDER BY c.timestamp ASC
LIMIT $limit
`;

export const filesForRepoQuery = () => `
MATCH (ck:CodeKey)-[:HAS_STATE]->(cs:CodeState)
WHERE ck.relation_type = 'file'
  AND cs.value_json CONTAINS $repoNeedle
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value, cs.commit_hash AS commitHash
`;

export const directoriesForRepoQuery = () => `
MATCH (ck:CodeKey)-[:HAS_STATE]->(cs:CodeState)
WHERE ck.relation_type = 'directory'
  AND cs.value_json CONTAINS $repoNeedle
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value, cs.commit_hash AS commitHash
`;

export const containmentEdgesForRepoQuery = () => `
MATCH (ck:CodeKey)-[:HAS_STATE]->(cs:CodeState)
WHERE ck.relation_type = 'contains'
  AND cs.value_json CONTAINS $repoNeedle
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value, cs.commit_hash AS commitHash
`;

// Backward-compatible aliases for stale Vite HMR clients after the project-view
// query names were changed from timestamp-based to repo-based loading.
export const filesAtTimestampQuery = filesForRepoQuery;
export const directoriesAtTimestampQuery = directoriesForRepoQuery;
export const containmentEdgesAtTimestampQuery = containmentEdgesForRepoQuery;

export const symbolsInFileAtTimestampQuery = () => `
MATCH (containsKey:CodeKey)
WHERE containsKey.relation_type = 'contains'
  AND containsKey.entity_id STARTS WITH $filePath + '->'
MATCH (containsKey)-[:HAS_STATE]->(containsState:CodeState)
WHERE containsState.valid_from <= datetime($timestamp)
  AND (containsState.valid_to IS NULL OR containsState.valid_to > datetime($timestamp))
WITH DISTINCT substring(containsKey.entity_id, size($filePath) + 2) AS symbolId
MATCH (symbolKey:CodeKey)-[:HAS_STATE]->(cs:CodeState)
WHERE symbolKey.entity_id = symbolId
  AND symbolKey.relation_type IN ['function', 'method', 'class', 'type', 'struct', 'interface', 'variable', 'constant']
  AND cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value, coalesce(cs.semantic_type, symbolKey.relation_type) AS kind
`;

export const callEdgesForSymbolsAtTimestampQuery = () => `
UNWIND $symbolIds AS symbolId
MATCH (callKey:CodeKey)
WHERE callKey.relation_type = 'calls'
  AND callKey.entity_id STARTS WITH symbolId + '->'
MATCH (callKey)-[:HAS_STATE]->(cs:CodeState)
WHERE cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN DISTINCT cs.state_id AS id, cs.code_key AS key, cs.value_json AS value
`;

export const importEdgesForFileAtTimestampQuery = () => `
MATCH (importKey:CodeKey)
WHERE importKey.relation_type IN ['import', 'imports']
  AND importKey.entity_id STARTS WITH $filePath + '->'
MATCH (importKey)-[:HAS_STATE]->(cs:CodeState)
WHERE cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN DISTINCT cs.state_id AS id, cs.code_key AS key, cs.value_json AS value
`;

export const changedAtCommitQuery = () => `
MATCH (c:Commit {hash: $commitHash})-[:CHANGED]->(cs:CodeState)
OPTIONAL MATCH (ck:CodeKey)-[:HAS_STATE]->(cs)
RETURN cs.state_id AS id, cs.code_key AS key, coalesce(cs.semantic_type, ck.relation_type) AS kind, cs.value_json AS value
LIMIT 500
`;
