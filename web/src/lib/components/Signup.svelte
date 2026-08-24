<script>
  import { signup } from '../auth.js';
  let { onLoggedIn, onSwitchToLogin, bootstrapToken = '', needsBootstrap = false } = $props();
  let email = $state('');
  let displayName = $state('');
  let password = $state('');
  let error = $state('');
  let message = $state('');
  let busy = $state(false);

  async function submit(e) {
    e.preventDefault();
    busy = true; error = ''; message = '';
    try {
      const user = await signup(email, displayName, password, bootstrapToken);
      if (user.email_verified === false) {
        message = 'Check your email for a verification link before logging in.';
        return;
      }
      onLoggedIn(user);
    } catch (err) {
      error = err.message || 'signup failed';
    } finally {
      busy = false;
    }
  }
</script>

<form class="auth" onsubmit={submit}>
  <h1>Create your account</h1>
  <input type="email" placeholder="Email" bind:value={email} autocomplete="username" required />
  <input type="text" placeholder="Display name" bind:value={displayName} autocomplete="name" required />
  <input type="password" placeholder="Password" bind:value={password} autocomplete="new-password" required />
  {#if needsBootstrap && !bootstrapToken}<p class="hint">First admin setup requires the bootstrap link from the server logs.</p>{/if}
  {#if error}<p class="err">{error}</p>{/if}
  {#if message}<p class="ok">{message}</p>{/if}
  <button disabled={busy || (needsBootstrap && !bootstrapToken)} type="submit">Create account</button>
  <button type="button" class="link" onclick={onSwitchToLogin}>Back to log in</button>
</form>

<style>
  .auth { max-width: 22rem; margin: 12vh auto; display: flex; flex-direction: column; gap: .75rem; padding: 0 1rem; }
  .auth h1 { text-align: center; font-family: var(--font-display); font-weight: 700; letter-spacing: 0.06em; color: var(--text-strong); }
  input { padding: .6rem .75rem; font-size: 1rem; background: var(--bg-inset); border: 1px solid var(--border-strong); border-radius: var(--radius-md); color: var(--text-body); font-family: var(--font-sans); }
  input:focus { outline: none; border-color: var(--neon-cyan); box-shadow: 0 0 0 2px rgba(31,224,255,0.3); }
  button[type="submit"] { padding: .6rem .75rem; font-size: 1rem; background: var(--neon-magenta); color: var(--text-on-accent); border: none; border-radius: var(--radius-md); font-family: var(--font-display); font-weight: 700; letter-spacing: 0.06em; cursor: pointer; }
  button[type="submit"]:disabled { opacity: 0.5; cursor: default; }
  .err { color: var(--status-danger); margin: 0; font-size: 0.875rem; }
  .ok { color: var(--status-success); margin: 0; font-size: 0.875rem; }
  .hint { color: var(--text-muted); margin: 0; font-size: 0.875rem; }
  .link { background: none; border: none; color: var(--neon-cyan); cursor: pointer; font-family: var(--font-sans); font-size: 0.9rem; }
</style>
