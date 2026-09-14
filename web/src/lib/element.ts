// Дефолты новых элементов графа и адаптация settings при смене протокола.
// Протокол выбирается внутри элемента (панель настроек), а не в тулбаре.
import type { GraphNode } from './api';
import { randomUUID } from './uuid';

// Какой протокол подставить свежесозданному элементу.
export const DEFAULT_PROTOCOL: Record<GraphNode['kind'], string> = {
  inbound: 'vless',
  outbound: 'direct',
  rule: 'match',
  balancer: 'urltest',
};

const UUID_PROTOS = ['vless', 'vmess', 'tuic'];
const PASSWORD_PROTOS = ['trojan', 'shadowsocks', 'hysteria2', 'tuic'];
const TRANSPORT_PROTOS = ['vless', 'vmess', 'trojan'];
const REALITY_PROTOS = ['vless', 'trojan'];
const TLS_REQUIRED = ['hysteria2', 'tuic'];

export function randPassword(): string {
  return (Math.random().toString(36).slice(2) + Math.random().toString(36).slice(2)).slice(0, 16);
}

// Пользователь с учётными данными под конкретный протокол.
export function userForProtocol(protocol: string, name: string): Record<string, any> {
  const u: Record<string, any> = { name };
  if (UUID_PROTOS.includes(protocol)) u.uuid = randomUUID();
  if (PASSWORD_PROTOS.includes(protocol)) u.password = randPassword();
  return u;
}

// Минимальные settings, чтобы новый элемент сразу проходил валидацию.
export function defaultSettings(kind: GraphNode['kind'], protocol: string): Record<string, any> {
  const s: Record<string, any> = {};
  if (kind === 'inbound') s.users = [userForProtocol(protocol, 'user1')];
  if (kind === 'balancer' && protocol === 'urltest') s.url = 'https://www.gstatic.com/generate_204';
  return s;
}

// Приводит settings в соответствие новому протоколу (смена протокола в панели):
// досоздаёт учётные данные пользователей, включает обязательный TLS,
// убирает поля, несовместимые с протоколом (transport/reality/flow).
export function applyProtocol(n: GraphNode): void {
  const s = (n.settings ??= {});
  if (n.kind === 'inbound') {
    s.users ??= [];
    if (s.users.length === 0) s.users.push(userForProtocol(n.protocol, 'user1'));
    for (const u of s.users) {
      if (UUID_PROTOS.includes(n.protocol) && !u.uuid) u.uuid = randomUUID();
      if (PASSWORD_PROTOS.includes(n.protocol) && !u.password) u.password = randPassword();
      if (n.protocol !== 'vless') delete u.flow;
    }
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
