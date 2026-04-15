export const listRepositoriesQuery = () => `
MATCH (ck:RepositoryKey)-[:HAS_STATE]->(cs:RepositoryState)
WHERE cs.valid_to IS NULL
RETURN ck.entity_id AS repo_id, cs.value_json AS info
`;

export const listCommitsQuery = () => `
MATCH (c:Commit)
RETURN c.hash AS hash, c.timestamp AS timestamp, c.actor AS actor
ORDER BY c.timestamp ASC
LIMIT $limit
`;

export const filesAtTimestampQuery = () => `
MATCH (cs:CodeFileState)
WHERE cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value
`;

export const directoriesAtTimestampQuery = () => `
MATCH (cs:DirectoryState)
WHERE cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value
`;

export const containmentEdgesAtTimestampQuery = () => `
MATCH (cs:ContainsEdgeState)
WHERE cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value
`;

export const symbolsInFileAtTimestampQuery = () => `
MATCH (containsKey:CodeKey)
WHERE containsKey.relation_type = 'contains'
  AND containsKey.entity_id STARTS WITH $filePath + '->'
MATCH (containsKey)-[:HAS_STATE]->(containsState:ContainsEdgeState)
WHERE containsState.valid_from <= datetime($timestamp)
  AND (containsState.valid_to IS NULL OR containsState.valid_to > datetime($timestamp))
WITH DISTINCT substring(containsKey.entity_id, size($filePath) + 2) AS symbolId
MATCH (symbolKey:CodeKey)-[:HAS_STATE]->(cs:CodeState)
WHERE symbolKey.entity_id = symbolId
  AND symbolKey.relation_type IN ['function', 'method', 'class', 'type', 'struct', 'interface', 'variable', 'constant']
  AND cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value, cs.semantic_type AS kind
`;

export const callEdgesForSymbolsAtTimestampQuery = () => `
UNWIND $symbolIds AS symbolId
MATCH (callKey:CodeKey)
WHERE callKey.relation_type = 'calls'
  AND callKey.entity_id STARTS WITH symbolId + '->'
MATCH (callKey)-[:HAS_STATE]->(cs:CallEdgeState)
WHERE cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN DISTINCT cs.state_id AS id, cs.code_key AS key, cs.value_json AS value
`;

export const importEdgesForFileAtTimestampQuery = () => `
MATCH (importKey:CodeKey)
WHERE importKey.relation_type IN ['import', 'imports']
  AND importKey.entity_id STARTS WITH $filePath + '->'
MATCH (importKey)-[:HAS_STATE]->(cs:ImportEdgeState)
WHERE cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN DISTINCT cs.state_id AS id, cs.code_key AS key, cs.value_json AS value
`;

export const changedAtCommitQuery = () => `
MATCH (c:Commit {hash: $commitHash})-[:CHANGED]->(cs:CodeState)
RETURN cs.state_id AS id, cs.code_key AS key, cs.semantic_type AS kind, cs.value_json AS value
LIMIT 500
`;
