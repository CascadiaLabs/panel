let { onclose } = $props();
<script>
  import { changePassword } from '$lib/api';

  let current = $state('');
  let next = $state('');
  let confirm = $state('');
  let error = $state('');
  let notice = $state('');
  let busy = $state(false);

  async function submit(e) {
    e.preventDefault();
    error = notice = '';
    if (next !== confirm) {
      error = 'Новые пароли не совпадают';
      return;
    }
    busy = true;
    try {
      await changePassword(current, next);
      notice = 'Пароль изменён. Прочие сессии завершены.';
      current = next = confirm = '';
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }
</script>

<div class="modal">
  <div class="modal-content">
    <div class="modal-header">
      <h2>Смена пароля</h2>
      <button class="close" onclick={() => onclose?.()}>×</button>
    </div>
    {#if error}<p class="error">{error}</p>{/if}
    {#if notice}<p class="notice">{notice}</p>{/if}
    <form onsubmit={submit}>
      <div class="field">
        <label for="pw-current">Текущий пароль</label>
        <input id="pw-current" type="password" bind:value={current} autocomplete="current-password" required />
      </div>
      <div class="field">
        <label for="pw-next">Новый пароль (мин. 8 символов)</label>
        <input id="pw-next" type="password" bind:value={next} autocomplete="new-password" minlength="8" required />
      </div>
      <div class="field">
        <label for="pw-confirm">Повторите новый пароль</label>
        <input id="pw-confirm" type="password" bind:value={confirm} autocomplete="new-password" required />
      </div>
      <button type="submit" disabled={busy}>{busy ? 'Сохраняем…' : 'Сменить пароль'}</button>
    </form>
  </div>
</div>
