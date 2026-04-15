# git-to-graph

`git-to-graph` is a Go CLI that turns your repository history into a temporal code knowledge graph and loads it into NornicDB.

It replays commits, extracts code facts (files, symbols, relationships), versions those facts over time, and writes both:

- ledger artifacts (`.jsonl`)
- graph load artifacts (`.cypher`)
- optional direct inserts into NornicDB

## Why use it

Use this when you want a queryable graph of your codebase with commit-aware evolution:

- Track how symbols and relationships change over time
- Ask commit-to-commit questions (`:CHANGED`, `:IMPACTS`, `valid_from`/`valid_to`)
- Build agent/context tooling on top of a persistent knowledge graph

## Quickstart (local)

### 1) Build a local binary

```bash
go mod tidy
go build -o g2g ./cmd/g2g
```

### 2) Install the CLI

Install the binary into your Go bin directory so you can run it from any folder:

```bash
go install ./cmd/g2g
```

Make sure your Go bin directory is on `PATH`.

On macOS and Linux, add this to your shell profile if needed:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

Verify the install:

```bash
g2g --help
```

### 3) Run index + apply to NornicDB

Defaults are already local-friendly:

- `--db-uri bolt://localhost:7687`
- `--db-user admin`
- `--db-password password`

So this is enough:

```bash
g2g index .
```

Equivalent explicit command:

```bash
g2g index . \
  --parser-backend auto \
  --db-uri bolt://localhost:7687 \
  --db-user admin \
  --db-password password
```

## What `index` does

For each run, `g2g`:

1. Scans commit history
2. Rebuilds code state by commit
3. Writes temporal ledger artifacts
4. Exports Cypher statements
5. Applies to NornicDB (unless disabled)

During apply, `g2g` auto-generates and runs `g2g-bootstrap.cypher` first to create indexes/constraints with `IF NOT EXISTS`.

## Commands

- `index`: build ledger, export graph artifacts, optionally apply to DB
- `asof`: reconstruct graph snapshot from ledger at a timestamp

## Common usage patterns

### Index current repository

```bash
g2g index .
```

### Index another repository path

```bash
g2g index /absolute/path/to/repo
```

### Export only (no DB apply)

Pass an empty DB URI:

```bash
g2g index . --db-uri "" --out ./.git2graph
```

### Use GraphQL transport

```bash
g2g index . \
  --db-uri http://localhost:7474/graphql \
  --db-user admin \
  --db-password password
```

### Add your own bootstrap schema

```bash
g2g index . \
  --bootstrap-cypher /absolute/path/to/bootstrap.cypher
```

Your bootstrap file runs after the auto-bootstrap and before data inserts.

## Parser backends

`--parser-backend`:

- `auto` (default): prefer SCIP when available, otherwise Tree-sitter
- `scip`: prefer SCIP, fallback to Tree-sitter
- `tree-sitter`: force Tree-sitter
- `regex`: minimal fallback

## Output files

`--out` defaults to `./.git2graph` (or a temp dir when direct DB apply is enabled).

Artifacts:

- `ledger_versions.jsonl`
- `mutation_events.jsonl`
- `nornic_versions.cypher`
- `nornic_events.cypher`

## As-of snapshot

```bash
g2g asof \
  --ledger ./.git2graph/ledger_versions.jsonl \
  --time 2025-01-01T00:00:00Z
```

## Performance tuning

Environment variables:

- `G2G_DB_BATCH_SIZE` (default `25`)
- `G2G_DB_STATEMENT_TIMEOUT_SEC` (default `120`)

Example:

```bash
G2G_DB_BATCH_SIZE=100 G2G_DB_STATEMENT_TIMEOUT_SEC=180 g2g index .
```

## Verify graph load quickly

Run these in Nornic query UI:

```cypher
MATCH (cs:CodeState) RETURN count(*) AS c;
```

```cypher
MATCH (cc:CodeChange) RETURN count(*) AS c;
```

```cypher
MATCH (c:Commit) RETURN count(*) AS c;
```

```cypher
MATCH (:CodeKey)-[:HAS_STATE]->(:CodeState) RETURN count(*) AS c;
```

## Query Nornic directly with curl

`g2g` uses Nornic GraphQL `executeCypher`, so you can run ad-hoc graph queries the same way.

Base endpoint (default local):

- `http://localhost:7474/graphql`

Example request:

```bash
curl -s -u admin:password \
  -H 'Content-Type: application/json' \
  http://localhost:7474/graphql \
  --data-binary @- <<'JSON'
{"query":"mutation ExecuteCypher($input: CypherInput!) { executeCypher(input: $input) { rowCount } }","variables":{"input":{"statement":"MATCH (c:Commit) RETURN c.hash LIMIT 10"}}}
JSON
```

## Query cookbook (verified)

These queries were validated against local Nornic using the `curl` pattern above.

Find commit and change context by transaction id:

```cypher
MATCH (c:Commit)
WHERE c.tx_id = '005e98bb-894e-4d71-99f6-2810fec1ddcc'
RETURN c.hash, c.timestamp, c.actor
LIMIT 5
```

Commit to changed versions:

```cypher
MATCH (c:Commit)-[:CHANGED]->(cs:CodeState)
RETURN c.hash, cs.state_id
LIMIT 10
```

Mutation events to affected versions:

```cypher
MATCH (cc:CodeChange)-[:IMPACTS]->(cs:CodeState)
RETURN cc.change_id, cs.state_id
LIMIT 10
```

Fact keys to versions:

```cypher
MATCH (ck:CodeKey)-[:HAS_STATE]->(cs:CodeState)
RETURN ck.relation_type, cs.state_id
LIMIT 10
```

Explore call facts:

```cypher
MATCH (cs:CodeState)
WHERE cs.code_key CONTAINS 'repo_fact|calls|'
RETURN cs.code_key, cs.commit_hash
LIMIT 10
```

Explore import facts:

```cypher
MATCH (cs:CodeState)
WHERE cs.code_key CONTAINS 'repo_fact|import|'
RETURN cs.code_key, cs.commit_hash
LIMIT 10
```

## Notes

- `CodeState(state_id)` is indexed by default (not unique-constrained) to avoid startup failures on pre-existing duplicate historical rows.
- For strict uniqueness, clean duplicates first, then add your own unique constraint.
