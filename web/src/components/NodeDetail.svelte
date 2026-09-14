<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { createEventDispatcher } from 'svelte';
  import StatusBadge from './StatusBadge.svelte';

  export let id;
  const dispatch = createEventDispatcher();

  let node = null;
  let status = null;
  let config = '';
  let error = '';
  let notice = '';
  let edit = { name: '', grpc_url: '', token: '', cert_pem: '' };
  let showEdit = false;

  function startEdit() {
    edit = {
      name: node?.name || '',
      grpc_url: node?.grpc_url || '',
      token: node?.token || '',
      cert_pem: node?.cert_pem || '',
    };
    showEdit = true;
  }

  onMount(async () => {
    try {
      node = await api(`/nodes/${id}`);
      config = node.config_json || '';
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  });

  async function refreshStatus() {
    error = notice = '';
    status = null;
    try {
      status = await api(`/nodes/${id}/status`);
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function saveConnection() {
    error = notice = '';
    try {
      await api(`/nodes/${id}`, {
        method: 'PUT',
        body: JSON.stringify({ ...node, ...edit }),
      });
      node = await api(`/nodes/${id}`);
      showEdit = false;
      status = null;
      notice = 'Saved';
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function pushConfig() {
    error = notice = '';
    try {
      await api(`/nodes/${id}/push`, {
        method: 'POST',
        body: JSON.stringify({ config_json: config }),
      });
      node = await api(`/nodes/${id}`);
      notice = 'Config pushed';
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  function fmtUptime(sec) {
    if (!sec || sec < 0) return '-';
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = Math.floor(sec % 60);
    return h > 0 ? `${h}h ${m}m` : m > 0 ? `${m}m ${s}s` : `${s}s`;
  }
</script>

<div class="container">
  <header>
    <button class="link" on:click={() => dispatch('back')}>← Back</button>
    <h1>{node?.name || id}</h1>
    <button class="link" on:click={startEdit}>Edit</button>
  </header>

  {#if error}<p class="error">{error}</p>{/if}
  {#if notice}<p class="notice">{notice}</p>{/if}

  {#if showEdit}
    <section>
      <h2>Connection</h2>
      <div class="field">
        <label for="ed-name">Name</label>
        <input id="ed-name" bind:value={edit.name} />
      </div>
      <div class="field">
        <label for="ed-url">gRPC URL</label>
        <input id="ed-url" bind:value={edit.grpc_url} placeholder="node1:6237" />
      </div>
      <div class="field">
        <label for="ed-token">Node API token</label>
        <input id="ed-token" type="password" bind:value={edit.token} />
      </div>
      <div class="field">
        <label for="ed-cert">Node certificate (PEM) — leave empty for insecure connection</label>
        <textarea id="ed-cert" rows="8" bind:value={edit.cert_pem}></textarea>
      </div>
      <button on:click={saveConnection}>Save</button>
      <button class="link" on:click={() => (showEdit = false)}>Cancel</button>
    </section>
  {/if}

  <section>
    <h2>Status</h2>
    <button on:click={refreshStatus}>Refresh</button>
    {#if status}
      <div class="status-grid">
        <StatusBadge status={status} />
        <div>sing-box <strong>{status.singbox_version || '—'}</strong></div>
        <div>uptime <strong>{fmtUptime(status.uptime_seconds)}</strong></div>
        <div>inbounds <strong>{status.inbounds}</strong></div>
        <div>outbounds <strong>{status.outbounds}</strong></div>
      </div>
    {:else}
      <p>Click Refresh to fetch status</p>
    {/if}
  </section>

  <section>
    <h2>Config</h2>
    <textarea rows="12" bind:value={config}></textarea>
    <button on:click={pushConfig}>Push Config</button>
  </section>
</div>
