export interface Node {
  id: string;
  name: string;
  grpc_url: string;
  token: string;
  cert_pem: string;
  config_json: string;
  created_at: number;
  updated_at: number;
}

export interface Status {
  running: boolean;
  singbox_version: string;
  uptime_seconds: number;
  inbounds: number;
  outbounds: number;
  last_update_unix: number;
}

// Ответ /api/nodes/statuses: id → {status} либо {error}.
export type StatusesMap = Record<string, { status?: Status; error?: string }>;

export interface GraphMeta {
  id: string;
  name: string;
  created_at: number;
  updated_at: number;
}

export interface GraphNode {
  id: string;
  node_id: string;
  kind: 'inbound' | 'outbound' | 'rule' | 'balancer';
  protocol: string;
  tag: string;
  settings: any;
  pos_x: number;
  pos_y: number;
  entry: boolean;
  exit: boolean;
}

export interface GraphEdge {
  id: string;
  source_id: string;
  target_id: string;
}

export interface GraphState {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface Issue {
  code: string;
  element: string;
  message: string;
}

export interface Validation {
  errors: Issue[];
  warnings: Issue[];
}

export interface GraphResponse {
  graph: GraphMeta;
  state: GraphState;
  validation?: Validation;
}

export interface DeployReport {
  node_id: string;
  name: string;
  ok: boolean;
  error?: string;
}

// Событие разлогина: 401 из любого вызова.
export const authEvents = new EventTarget();

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, {
    ...init,
    credentials: 'same-origin',
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers || {}),
    },
  });
  if (res.status === 401) {
    authEvents.dispatchEvent(new Event('unauthorized'));
  }
  if (!res.ok) {
    const text = await res.text();
    // JSON-ошибки вида {"error": "..."} показываем красивее
    try {
      const parsed = JSON.parse(text);
      if (parsed && typeof parsed.error === 'string') {
        throw new ApiError(res.status, parsed.error);
      }
    } catch (e) {
      if (e instanceof ApiError) throw e;
    }
    throw new ApiError(res.status, text || res.statusText);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

// --- Auth ---

export interface Me {
  user: { id: string; username: string };
  bearer?: boolean;
}

export function login(username: string, password: string): Promise<Me> {
  return api<Me>('/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) });
}

export function logout(): Promise<void> {
  return api<void>('/auth/logout', { method: 'POST' });
}

export function me(): Promise<Me> {
  return api<Me>('/auth/me');
}

export function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  return api<void>('/auth/password', {
    method: 'PUT',
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  });
}

// --- Утилиты генерации секретов ---

export function generateSecret(kind: 'uuid' | 'password' | 'shortid'): Promise<string> {
  return api<{ value: string }>('/util/generate', {
    method: 'POST',
    body: JSON.stringify({ kind }),
  }).then((r) => r.value);
}

export function generateRealityPair(): Promise<{ private_key: string; public_key: string }> {
  return api<{ private_key: string; public_key: string }>('/util/generate', {
    method: 'POST',
    body: JSON.stringify({ kind: 'reality' }),
  });
}

// --- VPN-пользователи ---

export interface PanelUser {
  id: string;
  name: string;
  graph_id: string;
  graph_name?: string;
  uuid: string;
  password: string;
  flow: string;
  remark: string;
  sub_token: string;
  enabled: boolean;
  created_at: number;
  updated_at: number;
}

export interface DeployResult {
  deployed: boolean;
  results: { node_id: string; name: string; ok: boolean; error?: string }[];
}

export interface UserOpResponse {
  user: PanelUser;
  deploy?: DeployResult;
  warning?: string;
}

export function listUsers(): Promise<PanelUser[]> {
  return api<PanelUser[]>('/users');
}

export function createUser(body: { name: string; graph_id: string; remark?: string; flow?: string }): Promise<UserOpResponse> {
  return api<UserOpResponse>('/users', { method: 'POST', body: JSON.stringify(body) });
}

export function updateUser(id: string, body: { name?: string; remark?: string; flow?: string; graph_id?: string; enabled?: boolean }): Promise<UserOpResponse> {
  return api<UserOpResponse>(`/users/${id}`, { method: 'PUT', body: JSON.stringify(body) });
}

export function deleteUser(id: string): Promise<void> {
  return api<void>(`/users/${id}`, { method: 'DELETE' });
}

export function subUrl(token: string): string {
  return `${location.origin}/sub/${token}`;
}
