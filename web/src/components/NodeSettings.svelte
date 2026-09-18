<script lang="ts">
  // Панель настроек элемента графа: формы по kind/protocol.
  // Мутирует graphNode.settings напрямую; после каждого изменения дергается onChanged().
  import { generateSecret, generateRealityPair } from '$lib/api';
  import { randomUUID } from '$lib/uuid';
  import { applyProtocol } from '$lib/element';
  import type { GraphNode, Node } from '$lib/api';

  let {
    graphNode,
    physNodes,
    cascadeTarget, // GraphNode | null — inbound, к которому подключён этот outbound
    onChanged,
    onDelete,
    onClose,
  } = $props();

  // settings гарантированно существует (GraphEditor создаёт элементы с settings: {})
  const s = $derived(graphNode.settings ?? {});
  const protocols = $derived(
    graphNode.kind === 'inbound'
      ? ['vless', 'vmess', 'trojan', 'shadowsocks', 'hysteria2', 'tuic']
      : graphNode.kind === 'outbound'
        ? ['vless', 'vmess', 'trojan', 'shadowsocks', 'hysteria2', 'tuic', 'direct']
        : graphNode.kind === 'balancer'
          ? ['urltest', 'selector']
          : ['match']
  );

  let busy = $state(false);

  // список через запятую для полей rule
  function ruleList(field) {
    return (s[field] ?? []).join(',');
  }
  function setList(field, value) {
    s[field] = value.split(',').map((v) => v.trim()).filter(Boolean);
  }

  function touch() {
    onChanged();
  }

  async function genReality() {
    busy = true;
    try {
      const pair = await generateRealityPair();
      s.tls ??= {};
      s.tls.reality ??= {};
      s.tls.reality.private_key = pair.private_key;
      s.tls.reality.public_key = pair.public_key;
      s.tls.reality._public = pair.public_key;
      touch();
    } finally {
      busy = false;
    }
  }

  async function genShortID() {
    s.tls ??= {};
    s.tls.reality ??= {};
    s.tls.reality.short_ids = [await generateSecret('shortid')];
    touch();
  }

  function copyText(text) {
    navigator.clipboard?.writeText(text ?? '');
  }
</script>

<aside class="panel">
  <header>
    <h2>{graphNode.kind} · {graphNode.tag}</h2>
    <div class="head-actions">
      <button class="danger small" onclick={onDelete}>Удалить</button>
      <button class="close" onclick={onClose}>×</button>
    </div>
  </header>

  <div class="body">
    <div class="field">
      <label for="el-tag">Tag (уникален в графе)</label>
      <input id="el-tag" bind:value={graphNode.tag} oninput={touch} />
    </div>

    {#if graphNode.kind !== 'rule'}
      <div class="field">
        <label for="el-proto">Протокол</label>
        <select id="el-proto" value={graphNode.protocol}
                onchange={(e) => { graphNode.protocol = e.target.value; applyProtocol(graphNode); touch(); }}>
          {#each protocols as p}<option value={p}>{p}</option>{/each}
        </select>
        {#if graphNode.kind === 'balancer'}
          <p class="hint">urltest — автоматический выбор адресата с меньшим пингом; selector — ручной выбор (default).</p>
        {/if}
      </div>
    {/if}

    <div class="field">
      <label for="el-node">Физическая нода</label>
      <select id="el-node" bind:value={graphNode.node_id} onchange={touch}>
        {#each physNodes as n (n.id)}<option value={n.id}>{n.name}</option>{/each}
      </select>
    </div>

    {#if graphNode.kind === 'inbound'}
      <div class="field">
        <label for="el-subscription-order">Порядок в подписке</label>
        <input id="el-subscription-order" type="number" step="1" bind:value={s.subscription_order} oninput={touch} />
        <p class="hint">Меньшее число отображается раньше. Одинаковые значения сортируются по тегу.</p>
      </div>
      <div class="field">
        <label for="el-port">Listen port</label>
        <input id="el-port" type="number" min="1" max="65535" bind:value={s.listen_port} oninput={touch} />
      </div>
      <div class="field">
        <label for="el-host">Public host (для каскадных подключений; пусто = хост gRPC-адреса ноды)</label>
        <input id="el-host" bind:value={s.public_host} oninput={touch} placeholder="напр. node1.example.com" />
      </div>

      {#if graphNode.protocol === 'shadowsocks'}
        <div class="field">
          <label for="el-method">Метод шифрования</label>
          <select id="el-method" bind:value={s.method} onchange={touch}>
            {#each ['2022-blake3-aes-128-gcm', '2022-blake3-aes-256-gcm', '2022-blake3-chacha20-poly1305', 'aes-128-gcm', 'aes-256-gcm', 'chacha20-ietf-poly1305'] as m}
              <option value={m}>{m}</option>
            {/each}
          </select>
        </div>
      {/if}

      {#if graphNode.protocol === 'hysteria2'}
        <div class="field two">
          <div>
            <label for="el-up">Up Mbps</label>
            <input id="el-up" type="number" bind:value={s.up_mbps} oninput={touch} />
          </div>
          <div>
            <label for="el-down">Down Mbps</label>
            <input id="el-down" type="number" bind:value={s.down_mbps} oninput={touch} />
          </div>
        </div>
        <div class="field">
          <label for="el-obfs">Obfs password (salamander; пусто = без обфускации)</label>
          <input id="el-obfs" bind:value={s.obfs_password} oninput={touch} />
        </div>
      {/if}

      {#if graphNode.protocol === 'tuic'}
        <div class="field">
          <label for="el-cc">Congestion control</label>
          <select id="el-cc" bind:value={s.congestion_control} onchange={touch}>
            <option value="bbr">bbr</option>
            <option value="cubic">cubic</option>
            <option value="new_reno">new_reno</option>
          </select>
        </div>
      {/if}

      {#if ['vless', 'vmess', 'trojan', 'hysteria2', 'tuic'].includes(graphNode.protocol)}
        <h3>TLS</h3>
        <div class="field">
          <label class="check">
            <input type="checkbox" checked={s.tls?.enabled ?? false} onchange={(e) => { s.tls ??= {}; s.tls.enabled = e.target.checked; touch(); }} />
            Включить TLS {#if ['hysteria2', 'tuic'].includes(graphNode.protocol)}(обязательно){/if}
          </label>
        </div>
        {#if ['vless', 'trojan'].includes(graphNode.protocol)}
        <div class="field">
          <label class="check">
            <input type="checkbox" checked={s.tls?.reality?.enabled ?? false}
                   onchange={(e) => { s.tls ??= {}; s.tls.reality ??= {}; s.tls.reality.enabled = e.target.checked; if (e.target.checked) s.tls.enabled = true; touch(); }} />
            Reality (маскировка под чужой TLS)
          </label>
        </div>
      {/if}
      {#if s.tls?.reality?.enabled}
            <div class="board">
              <div class="board-head">
                <span class="badge">REALITY</span>
                <span class="board-sub">маскировка под чужой TLS-сайт</span>
              </div>

              <div class="field">
                <label for="el-hs-server">Handshake сервер</label>
                <input id="el-hs-server" list="reality-hosts" bind:value={s.tls.reality.handshake_server}
                       oninput={touch} placeholder="www.microsoft.com" />
                <datalist id="reality-hosts">
                  {#each ['www.microsoft.com', 'www.apple.com', 'www.google.com', 'dl.google.com', 'cloudflare.com', 'www.yahoo.com', 'www.amazon.com'] as h}
                    <option value={h}></option>
                  {/each}
                </datalist>
              </div>
              <div class="field two">
                <div>
                  <label for="el-hs-port">Port</label>
                  <input id="el-hs-port" type="number" bind:value={s.tls.reality.handshake_port} oninput={touch} placeholder="443" />
                </div>
                <div>
                  <label for="el-sni">SNI (как handshake)</label>
                  <input id="el-sni" bind:value={s.tls.server_name} oninput={touch} placeholder="домен handshake" />
                </div>
              </div>

              <div class="field">
                <label for="el-reality-priv">
                  Private key (x25519)
                  <button class="link" disabled={busy} onclick={genReality} title="Сгенерировать пару ключей">сгенерировать ⟳</button>
                </label>
                <input id="el-reality-priv" type="password" bind:value={s.tls.reality.private_key} oninput={touch} />
                {#if s.tls.reality._public}
                  <div class="kv">
                    <span class="kv-key">Public key (для клиента)</span>
                    <code>{s.tls.reality._public}</code>
                    <button class="small" title="Скопировать" onclick={() => copyText(s.tls.reality._public)}>⧉</button>
                  </div>
                {/if}
              </div>

              <div class="field">
                <label for="el-reality-sid">
                  Short ID
                  <button class="link" onclick={genShortID} title="Сгенерировать">сгенерировать ⟳</button>
                </label>
                <input id="el-reality-sid" value={s.tls.reality.short_ids?.[0] ?? ''}
                       oninput={(e) => { s.tls.reality.short_ids = e.target.value ? [e.target.value] : []; touch(); }} />
                <p class="hint">Пусто = принимать любой. Клиенту укажите тот же Short ID.</p>
              </div>

              <details class="sum">
                <summary>Параметры для клиента</summary>
                <div class="kv-grid">
                  <span>Address</span><code>{s.public_host || 'хост ноды'}</code>
                  <span>Port</span><code>{s.listen_port || '…'}</code>
                  <span>SNI</span><code>{s.tls.server_name || '…'}</code>
                  <span>uTLS</span><code>chrome</code>
                  {#if graphNode.protocol === 'vless'}
                    <span>Flow</span><code>xtls-rprx-vision (у пользователя)</code>
                  {/if}
                  {#if s.tls.reality._public}
                    <span>pbk</span><code>{s.tls.reality._public}</code>
                  {/if}
                  {#if s.tls.reality.short_ids?.[0]}
                    <span>sid</span><code>{s.tls.reality.short_ids[0]}</code>
                  {/if}
                </div>
              </details>
            </div>
          {:else}
            {#if s.tls?.enabled}
              <div class="field">
                <label for="el-sni">Server name (SNI)</label>
                <input id="el-sni" bind:value={s.tls.server_name} oninput={touch} />
              </div>
              <div class="field">
                <label for="el-cert">Certificate PEM (пусто = самоподписанный)</label>
                <textarea id="el-cert" rows="4" bind:value={s.tls.cert_pem} oninput={touch}></textarea>
              </div>
              <div class="field">
                <label for="el-key">Key PEM</label>
                <textarea id="el-key" rows="4" bind:value={s.tls.key_pem} oninput={touch}></textarea>
              </div>
            {/if}
          {/if}

        {#if ['vless', 'vmess', 'trojan'].includes(graphNode.protocol)}
          <h3>Transport</h3>
          <div class="field">
            <label for="el-transport">Тип</label>
            <select id="el-transport" value={s.transport?.type ?? ''}
                    onchange={(e) => { s.transport = e.target.value ? { type: e.target.value } : undefined; touch(); }}>
              <option value="">tcp (по умолчанию)</option>
              <option value="ws">ws</option>
              <option value="grpc">grpc</option>
              <option value="http">http</option>
              <option value="httpupgrade">httpupgrade</option>
            </select>
          </div>
          {#if ['ws', 'http', 'httpupgrade'].includes(s.transport?.type)}
            <div class="field">
              <label for="el-tr-path">Path</label>
              <input id="el-tr-path" bind:value={s.transport.path} oninput={touch} placeholder="/" />
            </div>
            <div class="field">
              <label for="el-tr-host">Host</label>
              <input id="el-tr-host" bind:value={s.transport.host} oninput={touch} />
            </div>
          {/if}
          {#if s.transport?.type === 'grpc'}
            <div class="field">
              <label for="el-tr-svc">Service name</label>
              <input id="el-tr-svc" bind:value={s.transport.service_name} oninput={touch} />
            </div>
          {/if}
        {/if}
      {/if}
    {:else if graphNode.kind === 'outbound'}
      {#if graphNode.protocol === 'direct'}
        <p class="hint">Прямое соединение с интернетом. Терминальный элемент: подключите к нему inbound
          (правило маршрутизации) или отметьте «выход в интернет» справа.</p>
      {:else if cascadeTarget}
        <div class="derived">
          <h3>Каскад → {cascadeTarget.tag}</h3>
          <p class="hint">Outbound подключается к inbound <b>{cascadeTarget.tag}</b> другой ноды.
            Адрес, порт и учётные данные наследуются автоматически из целевого inbound —
            это единый источник правды. Здесь можно задать только uTLS fingerprint.</p>
          <div class="field">
            <label for="el-utls">uTLS fingerprint (для reality обычно chrome)</label>
            <select id="el-utls" value={s.tls?.utls_fingerprint ?? ''}
                    onchange={(e) => { s.tls ??= {}; s.tls.utls_fingerprint = e.target.value; touch(); }}>
              <option value="">(авто: chrome при reality)</option>
              {#each ['chrome', 'firefox', 'edge', 'safari', 'ios', 'android', 'random'] as f}
                <option value={f}>{f}</option>
              {/each}
            </select>
          </div>
        </div>
      {:else}
        <p class="hint">Выход в интернет: подключите этот outbound к inbound (слева) и отметьте
          его портом «Интернет» справа. Параметры сервера задаются вручную.</p>
        <div class="field two">
          <div>
            <label for="el-server">Server</label>
            <input id="el-server" bind:value={s.server} oninput={touch} />
          </div>
          <div>
            <label for="el-sport">Server port</label>
            <input id="el-sport" type="number" bind:value={s.server_port} oninput={touch} />
          </div>
        </div>
        {#if graphNode.protocol !== 'direct'}
          {#if ['vless', 'vmess', 'tuic'].includes(graphNode.protocol)}
            <div class="field">
              <label for="el-uuid">UUID</label>
              <div class="gen-wrap">
                <input id="el-uuid" bind:value={s.uuid} oninput={touch} />
                <button class="small" onclick={() => { s.uuid = randomUUID(); touch(); }}>⟳</button>
              </div>
            </div>
          {/if}
          {#if ['trojan', 'shadowsocks', 'hysteria2', 'tuic'].includes(graphNode.protocol)}
            <div class="field">
              <label for="el-pass">Пароль</label>
              <input id="el-pass" type="password" bind:value={s.password} oninput={touch} />
            </div>
          {/if}
          {#if graphNode.protocol === 'shadowsocks'}
            <div class="field">
              <label for="el-ss-method">Метод</label>
              <select id="el-ss-method" bind:value={s.method} onchange={touch}>
                {#each ['2022-blake3-aes-128-gcm', '2022-blake3-aes-256-gcm', 'aes-128-gcm', 'aes-256-gcm', 'chacha20-ietf-poly1305'] as m}
                  <option value={m}>{m}</option>
                {/each}
              </select>
            </div>
          {/if}
          <div class="field">
            <label class="check">
              <input type="checkbox" checked={s.tls?.enabled ?? false}
                     onchange={(e) => { s.tls ??= {}; s.tls.enabled = e.target.checked; touch(); }} />
              Включить TLS
            </label>
          </div>
          {#if s.tls?.enabled}
            <div class="field">
              <label for="el-osni">Server name (SNI)</label>
              <input id="el-osni" bind:value={s.tls.server_name} oninput={touch} />
            </div>
            <div class="field">
              <label class="check">
                <input type="checkbox" checked={s.tls.insecure ?? false}
                       onchange={(e) => { s.tls.insecure = e.target.checked; touch(); }} />
                Insecure (самоподписанный сертификат сервера)
              </label>
            </div>
            {#if ['vless', 'trojan'].includes(graphNode.protocol)}
              <div class="field">
                <label class="check">
                  <input type="checkbox" checked={s.tls.reality?.enabled ?? false}
                         onchange={(e) => { s.tls.reality ??= {}; s.tls.reality.enabled = e.target.checked; touch(); }} />
                  Reality
                </label>
              </div>
              {#if s.tls.reality?.enabled}
                <div class="field">
                  <label for="el-rpub">Reality public key</label>
                  <input id="el-rpub" bind:value={s.tls.reality.public_key} oninput={touch} />
                </div>
                <div class="field">
                  <label for="el-rsid">Reality short id</label>
                  <input id="el-rsid" bind:value={s.tls.reality.short_id} oninput={touch} />
                </div>
              {/if}
            {/if}
          {/if}
        {/if}
      {/if}
    {:else if graphNode.kind === 'balancer'}
      {#if graphNode.protocol === 'urltest'}
        <p class="hint">Автовыбор адресата с наименьшим пингом (urltest). Подключите inbound слева,
          outbound-кандидаты или inbound (в том числе на этой ноде) справа.</p>
        <div class="field">
          <label for="el-bal-url">Test URL</label>
          <input id="el-bal-url" bind:value={s.url} oninput={touch} placeholder="https://www.gstatic.com/generate_204" />
        </div>
        <div class="field two">
          <div>
            <label for="el-bal-int">Interval</label>
            <input id="el-bal-int" bind:value={s.interval} oninput={touch} placeholder="3m" />
          </div>
          <div>
            <label for="el-bal-tol">Tolerance (мс)</label>
            <input id="el-bal-tol" type="number" bind:value={s.tolerance} oninput={touch} placeholder="50" />
          </div>
        </div>
      {:else}
        <p class="hint">Ручной выбор адресата (selector). Default — tag outbound'а или inbound'а по умолчанию.</p>
        <div class="field">
          <label for="el-bal-def">Default outbound tag</label>
          <input id="el-bal-def" bind:value={s.default} oninput={touch} />
        </div>
      {/if}
    {:else if graphNode.kind === 'rule'}
      <p class="hint">Правило маршрутизации: inbound слева, справа — outbound/balancer или inbound
        другой ноды (подключение к ней будет создано автоматически).
        Пустые условия = catch-all (весь трафик этого inbound).</p>
      <div class="field">
        <label for="el-rule-net">Network (через запятую)</label>
        <input id="el-rule-net" value={ruleList('network')} oninput={(e) => setList('network', e.target.value)} placeholder="tcp,udp" />
      </div>
      <div class="field">
        <label for="el-rule-dom">Domain (через запятую)</label>
        <input id="el-rule-dom" value={ruleList('domain')} oninput={(e) => setList('domain', e.target.value)} placeholder="example.com" />
      </div>
      <div class="field">
        <label for="el-rule-suffix">Domain suffix (через запятую)</label>
        <input id="el-rule-suffix" value={ruleList('domain_suffix')} oninput={(e) => setList('domain_suffix', e.target.value)} placeholder=".ru,.org" />
      </div>
      <div class="field">
        <label for="el-rule-kw">Domain keyword (через запятую)</label>
        <input id="el-rule-kw" value={ruleList('domain_keyword')} oninput={(e) => setList('domain_keyword', e.target.value)} />
      </div>
      <div class="field">
        <label for="el-rule-ip">IP CIDR (через запятую)</label>
        <input id="el-rule-ip" value={ruleList('ip_cidr')} oninput={(e) => setList('ip_cidr', e.target.value)} placeholder="10.0.0.0/8" />
      </div>
      <div class="field">
        <label for="el-rule-port">Ports (через запятую)</label>
        <input id="el-rule-port" value={ruleList('port')} oninput={(e) => setList('port', e.target.value)} placeholder="80,443" />
      </div>
      <div class="field">
        <label class="check">
          <input type="checkbox" checked={s.invert ?? false} onchange={(e) => { s.invert = e.target.checked; touch(); }} />
          Инвертировать
        </label>
      </div>
    {/if}
  </div>
</aside>

<style>
  .panel {
    width: 340px;
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
  h3 { font-size: 0.8125rem; margin: 1rem 0 0.5rem; color: #94a3b8; text-transform: uppercase; letter-spacing: 0.04em; }
  .head-actions { display: flex; gap: 0.5rem; align-items: center; }
  .body {
    padding: 1rem;
    overflow-y: auto;
    flex: 1;
  }
  .close { background: none; color: #94a3b8; font-size: 1.25rem; padding: 0; }
  .close:hover { background: none; color: #e2e8f0; }
  button.small { padding: 0.25rem 0.5rem; font-size: 0.75rem; }
  .field.two { display: grid; grid-template-columns: 1fr 1fr; gap: 0.5rem; }
  .gen-wrap { display: flex; gap: 0.35rem; flex: 1; min-width: 0; }
  .gen-wrap input { flex: 1; min-width: 0; }
  .hint { color: #94a3b8; font-size: 0.75rem; margin-bottom: 0.75rem; }
  .derived { background: #0f172a; border-radius: 0.5rem; padding: 0.75rem; margin-bottom: 1rem; }
  .derived h3 { margin-top: 0; }
  .check { display: flex; align-items: center; gap: 0.5rem; cursor: pointer; }
  .check input { width: auto; }
  .board { background: #0f172a; border: 1px solid #1e293b; border-radius: 0.5rem; padding: 0.75rem; margin-bottom: 0.5rem; }
  .board-head { display: flex; align-items: center; gap: 0.5rem; margin-bottom: 0.75rem; }
  .badge { background: #155e75; color: #a5f3fc; font-size: 0.65rem; font-weight: 700; letter-spacing: 0.08em; padding: 0.15rem 0.4rem; border-radius: 0.25rem; }
  .board-sub { color: #64748b; font-size: 0.7rem; }
  button.link { background: none; color: #38bdf8; font-size: 0.7rem; padding: 0; margin-left: 0.5rem; text-decoration: underline; }
  button.link:hover { background: none; color: #7dd3fc; }
  .kv { display: flex; align-items: center; gap: 0.35rem; margin-top: 0.35rem; flex-wrap: wrap; }
  .kv-key { color: #94a3b8; font-size: 0.7rem; }
  .kv code { color: #38bdf8; font-size: 0.7rem; word-break: break-all; }
  .sum { margin-top: 0.5rem; border-top: 1px solid #1e293b; padding-top: 0.5rem; }
  .sum summary { cursor: pointer; color: #94a3b8; font-size: 0.75rem; }
  .kv-grid { display: grid; grid-template-columns: auto 1fr; gap: 0.25rem 0.5rem; margin-top: 0.5rem; font-size: 0.72rem; }
  .kv-grid span { color: #94a3b8; }
  .kv-grid code { color: #e2e8f0; word-break: break-all; }
</style>
