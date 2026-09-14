<script>
  // Карточка элемента каскада на канвасе (кастомная нода svelte-flow).
  // Поток всегда слева-направо: target-порт слева, source-порт справа.
  // data: { graphNode: GraphNode, nodeName: string, issues: Issue[] }
  import { Handle, Position } from '@xyflow/svelte';

  let { data } = $props();

  const kindIcons = {
    inbound: '↓',
    outbound: '↑',
    rule: '⚙',
    balancer: '⚖',
  };

  const gn = $derived(data.graphNode);
  const errCount = $derived(data.issues?.filter((i) => i.severity === 'error').length ?? 0);
  const warnCount = $derived((data.issues?.length ?? 0) - errCount);
</script>

<div class="card kind-{gn.kind} {errCount ? 'has-error' : warnCount ? 'has-warn' : ''}"
     class:entry={gn.entry} class:exit={gn.exit}>
  <Handle type="target" position={Position.Left} id="in" />
  <div class="head">
    <span class="kind">{kindIcons[gn.kind]}</span>
    <span class="tag" title={gn.tag}>{gn.tag}</span>
    {#if errCount}<span class="flag error" title="Ошибки валидации">!{errCount}</span>
    {:else if warnCount}<span class="flag warn" title="Предупреждения">?{warnCount}</span>{/if}
  </div>
  <div class="meta">
    <span class="proto">{gn.protocol}</span>
    <span class="node" title={data.nodeName}>{data.nodeName || 'нет ноды'}</span>
  </div>
  <Handle type="source" position={Position.Right} id="out" />
</div>

<style>
  .card {
    background: #1e293b;
    border: 2px solid #334155;
    border-radius: 0.5rem;
    padding: 0.5rem 0.75rem;
    min-width: 170px;
    font-size: 0.8125rem;
    position: relative;
  }
  /* Порты карточки всегда доступны для перетаскивания линии. */
  .card :global(.svelte-flow__handle) {
    opacity: 0;
    width: 22px;
    height: 22px;
    min-width: 22px;
    min-height: 22px;
    border: none;
    background: transparent;
    cursor: crosshair;
  }
  .card :global(.svelte-flow__handle)::after {
    content: '';
    position: absolute;
    inset: 3px;
    border-radius: 50%;
    background: #38bdf8;
    border: 3px solid #0f172a;
    box-shadow: 0 0 0 2px #38bdf866;
  }
  .card :global(.svelte-flow__handle:hover)::after,
  .card :global(.svelte-flow__handle.connectingto)::after,
  .card :global(.svelte-flow__handle.connectingfrom)::after {
    background: #e0f2fe;
    box-shadow: 0 0 0 4px #38bdf899;
  }
  .card :global(.svelte-flow__handle-right) { right: -12px; }
  .card :global(.svelte-flow__handle-left) { left: -12px; }
  .card.kind-inbound { border-color: #0891b2; }
  .card.kind-outbound { border-color: #7c3aed; }
  .card.kind-rule { border-color: #ca8a04; }
  .card.kind-balancer { border-color: #db2777; }
  .card.has-error { border-color: #dc2626; box-shadow: 0 0 0 2px rgba(220, 38, 38, 0.35); }
  .card.has-warn { box-shadow: 0 0 0 2px rgba(234, 179, 8, 0.25); }
  .card.entry::before {
    content: 'КЛИЕНТ';
    position: absolute;
    top: -9px;
    left: 8px;
    background: #0891b2;
    color: #e0f2fe;
    font-size: 0.5625rem;
    font-weight: 700;
    letter-spacing: 0.05em;
    padding: 0 0.35rem;
    border-radius: 0.25rem;
  }
  .card.exit::before {
    content: 'ИНТЕРНЕТ';
    position: absolute;
    top: -9px;
    right: 8px;
    background: #7c3aed;
    color: #ede9fe;
    font-size: 0.5625rem;
    font-weight: 700;
    letter-spacing: 0.05em;
    padding: 0 0.35rem;
    border-radius: 0.25rem;
  }
  .head {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }
  .kind { font-weight: 700; }
  .tag {
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 140px;
  }
  .flag.error { color: #f87171; font-weight: 800; }
  .flag.warn { color: #facc15; font-weight: 800; }
  .meta {
    display: flex;
    justify-content: space-between;
    gap: 0.5rem;
    margin-top: 0.2rem;
    color: #94a3b8;
    font-size: 0.6875rem;
  }
  .proto { text-transform: uppercase; font-weight: 700; }
  .node { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 90px; }
</style>
