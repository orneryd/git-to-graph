export const listRepositoriesQuery = () => `
MATCH (ck:RepositoryKey)-[:HAS_STATE]->(cs:RepositoryState)
WHERE cs.valid_to IS NULL
RETURN ck.entity_id AS repo_id, cs.value_json AS info
`

export const listCommitsQuery = () => `
MATCH (c:Commit)
RETURN c.hash AS hash, c.timestamp AS timestamp, c.actor AS actor
ORDER BY c.timestamp ASC
LIMIT $limit
`

export const filesAtTimestampQuery = () => `
MATCH (cs:CodeFileState)
WHERE cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value
`

export const directoriesAtTimestampQuery = () => `
MATCH (cs:DirectoryState)
WHERE cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value
`

export const containmentEdgesAtTimestampQuery = () => `
MATCH (cs:ContainsEdgeState)
WHERE cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value
`

export const symbolsInFileAtTimestampQuery = () => `
MATCH (cs:CodeState)
WHERE cs.semantic_type IN ['function', 'method', 'class', 'type', 'struct', 'interface', 'variable', 'constant']
  AND cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
  AND cs.code_key CONTAINS $filePath
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value, cs.semantic_type AS kind
`

export const callEdgesAtTimestampQuery = () => `
MATCH (cs:CallEdgeState)
WHERE cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value
`

export const importEdgesAtTimestampQuery = () => `
MATCH (cs:ImportEdgeState)
WHERE cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value
`

export const changedAtCommitQuery = () => `
MATCH (c:Commit {hash: $commitHash})-[:CHANGED]->(cs:CodeState)
RETURN cs.state_id AS id, cs.code_key AS key, cs.semantic_type AS kind, cs.value_json AS value
LIMIT 500
`
