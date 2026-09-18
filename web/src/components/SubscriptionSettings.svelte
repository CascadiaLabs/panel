<script lang="ts">
  // Панель настройки метаданных подписки и клиентской маршрутизации.
  import { onMount } from 'svelte';
  import { api, subscriptionInfo } from '$lib/api';
  import type { SubscriptionResponse } from '$lib/api';
  
  let {
    graphId,
    graphName,
    onChanged,
  } = $props();

  let info: SubscriptionResponse | null = null;
  let busy = $state(false);
  let error = $state('');
  let notice = $state('');

  // Локальные редактируемые значения
  let name = $state('');
  let desc = $state('');
  let site = $state('');
  let support = $state('');

  onMount(async () => {
    try {
      error = '';
      info = await subscriptionInfo(graphId);
      name = info.name ?? '';
      desc = info.desc ?? '';
      site = info.site ?? '';
      support = info.support ?? '';
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  });

  async function load() {
    try {
      error = '';
      info = await subscriptionInfo(graphId);
      name = info.name ?? '';
      desc = info.desc ?? '';
      site = info.site ?? '';
      support = info.support ?? '';
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function save() {
    busy = true;
    error = '';
    notice = '';
    try {
      // Отправляем только те поля, которые изменились, через PUT /graphs/{id}
      const body: Record<string, string> = {};
      if (name !== (info?.name ?? '')) body.subscription_name = name;
      if (desc !== (info?.desc ?? '')) body.subscription_desc = desc;
      if (site !== (info?.site ?? '')) body.subscription_site = site;
      if (support !== (info?.support ?? '')) body.subscription_support = support;
      if (Object.keys(body).length > 0) {
        await api(`/graphs/${graphId}`, {
          method: 'PUT',
          body: JSON.stringify(body),
        });
        info = { ...(info ?? {} as SubscriptionResponse), ...body, name, desc, site, support, subscription_id: info?.subscription_id ?? graphId, links: info?.links ?? [] };
        notice = 'Сохранено';
        onChanged?.();
      }
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }
</script>

<aside class="panel">
  <header>
    <h2>Подписка: {graphName}</h2>
  </header>

  {#if error}<p class="error">{error}</p>{/if}
  {#if notice}<p class="notice">{notice}</p>{/if}

  <div class="body">
    <div class="field">
      <label for="sub-name">Название подписки</label>
      <input id="sub-name" bind:value={name} placeholder="Моя подписка" />
      <p class="hint">Отображается в клиенте (v2ray/happ и др.).</p>
    </div>

    <div class="field">
      <label for="sub-desc">Описание</label>
      <textarea id="sub-desc" rows="3" bind:value={desc} placeholder="Описание сервиса"></textarea>
    </div>

    <div class="field">
      <label for="sub-site">Сайт / Канал</label>
      <input id="sub-site" bind:value={site} placeholder="https://example.com" />
      <p class="hint">Ссылка на сайт или канал подписки.</p>
    </div>

    <div class="field">
      <label for="sub-support">Поддержка</label>
      <input id="sub-support" bind:value={support} placeholder="https://t.me/support" />
      <p class="hint">Ссылка на поддержку или канал помощи.</p>
    </div>


    <button onclick={save} disabled={busy}>
      {busy ? 'Сохранение…' : 'Сохранить настройки подписки'}
    </button>
  </div>
</aside>

<style>
  .panel {
    width: 320px;
    background: #1e293b;
    border-left: 1px solid #334155;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid #334155;
  }
  h2 { font-size: 0.9375rem; word-break: break-all; }
  .body {
    padding: 1rem;
    overflow-y: auto;
    flex: 1;
  }
  .field {
    margin-bottom: 1rem;
  }
  label {
    display: block;
    font-size: 0.8125rem;
    color: #94a3b8;
    margin-bottom: 0.25rem;
  }
  input, textarea {
    width: 100%;
    background: #0f172a;
    border: 1px solid #334155;
    border-radius: 0.375rem;
    color: #e2e8f0;
    padding: 0.5rem;
    font-size: 0.875rem;
    box-sizing: border-box;
  }
  input:focus, textarea:focus {
    outline: none;
    border-color: #38bdf8;
  }
  textarea {
    resize: vertical;
    font-family: monospace;
  }
  .hint {
    color: #64748b;
    font-size: 0.7rem;
    margin-top: 0.25rem;
  }
  .error { color: #f87171; font-size: 0.8125rem; }
  .notice { color: #34d399; font-size: 0.8125rem; }
  button {
    width: 100%;
    background: #2563eb;
    color: #e2e8f0;
    border: none;
    border-radius: 0.375rem;
    padding: 0.6rem;
    font-size: 0.875rem;
    cursor: pointer;
  }
  button:hover:not(:disabled) { background: #1d4ed8; }
  button:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
