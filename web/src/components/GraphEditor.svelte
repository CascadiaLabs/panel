<script lang="ts">
  // Редактор графа каскада: слева док «Клиент», справа док «Интернет»,
  // посередине канвас svelte-flow с элементами inbound/outbound/rule/balancer.
  import { onMount } from 'svelte';
  import {
    SvelteFlow,
    Background,
    Controls,
    MiniMap,
  } from '@xyflow/svelte';
  import '@xyflow/svelte/dist/style.css';
  import {
    api,
    me,
    logout,
    type GraphMeta,
    type GraphState,
    type GraphNode,
    type GraphEdge,
    type GraphResponse,
    type Issue,
    type Validation,
    type DeployReport,
    type Node as PhysNode,
  } from '$lib/api';
  import { randomUUID } from '$lib/uuid';
  import { DEFAULT_PROTOCOL, defaultSettings } from '$lib/element';
  import GraphNodeCard from './GraphNodeCard.svelte';
  import GraphDock from './GraphDock.svelte';
  import NodeSettings from './NodeSettings.svelte';

  let { graphId, onBack } = $props();

  // Bind the child canvas viewport; hooks here have no SvelteFlow context yet.
  let viewport = $state({ x: 0, y: 0, zoom: 1 });

  // --- данные ---
  let graph = $state<GraphMeta | null>(null);
  let gnodes = $state<GraphNode[]>([]);
  let gedges = $state<GraphEdge[]>([]);
  let physNodes = $state<PhysNode[]>([]);
  let validation = $state<Validation | null>(null);

  // --- UI-состояние ---
  let selectedId = $state('');
  let saving = $state(false);
  let saveError = $state('');
  let lastSaved = $state(0);
  let selectedLink = $state<{ kind: 'edge' | 'client' | 'internet'; id: string } | null>(null);
  let showIssues = $state(false);
  const CLIENT = '__cascadia_client__';
  const INTERNET = '__cascadia_internet__';
  const DOCK_WIDTH = 200;
  let showPreview = $state(false);
  let previewConfigs = $state<Record<string, string>>({});
  let deploying = $state(false);
  let deployResults = $state<DeployReport[] | null>(null);
  let deployError = $state('');
  let flowWrapper = $state<HTMLDivElement | null>(null);
  let wrapperSize = $state({ w: 800, h: 600 });

  let saveTimer: ReturnType<typeof setTimeout> | undefined;

  const selected = $derived(gnodes.find((n) => n.id === selectedId) ?? null);

  // cascadeTarget для выбранного outbound: inbound, к которому он подключён
  const cascadeTarget = $derived(
    selected?.kind === 'outbound'
      ? (() => {
          const edge = gedges.find((e) => e.source_id === selected.id);
          if (!edge) return null;
          const target = gnodes.find((n) => n.id === edge.target_id);
          return target?.kind === 'inbound' ? target : null;
        })()
      : null
  );

  // issues по элементам (element = id) для бейджей карточек
  const issuesByElement = $derived.by(() => {
    const map: Record<string, Issue[]> = {};
    if (!validation) return map;
    for (const i of [...validation.errors.map((x) => ({ ...x, severity: 'error' })),
                      ...validation.warnings.map((x) => ({ ...x, severity: 'warning' }))]) {
      // element может быть id элемента, id ребра или цепочкой цикла (через запятую)
      if (!i.element) continue;
      for (const id of i.element.split(',')) {
        (map[id] ??= []).push(i);
      }
    }
    return map;
  });

  // Dock handles are real Flow nodes, pinned to screen-space rail boundaries.
  // Their synthetic edges map back to the existing entry/exit flags on save.
  function dockNode(id: string, kind: string, x: number) {
    return {
      id, type: 'dock', data: { kind }, draggable: false, selectable: false,
      deletable: false, focusable: false, zIndex: 100,
      position: { x: (x - viewport.x) / viewport.zoom, y: (wrapperSize.h / 2 - viewport.y) / viewport.zoom },
      origin: (kind === 'client' ? [1, 0.5] : [0, 0.5]) as [number, number],
      style: `transform:scale(${1 / viewport.zoom});transform-origin:${kind === 'client' ? 'right' : 'left'} center`,
    };
  }
  const flowNodes = $derived([
    ...gnodes.map(n => ({
      id: n.id, type: 'cascade', position: { x: n.pos_x, y: n.pos_y },
      selected: selectedId === n.id,
      data: { graphNode: n, nodeName: physNodes.find(p => p.id === n.node_id)?.name ?? '', issues: issuesByElement[n.id] ?? [] },
    })),
    dockNode(CLIENT, 'client', DOCK_WIDTH),
    dockNode(INTERNET, 'internet', wrapperSize.w - DOCK_WIDTH),
  ]);
  const clientNodes = $derived(gnodes.filter(n => n.entry));
  const exitNodes = $derived(gnodes.filter(n => n.exit));
  const flowEdges = $derived([
    ...gedges.map(e => ({ id: e.id, source: e.source_id, target: e.target_id, data: { kind: 'edge', id: e.id } })),
    ...clientNodes.map(n => ({ id: `client:${n.id}`, source: CLIENT, target: n.id, data: { kind: 'client', id: n.id } })),
    ...exitNodes.map(n => ({ id: `internet:${n.id}`, source: n.id, target: INTERNET, data: { kind: 'internet', id: n.id } })),
  ].map(e => ({
    ...e, sourceHandle: 'out', targetHandle: 'in',
    selected: selectedLink?.kind === e.data.kind && selectedLink?.id === e.data.id,
    interactionWidth: 24,
    style: `stroke:${selectedLink?.kind === e.data.kind && selectedLink?.id === e.data.id ? '#38bdf8' : '#94a3b8'};stroke-width:3;`,
  })));
  const nodeTypes = { cascade: GraphNodeCard, dock: GraphDock };

  // --- загрузка ---
  onMount(() => {
    const ro = new ResizeObserver((entries) => {
      for (const e of entries) {
        wrapperSize = { w: e.contentRect.width, h: e.contentRect.height };
      }
    });
    if (flowWrapper) ro.observe(flowWrapper);
    void (async () => {
    try {
      const [resp, nodes] = await Promise.all([
        api<GraphResponse>(`/graphs/${graphId}`),
        api<PhysNode[]>('/nodes'),
      ]);
      graph = resp.graph;
      gnodes = resp.state.nodes ?? [];
      gedges = resp.state.edges ?? [];
      physNodes = nodes;
      await doValidate();
    } catch (e) {
      saveError = e instanceof Error ? e.message : String(e);
    }
    })();
    return () => {
      ro.disconnect();
      if (saveTimer) {
        clearTimeout(saveTimer);
        void doSave();
      }
    };
  });

  // --- сохранение (autosave с дебаунсом) ---
  function scheduleSave() {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => {
      saveTimer = undefined;
      void doSave();
    }, 800);
  }

  async function doSave() {
    saving = true;
    saveError = '';
    try {
      const resp = await api<GraphResponse>(`/graphs/${graphId}`, {
        method: 'PUT',
        body: JSON.stringify({
          state: { nodes: gnodes, edges: gedges },
        }),
      });
      validation = resp.validation ?? null;
      lastSaved = Date.now();
    } catch (e) {
      saveError = e instanceof Error ? e.message : String(e);
    } finally {
      saving = false;
    }
  }

  async function doValidate() {
    try {
      const v = await api<Validation>(`/graphs/${graphId}/validate`, { method: 'POST' });
      validation = v;
    } catch (e) {
      saveError = e instanceof Error ? e.message : String(e);
    }
  }

  // --- операции с элементами ---
  // Одна кнопка на kind: протокол и все параметры задаются в панели настроек.
  function addNode(kind: GraphNode['kind']) {
    if (physNodes.length === 0) {
      saveError = 'Сначала добавьте физическую ноду во вкладке Nodes';
      return;
    }
    const protocol = DEFAULT_PROTOCOL[kind];
    const n: GraphNode = {
      id: randomUUID(),
      node_id: physNodes[0].id,
      kind,
      protocol,
      tag: `${kind}-${gnodes.length + 1}`,
      settings: defaultSettings(kind, protocol),
      pos_x: 260 + (gnodes.length % 4) * 60,
      pos_y: 60 + Math.random() * 200,
      entry: false,
      exit: false,
    };
    if (kind === 'inbound') n.settings.listen_port = 40000 + gnodes.length;
    gnodes.push(n);
    selectedId = n.id; // панель настроек открывается сразу — настройка внутри элемента
    scheduleSave();
  }

  function deleteSelected() {
    if (!selected) return;
    const id = selected.id;
    gnodes = gnodes.filter(n => n.id !== id);
    gedges = gedges.filter(e => e.source_id !== id && e.target_id !== id);
    selectedId = '';
    selectedLink = null;
    scheduleSave();
  }

  // --- соединения ---
  // Матрица валидных source-kind → target-kind (клиентская проверка).
  function canConnect(source: GraphNode, target: GraphNode): string | null {
    if (target.kind === 'inbound' && source.kind === 'balancer') {
      if (gedges.some(e => e.source_id === source.id && e.target_id === target.id)) return 'Соединение уже существует';
      return null;
    }
    if (target.kind === 'inbound' && ['inbound', 'rule'].includes(source.kind)) {
      if (source.node_id === target.node_id) return 'Каскад должен вести на другую физическую ноду';
      if (gedges.some(e => e.source_id === source.id && e.target_id === target.id)) return 'Соединение уже существует';
      return null;
    }
    const kindPair = `${source.kind}->${target.kind}`;
    const matrix: Record<string, string> = {
      'inbound->rule': '',
      'inbound->balancer': '',
      'inbound->outbound': '',
      'rule->outbound': '',
      'rule->balancer': '',
      'balancer->outbound': '',
      'outbound->inbound': '',
    };
    if (!(kindPair in matrix)) return 'Запрещённое соединение';
    if (source.id === target.id) return 'Нельзя соединить элемент с самим собой';
    if (gedges.some((e) => e.source_id === source.id && e.target_id === target.id)) return 'Соединение уже существует';
    if (kindPair !== 'outbound->inbound' && source.node_id !== target.node_id) {
      return 'Разные типы соединяются только внутри одной физической ноды (каскад outbound→inbound — между нодами)';
    }
    if (kindPair === 'outbound->inbound') {
      if (source.node_id === target.node_id) return 'Каскад outbound→inbound должен вести на ДРУГУЮ ноду';
      if (source.protocol !== target.protocol) return `Протоколы должны совпадать: ${source.protocol} → ${target.protocol}`;
    }
    return null;
  }

  function validConnection({ source, target }) {
    const src = gnodes.find(n => n.id === source);
    const dst = gnodes.find(n => n.id === target);
    if (source === CLIENT) return dst?.kind === 'inbound' && !dst.entry;
    if (target === INTERNET) return !!src && ['inbound', 'outbound'].includes(src.kind) && !src.exit;
    return !!src && !!dst && canConnect(src, dst) === null;
  }

  function onConnect(connection) {
    if (!validConnection(connection)) return;
    if (connection.source === CLIENT || connection.target === INTERNET) {
      const isEntry = connection.source === CLIENT;
      const n = gnodes.find(n => n.id === (isEntry ? connection.target : connection.source));
      if (isEntry) n.entry = true;
      else n.exit = true;
      scheduleSave();
      return;
    }
    const source = gnodes.find((n) => n.id === connection.source);
    const target = gnodes.find((n) => n.id === connection.target);
    if (!source || !target) return;
    const err = canConnect(source, target);
    if (err) {
      saveError = err;
      return;
    }
    saveError = '';
    gedges.push({ id: randomUUID(), source_id: connection.source, target_id: connection.target });
    scheduleSave();
  }

  function onEdgeClick({ event, edge }) {
    event.stopPropagation();
    selectedId = '';
    selectedLink = edge.data;
  }

  function deleteConnection() {
    if (!selectedLink) return;
    const { kind, id } = selectedLink;
    if (kind === 'edge') gedges = gedges.filter(e => e.id !== id);
    else unlinkDock(kind, id);
    selectedLink = null;
    scheduleSave();
  }

  function focusIssue(issue: Issue) {
    const ids = (issue.element || '').split(',').filter(Boolean);
    if (!ids.length) { selectedId = ''; selectedLink = null; return; }
    const edge = gedges.find(e => ids.includes(e.id));
    // element может быть id элемента, tag или id ребра — поддерживаем все варианты
    const node = gnodes.find(n => ids.includes(n.id) || ids.includes(n.tag) || n.id === edge?.source_id);
    selectedLink = edge ? { kind: 'edge', id: edge.id } : null;
    selectedId = node?.id ?? '';
    showIssues = false;
    if (node) viewport = {
      x: wrapperSize.w / 2 - (node.pos_x + 95) * viewport.zoom,
      y: wrapperSize.h / 2 - (node.pos_y + 31) * viewport.zoom,
      zoom: viewport.zoom,
    };
  }

  function onNodeClick({ event, node }) {
    event.stopPropagation();
    if (node.type === 'dock') return;
    selectedLink = null;
    selectedId = node.id;
  }

  function unlinkDock(which: 'client' | 'internet', nodeId: string) {
    const n = gnodes.find((x) => x.id === nodeId);
    if (!n) return;
    if (which === 'client') n.entry = false;
    else n.exit = false;
    scheduleSave();
  }

  function onNodeDragStop({ nodes }) {
    for (const node of nodes) {
      const gn = gnodes.find((n) => n.id === node.id);
      if (!gn) continue;
      gn.pos_x = node.position.x;
      gn.pos_y = node.position.y;
    }
    scheduleSave();
  }

  function onKeydown(e: KeyboardEvent) {
    if ((e.target as HTMLElement)?.closest('input, textarea, select, [contenteditable="true"]')) return;
    if (e.key === 'Escape') { selectedId = ''; selectedLink = null; }
    if (e.key === 'Delete' || e.key === 'Backspace') {
      if (selectedLink) { e.preventDefault(); deleteConnection(); }
      else if (selectedId) { e.preventDefault(); deleteSelected(); }
    }
  }

  // --- превью конфигов и деплой ---
  async function preview() {
    await doSave();
    try {
      const resp = await api<{ configs: Record<string, string> }>(`/graphs/${graphId}/configs`);
      previewConfigs = resp.configs;
      showPreview = true;
    } catch (e) {
      saveError = e instanceof Error ? e.message : String(e);
    }
  }

  async function deploy() {
    if (validation?.errors?.length) {
      saveError = 'Граф содержит ошибки валидации — исправьте их перед деплоем';
      return;
    }
    await doSave();
    deploying = true;
    deployError = '';
    deployResults = null;
    try {
      const resp = await api<{ deployed: boolean; results: DeployReport[] }>(`/graphs/${graphId}/deploy`, {
        method: 'POST',
      });
      deployResults = resp.results;
      if (!resp.deployed) {
        deployError = 'Не все ноды обновлены — смотрите отчёт';
      }
    } catch (e) {
      deployError = e instanceof Error ? e.message : String(e);
    } finally {
      deploying = false;
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="editor">
  <aside class="dock left" aria-label="Клиент">
    <div class="dock-icon">👤</div>
    <div class="dock-title">Клиент</div>
    <p>Потяните линию от точки справа к входу inbound.</p>
    <div class="dock-links">
      {#each clientNodes as n (n.id)}
        <button class="dock-link" aria-label="Отключить Клиент → {n.tag}" onclick={() => unlinkDock('client', n.id)}>{n.tag} ×</button>
      {/each}
    </div>
  </aside>

  <div class="canvas-area" bind:this={flowWrapper}>

    <SvelteFlow
      bind:viewport
      nodes={flowNodes}
      edges={flowEdges}
      {nodeTypes}
      deleteKey={null}
      connectionRadius={40}
      autoPanOnConnect={false}
      onpaneclick={() => { selectedLink = null; selectedId = ''; }}
      onconnect={onConnect}
      onnodeclick={onNodeClick}
      onnodedragstop={onNodeDragStop}
      onedgeclick={onEdgeClick}
      isValidConnection={validConnection}
    >
      <Background />
      <Controls />
      <MiniMap />
    </SvelteFlow>

    <!-- Тулбар -->
    <div class="toolbar" role="presentation" onclick={(e) => e.stopPropagation()}>
      <div class="group">
        <button class="tool" onclick={onBack}>← Графы</button>
        <button class="tool" onclick={() => addNode('inbound')} title="Вход подключения (vless/vmess/trojan/shadowsocks/hysteria2/tuic). Протокол и параметры — в панели настроек элемента">+ inbound</button>
        <button class="tool" onclick={() => addNode('outbound')} title="Выход: direct или внешний сервер вне панели (адрес и учётные данные вручную). Для каскада между физическими нодами outbound не нужен — соединяйте inbound → inbound напрямую">+ outbound</button>
        <button class="tool" onclick={() => addNode('rule')}>+ rule</button>
        <button class="tool" onclick={() => addNode('balancer')}>+ balancer</button>
      </div>
      <div class="group">
        <button class="tool" onclick={() => doValidate()}>Проверить</button>
        <button class="tool" onclick={preview}>Конфиги</button>
        <button class="tool primary" disabled={deploying} onclick={deploy}>
          {deploying ? 'Деплой…' : 'Деплой на ноды'}
        </button>
      </div>
    </div>

    <!-- Статус сохранения и ошибки -->
    <div class="statusbar">
      <span class="graph-name" title="Переименовать можно в списке графов">{graph?.name ?? ''}</span>
      {#if saving}<span class="muted">сохранение…</span>
      {:else if lastSaved}<span class="muted">сохранено {new Date(lastSaved).toLocaleTimeString()}</span>{/if}
      {#if validation}
        <button class="badge {validation.errors.length ? 'red' : 'green'}" aria-expanded={showIssues} onclick={() => (showIssues = !showIssues)}>
          {validation.errors.length} ошибок
        </button>
        <button class="badge {validation.warnings.length ? 'yellow' : 'gray'}" aria-expanded={showIssues} onclick={() => (showIssues = !showIssues)}>
          {validation.warnings.length} предупреждений ▾
        </button>
      {/if}
      {#if selectedLink}
        <button class="badge blue" onclick={deleteConnection}>Удалить связь ×</button>
      {/if}
    </div>

    {#if saveError}
      <div class="error-float" role="alert">
        {saveError}
        <button class="close-float" onclick={() => (saveError = '')}>×</button>
      </div>
    {/if}

    {#if showIssues && validation && (validation.errors.length || validation.warnings.length)}
      <div class="issues-list">
        <button class="close-float" aria-label="Закрыть список" onclick={() => (showIssues = false)}>×</button>
        {#each validation.errors as issue (issue.code + issue.element)}
          <button class="issue error" onclick={() => focusIssue(issue)}>{issue.message}</button>
        {/each}
        {#each validation.warnings as issue (issue.code + issue.element)}
          <button class="issue warning" onclick={() => focusIssue(issue)}>{issue.message}</button>
        {/each}
      </div>
    {/if}
  </div>

  <!-- Правый док: интернет -->
  <aside class="dock right" aria-label="Интернет">
    <div class="dock-icon">🌐</div>
    <div class="dock-title">Интернет</div>
    <p>Подключите выход inbound или outbound к точке слева.</p>
    <div class="dock-links">
      {#each exitNodes as n (n.id)}
        <button class="dock-link" aria-label="Отключить {n.tag} → Интернет" onclick={() => unlinkDock('internet', n.id)}>{n.tag} ×</button>
      {/each}
    </div>
  </aside>

  <!-- Панель настроек -->
  {#if selected}
    <NodeSettings
      graphNode={selected}
      {physNodes}
      {cascadeTarget}
      onChanged={scheduleSave}
      onDelete={deleteSelected}
      onClose={() => (selectedId = '')}
    />
  {/if}
</div>

<!-- Модал превью конфигов -->
{#if showPreview}
  <div class="modal" role="presentation" onclick={(e) => { if (e.target === e.currentTarget) showPreview = false; }}>
    <div class="modal-content wide">
      <div class="modal-header">
        <h2>Сгенерированные конфиги ({Object.keys(previewConfigs).length} нод)</h2>
        <button class="close" onclick={() => (showPreview = false)}>×</button>
      </div>
      {#each Object.entries(previewConfigs) as [nodeId, cfg] (nodeId)}
        {@const nodeName = physNodes.find((p) => p.id === nodeId)?.name ?? nodeId}
        <details>
          <summary>{nodeName}</summary>
          <pre>{cfg}</pre>
        </details>
      {/each}
    </div>
  </div>
{/if}

<!-- Модал отчёта деплоя -->
{#if deployResults}
  <div class="modal" role="presentation" onclick={(e) => { if (e.target === e.currentTarget) deployResults = null; }}>
    <div class="modal-content">
      <div class="modal-header">
        <h2>Отчёт деплоя</h2>
        <button class="close" onclick={() => (deployResults = null)}>×</button>
      </div>
      {#if deployError}<p class="error">{deployError}</p>{/if}
      {#each deployResults as r (r.node_id)}
        <div class="deploy-row {r.ok ? 'ok' : 'fail'}">
          <span class="badge {r.ok ? 'green' : 'red'}">{r.ok ? 'OK' : 'FAIL'}</span>
          <span>{r.name}</span>
          {#if r.error}<code class="deploy-err">{r.error}</code>{/if}
        </div>
      {/each}
    </div>
  </div>
{/if}

<style>
  .editor {
    display: flex;
    height: 100vh;
    position: relative;
    overflow: hidden;
  }

  /* Доки */
  .dock {
    width: 200px;
    min-width: 200px;
    background: #0b1220;
    border-right: 1px solid #1e293b;
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 1.5rem 0.75rem;
    z-index: 10;
  }
  .dock.right {
    border-right: none;
    border-left: 1px solid #1e293b;
  }
  .dock p { color: #64748b; font-size: 0.75rem; text-align: center; margin-top: 0.75rem; }
  .dock-icon {
    font-size: 3rem;
    user-select: none;
    padding: 1rem;
    border-radius: 0.75rem;
  }
  .dock-title {
    margin-top: 0.75rem;
    color: #94a3b8;
    font-size: 0.8125rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
  .dock-links {
    margin-top: 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    width: 100%;
    align-items: center;
  }
  .dock-link {
    background: #1e293b;
    border: 1px solid #334155;
    color: #e2e8f0;
    border-radius: 0.375rem;
    padding: 0.2rem 0.5rem;
    font-size: 0.75rem;
    cursor: pointer;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .dock-link:hover { border-color: #dc2626; }

  /* Канвас */
  .canvas-area {
    flex: 1;
    position: relative;
    min-width: 0;
  }
  :global(.svelte-flow__node-cascade) { cursor: grab; }
  /* Клик по связи выбирает её:interaction-path ловит клики всей своей шириной. */
  :global(.svelte-flow__edge path.svelte-flow__edge-interaction) { pointer-events: stroke; }
  :global(.svelte-flow__edge) { cursor: pointer; }
  :global(.svelte-flow__node-dock) { pointer-events: none; }
  :global(.svelte-flow__node-dock .svelte-flow__handle) { pointer-events: all; }

  /* Тулбар */
  .toolbar {
    position: absolute;
    top: 0.75rem;
    right: 0.75rem;
    left: auto;
    transform: none;
    display: flex;
    gap: 0.75rem;
    z-index: 20;
    background: rgba(15, 23, 42, 0.92);
    border: 1px solid #334155;
    border-radius: 0.5rem;
    padding: 0.5rem 0.75rem;
    flex-wrap: wrap;
    max-width: calc(100% - 120px);
  }
  .toolbar .group { display: flex; gap: 0.35rem; flex-wrap: wrap; }
  .tool {
    background: #1e293b;
    border: 1px solid #334155;
    color: #e2e8f0;
    font-size: 0.75rem;
    padding: 0.3rem 0.6rem;
    border-radius: 0.375rem;
    white-space: nowrap;
  }
  .tool:hover { background: #334155; }
  .tool.primary { background: #2563eb; border-color: #2563eb; }
  .tool.primary:hover { background: #1d4ed8; }
  .tool:disabled { opacity: 0.5; cursor: not-allowed; }

  /* Статусбар */
  .statusbar {
    position: absolute;
    bottom: 0.75rem;
    left: 50%;
    transform: translateX(-50%);
    display: flex;
    gap: 0.75rem;
    align-items: center;
    z-index: 20;
    background: rgba(15, 23, 42, 0.92);
    border: 1px solid #334155;
    border-radius: 0.5rem;
    padding: 0.4rem 0.9rem;
    font-size: 0.75rem;
    white-space: nowrap;
  }
  .graph-name { font-weight: 600; color: #e2e8f0; }
  .muted { color: #94a3b8; }
  .badge {
    padding: 0.1rem 0.45rem;
    border-radius: 0.25rem;
    font-weight: 700;
    font-size: 0.6875rem;
  }
  .badge.blue { background: #1d4ed8; color: #dbeafe; cursor: pointer; }
  .badge.red, .badge.green, .badge.yellow, .badge.gray { cursor: pointer; }

  /* Список диагностики */
  .issues-list {
    position: absolute;
    bottom: 3.25rem;
    left: 50%;
    transform: translateX(-50%);
    z-index: 30;
    background: #1e293b;
    border: 1px solid #334155;
    border-radius: 0.5rem;
    padding: 0.75rem;
    max-width: 560px;
    max-height: 300px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }
  .issue {
    text-align: left;
    font-size: 0.75rem;
    padding: 0.4rem 0.6rem;
    border-radius: 0.375rem;
    background: #0f172a;
  }
  .issue.error { color: #fecaca; border: 1px solid #7f1d1d; }
  .issue.warning { color: #fef08a; border: 1px solid #713f12; }
  .issue:hover { filter: brightness(1.2); }

  /* Плавающая ошибка */
  .error-float {
    position: absolute;
    top: 4rem;
    left: 50%;
    transform: translateX(-50%);
    z-index: 30;
    background: #991b1b;
    color: #fecaca;
    padding: 0.5rem 0.9rem;
    border-radius: 0.5rem;
    font-size: 0.8125rem;
    display: flex;
    gap: 0.75rem;
    align-items: center;
    max-width: 80%;
  }
  .close-float {
    background: none;
    color: #fecaca;
    font-size: 1rem;
    padding: 0;
    font-weight: 700;
  }

  /* svelte-flow тёмная тема */
  :global(.svelte-flow) { background: #0f172a; }
  :global(.svelte-flow__node) { cursor: grab; }
  :global(.svelte-flow__handle) {
    width: 10px;
    height: 10px;
    background: #64748b;
    border: 2px solid #0f172a;
  }
  :global(.svelte-flow__edge:hover .svelte-flow__edge-path) { stroke: #38bdf8; }
  :global(.svelte-flow__controls) {
    box-shadow: none;
    border: 1px solid #334155;
    border-radius: 0.375rem;
    overflow: hidden;
  }
  :global(.svelte-flow__controls-button) {
    background: #1e293b;
    border-bottom: 1px solid #334155;
  }
  :global(.svelte-flow__controls-button svg) { fill: #e2e8f0; }
  :global(.svelte-flow__minimap) {
    background: #1e293b;
    border: 1px solid #334155;
    border-radius: 0.375rem;
  }
  :global(.svelte-flow__attribution) { display: none; }

  /* Модалы */
  .modal {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.7);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }
  .modal-content {
    background: #1e293b;
    padding: 1.5rem;
    border-radius: 0.5rem;
    width: 100%;
    max-width: 640px;
    max-height: 90vh;
    overflow-y: auto;
  }
  .modal-content.wide { max-width: 860px; }
  .modal-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; }
  .modal-header h2 { font-size: 1.125rem; }
  details { margin-bottom: 0.75rem; }
  summary { cursor: pointer; font-weight: 600; padding: 0.5rem 0; }
  pre {
    background: #0f172a;
    padding: 1rem;
    border-radius: 0.375rem;
    overflow-x: auto;
    font-size: 0.8125rem;
    max-height: 400px;
    overflow-y: auto;
  }
  .deploy-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.5rem 0.75rem;
    border-radius: 0.375rem;
    margin-bottom: 0.4rem;
    background: #0f172a;
  }
  .deploy-err {
    color: #fca5a5;
    font-size: 0.75rem;
    word-break: break-all;
    flex: 1;
  }
  .error {
    color: #fecaca;
    background: #991b1b;
    padding: 0.5rem 0.75rem;
    border-radius: 0.375rem;
    margin-bottom: 1rem;
    font-size: 0.875rem;
    word-break: break-word;
  }
  .close { background: none; color: #94a3b8; font-size: 1.25rem; padding: 0; }
</style>
