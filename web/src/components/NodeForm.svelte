<script>
  import { createEventDispatcher } from 'svelte';
  import { api } from '$lib/api';

  const dispatch = createEventDispatcher();

  let name = '';
  let grpc_url = '';
  let token = '';
  let cert_pem = '';
  let config_json = '';
  let error = '';

  async function submit() {
    try {
      await api('/nodes', {
        method: 'POST',
        body: JSON.stringify({ name, grpc_url, token, cert_pem, config_json }),
      });
      dispatch('created');
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }
</script>

<div class="modal">
  <div class="modal-content">
    <div class="modal-header">
      <h2>Add Node</h2>
      <button class="close" on:click={() => dispatch('close')}>×</button>
    </div>
    {#if error}<p class="error">{error}</p>{/if}
    <form on:submit|preventDefault={submit}>
      <div class="field">
        <label for="node-name">Name</label>
        <input id="node-name" bind:value={name} required />
      </div>
      <div class="field">
        <label for="node-url">gRPC URL</label>
        <input id="node-url" bind:value={grpc_url} placeholder="192.168.1.10:6237" required />
      </div>
      <div class="field">
        <label for="node-token">Token</label>
        <input id="node-token" type="password" bind:value={token} required />
      </div>
      <div class="field">
        <label for="node-cert">Node certificate (PEM); empty only for plaintext local testing</label>
        <textarea id="node-cert" rows="4" bind:value={cert_pem}></textarea>
      </div>
      <div class="field">
        <label for="node-config">Config JSON (optional)</label>
        <textarea id="node-config" rows="6" bind:value={config_json}></textarea>
      </div>
      <button type="submit">Save</button>
    </form>
  </div>
</div>
