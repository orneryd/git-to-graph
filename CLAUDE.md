# Graph Explorer — Code Knowledge Graph Visualizer

Build a standalone Vite/React/TypeScript app that connects to a local NornicDB instance and provides an interactive D3.js force-directed graph visualization of code repositories indexed by `git-to-graph`.

## What this app does

After an engineer runs `g2g index .` (the git-to-graph CLI), a temporal code knowledge graph is loaded into NornicDB with commits, files, symbols, and relationships — all versioned with `valid_from`/`valid_to` windows. This UI lets the engineer visually explore that graph and scrub through commit history with a slider to see the codebase at any point in time.

## NornicDB Connection

Default local connection (no env config needed for dev):

- **HTTP API**: `http://localhost:7474`
- **Auth**: Basic auth, `admin` / `password`
- **Default database**: `nornic`

Two API surfaces are used:

1. **Cypher endpoint**: `POST /db/{database}/tx/commit` — for all data queries
2. **Graph REST API**: `POST /nornicdb/graph/{database}/neighborhood|temporal|diff` — for graph traversal

## Tech Stack

Match the NornicDB UI conventions exactly:

- **Vite** with `@vitejs/plugin-react`
- **React 19** (`react@^19.2.5`, `react-dom@^19.2.5`)
- **TypeScript**
- **D3.js** (`d3-force`, `d3-zoom`, `d3-selection`, `d3-scale`, `d3-color`) for graph rendering into SVG
- **Tailwind CSS 4** (`tailwindcss@^4.2.2`, `@tailwindcss/postcss@^4.2.2`)
- **zustand** (`zustand@^5.0.12`) for state management
- **lucide-react** (`lucide-react@^1.8.0`) for icons

## API Client

Adapt the NornicDB client pattern. The client needs two methods:

### executeCypher(statement, parameters?, database?)

```typescript
// POST /db/{database}/tx/commit
// Body: { statements: [{ statement, parameters }] }
// Response: { results: [{ columns: string[], data: [{ row: unknown[], meta: unknown[] }] }], errors?: [...] }

interface CypherResponse {
  results: Array<{
    columns: string[];
    data: Array<{ row: unknown[]; meta: unknown[] }>;
  }>;
  errors?: Array<{ code: string; message: string }>;
}
```

### Graph REST API calls

```typescript
// All POST, all return the same shape:
interface GraphPayload {
  nodes: Array<{
    id: string;
    labels: string[];
    properties: Record<string, unknown>;
    status?: string; // "added" | "removed" | "changed" | "" (only on diff)
  }>;
  edges: Array<{
    id: string;
    source: string;
    target: string;
    type: string;
    properties?: Record<string, unknown>;
    semantic?: boolean;
    status?: string; // same as above
  }>;
  meta: {
    database: string;
    generated_from: string;
    depth?: number;
    as_of?: string;
    compare_to?: string;
    node_count: number;
    edge_count: number;
    truncated: boolean;
  };
}

// POST /nornicdb/graph/{database}/neighborhood
// Body: { node_ids: string[], depth?: number, limit?: number, labels?: string[], relationship_types?: string[] }

// POST /nornicdb/graph/{database}/temporal  
// Body: { node_ids: string[], as_of: string (ISO 8601 datetime) }

// POST /nornicdb/graph/{database}/diff
// Body: { node_ids: string[], as_of: string, compare_to?: string }
```

Use Basic auth header on all requests: `Authorization: Basic ${btoa('admin:password')}`.

## Cypher Queries

All queries are designed to use indexed fields. Place these in a dedicated `src/api/queries.ts` file as parameterized template functions.

### Q1: List indexed repositories
```cypher
MATCH (ck:RepositoryKey)-[:HAS_STATE]->(cs:RepositoryState)
WHERE cs.valid_to IS NULL
RETURN ck.entity_id AS repo_id, cs.value_json AS info
```

### Q2: List commits ordered by time
```cypher
MATCH (c:Commit)
RETURN c.hash AS hash, c.timestamp AS timestamp, c.actor AS actor
ORDER BY c.timestamp ASC
LIMIT $limit
```
Parameters: `{ limit: 500 }`

### Q3: Files alive at a commit timestamp
```cypher
MATCH (cs:CodeFileState)
WHERE cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value
```
Parameters: `{ timestamp: "2025-01-15T00:00:00Z" }`

### Q4: Directories alive at a commit timestamp
```cypher
MATCH (cs:DirectoryState)
WHERE cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value
```

### Q5: Containment edges at a commit timestamp
```cypher
MATCH (cs:ContainsEdgeState)
WHERE cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value
```

### Q6: Symbols in a file at a commit timestamp (drill-down)
```cypher
MATCH (cs:CodeState)
WHERE cs.semantic_type IN ['function', 'method', 'class', 'type', 'struct', 'interface', 'variable', 'constant']
  AND cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
  AND cs.code_key CONTAINS $filePath
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value, cs.semantic_type AS kind
```
Parameters: `{ timestamp: "...", filePath: "internal/model/model.go" }`

### Q7: Call edges at a commit timestamp (drill-down)
```cypher
MATCH (cs:CallEdgeState)
WHERE cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value
```

### Q8: Import edges at a commit timestamp
```cypher
MATCH (cs:ImportEdgeState)
WHERE cs.valid_from <= datetime($timestamp)
  AND (cs.valid_to IS NULL OR cs.valid_to > datetime($timestamp))
RETURN cs.state_id AS id, cs.code_key AS key, cs.value_json AS value
```

### Q9: What changed at a specific commit
```cypher
MATCH (c:Commit {hash: $commitHash})-[:CHANGED]->(cs:CodeState)
RETURN cs.state_id AS id, cs.code_key AS key, cs.semantic_type AS kind, cs.value_json AS value
LIMIT 500
```
Parameters: `{ commitHash: "abc123..." }`

## Data Model Notes

The `value_json` property on CodeState nodes is a JSON string that needs to be parsed. Its shape depends on the semantic type:

- **file**: `{ "repo": "...", "path": "src/main.go", "lang": "go", "name": "main.go" }`
- **directory**: `{ "repo": "...", "path": "internal/model", "name": "model" }`
- **function/method/class/etc**: `{ "repo": "...", "id": "...", "name": "BuildFacts", "kind": "function", "file": "internal/graph/builder.go", "lang": "go", "line_number": 13 }`
- **calls edge**: `{ "repo": "...", "source": "caller_id", "target": "callee_id", "line_number": 42 }`
- **contains edge**: `{ "repo": "...", "source": "parent", "source_type": "directory|file|repository", "target": "child", "target_type": "directory|file|symbol" }`
- **imports edge**: `{ "repo": "...", "source": "file_path", "target": "module_name" }`

The `code_key` follows the pattern: `repo_fact|{predicate}|{id}` where predicate is `file`, `directory`, `function`, `calls`, `contains`, `imports`, etc.

## UI Layout

```
┌─────────────────────────────────────────────────────────┐
│  Header: "Graph Explorer" + repo selector dropdown      │
├──────┬──────────────────────────────────────┬───────────┤
│      │                                      │           │
│ Repo │        D3 Force Graph Canvas         │  Detail   │
│ List │        (SVG, pan/zoom)               │  Panel    │
│      │                                      │           │
│      │                                      │ (click a  │
│      │                                      │  node to  │
│      │                                      │  see its  │
│      │                                      │  props)   │
│      │                                      │           │
├──────┴──────────────────────────────────────┴───────────┤
│  Commit Slider: ◄──●──────────────────────────────────► │
│  "abc123 - Fix parser bug - alice - 2025-01-15"         │
│  [12 changes at this commit]                            │
└─────────────────────────────────────────────────────────┘
```

## UI Components

### 1. RepoPicker (left sidebar, ~200px)
- On mount, run Q1 to list repos with active RepositoryState
- Each repo is a clickable card showing repo name
- Selecting a repo loads commits (Q2) and initial graph

### 2. CommitSlider (bottom bar, full width, ~80px)
- Horizontal range slider with commits as discrete stops
- Display: short hash, message preview, author, timestamp
- On change: re-run Q3+Q4+Q5 for the selected commit's timestamp to reload graph
- Badge showing change count from Q9

### 3. GraphCanvas (main area, D3 force-directed SVG)
This is the core component. Two view modes:

**Project View (default)**:
- Nodes = directories + files from Q3+Q4
- Edges = containment from Q5 (parse `value_json` to get source→target)
- Directory nodes: large, dark, folder icon shape (rounded rect)
- File nodes: smaller, colored by language:
  - Go: `#00ADD8`
  - TypeScript/JavaScript: `#3178C6`
  - Python: `#3776AB`
  - Rust: `#DEA584`
  - Default: `#6B7280`
- Containment edges: gray, no arrow
- D3 force: `forceLink` (short distance for same-directory), `forceManyBody` (repulsion), `forceCenter`

**Symbol View (drill-down on file click)**:
- Run Q6 for symbols in clicked file, Q7 for calls, Q8 for imports
- Nodes = symbols (functions, methods, classes, etc.)
- Edges = call edges (solid arrow) + import edges (dashed arrow)
- Node shape by kind: function=circle, class=rounded-rect, method=diamond, type=hexagon
- Show a "← Back to project" button
- Parse call edge `value_json` to resolve source→target symbol IDs

**Interaction**:
- D3 zoom/pan on the SVG
- Drag nodes to reposition
- Click node → populate DetailPanel
- Double-click file node → drill into Symbol View
- Hover node → tooltip with name + kind

### 4. DetailPanel (right sidebar, ~280px, collapsible)
- Shows when a node is selected
- Display: name, kind/type, file path, language, line number
- Raw properties from `value_json`
- For files: list of symbols at current timestamp
- Collapse when nothing selected

### 5. DiffBadge (on CommitSlider)
- Small badge next to the slider showing how many entities changed at this commit
- Run Q9 when slider stops
- Tooltip listing changed entity names

## File Structure

```
explorer/
├── package.json
├── tsconfig.json
├── vite.config.ts
├── postcss.config.js
├── index.html
├── CLAUDE.md
└── src/
    ├── main.tsx
    ├── App.tsx
    ├── index.css            # Tailwind imports
    ├── api/
    │   ├── client.ts        # NornicDB API client (Cypher + Graph REST)
    │   └── queries.ts       # Q1-Q9 as parameterized functions
    ├── store/
    │   └── explorerStore.ts # zustand: selectedRepo, commits, currentCommitIndex, graphData, selectedNode, viewMode
    ├── components/
    │   ├── RepoPicker.tsx
    │   ├── CommitSlider.tsx
    │   ├── GraphCanvas.tsx  # D3 force graph (SVG ref, simulation lifecycle)
    │   ├── DetailPanel.tsx
    │   └── DiffBadge.tsx
    └── utils/
        ├── graphLayout.ts   # D3 force config, tick handler
        ├── colors.ts        # Language color map, kind color map
        └── parseValue.ts    # Parse value_json, extract source/target from edge keys
```

## Zustand Store Shape

```typescript
interface ExplorerState {
  // Connection
  database: string; // default "nornic"
  
  // Repos
  repos: Array<{ id: string; name: string }>;
  selectedRepo: string | null;
  
  // Commits  
  commits: Array<{ hash: string; timestamp: string; actor: string }>;
  currentCommitIndex: number;
  
  // Graph data (processed, ready for D3)
  nodes: Array<{ id: string; label: string; kind: string; lang?: string; properties: Record<string, unknown> }>;
  edges: Array<{ id: string; source: string; target: string; type: string }>;
  
  // UI state
  viewMode: 'project' | 'symbol';
  selectedNodeId: string | null;
  selectedFilePath: string | null; // for symbol view drill-down
  changedAtCommit: string[]; // entity IDs changed at current commit
  
  // Actions
  loadRepos: () => Promise<void>;
  selectRepo: (repoId: string) => Promise<void>;
  setCommitIndex: (index: number) => Promise<void>;
  drillIntoFile: (filePath: string) => Promise<void>;
  backToProject: () => void;
  selectNode: (nodeId: string | null) => void;
}
```

## Important Implementation Notes

1. **D3 in React**: Use a `useRef<SVGSVGElement>` for the SVG element. Create/update the D3 force simulation in a `useEffect` that depends on `nodes` and `edges`. Clean up simulation on unmount. Do NOT let React manage SVG children — D3 manages the DOM inside the SVG.

2. **Parsing value_json**: Every `value_json` from NornicDB is a JSON string, not an object. Always `JSON.parse()` it.

3. **Resolving containment edges to graph edges**: The `code_key` for containment facts follows `repo_fact|contains|{source}->{target}`. The `value_json` has `source`, `target`, `source_type`, `target_type`. Match these to node IDs built from file/directory paths.

4. **Node ID mapping**: Nodes in the D3 graph should use the entity path (from `value_json.path` for files/dirs, `value_json.id` for symbols) as the stable ID, since that's what containment edges reference.

5. **Commit timestamp format**: Commits store `timestamp` as a NornicDB `datetime()`. When passing to queries, format as ISO 8601: `"2025-01-15T12:00:00Z"`.

6. **Auth**: All fetch calls need `credentials: "include"` and `headers: { "Authorization": "Basic " + btoa("admin:password") }`.

7. **Graph REST API base**: Same origin as Cypher endpoint: `http://localhost:7474`.

8. **Error handling**: If a query returns `errors`, show a toast/alert. Don't crash the UI.

9. **Performance**: Keep D3 node count under 500 for smooth rendering. The queries already have `LIMIT` clauses. If the graph is too large, show only direct children of the selected directory (collapse subtrees).
