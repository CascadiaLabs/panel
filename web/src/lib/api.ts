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
  subscription_name?: string;
  subscription_desc?: string;
  subscription_site?: string;
  subscription_support?: string;
  client_route?: string; // JSON-строка клиентской маршрутизации
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
  used_upload: number;
  used_download: number;
  total_traffic: number;
  expire_time: number;
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

export function updateUser(id: string, body: {
  name?: string;
  remark?: string;
  flow?: string;
  graph_id?: string;
  enabled?: boolean;
  used_upload?: number;
  used_download?: number;
  total_traffic?: number;
  expire_time?: number;
}): Promise<UserOpResponse> {
  return api<UserOpResponse>(`/users/${id}`, { method: 'PUT', body: JSON.stringify(body) });
}

export function deleteUser(id: string): Promise<void> {
  return api<void>(`/users/${id}`, { method: 'DELETE' });
}

export function subUrl(token: string): string {
  return `${location.origin}/sub/${token}`;
}

// Правила маршрутизации
export interface RouteRule {
  id: string;
  graph_id: string;
  name: string;
  rules_json: string;
  is_default: boolean;
  created_at: number;
  updated_at: number;
}

export function listRouteRules(graphId: string): Promise<RouteRule[]> {
  return api<RouteRule[]>(`/graphs/${graphId}/routes`);
}

export function createRouteRule(graphId: string, body: { name: string; rules_json: string }): Promise<RouteRule> {
  return api<RouteRule>(`/graphs/${graphId}/routes`, { method: 'POST', body: JSON.stringify(body) });
}

export function updateRouteRule(id: string, body: { name: string; rules_json: string }): Promise<void> {
  return api<void>(`/graphs/routes/${id}`, { method: 'PUT', body: JSON.stringify(body) });
}

export function deleteRouteRule(id: string): Promise<void> {
  return api<void>(`/graphs/routes/${id}`, { method: 'DELETE' });
}

// Получение всех route-rule, назначенных на inbound (может быть несколько).
export function getInboundRouteAssignments(inboundId: string): Promise<{ route_rule_ids: string[] }> {
  return api<{ route_rule_ids: string[] }>(`/routes/inbound/${inboundId}`);
}

export function assignInboundRoute(inboundId: string, routeRuleId: string): Promise<void> {
  return api<void>(`/routes/inbound/${inboundId}`, { method: 'PUT', body: JSON.stringify({ route_rule_id: routeRuleId }) });
}

export function unassignInboundRoute(inboundId: string, routeRuleId: string): Promise<void> {
  return api<void>(`/routes/inbound/${inboundId}/${routeRuleId}`, { method: 'DELETE' });
}

// Подписка с метаданными и клиентской маршрутизацией.
export interface SubscriptionResponse {
  name: string;
  desc?: string;
  site?: string;
  support?: string;
  subscription_id: string;
  links?: string[];
  userinfo?: string;
  profile_title?: string;
  profile_update_interval?: number;
  profile_web_page_url?: string;
  support_url?: string;
  config?: Record<string, any>; // Sing-box JSON конфиг
  client_route?: string;
}

export function subscriptionInfo(graphId: string): Promise<SubscriptionResponse> {
  return api<SubscriptionResponse>(`/graphs/${graphId}/subscription`);
}
