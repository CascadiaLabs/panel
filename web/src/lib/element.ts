// Дефолты новых элементов графа и адаптация settings при смене протокола.
// Протокол выбирается внутри элемента (панель настроек), а не в тулбаре.
import type { GraphNode } from './api';

// Какой протокол подставить свежесозданному элементу.
export const DEFAULT_PROTOCOL: Record<GraphNode['kind'], string> = {
  inbound: 'vless',
  outbound: 'direct',
  rule: 'match',
  balancer: 'urltest',
};

const TRANSPORT_PROTOS = ['vless', 'vmess', 'trojan'];
const REALITY_PROTOS = ['vless', 'trojan'];
const TLS_REQUIRED = ['hysteria2', 'tuic'];

// Минимальные settings, чтобы новый элемент сразу проходил валидацию.
export function defaultSettings(kind: GraphNode['kind'], protocol: string): Record<string, any> {
  const s: Record<string, any> = {};
  if (kind === 'balancer' && protocol === 'urltest') s.url = 'https://www.gstatic.com/generate_204';
  return s;
}

// Приводит settings в соответствие новому протоколу (смена протокола в панели):
// включает обязательный TLS, убирает поля, несовместимые с протоколом (transport/reality/flow).
export function applyProtocol(n: GraphNode): void {
  const s = (n.settings ??= {});
  if (n.kind === 'inbound') {
    if (n.protocol === 'shadowsocks') s.method ??= '2022-blake3-aes-128-gcm';
    if (n.protocol === 'tuic') s.congestion_control ??= 'bbr';
    if (!TRANSPORT_PROTOS.includes(n.protocol)) delete s.transport;
    if (TLS_REQUIRED.includes(n.protocol)) {
      s.tls ??= {};
      s.tls.enabled = true;
    }
    if (s.tls?.reality && !REALITY_PROTOS.includes(n.protocol)) s.tls.reality.enabled = false;
  }
  if (n.kind === 'balancer' && n.protocol === 'urltest') {
    s.url ??= 'https://www.gstatic.com/generate_204';
  }
}
