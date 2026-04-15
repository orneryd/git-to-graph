import { create } from "zustand";
import { executeCypher } from "../api/client";
import {
  callEdgesForSymbolsAtTimestampQuery,
  changedAtCommitQuery,
  containmentEdgesForRepoQuery,
  directoriesForRepoQuery,
  filesForRepoQuery,
  importEdgesForFileAtTimestampQuery,
  listCommitsQuery,
  listRepositoriesQuery,
  symbolsInFileAtTimestampQuery,
} from "../api/queries";
import { asString, extractPathOrId, parseValue } from "../utils/parseValue";

export interface RepoItem {
  id: string;
  name: string;
}

export interface CommitItem {
  hash: string;
  timestamp: string;
  actor: string;
}

export interface ExplorerNode {
  id: string;
  label: string;
  kind: string;
  lang?: string;
  properties: Record<string, unknown>;
}

export interface ExplorerEdge {
  id: string;
  source: string;
  target: string;
  type: string;
}

interface ProjectStateRow {
  id: string;
  key: string;
  value: unknown;
  commitHash: string;
}

interface ExplorerState {
  database: string;
  repos: RepoItem[];
  selectedRepo: string | null;
  commits: CommitItem[];
  currentCommitIndex: number;
  nodes: ExplorerNode[];
  edges: ExplorerEdge[];
  viewMode: "project" | "symbol";
  selectedNodeId: string | null;
  selectedFilePath: string | null;
  changedAtCommit: string[];
  isLoading: boolean;
  error: string | null;
  loadRepos: () => Promise<void>;
  selectRepo: (repoId: string) => Promise<void>;
  setCommitIndex: (index: number) => Promise<void>;
  drillIntoFile: (filePath: string) => Promise<void>;
  backToProject: () => Promise<void>;
  selectNode: (nodeId: string | null) => void;
}

let lastAlertMessage = "";
let lastAlertAt = 0;

function toIsoTimestamp(value: unknown): string {
  if (typeof value === "string") {
    const date = new Date(value);
    if (!Number.isNaN(date.getTime())) {
      return date.toISOString();
    }
    return value;
  }
  return new Date().toISOString();
}

function safeAlert(message: string): void {
  if (typeof window !== "undefined") {
    const now = Date.now();
    if (message === lastAlertMessage && now - lastAlertAt < 1500) {
      return;
    }
    lastAlertMessage = message;
    lastAlertAt = now;
    window.alert(message);
  }
}

function reduceGraphIfLarge(
  nodes: ExplorerNode[],
  edges: ExplorerEdge[],
): { nodes: ExplorerNode[]; edges: ExplorerEdge[] } {
  if (nodes.length <= 500) {
    return { nodes, edges };
  }

  const directories = nodes.filter((n) => n.kind === "directory");
  const selected = directories.sort(
    (a, b) => a.id.split("/").length - b.id.split("/").length,
  )[0]?.id;
  if (!selected) {
    return { nodes: nodes.slice(0, 500), edges: edges.slice(0, 700) };
  }

  const keptEdges = edges.filter((e) => e.source === selected);
  const keepIds = new Set<string>([selected]);
  for (const edge of keptEdges) {
    keepIds.add(edge.target);
  }

  return {
    nodes: nodes.filter((n) => keepIds.has(n.id)),
    edges: keptEdges,
  };
}

async function loadProjectGraph(
  selectedRepo: string,
  database: string,
  currentCommitIndex: number,
  commits: CommitItem[],
): Promise<{ nodes: ExplorerNode[]; edges: ExplorerEdge[] }> {
  const repoNeedle = `"repo":"${selectedRepo}"`;
  const [filesRes, dirsRes, containsRes] = await Promise.all([
    executeCypher(filesForRepoQuery(), { repoNeedle }, database),
    executeCypher(directoriesForRepoQuery(), { repoNeedle }, database),
    executeCypher(containmentEdgesForRepoQuery(), { repoNeedle }, database),
  ]);

  const commitIndexByHash = new Map<string, number>(
    commits.map((commit, index) => [commit.hash, index]),
  );

  const latestRowsForKey = (rows: ProjectStateRow[]): ProjectStateRow[] => {
    const latest = new Map<string, { row: ProjectStateRow; index: number }>();

    for (const row of rows) {
      if (!row.key || !row.commitHash) {
        continue;
      }

      const commitIndex = commitIndexByHash.get(row.commitHash);
      if (commitIndex === undefined || commitIndex > currentCommitIndex) {
        continue;
      }

      const existing = latest.get(row.key);
      if (!existing || commitIndex >= existing.index) {
        latest.set(row.key, { row, index: commitIndex });
      }
    }

    return Array.from(latest.values()).map((item) => item.row);
  };

  const fileRows = latestRowsForKey(
    (filesRes.results[0]?.data ?? []).map((item) => ({
      id: asString(item.row[0]) ?? "",
      key: asString(item.row[1]) ?? "",
      value: item.row[2],
      commitHash: asString(item.row[3]) ?? "",
    })),
  );
  const dirRows = latestRowsForKey(
    (dirsRes.results[0]?.data ?? []).map((item) => ({
      id: asString(item.row[0]) ?? "",
      key: asString(item.row[1]) ?? "",
      value: item.row[2],
      commitHash: asString(item.row[3]) ?? "",
    })),
  );
  const containsRows = latestRowsForKey(
    (containsRes.results[0]?.data ?? []).map((item) => ({
      id: asString(item.row[0]) ?? "",
      key: asString(item.row[1]) ?? "",
      value: item.row[2],
      commitHash: asString(item.row[3]) ?? "",
    })),
  );

  const nodesMap = new Map<string, ExplorerNode>();

  const addNode = (row: ProjectStateRow, kind: "file" | "directory"): void => {
    const parsed = parseValue(row.value);
    if (
      selectedRepo &&
      asString(parsed.repo) &&
      asString(parsed.repo) !== selectedRepo
    ) {
      return;
    }
    const id = extractPathOrId(parsed) ?? row.key ?? row.id;
    if (!id) return;
    const label = asString(parsed.name) ?? asString(parsed.path) ?? id;
    const lang = asString(parsed.lang) ?? undefined;
    nodesMap.set(id, { id, label, kind, lang, properties: parsed });
  };

  for (const row of fileRows) addNode(row, "file");
  for (const row of dirRows) addNode(row, "directory");

  const edges: ExplorerEdge[] = [];
  for (const row of containsRows) {
    const parsed = parseValue(row.value);
    if (
      selectedRepo &&
      asString(parsed.repo) &&
      asString(parsed.repo) !== selectedRepo
    ) {
      continue;
    }
    const source = asString(parsed.source);
    const target = asString(parsed.target);
    const id = row.id || `${source}->${target}`;
    if (!source || !target || !nodesMap.has(source) || !nodesMap.has(target)) {
      continue;
    }
    edges.push({ id, source, target, type: "contains" });
  }

  return reduceGraphIfLarge(Array.from(nodesMap.values()), edges);
}

async function loadChangedAtCommit(
  commitHash: string,
  database: string,
): Promise<string[]> {
  const changedRes = await executeCypher(
    changedAtCommitQuery(),
    { commitHash },
    database,
  );
  return (changedRes.results[0]?.data ?? [])
    .map((row) => {
      const parsed = parseValue(row.row[3]);
      return (
        extractPathOrId(parsed) ??
        asString(row.row[1]) ??
        asString(row.row[0]) ??
        ""
      );
    })
    .filter(Boolean);
}

export const useExplorerStore = create<ExplorerState>((set, get) => ({
  database: "nornic",
  repos: [],
  selectedRepo: null,
  commits: [],
  currentCommitIndex: 0,
  nodes: [],
  edges: [],
  viewMode: "project",
  selectedNodeId: null,
  selectedFilePath: null,
  changedAtCommit: [],
  isLoading: false,
  error: null,

  loadRepos: async () => {
    set({ isLoading: true, error: null });
    try {
      const { database } = get();
      const res = await executeCypher(listRepositoriesQuery(), {}, database);
      const reposById = new Map<string, RepoItem>();
      for (const item of res.results[0]?.data ?? []) {
        const id = asString(item.row[0]) ?? "";
        const info = parseValue(item.row[1]);
        if (!id || reposById.has(id)) {
          continue;
        }
        reposById.set(id, {
          id,
          name: asString(info.name) ?? asString(info.repo) ?? id,
        });
      }
      const repos = Array.from(reposById.values())
        .map((item) => {
          return item;
        })
        .filter((repo) => repo.id);

      set({ repos, isLoading: false });
      if (repos.length > 0 && !get().selectedRepo) {
        await get().selectRepo(repos[0].id);
      }
    } catch (err) {
      const message =
        err instanceof Error ? err.message : "Failed to load repositories";
      set({ isLoading: false, error: message });
      safeAlert(message);
    }
  },

  selectRepo: async (repoId: string) => {
    set({
      isLoading: true,
      selectedRepo: repoId,
      viewMode: "project",
      selectedFilePath: null,
      selectedNodeId: null,
      error: null,
    });
    try {
      const { database } = get();
      const commitsRes = await executeCypher(
        listCommitsQuery(),
        { limit: 500 },
        database,
      );
      const commits = (commitsRes.results[0]?.data ?? [])
        .map((row) => ({
          hash: asString(row.row[0]) ?? "",
          timestamp: toIsoTimestamp(row.row[1]),
          actor: asString(row.row[2]) ?? "unknown",
        }))
        .filter((c) => c.hash);

      const repoNeedle = `"repo":"${repoId}"`;
      const [repoFilesRes, repoDirsRes, repoContainsRes] = await Promise.all([
        executeCypher(filesForRepoQuery(), { repoNeedle }, database),
        executeCypher(directoriesForRepoQuery(), { repoNeedle }, database),
        executeCypher(containmentEdgesForRepoQuery(), { repoNeedle }, database),
      ]);

      const repoCommitHashes = new Set<string>();
      for (const result of [repoFilesRes, repoDirsRes, repoContainsRes]) {
        for (const item of result.results[0]?.data ?? []) {
          const commitHash = asString(item.row[3]);
          if (commitHash) {
            repoCommitHashes.add(commitHash);
          }
        }
      }

      const repoCommits = commits.filter((commit) =>
        repoCommitHashes.has(commit.hash),
      );

      if (repoCommits.length === 0) {
        set({
          commits: [],
          nodes: [],
          edges: [],
          changedAtCommit: [],
          isLoading: false,
        });
        return;
      }

      const latestIndex = repoCommits.length - 1;
      const latest = repoCommits[latestIndex];
      const [graph, changedAtCommit] = await Promise.all([
        loadProjectGraph(repoId, database, latestIndex, repoCommits),
        loadChangedAtCommit(latest.hash, database),
      ]);

      set({
        commits: repoCommits,
        currentCommitIndex: latestIndex,
        nodes: graph.nodes,
        edges: graph.edges,
        changedAtCommit,
        isLoading: false,
      });
    } catch (err) {
      const message =
        err instanceof Error ? err.message : "Failed to select repository";
      set({ isLoading: false, error: message });
      safeAlert(message);
    }
  },

  setCommitIndex: async (index: number) => {
    const { commits, selectedRepo, database } = get();
    if (
      !selectedRepo ||
      commits.length === 0 ||
      index < 0 ||
      index >= commits.length
    ) {
      return;
    }

    set({
      isLoading: true,
      currentCommitIndex: index,
      viewMode: "project",
      selectedFilePath: null,
      selectedNodeId: null,
      error: null,
    });

    try {
      const commit = commits[index];
      const [graph, changedAtCommit] = await Promise.all([
        loadProjectGraph(selectedRepo, database, index, commits),
        loadChangedAtCommit(commit.hash, database),
      ]);
      set({
        nodes: graph.nodes,
        edges: graph.edges,
        changedAtCommit,
        isLoading: false,
      });
    } catch (err) {
      const message =
        err instanceof Error ? err.message : "Failed to load commit graph";
      set({ isLoading: false, error: message });
      safeAlert(message);
    }
  },

  drillIntoFile: async (filePath: string) => {
    const { commits, currentCommitIndex, database } = get();
    const commit = commits[currentCommitIndex];
    if (!commit) return;

    set({
      isLoading: true,
      selectedFilePath: filePath,
      viewMode: "symbol",
      selectedNodeId: null,
      error: null,
    });

    try {
      const timestamp = commit.timestamp;
      const symbolsRes = await executeCypher(
        symbolsInFileAtTimestampQuery(),
        { timestamp, filePath },
        database,
      );

      const symbolNodes = new Map<string, ExplorerNode>();
      const symbolRows = symbolsRes.results[0]?.data ?? [];
      for (const item of symbolRows) {
        const parsed = parseValue(item.row[2]);
        const id = extractPathOrId(parsed) ?? asString(item.row[0]);
        if (!id) continue;
        const kind = asString(item.row[3]) ?? asString(parsed.kind) ?? "symbol";
        symbolNodes.set(id, {
          id,
          label: asString(parsed.name) ?? id,
          kind,
          lang: asString(parsed.lang) ?? undefined,
          properties: parsed,
        });
      }

      const symbolIds = Array.from(symbolNodes.keys());
      const [callsRes, importsRes] = await Promise.all([
        symbolIds.length > 0
          ? executeCypher(
              callEdgesForSymbolsAtTimestampQuery(),
              { timestamp, symbolIds },
              database,
            )
          : Promise.resolve({
              results: [{ columns: [], data: [] }],
              errors: [],
            }),
        executeCypher(
          importEdgesForFileAtTimestampQuery(),
          { timestamp, filePath },
          database,
        ),
      ]);

      const edges: ExplorerEdge[] = [];

      for (const item of callsRes.results[0]?.data ?? []) {
        const parsed = parseValue(item.row[2]);
        const source = asString(parsed.source);
        const target = asString(parsed.target);
        if (
          !source ||
          !target ||
          !symbolNodes.has(source) ||
          !symbolNodes.has(target)
        )
          continue;
        edges.push({
          id: asString(item.row[0]) ?? `${source}->${target}`,
          source,
          target,
          type: "calls",
        });
      }

      for (const item of importsRes.results[0]?.data ?? []) {
        const parsed = parseValue(item.row[2]);
        const source = asString(parsed.source);
        const target = asString(parsed.target);
        if (!source || !target || source !== filePath) continue;

        const sourceNodeId = `${filePath}#file`;
        if (!symbolNodes.has(sourceNodeId)) {
          symbolNodes.set(sourceNodeId, {
            id: sourceNodeId,
            label: filePath.split("/").pop() ?? filePath,
            kind: "file",
            properties: { path: filePath },
          });
        }

        const moduleNodeId = `import:${target}`;
        if (!symbolNodes.has(moduleNodeId)) {
          symbolNodes.set(moduleNodeId, {
            id: moduleNodeId,
            label: target,
            kind: "module",
            properties: { module: target },
          });
        }

        edges.push({
          id: asString(item.row[0]) ?? `${sourceNodeId}->${moduleNodeId}`,
          source: sourceNodeId,
          target: moduleNodeId,
          type: "imports",
        });
      }

      set({ nodes: Array.from(symbolNodes.values()), edges, isLoading: false });
    } catch (err) {
      const message =
        err instanceof Error ? err.message : "Failed to drill into file";
      set({ isLoading: false, error: message });
      safeAlert(message);
    }
  },

  backToProject: async () => {
    const { commits, currentCommitIndex, selectedRepo, database } = get();
    const commit = commits[currentCommitIndex];
    if (!commit || !selectedRepo) {
      set({ viewMode: "project", selectedFilePath: null });
      return;
    }

    set({
      isLoading: true,
      viewMode: "project",
      selectedFilePath: null,
      selectedNodeId: null,
    });
    try {
      const graph = await loadProjectGraph(
        selectedRepo,
        database,
        currentCommitIndex,
        commits,
      );
      set({ nodes: graph.nodes, edges: graph.edges, isLoading: false });
    } catch (err) {
      const message =
        err instanceof Error ? err.message : "Failed to return to project view";
      set({ isLoading: false, error: message });
      safeAlert(message);
    }
  },

  selectNode: (nodeId: string | null) => {
    set({ selectedNodeId: nodeId });
  },
}));
