import type { Node, Status, StatusesMap } from './api';
import { api } from './api';

export interface NodesStore {
  readonly nodes: Node[];
  readonly statuses: StatusesMap;
  load: () => Promise<void>;
  loadStatuses: () => Promise<void>;
  add: (n: Node) => Promise<void>;
  update: (id: string, n: Partial<Node>) => Promise<void>;
  remove: (id: string) => Promise<void>;
  refresh: (id: string) => Promise<Status>;
  push: (id: string, config: string) => Promise<void>;
}

export function createNodesStore(): NodesStore {
  let nodes = $state<Node[]>([]);
  let statuses = $state<StatusesMap>({});

  return {
    get nodes() {
      return nodes;
    },
    get statuses() {
      return statuses;
    },
    async load() {
      nodes = await api<Node[]>('/nodes');
    },
    async loadStatuses() {
      statuses = await api<StatusesMap>('/nodes/statuses');
    },
    async add(n) {
      await api('/nodes', {
        method: 'POST',
        body: JSON.stringify(n),
      });
      await this.load();
    },
    async update(id, patch) {
      const existing = nodes.find((n) => n.id === id);
      if (!existing) throw new Error(`node ${id} not found`);
      await api(`/nodes/${id}`, {
        method: 'PUT',
        body: JSON.stringify({ ...existing, ...patch }),
      });
      await this.load();
    },
    async remove(id) {
      await api(`/nodes/${id}`, { method: 'DELETE' });
      nodes = nodes.filter((n) => n.id !== id);
    },
    async refresh(id) {
      return await api<Status>(`/nodes/${id}/status`);
    },
    async push(id, config) {
      await api(`/nodes/${id}/push`, {
        method: 'POST',
        body: JSON.stringify({ config_json: config }),
      });
    },
  };
}
