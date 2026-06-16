<script>
  import { login } from '../auth.js';
  let { onLoggedIn, canSignup = false, onSwitchToSignup } = $props();
  let email = $state('');
  let password = $state('');
  let error = $state('');
  let busy = $state(false);

  async function submit(e) {
    e.preventDefault();
    busy = true; error = '';
    try {
      onLoggedIn(await login(email, password));
    } catch (err) {
      error = err.message || 'login failed';
    } finally {
      busy = false;
    }
  }
</script>

<form class="auth" onsubmit={submit}>
  <h1>Exit 66 Jukebox</h1>
  <input type="email" placeholder="Email" bind:value={email} autocomplete="username" required />
  <input type="password" placeholder="Password" bind:value={password} autocomplete="current-password" required />
  {#if error}<p class="err">{error}</p>{/if}
  <button disabled={busy} type="submit">Log in</button>
  {#if canSignup}
    <button type="button" class="link" onclick={onSwitchToSignup}>Create an account</button>
  {/if}
</form>

<style>
  .auth { max-width: 22rem; margin: 12vh auto; display: flex; flex-direction: column; gap: .75rem; padding: 0 1rem; }
  .auth h1 { text-align: center; font-family: var(--font-display); font-weight: 700; letter-spacing: 0.06em; color: var(--text-strong); }
  input { padding: .6rem .75rem; font-size: 1rem; background: var(--bg-inset); border: 1px solid var(--border-strong); border-radius: var(--radius-md); color: var(--text-body); font-family: var(--font-sans); }
  input:focus { outline: none; border-color: var(--neon-cyan); box-shadow: 0 0 0 2px rgba(31,224,255,0.3); }
  button[type="submit"] { padding: .6rem .75rem; font-size: 1rem; background: var(--neon-magenta); color: var(--text-on-accent); border: none; border-radius: var(--radius-md); font-family: var(--font-display); font-weight: 700; letter-spacing: 0.06em; cursor: pointer; }
  button[type="submit"]:disabled { opacity: 0.5; cursor: default; }
  .err { color: var(--status-danger); margin: 0; font-size: 0.875rem; }
  .link { background: none; border: none; color: var(--neon-cyan); cursor: pointer; font-family: var(--font-sans); font-size: 0.9rem; }
</style>
