<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type GraphMeta } from '$lib/api';
  import GraphEditor from './GraphEditor.svelte';

  let { onBack } = $props();

  let graphs = $state<GraphMeta[]>([]);
  let error = $state('');
  let newName = $state('');
  let creating = $state(false);
  let openId = $state<string | null>(null);

  async function load() {
    try {
      error = '';
      graphs = await api<GraphMeta[]>('/graphs');
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  onMount(load);

  async function create(e) {
    e.preventDefault();
    if (creating) return;
    if (!newName.trim()) {
      error = 'Введите имя графа';
      return;
    }
    creating = true;
    error = '';
    try {
      await api('/graphs', { method: 'POST', body: JSON.stringify({ name: newName.trim() }) });
      newName = '';
      await load();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      creating = false;
    }
  }

  async function remove(g) {
    if (!confirm(`Удалить граф «${g.name}»? Конфиги нод не изменятся.`)) return;
    try {
      await api(`/graphs/${g.id}`, { method: 'DELETE' });
      await load();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    }
  }

  async function rename(g) {
    const name = prompt('Новое имя графа', g.name);
    if (!name || name === g.name) return;
    try {
      await api(`/graphs/${g.id}`, { method: 'PUT', body: JSON.stringify({ name }) });
      await load();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    }
  }

  function fmt(ts) {
    return new Date(ts * 1000).toLocaleString();
  }
</script>

{#if openId}
  <GraphEditor graphId={openId} onBack={() => { openId = null; load(); }} />
{:else}
  <div class="container">
    <header>
      <button class="link" onclick={onBack}>← Ноды</button>
      <h1>Графы каскадов</h1>
    </header>

    {#if error}<p class="error">{error}</p>{/if}

    <form class="new-graph" onsubmit={create}>
      <input aria-label="Имя нового графа" placeholder="Имя нового графа" bind:value={newName} required />
      <button type="submit" disabled={creating}>{creating ? 'Создание…' : '+ Создать граф'}</button>
    </form>

    {#if graphs.length === 0}
      <p class="empty">
        Пока нет графов. Создайте граф и соберите каскад: клиент → inbound → outbound ⇒
        inbound следующей ноды → … → интернет.
      </p>
    {:else}
      <table>
        <thead>
          <tr>
            <th>Имя</th>
            <th>Обновлён</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each graphs as g (g.id)}
            <tr>
              <td><button class="link" onclick={() => (openId = g.id)}>{g.name}</button></td>
              <td>{fmt(g.updated_at)}</td>
              <td class="row-actions">
                <button class="link" onclick={() => rename(g)}>Переименовать</button>
                <button class="link danger" onclick={() => remove(g)}>Удалить</button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>
{/if}

<style>
  .new-graph {
    display: flex;
    gap: 0.5rem;
    margin-bottom: 1.5rem;
  }
  .new-graph input { flex: 1; }
  .empty { color: #94a3b8; }
  .row-actions { text-align: right; }
  .link.danger { color: #f87171; }
</style>
