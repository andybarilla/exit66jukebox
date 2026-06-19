<script>
  import { login, completeMfaLogin, requestPasswordReset } from '../auth.js';
  let { onLoggedIn, canSignup = false, onSwitchToSignup } = $props();
  let email = $state('');
  let password = $state('');
  let mfaTicket = $state('');
  let mfaCode = $state('');
  let mfaRecoveryCode = $state('');
  let useRecoveryCode = $state(false);
  let error = $state('');
  let info = $state('');
  let busy = $state(false);

  async function submit(e) {
    e.preventDefault();
    busy = true; error = '';
    try {
      const result = await login(email, password);
      if (result.mfa_required) {
        mfaTicket = result.ticket;
        mfaCode = '';
        mfaRecoveryCode = '';
        useRecoveryCode = false;
        info = '';
        return;
      }
      onLoggedIn(result);
    } catch (err) {
      error = err.message || 'login failed';
    } finally {
      busy = false;
    }
  }

  async function submitMfa(e) {
    e.preventDefault();
    busy = true; error = '';
    try {
      const user = useRecoveryCode
        ? await completeMfaLogin(mfaTicket, mfaRecoveryCode, true)
        : await completeMfaLogin(mfaTicket, mfaCode, false);
      onLoggedIn(user);
    } catch (err) {
      error = err.message || 'MFA verification failed';
    } finally {
      busy = false;
    }
  }

  function backToPasswordLogin() {
    mfaTicket = '';
    mfaCode = '';
    mfaRecoveryCode = '';
    useRecoveryCode = false;
    error = '';
    info = '';
  }

  async function forgotPassword() {
    busy = true; error = ''; info = '';
    try {
      await requestPasswordReset(email);
      info = 'If that account exists, a reset email will arrive shortly.';
    } catch (err) {
      error = err.message || 'password reset failed';
    } finally {
      busy = false;
    }
  }
</script>

{#if mfaTicket}
  <form class="auth" onsubmit={submitMfa}>
    <h1>Exit 66 Jukebox</h1>
    <p class="prompt">Enter your authenticator code for {email}.</p>
    {#if useRecoveryCode}
      <input type="text" placeholder="Recovery code" bind:value={mfaRecoveryCode} autocomplete="one-time-code" required />
    {:else}
      <input type="text" inputmode="numeric" pattern="[0-9]*" maxlength="6" placeholder="6-digit code" bind:value={mfaCode} autocomplete="one-time-code" required />
    {/if}
    {#if error}<p class="err">{error}</p>{/if}
    <button disabled={busy} type="submit">Verify</button>
    <button type="button" class="link" disabled={busy} onclick={() => useRecoveryCode = !useRecoveryCode}>{useRecoveryCode ? 'Use authenticator code' : 'Use a recovery code'}</button>
    <button type="button" class="link" disabled={busy} onclick={backToPasswordLogin}>Change email</button>
  </form>
{:else}
  <form class="auth" onsubmit={submit}>
    <h1>Exit 66 Jukebox</h1>
    <input type="email" placeholder="Email" bind:value={email} autocomplete="username" required />
    <input type="password" placeholder="Password" bind:value={password} autocomplete="current-password" required />
    {#if error}<p class="err">{error}</p>{/if}
    {#if info}<p class="ok">{info}</p>{/if}
    <button disabled={busy} type="submit">Log in</button>
    <button type="button" class="link" disabled={busy || !email} onclick={forgotPassword}>Forgot password?</button>
    {#if canSignup}
      <button type="button" class="link" onclick={onSwitchToSignup}>Create an account</button>
    {/if}
  </form>
{/if}

<style>
  .auth { max-width: 22rem; margin: 12vh auto; display: flex; flex-direction: column; gap: .75rem; padding: 0 1rem; }
  .auth h1 { text-align: center; font-family: var(--font-display); font-weight: 700; letter-spacing: 0.06em; color: var(--text-strong); }
  input { padding: .6rem .75rem; font-size: 1rem; background: var(--bg-inset); border: 1px solid var(--border-strong); border-radius: var(--radius-md); color: var(--text-body); font-family: var(--font-sans); }
  input:focus { outline: none; border-color: var(--neon-cyan); box-shadow: 0 0 0 2px rgba(31,224,255,0.3); }
  button[type="submit"] { padding: .6rem .75rem; font-size: 1rem; background: var(--neon-magenta); color: var(--text-on-accent); border: none; border-radius: var(--radius-md); font-family: var(--font-display); font-weight: 700; letter-spacing: 0.06em; cursor: pointer; }
  button[type="submit"]:disabled { opacity: 0.5; cursor: default; }
  .err { color: var(--status-danger); margin: 0; font-size: 0.875rem; }
  .ok { color: var(--status-success); margin: 0; font-size: 0.875rem; }
  .prompt { color: var(--text-muted); margin: 0; font-family: var(--font-sans); font-size: 0.9rem; text-align: center; }
  .link { background: none; border: none; color: var(--neon-cyan); cursor: pointer; font-family: var(--font-sans); font-size: 0.9rem; }
  .link:disabled { opacity: 0.5; cursor: default; }
</style>
