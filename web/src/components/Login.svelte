<script>
  import { login } from '$lib/api';

  let username = $state('');
  let password = $state('');
  let error = $state('');
  let busy = $state(false);

  async function submit(e) {
    e.preventDefault();
    error = '';
    busy = true;
    try {
      await login(username, password);
      window.location.reload();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
      busy = false;
    }
  }
</script>

<div class="login-wrap">
  <form class="login-card" onsubmit={submit}>
    <h1>Cascadia Panel</h1>
    <p class="hint">Войдите, чтобы управлять нодами и каскадами</p>
    {#if error}<p class="error">{error}</p>{/if}
    <div class="field">
      <label for="login-username">Логин</label>
      <input id="login-username" bind:value={username} autocomplete="username" required />
    </div>
    <div class="field">
      <label for="login-password">Пароль</label>
      <input id="login-password" type="password" bind:value={password} autocomplete="current-password" required />
    </div>
    <button type="submit" disabled={busy}>{busy ? 'Входим…' : 'Войти'}</button>
  </form>
</div>

<style>
  .login-wrap {
    position: fixed;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    background: #0f172a;
  }
  .login-card {
    background: #1e293b;
    padding: 2rem;
    border-radius: 0.75rem;
    width: 100%;
    max-width: 380px;
    box-shadow: 0 10px 40px rgba(0, 0, 0, 0.4);
  }
  h1 {
    font-size: 1.375rem;
    margin-bottom: 0.25rem;
  }
  .hint {
    color: #94a3b8;
    font-size: 0.875rem;
    margin-bottom: 1.5rem;
  }
</style>
