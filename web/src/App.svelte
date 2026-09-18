<script lang="ts">
  import { onMount } from 'svelte';
  import { createNodesStore } from '$lib/stores.svelte';
  import { authEvents, me, logout, type Me } from '$lib/api';
  import Login from '$components/Login.svelte';
  import PasswordChange from '$components/PasswordChange.svelte';
  import NodeForm from '$components/NodeForm.svelte';
  import NodeDetail from '$components/NodeDetail.svelte';
  import GraphsPage from '$components/GraphsPage.svelte';
  import UsersPage from '$components/UsersPage.svelte';
  import RouteRulesPage from '$components/RouteRulesPage.svelte';

  const store = createNodesStore();

  let authState = $state<'checking' | 'login' | 'ok'>('checking');
  let user = $state<Me | null>(null);
  let view = $state<'nodes' | 'graphs' | 'users' | 'routes' | 'detail'>('nodes');
  let selectedId = $state('');
  let showForm = $state(false);
  let showPassword = $state(false);
  let error = $state('');

  async function load() {
    try {
      error = '';
      await store.load();
      await refreshStatuses();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function refreshStatuses() {
    try {
      await store.loadStatuses();
    } catch {
      // статус — не критичен: список нод остаётся рабочим
    }
  }

  onMount(async () => {
    authEvents.addEventListener('unauthorized', () => {
      authState = 'login';
    });
    try {
      user = await me();
      authState = 'ok';
      await load();
    } catch {
      authState = 'login';
    }
    const timer = setInterval(refreshStatuses, 15000);
    return () => clearInterval(timer);
  });

  function switchView(v) {
    view = v;
    if (v === 'nodes') load();
  }

  async function doLogout() {
    try {
      await logout();
    } catch {
      // cookie всё равно чистится на сервере по максимуму
    }
    window.location.reload();
  }

  // rendering helpers
  function badge(nodeId) {
    const s = store.statuses[nodeId];
    if (!s) return { cls: 'gray', text: '…', title: '' };
    if (s.error) return { cls: 'gray', text: 'unreachable', title: s.error };
    return s.status?.running
      ? { cls: 'green', text: 'running', title: '' }
      : { cls: 'red', text: 'stopped', title: '' };
  }
</script>

{#if authState === 'checking'}
  <div class="checking">Загрузка…</div>
{:else if authState === 'login'}
  <Login />
{:else if view === 'detail' && selectedId}
  <NodeDetail id={selectedId} on:back={() => switchView('nodes')} />
{:else if view === 'routes'}
  <RouteRulesPage onBack={() => switchView('nodes')} />
{:else if view === 'graphs'}
  <GraphsPage onBack={() => switchView('nodes')} />
{:else if view === 'users'}
  <UsersPage onBack={() => switchView('nodes')} />
{:else}
  <div class="container">
    <header>
      <div class="brand">
        <h1>Cascadia Panel</h1>
        <nav>
          <button class="nav-btn {view === 'nodes' ? 'active' : ''}" onclick={() => switchView('nodes')}>Ноды</button>
          <button class="nav-btn" onclick={() => switchView('graphs')}>Графы</button>
          <button class="nav-btn {view === 'routes' ? 'active' : ''}" onclick={() => switchView('routes')}>Маршрутизация</button>
          <button class="nav-btn {view === 'users' ? 'active' : ''}" onclick={() => switchView('users')}>Пользователи</button>
        </nav>
      </div>
      <div class="actions">
        <span class="whoami" title={user?.user?.username}>{user?.user?.username}</span>
        <button onclick={() => (showPassword = true)}>Смена пароля</button>
        <button class="danger" onclick={doLogout}>Выйти</button>
        <button onclick={() => (showForm = true)}>+ Добавить ноду</button>
      </div>
    </header>

    {#if error}<p class="error">{error}</p>{/if}

    {#if showForm}
      <NodeForm on:created={() => { showForm = false; load(); }} on:close={() => (showForm = false)} />
    {/if}

    <table>
      <thead>
        <tr>
          <th>Имя</th>
          <th>gRPC URL</th>
          <th>Статус</th>
          <th>Inbounds</th>
          <th>Outbounds</th>
          <th>Обновлена</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {#each store.nodes as node (node.id)}
          {@const b = badge(node.id)}
          {@const s = store.statuses[node.id]?.status}
          <tr>
            <td><button class="link" onclick={() => { selectedId = node.id; view = 'detail'; }}>{node.name}</button></td>
            <td style="font-family:monospace; font-size:0.875rem;">{node.grpc_url}</td>
            <td><span class="badge {b.cls}" title={b.title}>{b.text}</span></td>
            <td>{s ? s.inbounds : '-'}</td>
            <td>{s ? s.outbounds : '-'}</td>
            <td>{new Date(node.updated_at * 1000).toLocaleString()}</td>
            <td><button class="link" onclick={() => { selectedId = node.id; view = 'detail'; }}>Управление</button></td>
          </tr>
        {/each}
      </tbody>
    </table>

    <p class="hint">
      Статусы обновляются каждые 15 секунд. Графовый редактор каскадов — во вкладке «Графы».
    </p>
  </div>

  {#if showPassword}
    <PasswordChange onclose={() => (showPassword = false)} />
  {/if}
{/if}

<style>
  .checking {
    position: fixed;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    color: #94a3b8;
  }
  .brand { display: flex; align-items: center; gap: 1.5rem; }
  nav { display: flex; gap: 0.25rem; }
  .nav-btn {
    background: none;
    color: #94a3b8;
    padding: 0.375rem 0.75rem;
    border-radius: 0.375rem;
  }
  .nav-btn:hover { background: #1e293b; color: #e2e8f0; }
  .nav-btn.active { background: #1e293b; color: #38bdf8; font-weight: 600; }
  .whoami { color: #94a3b8; font-size: 0.875rem; }
  .hint { color: #64748b; font-size: 0.8125rem; margin-top: 1rem; }
</style>
