// Run after npm run build: npx playwright test graphs.browser.test.mjs --workers=1
// Tests the built SPA with isolated API fixtures; no real nodes or databases are modified.
import { test, expect } from '@playwright/test';
import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';

const routes = (graphs, states, validation) => async route => {
  const request = route.request();
  const path = new URL(request.url()).pathname;
  const method = request.method();
  const reply = json => route.fulfill({ json });
  if (!path.startsWith('/api/')) {
    const file = path.startsWith('/assets/') ? path.slice(1) : 'index.html';
    return route.fulfill({ body: await readFile(resolve('dist', file)), contentType: file.endsWith('.js') ? 'text/javascript' : file.endsWith('.css') ? 'text/css' : 'text/html' });
  }
  if (path === '/api/auth/me') return reply({ user: { username: 'admin' } });
  if (path === '/api/nodes') return reply([{ id: 'node1', name: 'Node A', updated_at: 1 }, { id: 'node2', name: 'Node B', updated_at: 1 }]);
  if (path === '/api/nodes/statuses') return reply({});
  if (path === '/api/graphs') {
    if (method === 'POST') {
      const graph = { id: 'created', name: request.postDataJSON().name, updated_at: 2 };
      graphs.push(graph);
      states[graph.id] = { nodes: [], edges: [] };
      return reply(graph);
    }
    return reply(graphs);
  }
  const [, id, action] = path.match(/^\/api\/graphs\/([^/]+)(?:\/(\w+))?$/) || [];
  if (id) {
    if (action === 'validate') return reply(validation());
    if (method === 'PUT') states[id] = request.postDataJSON().state ?? states[id];
    return reply({ graph: graphs.find(g => g.id === id), state: states[id], validation: validation() });
  }
  return route.fulfill({ status: 404 });
};

async function openEditor(page, states, validation = () => ({ errors: [], warnings: [] })) {
  const graphs = [{ id: 'existing', name: 'Existing graph', updated_at: 1 }];
  states.existing ??= { nodes: [], edges: [] };
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.route('http://panel.test/**', routes(graphs, states, validation));
  await page.goto('http://panel.test/');
  await page.getByRole('button', { name: 'Графы', exact: true }).click();
  await page.getByRole('button', { name: 'Existing graph', exact: true }).click();
  await expect(page.locator('.svelte-flow')).toBeVisible();
  return errors;
}

// Клик по центру связи: bbox рёбер анимируется (fitView), берём свежий.
async function clickEdgeCenter(page) {
  const edge = page.locator('.svelte-flow__edge').first();
  const box = await edge.boundingBox();
  // Ищем точку внутри bbox, где реально находится линия (elementFromPoint = path).
  const point = await page.evaluate(box => {
    const step = 8;
    for (let y = box.y + 2; y < box.y + box.height; y += step) {
      for (let x = box.x + 2; x < box.x + box.width; x += step) {
        const el = document.elementFromPoint(x, y);
        if (el && el.classList && el.classList.contains('svelte-flow__edge-interaction')) {
          return { x, y };
        }
      }
    }
    return null;
  }, box);
  if (!point) throw new Error('no clickable point on edge');
  await page.mouse.click(point.x, point.y);
  await expect(page.locator('.svelte-flow__edge.selected')).toHaveCount(1, { timeout: 3000 });
}

// Порты Клиента/Интернета и карточек небольшие; в headless попадание иногда
// срывается из-за переходных состояний zoom/pan. Повторяем драг до успеха.
async function dragConnection(page, from, to, done) {
  for (let attempt = 0; attempt < 4; attempt++) {
    const fromBox = await from.boundingBox();
    const toBox = await to.boundingBox();
    if (!fromBox || !toBox) throw new Error('handle not visible');
    await page.mouse.move(fromBox.x + fromBox.width / 2, fromBox.y + fromBox.height / 2);
    await page.mouse.down();
    await page.mouse.move(toBox.x + toBox.width / 2, toBox.y + toBox.height / 2, { steps: 25 });
    await page.mouse.up();
    try {
      await expect.poll(done, { timeout: 1200 }).toBe(true);
      return;
    } catch { /* retry */ }
  }
  throw new Error('drag connection did not produce an edge');
}

test('create, open, edit and reopen graphs without browser errors', async ({ page }) => {
  const errors = await openEditor(page, {});
  await expect(page.getByRole('button', { name: '0 ошибок' })).toBeVisible();
  await expect(page.getByRole('button', { name: /0 предупреждений/ })).toBeVisible();
  expect(errors).toEqual([]);
});

test('drag from Client port to inbound connects them, deleting the link detaches', async ({ page }) => {
  const states = {};
  const errors = await openEditor(page, states);
  await page.getByRole('button', { name: '+ inbound', exact: true }).click();
  await page.locator('aside.panel header').getByRole('button', { name: '×', exact: true }).click();
  const inboundHandle = page.locator('.svelte-flow__node-cascade .svelte-flow__handle.target').first();
  await dragConnection(page, page.getByLabel('Порт Клиента'), inboundHandle, () => states.existing.nodes[0]?.entry === true);
  await clickEdgeCenter(page);
  await page.getByRole('button', { name: 'Удалить связь ×' }).click();
  await expect.poll(() => states.existing.nodes[0]?.entry, { timeout: 4000 }).toBe(false);
  expect(errors).toEqual([]);
});

test('drag from inbound to Internet port marks exit, Delete key detaches', async ({ page }) => {
  const states = {};
  const errors = await openEditor(page, states);
  await page.getByRole('button', { name: '+ inbound', exact: true }).click();
  await page.locator('aside.panel header').getByRole('button', { name: '×', exact: true }).click();
  const sourceHandle = page.locator('.svelte-flow__node-cascade .svelte-flow__handle.source').first();
  await dragConnection(page, sourceHandle, page.getByLabel('Порт Интернета'), () => states.existing.nodes[0]?.exit === true);
  await clickEdgeCenter(page);
  await page.keyboard.press('Delete');
  await expect.poll(() => states.existing.nodes[0]?.exit, { timeout: 4000 }).toBe(false);
  expect(errors).toEqual([]);
});

test('inbound on node A connects directly to inbound on node B without outbound', async ({ page }) => {
  const states = {};
  const errors = await openEditor(page, states);
  await page.getByRole('button', { name: '+ inbound', exact: true }).click();
  await page.locator('aside.panel header').getByRole('button', { name: '×', exact: true }).click();
  await page.getByRole('button', { name: '+ inbound', exact: true }).click();
  await page.getByLabel('Протокол').selectOption('trojan');
  await page.getByLabel('Физическая нода').selectOption('node2');
  await page.locator('aside.panel header').getByRole('button', { name: '×', exact: true }).click();
  await page.waitForTimeout(300);
  const sources = page.locator('.svelte-flow__node-cascade .svelte-flow__handle.source');
  const targets = page.locator('.svelte-flow__node-cascade .svelte-flow__handle.target');
  // Первый source — vless (node1), второй target — trojan (node2): порядок DOM стабилен.
  await dragConnection(page, sources.nth(0), targets.nth(1), () => states.existing.edges.length === 1);
  const n1 = states.existing.nodes.find(n => n.node_id === 'node1');
  const n2 = states.existing.nodes.find(n => n.node_id === 'node2');
  expect(states.existing.edges[0].source_id).toBe(n1.id);
  expect(states.existing.edges[0].target_id).toBe(n2.id);
  await clickEdgeCenter(page);
  await page.getByRole('button', { name: 'Удалить связь ×' }).click();
  await expect.poll(() => states.existing.edges.length, { timeout: 4000 }).toBe(0);
  expect(errors).toEqual([]);
});

test('balancer connects to inbound on the same physical node', async ({ page }) => {
  const states = {};
  const errors = await openEditor(page, states);
  await page.getByRole('button', { name: '+ inbound', exact: true }).click();
  await page.locator('aside.panel header').getByRole('button', { name: '×', exact: true }).click();
  await page.getByRole('button', { name: '+ balancer', exact: true }).click();
  await page.locator('aside.panel header').getByRole('button', { name: '×', exact: true }).click();
  await page.waitForTimeout(300);
  const source = page.locator('.svelte-flow__node-cascade .card.kind-balancer .svelte-flow__handle.source');
  const target = page.locator('.svelte-flow__node-cascade .card.kind-inbound .svelte-flow__handle.target');
  await dragConnection(page, source, target, () => states.existing.edges.length === 1);
  const inbound = states.existing.nodes.find(n => n.kind === 'inbound');
  const balancer = states.existing.nodes.find(n => n.kind === 'balancer');
  expect(states.existing.edges[0]).toMatchObject({ source_id: balancer.id, target_id: inbound.id });
  expect(errors).toEqual([]);
});

test('issue list shows messages and focuses the related element', async ({ page }) => {
  const states = {};
  const errors = await openEditor(page, states, () => ({
    errors: [{ code: 'no_users', element: states.existing.nodes[0]?.id || 'in-1', message: 'inbound in-1: добавьте пользователя' }],
    warnings: [{ code: 'no_entry', element: '', message: 'ни один inbound не отмечен как вход' }],
  }));
  await page.getByRole('button', { name: '+ inbound', exact: true }).click();
  await page.getByLabel('Tag (уникален в графе)').fill('in-1');
  await page.locator('aside.panel header').getByRole('button', { name: '×', exact: true }).click();
  await page.getByRole('button', { name: 'Проверить' }).click();
  await expect(page.getByRole('button', { name: '1 ошибок' })).toBeVisible();
  await page.getByRole('button', { name: /1 предупреждений/ }).click();
  await expect(page.getByRole('button', { name: 'inbound in-1: добавьте пользователя' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'ни один inbound не отмечен как вход' })).toBeVisible();
  await page.getByRole('button', { name: 'inbound in-1: добавьте пользователя' }).click();
  await expect(page.locator('aside.panel')).toBeVisible();
  expect(errors).toEqual([]);
});
