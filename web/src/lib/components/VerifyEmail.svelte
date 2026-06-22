<script>
  import { verifyEmail } from '../auth.js';

  let { onComplete } = $props();
  let token = window.location.pathname.replace(/^\/verify\//, '');
  let busy = $state(true);
  let error = $state('');
  let done = $state(false);

  async function redeem() {
    if (!token) {
      error = 'Verification link failed.';
      busy = false;
      return;
    }
    try {
      await verifyEmail(token);
      done = true;
    } catch (err) {
      error = err.message || 'Verification link failed.';
    } finally {
      busy = false;
    }
  }

  redeem();
</script>

<div class="verify-card">
  {#if busy}
    <h1>Verifying…</h1>
    <p class="muted">Checking your Exit 66 email link.</p>
  {:else if done}
    <h1>Email verified</h1>
    <p class="ok">You can log in now.</p>
    <button type="button" onclick={onComplete}>Back to log in</button>
  {:else}
    <h1>Verification link failed</h1>
    <p class="err">{error}</p>
    <button type="button" onclick={onComplete}>Back to log in</button>
  {/if}
</div>

<style>
  .verify-card { max-width: 24rem; margin: 12vh auto; padding: 1.25rem; display: grid; gap: .8rem; border: 1px solid var(--neon-cyan); border-radius: var(--radius-lg); background: radial-gradient(circle at top left, rgba(31,224,255,0.16), transparent 48%), var(--bg-surface); box-shadow: 0 24px 80px rgba(0,0,0,.36); }
  h1 { margin: 0; font-family: var(--font-display); letter-spacing: .06em; color: var(--text-strong); }
  .muted { color: var(--text-muted); margin: 0; }
  .ok { color: var(--status-success); margin: 0; }
  .err { color: var(--status-danger); margin: 0; }
  button { padding: .65rem .85rem; background: var(--neon-magenta); color: var(--text-on-accent); border: none; border-radius: var(--radius-md); font-family: var(--font-display); font-weight: 700; letter-spacing: .06em; cursor: pointer; }
</style>
