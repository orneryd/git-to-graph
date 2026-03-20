# git-to-graph

Go CLI to replay git history into a canonical temporal graph ledger and load it into NornicDB.

## What it does

- Reads commit history from a target repo.
- Reconstructs evolving code structure (`File`, `Function`, `Class`, calls/imports).
- Writes temporal fact versions (`valid_from` / `valid_to`) and mutation events.
- Exports batched Cypher artifacts.
- Optionally applies Cypher directly to NornicDB.

## Commands

- `index`: build ledger and optionally apply to DB.
- `asof`: reconstruct state from `ledger_versions.jsonl` at a timestamp.

## Build

```bash
go mod tidy
go build -o g2g ./cmd/g2g
```

## E2E run against NornicDB

### 1) Ensure NornicDB is running

- HTTP: `http://localhost:7474`
- Bolt: `bolt://localhost:7687`

### 2) Run index and apply directly (Bolt via `--db-uri`)

```bash
./g2g index . \
  --parser-backend auto \
  --db-uri bolt://localhost:7687 \
  --db-user admin \
  --db-password password
```

`index .` means “index the repo in the current working directory”.

Direct DB apply is automatic by default.

Before apply starts, `g2g` now auto-generates and runs a parser-safe bootstrap schema
(`g2g-bootstrap.cypher`) that creates required indexes/constraints with `IF NOT EXISTS`.

Note: `FactVersion(version_id)` is indexed (not unique-constrained) by default so startup
does not fail on existing historical duplicate rows. For strict uniqueness, first clean
duplicates, then add a unique constraint manually.

Default connection values:

- `--db-uri bolt://localhost:7687`
- `--db-user admin`
- `--db-password password`

So this works as-is:

```bash
./g2g index .
```

To run export-only mode, pass an empty URI:

```bash
./g2g index . --db-uri "" --out ./.git2graph
```

### 3) Optional: apply bootstrap schema before inserts

```bash
./g2g index . \
  --db-uri bolt://localhost:7687 \
  --db-user admin \
  --db-password password \
  --bootstrap-cypher /absolute/path/to/canonical-bootstrap.cypher
```

### 4) GraphQL transport fallback (inferred from URI)

```bash
./g2g index . \
  --db-uri http://localhost:7474/graphql \
  --db-user admin \
  --db-password password
```

### 5) Export-only mode (no DB)

If no `--db-uri` is provided, artifacts are written to `--out`.

```bash
./g2g index . --out ./.git2graph
```

## Parser backend selection

`--parser-backend` values:

- `auto` (default): prefer SCIP for supported language when `scip-<lang>` is installed, else Tree-sitter.
- `scip`: force SCIP preference (falls back to Tree-sitter if unavailable).
- `tree-sitter`: force Tree-sitter.
- `regex`: minimal fallback parser.

## Outputs

In `--out` (default `./.git2graph`):

- `ledger_versions.jsonl`
- `mutation_events.jsonl`
- `nornic_versions.cypher`
- `nornic_events.cypher`

## As-of reconstruction

```bash
./g2g asof \
  --ledger ./.git2graph/ledger_versions.jsonl \
  --time 2025-01-01T00:00:00Z
```
