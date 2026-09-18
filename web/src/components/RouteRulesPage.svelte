<script lang="ts">
  // Страница управления маршрутизацией — отдельная в навигации как "Графы" и "Пользователи".
  import { onMount } from 'svelte';
  import { api, listRouteRules, createRouteRule, updateRouteRule, deleteRouteRule, assignInboundRoute, type RouteRule } from '$lib/api';
  import RouteRuleEditor from './RouteRuleEditor.svelte';

  let { onBack } = $props();

  let rules: RouteRule[] = $state([]);
  let error = $state('');
  let notice = $state('');

  // Создание нового правила
  let newName = $state('');
  let newRuleEditorValue = $state('');
  let creating = $state(false);

  // Назначение на inbound (multi-select)
  let selectedRuleIds = $state<string[]>([]);
  let selectedInboundId = $state('');

  onMount(async () => {
    await load();
  });

  async function load() {
    try {
      error = '';
      rules = await listRouteRules('');
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function handleCreate() {
    if (!newName.trim()) return;
    creating = true;
    error = '';
    notice = '';
    try {
      await createRouteRule('', { name: newName.trim(), rules_json: newRuleEditorValue });
      newName = '';
      newRuleEditorValue = '';
      notice = 'Создано';
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      creating = false;
    }
  }

  async function handleUpdate(rule: RouteRule) {
    try {
      await updateRouteRule(rule.id, { name: rule.name, rules_json: rule.rules_json });
      notice = 'Сохранено';
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function handleDelete(rule: RouteRule) {
    if (!confirm(`Удалить правило «${rule.name}»?`)) return;
    try {
      await deleteRouteRule(rule.id);
      notice = 'Удалено';
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function handleAssign(inboundId: string) {
    if (selectedRuleIds.length === 0 || !inboundId) return;
    try {
      for (const ruleId of selectedRuleIds) {
        await assignInboundRoute(inboundId, ruleId);
      }
      notice = `Назначено ${selectedRuleIds.length} правило(л)`;
      selectedRuleIds = [];
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  function toggleRule(ruleId: string) {
    if (selectedRuleIds.includes(ruleId)) {
      selectedRuleIds = selectedRuleIds.filter((id) => id !== ruleId);
    } else {
      selectedRuleIds = [...selectedRuleIds, ruleId];
    }
  }
</script>

<div class="route-rules-page">
  <header>
    <div class="header-row">
      <button class="back-btn" onclick={() => onBack?.()}>← Назад</button>
      <h1>Маршрутизация</h1>
    </div>
  </header>

  {#if error}<p class="error">{error}</p>{/if}
  {#if notice}<p class="notice">{notice}</p>{/if}

  <!-- Создание нового правила -->
  <section class="section">
    <h2>Новое правило</h2>
    <div class="field">
      <input bind:value={newName} placeholder="Название правила" />
    </div>
    <RouteRuleEditor bind:value={newRuleEditorValue} />
    <button onclick={handleCreate} disabled={creating || !newName.trim()}>
      {creating ? 'Создание…' : '+ Создать правило'}
    </button>
  </section>

  <!-- Список правил -->
  <section class="section">
    <h2>Правила ({rules.length})</h2>
    {#if rules.length === 0}
      <p class="empty">Нет правил. Создайте первое правило выше.</p>
    {:else}
      {#each rules as rule}
        <div class="rule-card">
          <div class="rule-header">
            <input class="rule-name" bind:value={rule.name} onchange={() => handleUpdate(rule)} disabled={rule.is_default} />
            {#if rule.is_default}<span class="default-badge">Дефолтное</span>{/if}
            {#if !rule.is_default}<button class="remove" onclick={() => handleDelete(rule)}>✕</button>{/if}
          </div>
          <RouteRuleEditor bind:value={rule.rules_json} disabled={rule.is_default} />
        </div>
      {/each}
    {/if}
  </section>

  <!-- Назначение на inbound -->
  <section class="section">
    <h2>Назначить на inbound</h2>
    <div class="field">
      <select multiple rows="5" bind:value={selectedRuleIds}>
        {#each rules as rule}
          <option value={rule.id}>{rule.name}</option>
        {/each}
      </select>
      <p class="hint">Ctrl+Click для выбора нескольких правил</p>
    </div>
    <div class="field">
      <input bind:value={selectedInboundId} placeholder="Inbound tag (например: 'node1-inbound')" />
    </div>
    <button onclick={() => handleAssign(selectedInboundId)} disabled={selectedRuleIds.length === 0 || !selectedInboundId}>
      Назначить {selectedRuleIds.length > 0 ? `(${selectedRuleIds.length})` : ''}
    </button>
  </section>
</div>

<style>
  .route-rules-page {
    width: 100%;
    background: #0f172a;
    color: #e2e8f0;
    padding: 1.5rem;
    overflow-y: auto;
  }
  header {
    margin-bottom: 1.5rem;
  }
  h1 {
    font-size: 1.5rem;
    color: #f1f5f9;
  }
  h2 {
    font-size: 1rem;
    color: #94a3b8;
    margin: 0 0 0.75rem 0;
    border-bottom: 1px solid #334155;
    padding-bottom: 0.5rem;
  }
  .section {
    margin-bottom: 2rem;
    background: #1e293b;
    border: 1px solid #334155;
    border-radius: 0.5rem;
    padding: 1rem;
  }
  .field {
    margin-bottom: 0.75rem;
  }
  input, select {
    width: 100%;
    background: #0f172a;
    border: 1px solid #334155;
    border-radius: 0.375rem;
    color: #e2e8f0;
    padding: 0.5rem;
    font-size: 0.875rem;
    box-sizing: border-box;
  }
  input:focus, select:focus {
    outline: none;
    border-color: #38bdf8;
  }
  button {
    width: 100%;
    background: #2563eb;
    color: #e2e8f0;
    border: none;
    border-radius: 0.375rem;
    padding: 0.5rem;
    font-size: 0.875rem;
    cursor: pointer;
    margin-top: 0.5rem;
  }
  button:hover:not(:disabled) { background: #1d4ed8; }
  button:disabled { opacity: 0.5; cursor: not-allowed; }
  .rule-card {
    background: #0f172a;
    border: 1px solid #334155;
    border-radius: 0.375rem;
    margin-bottom: 0.75rem;
    padding: 0.75rem;
  }
  .rule-header {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    margin-bottom: 0.5rem;
  }
  .rule-name {
    flex: 1;
    background: #1e293b;
    border: 1px solid #334155;
    color: #e2e8f0;
    padding: 0.25rem;
    font-size: 0.875rem;
    border-radius: 0.25rem;
  }
  .rule-name:disabled { opacity: 0.6; }
  .default-badge {
    background: #1d4ed8;
    color: #a5f3fc;
    font-size: 0.7rem;
    padding: 0.15rem 0.5rem;
    border-radius: 0.25rem;
    font-weight: 600;
  }
  .remove {
    background: #7f1d1d;
    color: #fca5a5;
    border: none;
    border-radius: 0.25rem;
    padding: 0.2rem 0.5rem;
    cursor: pointer;
    font-size: 0.75rem;
    width: auto;
  }
  .remove:hover { background: #991b1b; }
  .header-row {
    display: flex;
    align-items: center;
    gap: 1rem;
  }
  .back-btn {
    background: #334155;
    color: #e2e8f0;
    border: none;
    border-radius: 0.375rem;
    padding: 0.5rem 1rem;
    cursor: pointer;
    font-size: 0.875rem;
    width: auto;
  }
  .back-btn:hover { background: #475569; }
  .error { color: #f87171; font-size: 0.875rem; }
  .notice { color: #34d399; font-size: 0.875rem; }
  .empty { color: #64748b; font-size: 0.875rem; text-align: center; padding: 1rem; }
  .hint { color: #64748b; font-size: 0.75rem; margin-top: 0.25rem; }
</style>
