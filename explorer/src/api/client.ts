export interface CypherResponse {
  results: Array<{
    columns: string[];
    data: Array<{ row: unknown[]; meta: unknown[] }>;
  }>;
  errors?: Array<{ code: string; message: string }>;
}

export interface GraphPayload {
  nodes: Array<{
    id: string;
    labels: string[];
    properties: Record<string, unknown>;
    status?: string;
  }>;
  edges: Array<{
    id: string;
    source: string;
    target: string;
    type: string;
    properties?: Record<string, unknown>;
    semantic?: boolean;
    status?: string;
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

const BASE_URL = "http://localhost:7474";
const DEFAULT_DB = "nornic";
const AUTH = `Basic ${btoa("admin:password")}`;

async function postJson<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: AUTH,
    },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    throw new Error(`HTTP ${res.status}: ${await res.text()}`);
  }

  return (await res.json()) as T;
}

function handleCypherErrors(response: CypherResponse): CypherResponse {
  if (response.errors && response.errors.length > 0) {
    const message = response.errors
      .map((e) => `${e.code}: ${e.message}`)
      .join("\n");
    throw new Error(message);
  }
  return response;
}

export async function executeCypher(
  statement: string,
  parameters: Record<string, unknown> = {},
  database = DEFAULT_DB,
): Promise<CypherResponse> {
  const response = await postJson<CypherResponse>(`/db/${database}/tx/commit`, {
    statements: [{ statement, parameters }],
  });
  return handleCypherErrors(response);
}

export async function graphNeighborhood(
  payload: {
    node_ids: string[];
    depth?: number;
    limit?: number;
    labels?: string[];
    relationship_types?: string[];
  },
  database = DEFAULT_DB,
): Promise<GraphPayload> {
  return postJson<GraphPayload>(
    `/nornicdb/graph/${database}/neighborhood`,
    payload,
  );
}

export async function graphTemporal(
  payload: { node_ids: string[]; as_of: string },
  database = DEFAULT_DB,
): Promise<GraphPayload> {
  return postJson<GraphPayload>(
    `/nornicdb/graph/${database}/temporal`,
    payload,
  );
}

export async function graphDiff(
  payload: { node_ids: string[]; as_of: string; compare_to?: string },
  database = DEFAULT_DB,
): Promise<GraphPayload> {
  return postJson<GraphPayload>(`/nornicdb/graph/${database}/diff`, payload);
}
