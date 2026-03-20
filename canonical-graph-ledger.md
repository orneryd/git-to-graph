# Canonical Graph + Mutation Log Guide

**Build a canonical truth store with constraints, temporal validity, and an audit‑grade mutation log.**

Last Updated: January 21, 2026

---

## Overview

This guide shows how to implement a **canonical graph** in NornicDB with:

- **Declarative constraints** (UNIQUE, EXISTS, NODE KEY, type, and temporal no‑overlap)
- **Versioned facts** with validity windows
- **Append‑only mutation log** (graph events + WAL txlog)
- **Receipts** for auditability (tx_id, wal_seq_start/end, hash)
- **Vector search** for semantic retrieval over canonical facts

Everything here works **as‑is** with current NornicDB features.

---

## Where This Pattern Is Useful

The canonical graph ledger pattern is especially useful when you need both **graph intelligence** and **auditability**:

- **Financial systems**: track rate/risk/collateral fact versions with non-overlapping validity windows and reconstruct state "as of" a regulator-requested timestamp.
- **Compliance and RegTech**: model KYC/AML assertions as immutable fact versions, keep actor/tx provenance, and prove mutation history with WAL-backed receipts.
- **Audit platforms**: correlate graph-level mutation events to WAL sequence ranges and receipt hashes for investigation and reconciliation workflows.
- **AI governance**: store model-produced assertions (`asserted_by=model:vX`) and human overrides as separate versions, then explain who changed what and when.
- **Data lineage systems**: preserve derivation chains and temporal validity so downstream reports can be replayed against historical truth states.

### Graph-RAG / RAG Pipeline Simplification

Canonical graph ledger modeling also simplifies LLM retrieval pipelines:

- Keep **facts, relationships, vector embeddings, and provenance** in one database.
- Run **hybrid retrieval** (vector + keyword) and **graph traversal** without moving data across separate vector and graph stores.
- Apply **as-of temporal reads** to answer time-bounded prompts ("what was true last quarter?").
- Return **audit context** (tx_id, WAL range, receipt hash) with retrieved facts for high-trust inference paths.

This reduces ETL glue code and lowers the risk of retrieval/lineage drift between systems.

---

## Prerequisites

- **Persistent storage** (Badger) — schema and constraints persist across restarts.
- **Embeddings enabled** (optional) if you want vector search.

If you use vector search:

- Configure your embedding provider and dimensions.
- Trigger embed worker after bulk loads:
  - `POST /nornicdb/embed/trigger`
  - `GET /nornicdb/embed/stats` to verify dimensions.

---

## Step 1 — Configure WAL retention (optional, opt‑in)

By default, auto‑compaction (snapshots + truncation) is enabled. For **ledger‑grade** retention, enable sealed segment retention and/or disable auto‑compaction.

**YAML:**

```yaml
database:
  wal_auto_compaction_enabled: true
  wal_retention_max_segments: 24
  wal_retention_max_age: "168h" # 7 days
  wal_ledger_retention_defaults: false
```

**Env:**

```bash
export NORNICDB_WAL_AUTO_COMPACTION_ENABLED=true
export NORNICDB_WAL_RETENTION_MAX_SEGMENTS=24
export NORNICDB_WAL_RETENTION_MAX_AGE=168h
export NORNICDB_WAL_LEDGER_RETENTION_DEFAULTS=false
```

**Notes:**
- Retention is **opt‑in** and does not change defaults unless you set it.
- Disabling auto‑compaction is recommended only when you have a durable WAL retention plan.

---

## Step 2 — Bootstrap the canonical schema

Run the idempotent bootstrap script once per database:

```bash
cat docs/plans/canonical-bootstrap.cypher | cypher-shell -u admin -p password
```

This creates:
- Required fields (EXISTS)
- Uniqueness constraints
- NODE KEY constraints
- Property type constraints
- **Temporal no‑overlap** constraint (NornicDB extension)
- Vector indexes
- Property indexes for lookup speed

Verify:

```cypher
CALL db.constraints();
SHOW INDEXES;
```

---

## Step 3 — Write canonical entities and facts

Canonical model:
- `(:Entity)` — canonical identity
- `(:FactKey)` — slot per `(subject_entity_id, predicate)`
- `(:FactVersion)` — immutable, versioned assertion

**Create entity + fact key:**

```cypher
CREATE (e:Entity {
  entity_id: 'product-123',
  entity_type: 'Product',
  display_name: 'Widget Pro',
  created_at: datetime()
})
MERGE (fk:FactKey {
  subject_entity_id: 'product-123',
  predicate: 'price'
})
MERGE (e)-[:HAS_FACT]->(fk);
```

**Create a fact version:**

```cypher
CREATE (fv:FactVersion {
  fact_key: 'product-123|price',
  value_json: '{"amount": 99.99, "currency": "USD"}',
  valid_from: datetime(),
  valid_to: null,
  asserted_at: datetime(),
  asserted_by: 'user:alice'
})
WITH fv
MATCH (fk:FactKey {subject_entity_id: 'product-123', predicate: 'price'})
MERGE (fk)-[:CURRENT]->(fv)
MERGE (fk)-[:HAS_VERSION]->(fv);
```

**Important:** The temporal no‑overlap constraint prevents overlapping validity windows for a given `fact_key`.

---

## Step 4 — Update facts with new versions

Close the previous version and create a new one:

```cypher
MATCH (fk:FactKey {subject_entity_id: 'product-123', predicate: 'price'})
MATCH (fv_old:FactVersion)-[:CURRENT]-(fk)
WHERE fv_old.valid_to IS NULL
SET fv_old.valid_to = datetime()
REMOVE fv_old:CURRENT

CREATE (fv_new:FactVersion {
  fact_key: 'product-123|price',
  value_json: '{"amount": 89.99, "currency": "USD"}',
  valid_from: datetime(),
  valid_to: null,
  asserted_at: datetime(),
  asserted_by: 'user:alice'
})
WITH fv_new, fk
MERGE (fk)-[:CURRENT]->(fv_new)
MERGE (fk)-[:HAS_VERSION]->(fv_new);
```

---

## Step 5 — Mutation log (events + WAL)

### Graph‑native events

```cypher
CREATE (me:MutationEvent {
  event_id: 'event-' + toString(timestamp()),
  tx_id: 'tx-' + toString(timestamp()),
  actor: 'user:alice',
  timestamp: datetime(),
  op_type: 'UPDATE_FACT_VERSION'
})
WITH me
MATCH (fv:FactVersion {fact_key: 'product-123|price'})
WHERE fv.valid_to IS NULL
MERGE (me)-[:AFFECTS]->(fv);
```

### WAL txlog queries (ledger view)

```cypher
CALL db.txlog.entries(1000, 1200) YIELD sequence, operation, tx_id, timestamp, data
RETURN sequence, operation, tx_id, timestamp, data
ORDER BY sequence;

CALL db.txlog.byTxId('tx-123', 200) YIELD sequence, operation, tx_id, timestamp, data
RETURN sequence, operation, tx_id, timestamp, data
ORDER BY sequence;
```

---

## Step 6 — Receipts (proof of mutation)

Receipts provide:
- `tx_id`
- `wal_seq_start`
- `wal_seq_end`
- `hash`

**HTTP:** `TransactionResponse.receipt` is returned for mutations.  
**MCP:** `store`, `link`, `task` responses include `receipt`.

You can use the receipt to fetch the associated WAL entries via `db.txlog.byTxId`.

---

## Step 7 — As‑of reads (temporal queries)

Use the temporal helper procedure:

```cypher
CALL db.temporal.asOf(
  'FactVersion',
  'fact_key',
  'product-123|price',
  'valid_from',
  'valid_to',
  datetime('2024-02-15T00:00:00Z')
) YIELD node
RETURN node;
```

---

## Step 8 — Vector search over canonical facts

Create a vector index (already in bootstrap):

```cypher
CALL db.index.vector.queryNodes('canonical_fact_idx', 10, 'price update for product x')
YIELD node, score
RETURN node, score;
```

---

## Operational Checklist

- ✅ Run `canonical-bootstrap.cypher`
- ✅ Ensure `FactVersion` validity windows don’t overlap
- ✅ Record `MutationEvent` nodes for provenance
- ✅ Use receipts for audit‑grade mutation proofs
- ✅ Configure WAL retention only when you need ledger‑grade durability

---

## Related Guides

- [Cypher Queries](cypher-queries.md)
- [Transactions](transactions.md)
- [Vector Search](vector-search.md)
- [Hybrid Search](hybrid-search.md)
- [Data Import/Export](data-import-export.md)
