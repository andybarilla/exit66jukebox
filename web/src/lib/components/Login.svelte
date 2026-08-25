<script>
  import { onMount } from 'svelte';
  import { login, completeMfaLogin, requestPasswordReset } from '../auth.js';
  let { onLoggedIn, canSignup = false, onSwitchToSignup, oidcEnabled = false, oidcName = '' } = $props();

  // An OIDC sign-in returns the browser here with its result in the query: a
  // failure reason, or oidc_mfa=1 when the account has TOTP enabled, which drops
  // straight into the same second-step form a password login uses. The ticket
  // itself is NOT in the query — it is in an HttpOnly cookie the server reads
  // when the request body carries none, so it never reaches history or a script.
  const oidcResult = new URLSearchParams(window.location.search);

  let email = $state('');
  let password = $state('');
  let mfaTicket = $state('');
  // Set when the second step is owed to a cookie-held ticket rather than to one
  // this component is holding, which is what makes the MFA form render with
  // mfaTicket still empty.
  let mfaPending = $state(oidcResult.get('oidc_mfa') === '1');
  let mfaCode = $state('');
  let mfaRecoveryCode = $state('');
  let useRecoveryCode = $state(false);
  let error = $state(oidcErrorText(oidcResult.get('oidc_error')));
  let info = $state('');
  let busy = $state(false);

  // The reason codes the callback redirects with. Anything unrecognised (or
  // absent) is no error at all, so a stray query parameter cannot put text on
  // this screen.
  function oidcErrorText(reason) {
    switch (reason) {
      case 'state': return 'That sign-in did not start here. Try again.';
      case 'provider':
      case 'exchange':
      case 'unreachable': return `Could not complete the sign-in with ${oidcName || 'your provider'}. Try again.`;
      case 'token': return 'Your provider\u2019s response could not be verified.';
      case 'email_taken': return 'An account here already uses that email address. Log in with your password instead.';
      case 'email_unverified': return 'Your provider did not confirm a verified email address, so no account could be created.';
      case 'no_signup': return 'This jukebox is not accepting new accounts.';
      case 'throttled': return 'Too many sign-in attempts. Wait a minute and try again.';
      case 'server': return 'Sign-in failed. Try again.';
      default: return '';
    }
  }

  // Take the result out of the address bar once it has been read: a reload
  // would otherwise replay a spent MFA ticket or a stale error.
  onMount(() => {
    if (oidcResult.has('oidc_mfa') || oidcResult.has('oidc_error')) {
      history.replaceState(null, '', window.location.pathname);
    }
  });

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
    mfaPending = false;
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

{#if mfaTicket || mfaPending}
  <form class="auth" onsubmit={submitMfa}>
    <h1>Exit 66 Jukebox</h1>
    <p class="prompt">Enter your authenticator code{email ? ` for ${email}` : ''}.</p>
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
    {#if oidcEnabled}
      <a class="sso" href="/api/auth/oidc/start">Continue with {oidcName}</a>
    {/if}
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
  .sso { padding: .6rem .75rem; text-align: center; text-decoration: none; background: var(--bg-inset); border: 1px solid var(--neon-cyan); border-radius: var(--radius-md); color: var(--neon-cyan); font-family: var(--font-display); font-weight: 700; letter-spacing: 0.06em; }
  .link { background: none; border: none; color: var(--neon-cyan); cursor: pointer; font-family: var(--font-sans); font-size: 0.9rem; }
  .link:disabled { opacity: 0.5; cursor: default; }
</style>
