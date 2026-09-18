<script lang="ts">
  // Визуальный редактор client_route для SubscriptionSettings.
  // Поддерживает оба формата: объект {rules: [...]} и массив [...].

  let { value = '', disabled = false } = $props();

  let jsonText = $state(value);
  let useJson = $state(false);
  let error = $state('');

  interface Rule {
    action: string;
    outbounds: string;
    domain: string;
    ip: string;
    final: boolean;
  }

  let rules = $state<Rule[]>(parseRules(value));

  // Пересинхронизируем rules когда value меняется снаружи
  $effect(() => {
    if (!useJson) {
      rules = parseRules(value);
    }
  });

  function parseRules(raw: string): Rule[] {
    if (!raw.trim()) return [];
    try {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) return parsed.map(normalizeRule);
      if (parsed.rules && Array.isArray(parsed.rules)) {
        return parsed.rules.map(normalizeRule);
      }
      return [];
    } catch {
      return [];
    }
  }

  function normalizeRule(r: any): Rule {
    return {
      action: r.action || 'route',
      outbounds: Array.isArray(r.outbounds) ? r.outbounds.join('\n') : '',
      domain: Array.isArray(r.domain) ? r.domain.join('\n') : '',
      ip: Array.isArray(r.ip) ? r.ip.join('\n') : '',
      final: r.final || false,
    };
  }

  function serializeRules(): string {
    const ruleObjs = rules.map((r) => {
      const obj: Record<string, any> = { action: r.action };
      if (r.outbounds.trim()) obj.outbounds = r.outbounds.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.domain.trim()) obj.domain = r.domain.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.ip.trim()) obj.ip = r.ip.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.final) obj.final = true;
      return obj;
    });
    return JSON.stringify({ rules: ruleObjs }, null, 2);
  }

  function syncValue() {
    value = serializeRules();
    jsonText = value;
  }

  function addRule() {
    rules = [...rules, { action: 'route', outbounds: '', domain: '', ip: '', final: false }];
    syncValue();
  }

  function removeRule(i: number) {
    rules = rules.filter((_, idx) => idx !== i);
    syncValue();
  }

  function updateRule(i: number, field: keyof Rule, val: string | boolean) {
    const newRules = rules.map((r, idx) => idx === i ? { ...r, [field]: val } : r);
    rules = newRules;
    syncValue();
  }

  function switchToJson() {
    useJson = true;
    jsonText = value;
    error = '';
  }

  function switchToVisual() {
    try {
      const parsed = JSON.parse(jsonText);
      value = JSON.stringify(parsed, null, 2);
      rules = parseRules(value);
      useJson = false;
      error = '';
    } catch {
      error = 'Неверный JSON';
    }
  }

  function onRuleAction(i: number, e: Event) {
    const target = e.target as HTMLSelectElement;
    updateRule(i, 'action', target.value);
  }

  function onRuleFieldChange(i: number, field: string, e: Event) {
    const target = e.target as HTMLTextAreaElement;
    updateRule(i, field as keyof Rule, target.value);
  }

  function onFinalChange(i: number, e: Event) {
    const target = e.target as HTMLInputElement;
    updateRule(i, 'final', target.checked);
  }
</script>

<div class="route-editor">
  <div class="toolbar">
    <span class="preview-hint">{rules.length} rule(s)</span>
    <div class="toolbar-actions">
      {#if !useJson}
        <button type="button" onclick={addRule} disabled={disabled}>+ Add rule</button>
        <button type="button" onclick={switchToJson} class="secondary">Edit JSON</button>
      {:else}
        <button type="button" onclick={switchToVisual}>Visual mode</button>
      {/if}
    </div>
  </div>

  {#if error}<p class="error">{error}</p>{/if}

  {#if !useJson}
    {#each rules as rule, i (i)}
      <div class="rule-block">
        <div class="rule-header">
          <span class="rule-number">#{i + 1}</span>
          <select value={rule.action} onchange={(e) => onRuleAction(i, e)} disabled={disabled}>
            <option value="route">route</option>
            <option value="selector">selector</option>
            <option value="block">block</option>
          </select>
          <label class="final-checkbox">
            <input type="checkbox" checked={rule.final} onchange={(e) => onFinalChange(i, e)} disabled={disabled} />
            final
          </label>
          <button type="button" class="remove" onclick={() => removeRule(i)} disabled={disabled}>✕</button>
        </div>
        <div class="rule-fields">
          <div class="field">
            <label>Outbounds</label>
            <textarea rows="2" value={rule.outbounds} oninput={(e) => onRuleFieldChange(i, 'outbounds', e)} placeholder="direct\nproxy" disabled={disabled}></textarea>
          </div>
          <div class="field">
            <label>Domain (geosite-*)</label>
            <textarea rows="2" value={rule.domain} oninput={(e) => onRuleFieldChange(i, 'domain', e)} placeholder="geosite:ru\ngeosite:google" disabled={disabled}></textarea>
          </div>
          <div class="field">
            <label>IP (geoip-*)</label>
            <textarea rows="2" value={rule.ip} oninput={(e) => onRuleFieldChange(i, 'ip', e)} placeholder="geoip:ru\ngeoip:private" disabled={disabled}></textarea>
          </div>
        </div>
      </div>
    {/each}

    {#if rules.length === 0}
      <p class="empty">No rules. Add a rule or switch to JSON mode.</p>
    {/if}
  {:else}
    <textarea rows="12" value={jsonText} oninput={(e) => { jsonText = e.target.value; }} placeholder="route JSON here" disabled={disabled}></textarea>
  {/if}

  {#if !useJson}
    <pre class="json-preview">{serializeRules()}</pre>
  {/if}
</div>

<style>
  .route-editor {
    border: 1px solid #334155;
    border-radius: 0.5rem;
    background: #0f172a;
    overflow: hidden;
  }
  .toolbar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.5rem 0.75rem;
    background: #1e293b;
    border-bottom: 1px solid #334155;
  }
  .preview-hint { color: #64748b; font-size: 0.75rem; }
  .toolbar-actions { display: flex; gap: 0.5rem; }
  .toolbar-actions button {
    background: #334155; color: #e2e8f0; border: none; border-radius: 0.25rem;
    padding: 0.25rem 0.5rem; font-size: 0.75rem; cursor: pointer;
  }
  .toolbar-actions button:hover:not(:disabled) { background: #475569; }
  .toolbar-actions button.secondary { background: #1d4ed8; }
  .toolbar-actions button.secondary:hover:not(:disabled) { background: #2563eb; }
  .toolbar-actions button:disabled { opacity: 0.5; cursor: not-allowed; }

  .rule-block {
    border: 1px solid #334155;
    border-radius: 0.375rem;
    margin: 0.5rem;
    background: #1e293b;
  }
  .rule-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem;
    border-bottom: 1px solid #334155;
    background: #0f172a;
  }
  .rule-number { color: #64748b; font-size: 0.75rem; font-weight: 600; }
  .rule-header select {
    background: #1e293b; color: #e2e8f0; border: 1px solid #334155;
    border-radius: 0.25rem; padding: 0.2rem; font-size: 0.75rem;
  }
  .final-checkbox {
    color: #94a3b8; font-size: 0.75rem; display: flex; align-items: center; gap: 0.25rem;
    margin-left: auto;
  }
  .remove {
    background: #7f1d1d; color: #fca5a5; border: none; border-radius: 0.25rem;
    padding: 0.15rem 0.4rem; font-size: 0.75rem; cursor: pointer;
  }
  .remove:hover:not(:disabled) { background: #991b1b; }
  .remove:disabled { opacity: 0.5; cursor: not-allowed; }

  .rule-fields {
    padding: 0.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .field label {
    display: block; font-size: 0.7rem; color: #94a3b8; margin-bottom: 0.15rem;
  }
  .field textarea {
    width: 100%; background: #0f172a; border: 1px solid #334155;
    border-radius: 0.25rem; color: #e2e8f0; padding: 0.4rem;
    font-family: monospace; font-size: 0.75rem; resize: vertical;
    box-sizing: border-box;
  }
  .field textarea:focus { outline: none; border-color: #38bdf8; }
  .field textarea:disabled { opacity: 0.6; }

  .empty {
    color: #64748b; font-size: 0.8125rem; padding: 1rem; text-align: center;
  }

  .json-preview {
    margin: 0.5rem; padding: 0.75rem; background: #0f172a;
    border: 1px solid #334155; border-radius: 0.375rem;
    font-family: monospace; font-size: 0.7rem; color: #94a3b8;
    white-space: pre-wrap; word-break: break-all;
    max-height: 200px; overflow-y: auto;
  }

  .error { color: #f87171; font-size: 0.8125rem; margin: 0.5rem; }
</style>
