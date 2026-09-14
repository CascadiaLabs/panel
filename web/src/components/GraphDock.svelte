<script lang="ts">
  import { Handle, Position } from '@xyflow/svelte';
  let { data } = $props();
</script>

<div class="dock-anchor">
  {#if data.kind === 'client'}
    <Handle type="source" position={Position.Right} id="out" aria-label="Порт Клиента" title="Потяните линию к входу inbound" />
  {:else}
    <Handle type="target" position={Position.Left} id="in" aria-label="Порт Интернета" title="Подключите выход inbound или outbound" />
  {/if}
</div>

<style>
  .dock-anchor { width: 24px; height: 24px; }
  .dock-anchor :global(.svelte-flow__handle) {
    width: 22px; height: 22px; background: transparent; border: none; cursor: crosshair;
  }
  .dock-anchor :global(.svelte-flow__handle)::after {
    content: ''; position: absolute; inset: 0; border-radius: 50%;
    background: #38bdf8; border: 3px solid #e0f2fe; box-shadow: 0 0 0 5px #38bdf833;
  }
</style>
