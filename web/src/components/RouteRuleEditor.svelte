<script lang="ts">
  // Визуальный редактор массива route rules (без обёртки {rules: [...]}).
  // Поддерживает оба формата: визуальный и JSON.
  // Полные поля sing-box route rule: https://sing-box.sagernet.org/configuration/route/rule/

  let { value = '', disabled = false } = $props();

  let jsonText = $state(value);
  let useJson = $state(false);
  let error = $state('');

  interface Rule {
    action: string;
    outbounds: string;
    domain: string;
    domain_suffix: string;
    domain_keyword: string;
    domain_regex: string;
    ip_cidr: string;
    source_ip_cidr: string;
    port: string;
    source_port: string;
    network: string;
    protocol: string;
    process: string;
    process_path: string;
    package_name: string;
    uid: string;
    gid: string;
    network_type: string;
    inbound: string;
    final: boolean;
  }

  const defaultRule = (): Rule => ({
    action: 'route',
    outbounds: '',
    domain: '',
    domain_suffix: '',
    domain_keyword: '',
    domain_regex: '',
    ip_cidr: '',
    source_ip_cidr: '',
    port: '',
    source_port: '',
    network: '',
    protocol: '',
    process: '',
    process_path: '',
    package_name: '',
    uid: '',
    gid: '',
    network_type: '',
    inbound: '',
    final: false,
  });

  let rules = $state<Rule[]>(parseRules(value));

  // Пересинхронизируем rules когда value меняется снаружи (не через syncValue)
  $effect(() => {
    if (!useJson) {
      const parsed = parseRules(value);
      const same = parsed.length === rules.length &&
        parsed.every((r, i) =>
          r.action === rules[i]?.action &&
          r.outbounds === rules[i]?.outbounds &&
          r.domain === rules[i]?.domain &&
          r.domain_suffix === rules[i]?.domain_suffix &&
          r.domain_keyword === rules[i]?.domain_keyword &&
          r.domain_regex === rules[i]?.domain_regex &&
          r.ip_cidr === rules[i]?.ip_cidr &&
          r.source_ip_cidr === rules[i]?.source_ip_cidr &&
          r.port === rules[i]?.port &&
          r.source_port === rules[i]?.source_port &&
          r.network === rules[i]?.network &&
          r.protocol === rules[i]?.protocol &&
          r.process === rules[i]?.process &&
          r.process_path === rules[i]?.process_path &&
          r.package_name === rules[i]?.package_name &&
          r.uid === rules[i]?.uid &&
          r.gid === rules[i]?.gid &&
          r.network_type === rules[i]?.network_type &&
          r.inbound === rules[i]?.inbound &&
          r.final === rules[i]?.final
        );
      if (!same) rules = parsed;
    }
  });

  function parseRules(raw: string): Rule[] {
    if (!raw.trim()) return [];
    try {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) return parsed.map(normalizeRule);
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
      domain_suffix: Array.isArray(r.domain_suffix) ? r.domain_suffix.join('\n') : '',
      domain_keyword: Array.isArray(r.domain_keyword) ? r.domain_keyword.join('\n') : '',
      domain_regex: Array.isArray(r.domain_regex) ? r.domain_regex.join('\n') : '',
      ip_cidr: Array.isArray(r.ip_cidr) ? r.ip_cidr.join('\n') : '',
      source_ip_cidr: Array.isArray(r.source_ip_cidr) ? r.source_ip_cidr.join('\n') : '',
      port: Array.isArray(r.port) ? r.port.join('\n') : '',
      source_port: Array.isArray(r.source_port) ? r.source_port.join('\n') : '',
      network: Array.isArray(r.network) ? r.network.join('\n') : '',
      protocol: Array.isArray(r.protocol) ? r.protocol.join('\n') : '',
      process: Array.isArray(r.process) ? r.process.join('\n') : '',
      process_path: Array.isArray(r.process_path) ? r.process_path.join('\n') : '',
      package_name: Array.isArray(r.package_name) ? r.package_name.join('\n') : '',
      uid: Array.isArray(r.uid) ? r.uid.join('\n') : '',
      gid: Array.isArray(r.gid) ? r.gid.join('\n') : '',
      network_type: Array.isArray(r.network_type) ? r.network_type.join('\n') : '',
      inbound: Array.isArray(r.inbound) ? r.inbound.join('\n') : '',
      final: r.final || false,
    };
  }

  function serializeRules(): string {
    const ruleObjs = rules.map((r) => {
      const obj: Record<string, any> = { action: r.action };
      if (r.outbounds.trim()) obj.outbounds = r.outbounds.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.domain.trim()) obj.domain = r.domain.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.domain_suffix.trim()) obj.domain_suffix = r.domain_suffix.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.domain_keyword.trim()) obj.domain_keyword = r.domain_keyword.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.domain_regex.trim()) obj.domain_regex = r.domain_regex.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.ip_cidr.trim()) obj.ip_cidr = r.ip_cidr.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.source_ip_cidr.trim()) obj.source_ip_cidr = r.source_ip_cidr.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.port.trim()) obj.port = r.port.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.source_port.trim()) obj.source_port = r.source_port.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.network.trim()) obj.network = r.network.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.protocol.trim()) obj.protocol = r.protocol.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.process.trim()) obj.process = r.process.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.process_path.trim()) obj.process_path = r.process_path.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.package_name.trim()) obj.package_name = r.package_name.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.uid.trim()) obj.uid = r.uid.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.gid.trim()) obj.gid = r.gid.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.network_type.trim()) obj.network_type = r.network_type.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.inbound.trim()) obj.inbound = r.inbound.split('\n').map((s: string) => s.trim()).filter(Boolean);
      if (r.final) obj.final = true;
      return obj;
    });
    return JSON.stringify(ruleObjs, null, 2);
  }

  function syncValue() {
    value = serializeRules();
    jsonText = value;
  }

  function addRule() {
    rules = [...rules, defaultRule()];
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
      if (!Array.isArray(parsed)) throw new Error('JSON must be an array');
      rules = parsed.map(normalizeRule);
      value = jsonText;
      useJson = false;
      error = '';
    } catch {
      error = 'Неверный JSON — ожидается массив объектов';
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

  // Поля для отображения в UI (в логическом порядке)
  const fields: { key: keyof Rule; label: string; placeholder: string; rows: number }[] = [
    { key: 'outbounds', label: 'Outbounds (по одному на строку)', placeholder: 'direct\nproxy', rows: 2 },
    { key: 'domain', label: 'Domain (geosite-*)', placeholder: 'geosite:ru\ngeosite:google', rows: 2 },
    { key: 'domain_suffix', label: 'Domain Suffix (например .ru)', placeholder: '.ru\n.su\n.xn--p1ai', rows: 2 },
    { key: 'domain_keyword', label: 'Domain Keyword', placeholder: 'example\nblocked', rows: 2 },
    { key: 'domain_regex', label: 'Domain Regex', placeholder: '.*\\.example\\.com', rows: 2 },
    { key: 'ip_cidr', label: 'IP CIDR (geoip-*)', placeholder: 'geoip:ru\n10.0.0.0/8', rows: 2 },
    { key: 'source_ip_cidr', label: 'Source IP CIDR', placeholder: '192.168.1.0/24', rows: 2 },
    { key: 'port', label: 'Ports', placeholder: '80\n443\n8080', rows: 2 },
    { key: 'source_port', label: 'Source Ports', placeholder: '1024-65535', rows: 2 },
    { key: 'network', label: 'Network (tcp,udp)', placeholder: 'tcp\nudp', rows: 2 },
    { key: 'protocol', label: 'Protocol (tls,http,quic,dns)', placeholder: 'tls\nhttp', rows: 2 },
    { key: 'process', label: 'Process Name', placeholder: 'chrome\nfirefox', rows: 2 },
    { key: 'process_path', label: 'Process Path', placeholder: '/usr/bin/curl', rows: 2 },
    { key: 'package_name', label: 'Package Name (Android)', placeholder: 'com.example.app', rows: 2 },
    { key: 'uid', label: 'UID', placeholder: '1000', rows: 1 },
    { key: 'gid', label: 'GID', placeholder: '1000', rows: 1 },
    { key: 'network_type', label: 'Network Type (wifi,ethernet,cellular)', placeholder: 'wifi\ncellular', rows: 2 },
    { key: 'inbound', label: 'Inbound Tags', placeholder: 'inbound-1\ninbound-2', rows: 2 },
  ];
</script>

<div class="route-rule-editor">
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
          {#each fields as f}
            <div class="field">
              <label for={`${f.key}-${i}`}>{f.label}</label>
              <textarea
                id={`${f.key}-${i}`}
                rows={f.rows}
                value={rule[f.key]}
                oninput={(e) => onRuleFieldChange(i, f.key, e)}
                placeholder={f.placeholder}
                disabled={disabled}
              ></textarea>
            </div>
          {/each}
        </div>
      </div>
    {/each}

    {#if rules.length === 0}
      <p class="empty">Нет правил. Добавьте правило или переключитесь в JSON-режим.</p>
    {/if}
  {:else}
    <textarea rows="12" bind:value={jsonText} disabled={disabled} placeholder="Paste route rules JSON array here"></textarea>
  {/if}

  {#if !useJson}
    <pre class="json-preview">{serializeRules()}</pre>
  {/if}
</div>

<style>
  .route-rule-editor {
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