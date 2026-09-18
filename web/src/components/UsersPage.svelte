<script lang="ts">
  import { onMount } from 'svelte';
  import QRCode from 'qrcode';
  import { api, listUsers, createUser, updateUser, deleteUser, subUrl, type GraphMeta, type PanelUser } from '$lib/api';

  let { onBack } = $props();

  let users = $state<PanelUser[]>([]);
  let graphs = $state<GraphMeta[]>([]);
  let error = $state('');
  let warning = $state('');

  let showForm = $state(false);
  let editingId = $state<string | null>(null);
  let newName = $state('');
  let newGraphId = $state('');
  let newRemark = $state('');
  let newFlow = $state('');
  let newUsedUpload = $state('');
  let newUsedDownload = $state('');
  let newTotalTraffic = $state('');
  let newExpireTime = $state('');
  let creating = $state(false);

  let qrUser = $state<PanelUser | null>(null);
  let qrCanvas = $state<HTMLCanvasElement | null>(null);
  let qrUrl = $state('');
  let copied = $state(false);

  async function load() {
    try {
      error = '';
      users = await listUsers();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  onMount(async () => {
    await load();
    try {
      graphs = await api<GraphMeta[]>('/graphs');
    } catch {
      // выбор графа будет недоступен, но список пользователей покажем
    }
  });

  function openCreate() {
    editingId = null;
    newName = '';
    newGraphId = '';
    newRemark = '';
    newFlow = '';
    newUsedUpload = '';
    newUsedDownload = '';
    newTotalTraffic = '';
    newExpireTime = '';
    showForm = true;
  }

  function openEdit(u: PanelUser) {
    editingId = u.id;
    newName = u.name;
    newGraphId = u.graph_id;
    newRemark = u.remark;
    newFlow = u.flow;
    newUsedUpload = u.used_upload || '';
    newUsedDownload = u.used_download || '';
    newTotalTraffic = u.total_traffic || '';
    newExpireTime = u.expire_time || '';
    showForm = true;
  }

  async function save(e) {
    e.preventDefault();
    if (creating) return;
    if (!newName.trim() || !newGraphId) {
      error = 'Введите имя и выберите граф';
      return;
    }
    creating = true;
    error = '';
    warning = '';
    try {
      const payload = {
        name: newName.trim(),
        graph_id: newGraphId,
        remark: newRemark.trim(),
        flow: newFlow,
        used_upload: newUsedUpload ? Number(newUsedUpload) : undefined,
        used_download: newUsedDownload ? Number(newUsedDownload) : undefined,
        total_traffic: newTotalTraffic ? Number(newTotalTraffic) : undefined,
        expire_time: newExpireTime ? Number(newExpireTime) : undefined,
      };
      const resp = editingId ? await updateUser(editingId, payload) : await createUser(payload);
      if (resp.warning) warning = `Сохранено, но деплой не выполнен: ${resp.warning}`;
      else if (resp.deploy && !resp.deploy.deployed) warning = 'Сохранено, но часть нод не обновилась — проверьте статусы нод.';
      newName = '';
      newGraphId = '';
      newRemark = '';
      newFlow = '';
      showForm = false;
      editingId = null;
      await load();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      creating = false;
    }
  }

  async function toggle(u: PanelUser) {
    error = '';
    warning = '';
    try {
      const resp = await updateUser(u.id, { enabled: !u.enabled });
      if (resp.warning) warning = resp.warning;
      await load();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    }
  }

  async function remove(u: PanelUser) {
    if (!confirm(`Удалить пользователя «${u.name}»? Его подписка перестанет работать.`)) return;
    try {
      await deleteUser(u.id);
      await load();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    }
  }

  function fmt(ts: number) {
    return new Date(ts * 1000).toLocaleString();
  }

  async function copyUserUrl() {
    try {
      await navigator.clipboard.writeText(qrUrl);
      copied = true;
      setTimeout(() => (copied = false), 1500);
    } catch {
      // нет доступа к буферу — юзер скопирует вручную
    }
  }

  $effect(() => {
    if (qrUser && qrCanvas) {
      QRCode.toCanvas(qrCanvas, qrUrl, { width: 256, margin: 1, errorCorrectionLevel: 'M' });
    }
  });
</script>

<div class="container">
  <header>
    <button class="link" onclick={onBack}>← Ноды</button>
    <h1>Пользователи</h1>
    <button onclick={openCreate}>+ Добавить пользователя</button>
  </header>

  {#if error}<p class="error">{error}</p>{/if}
  {#if warning}<p class="warn">{warning}</p>{/if}

  {#if showForm}
    <form class="new-user" onsubmit={save}>
      <input aria-label="Имя пользователя" placeholder="Имя пользователя" bind:value={newName} required />
      <select aria-label="Граф" bind:value={newGraphId} required>
        <option value="" disabled>— граф —</option>
        {#each graphs as g (g.id)}
          <option value={g.id}>{g.name}</option>
        {/each}
      </select>
      <input aria-label="Информация для клиента" placeholder="Информация для клиента (remark, необязательно)" bind:value={newRemark} />
      <select aria-label="Flow" bind:value={newFlow} title="flow для vless/reality (необязательно)">
        <option value="">без flow</option>
        <option value="xtls-rprx-vision">flow: xtls-rprx-vision</option>
      </select>
      <div class="traffic-fields">
        <input type="number" placeholder="Uploaded (байт)" bind:value={newUsedUpload} title="Использовано загрузки" />
        <input type="number" placeholder="Downloaded (байт)" bind:value={newUsedDownload} title="Использовано скачивания" />
        <input type="number" placeholder="Total (байт)" bind:value={newTotalTraffic} title="Лимит трафика (0 = безлимит)" />
        <input type="number" placeholder="Expires (Unix)" bind:value={newExpireTime} title="Истечение подписки (Unix timestamp)" />
      </div>
      <button type="submit" disabled={creating}>{creating ? 'Сохранение…' : editingId ? 'Сохранить' : '+ Создать'}</button>
      <button type="button" onclick={() => (showForm = false)}>Отмена</button>
    </form>
  {/if}

  {#if users.length === 0}
    <p class="empty">
      Пока нет пользователей. Создайте пользователя — панель вшивает его учётные
      данные во все entry-inbound выбранного графа и раздаёт их через одну подписку.
    </p>
  {:else}
    <table>
      <thead>
        <tr>
          <th>Имя</th>
          <th>Граф</th>
          <th>Загр./Скач.</th>
          <th>Лимит/Истек.</th>
          <th>Подписка</th>
          <th>Статус</th>
          <th>Создан</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {#each users as u (u.id)}
          <tr>
            <td>
              {u.name}
              {#if u.remark}<div class="remark" title="Информация для клиента">{u.remark}</div>{/if}
            </td>
            <td>{u.graph_name || '—'}</td>
            <td class="num">{u.used_upload > 0 ? (u.used_upload / 1024 / 1024).toFixed(1) + ' MB / ' : ''}{(u.used_download > 0 ? (u.used_download / 1024 / 1024).toFixed(1) + ' MB' : '—')}</td>
            <td class="num">{u.total_traffic > 0 ? (u.total_traffic / 1024 / 1024).toFixed(1) + ' MB' : '∞'} / {u.expire_time > 0 ? new Date(u.expire_time * 1000).toLocaleDateString() : '—'}</td>
            <td>
              <button
                class="link"
                onclick={() => { qrUser = u; qrUrl = subUrl(u.sub_token); copied = false; }}>
                URL / QR
              </button>
            </td>
            <td>
              <button class="badge-toggle {u.enabled ? 'on' : 'off'}" onclick={() => toggle(u)} title="Включить/отключить подписку">
                {u.enabled ? 'включён' : 'отключён'}
              </button>
            </td>
            <td>{fmt(u.created_at)}</td>
            <td class="row-actions">
              <button class="link" onclick={() => openEdit(u)}>Изменить</button>
              <button class="link danger" onclick={() => remove(u)}>Удалить</button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>

{#if qrUser}
  <div class="modal-mask" role="button" tabindex="0" aria-label="Закрыть" onclick={(e) => { if (e.target === e.currentTarget) qrUser = null; }} onkeydown={(e) => { if ((e.key === 'Enter' || e.key === ' ') && e.target === e.currentTarget) qrUser = null; }}>
    <div class="modal">
      <h2>Подписка: {qrUser.name}</h2>
      <p class="hint">Одна ссылка — клиент получит все entry-inbound графа. Подписка обновляется автоматически при изменении графа.</p>
      <canvas bind:this={qrCanvas}></canvas>
      <div class="sub-line">
        <input readonly value={qrUrl} onclick={(e) => (e.target as HTMLInputElement).select()} />
        <button onclick={copyUserUrl}>{copied ? 'Скопировано ✓' : 'Копировать'}</button>
      </div>
      <button onclick={() => (qrUser = null)}>Закрыть</button>
    </div>
  </div>
{/if}

<style>
  .num { font-family: monospace; font-size: 0.75rem; white-space: nowrap; }
  .traffic-fields { display: flex; gap: 0.5rem; margin: 0.5rem 0; flex-wrap: wrap; }
  .traffic-fields input { flex: 1; min-width: 120px; }
  header { display: flex; align-items: center; gap: 1rem; }
  header h1 { flex: 1; }
  .new-user {
    display: flex;
    gap: 0.5rem;
    margin-bottom: 1.5rem;
    flex-wrap: wrap;
  }
  .new-user input, .new-user select { flex: 1; min-width: 10rem; padding: 0.4rem 0.5rem; }
  .remark { color: #64748b; font-size: 0.8125rem; margin-top: 0.15rem; }
  .empty { color: #94a3b8; }
  .warn { color: #fbbf24; }
  .row-actions { text-align: right; }
  .link.danger { color: #f87171; }
  .badge-toggle {
    background: none;
    border-radius: 9999px;
    padding: 0.2rem 0.6rem;
    font-size: 0.8125rem;
  }
  .badge-toggle.on { color: #4ade80; border: 1px solid #166534; }
  .badge-toggle.off { color: #94a3b8; border: 1px solid #334155; }
  .modal-mask {
    position: fixed;
    inset: 0;
    background: rgba(2, 6, 23, 0.7);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 50;
  }
  .modal {
    background: #0f172a;
    border: 1px solid #1e293b;
    border-radius: 0.75rem;
    padding: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    align-items: center;
    max-width: 90vw;
  }
  .modal canvas { background: #fff; border-radius: 0.5rem; }
  .sub-line { display: flex; gap: 0.5rem; width: 100%; }
  .sub-line input { flex: 1; font-family: monospace; font-size: 0.8125rem; }
  .hint { color: #64748b; font-size: 0.8125rem; }
</style>