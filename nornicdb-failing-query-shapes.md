# NornicDB Failing Query Shapes (Captured During `g2g index .`)

This file captures the exact query shapes that failed to parse/execute in the earlier runs.
Status as of 2026-03-20: fixed in exporter (`internal/nornic/export.go`) by switching to parser-safe statement shapes.

## 1) Dynamic expression inside `MERGE` properties

### Shape
```cypher
MERGE (fk:FactKey {
  subject_entity_id: split(row.fact_key,'|')[2],
  predicate: split(row.fact_key,'|')[1]
})
```

### Error
```text
node MERGE failed: failed to create node: serializing node: invalid node properties:
invalid property value for key "predicate": unsupported property value type cypher.invalidPropertyValue
```

### Notes
- `split(...)[i]` property expressions in `MERGE` map were rejected by this NornicDB build.
- Fix: precompute `subject_entity_id` and `predicate` in Go, emit plain string literals.

---

## 2) `UNWIND` inline map literal batch

### Shape
```cypher
UNWIND [
  {event_id: '...', tx_id: '...', actor: '...', ts: datetime('...'), op_type: '...', commit_hash: '...', affected_fact: '...'}
] AS row
CREATE (me:MutationEvent {
  event_id: row.event_id,
  tx_id: row.tx_id,
  actor: row.actor,
  timestamp: row.ts,
  op_type: row.op_type,
  commit_hash: row.commit_hash
})
WITH me, row
MATCH (fv:FactVersion {fact_key: row.affected_fact, tx_id: row.tx_id})
MERGE (me)-[:AFFECTS]->(fv)
```

### Error
```text
UNWIND CREATE failed: invalid property key: "{ts" (must be alphanumeric starting with letter or underscore)
```

### Notes
- Parser appears to choke on this inline object literal batching style.
- Fix: exporter now emits one conservative statement per record (no `UNWIND` object-literal batching).

---

## 3) Variable scope lost before relationship `MERGE`

### Shape
```cypher
MERGE (fk:FactKey { ... })
CREATE (fv:FactVersion { ... })
MERGE (fk)-[:HAS_VERSION]->(fv)
```

### Error
```text
relationship MERGE failed: end node variable 'fv' not in context (available: [fk])
```

### Notes
- `fv` not carried in scope at relationship merge point in this parser/runtime path.
- Fix: exporter now links using explicit bindings:
  - `MATCH (fk:FactKey {...})`
  - `MATCH (fv:FactVersion {version_id: '...'})`
  - `MERGE (fk)-[:HAS_VERSION]->(fv)`

---

## 4) Multi-clause chain rejected as invalid pattern

### Shape
```cypher
MERGE (fk:FactKey {
  subject_entity_id: 'file::internal/indexer/indexer.go->symbol::internal/ledger/ledger.go::method::Versions',
  predicate: 'calls'
})
CREATE (fv:FactVersion {
  fact_key: 'repo_fact|calls|file::internal/indexer/indexer.go->symbol::internal/ledger/ledger.go::method::Versions',
  value_json: '{"repo":"git-to-graph","source":"file::internal/indexer/indexer.go","target":"symbol::internal/ledger/ledger.go::method::Versions"}',
  valid_from: datetime('2026-03-20T20:22:20Z'),
  valid_to: null,
  asserted_at: datetime('2026-03-20T20:22:20Z'),
  asserted_by: 'TJ Sweet',
  tx_id: 'tx-5671c64f-000001',
  commit_hash: '5671c64fcba850a6fd01ef68f2b9d592389f41c1'
})
WITH fk, fv
```

### Error
```text
node MERGE failed: invalid pattern: (fk:FactKey {...}) CREATE (fv:FactVersion {...}) WITH fk, fv
```

### Notes
- Parser treats this form as invalid chained pattern in one execution unit.
- Fix: split into separate statement units; no `MERGE ... CREATE ... WITH ...` chain in one statement.

---

## 5) Comment line execution artifact (splitter issue)

### Shape
```cypher
// Batched canonical fact-version upserts for NornicDB
```

### Error
```text
syntax error: query must start with a valid clause
```

### Notes
- Occurred when comment lines were accidentally sent as standalone statements.
- Fix: apply pipeline strips `//` comment lines before statement split/execute.

---

## Current safe shapes (exporter contract)

```cypher
MERGE (:FactKey {subject_entity_id: '...', predicate: '...'});
MERGE (:FactVersion {version_id: 'fv-...'}) SET
  fact_key = '...',
  tx_id = '...',
  commit_hash = '...',
  valid_from_iso = '2026-03-20T20:22:20Z',
  valid_from = datetime('2026-03-20T20:22:20Z'),
  value_json = '{\"k\":\"v\"}',
  valid_to = null,
  asserted_at = datetime('2026-03-20T20:22:20Z'),
  asserted_by = '...';
MATCH (fk:FactKey {subject_entity_id: '...', predicate: '...'})
MATCH (fv:FactVersion {version_id: 'fv-...'})
MERGE (fk)-[:HAS_VERSION]->(fv);
```

```cypher
MERGE (:MutationEvent {event_id: '...'}) SET
  tx_id = '...',
  actor = '...',
  timestamp = datetime('2026-03-20T20:22:20Z'),
  op_type = '...',
  commit_hash = '...';
MATCH (me:MutationEvent {event_id: '...'})
MATCH (fv:FactVersion {fact_key: '...', tx_id: '...'})
MERGE (me)-[:AFFECTS]->(fv);
```

These shapes are validated by:
- `go test ./internal/nornic`
- `go run ./cmd/g2g index .` (apply phase succeeds end-to-end)
