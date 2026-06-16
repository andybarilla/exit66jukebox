<script>
  import { onMount } from 'svelte';
  import { getSettings, setSettings, createInvite, listInvites, deleteInvite, listUsers, deleteUser } from '../auth.js';
  import Switch from './Switch.svelte';

  let { onClose } = $props();

  let signupEnabled = $state(false);
  let guestAccess = $state(false);
  let invites = $state([]);
  let users = $state([]);
  let loading = $state(true);
  let error = $state('');

  // invite creator
  let inviteEmail = $state('');
  let inviteIsAdmin = $state(false);
  let inviteBusy = $state(false);
  let inviteLink = $state('');
  let inviteError = $state('');
  let copied = $state(false);

  // user delete error
  let userError = $state('');

  onMount(async () => {
    try {
      const [settings, invList, userList] = await Promise.all([getSettings(), listInvites(), listUsers()]);
      signupEnabled = !!settings.signup_enabled;
      guestAccess = !!settings.guest_access_enabled;
      invites = invList;
      users = userList;
    } catch (e) {
      error = e.message || 'failed to load settings';
    } finally {
      loading = false;
    }
  });

  async function onToggleSignup(v) {
    signupEnabled = v;
    const r = await setSettings({ signup_enabled: v });
    signupEnabled = !!r.signup_enabled;
    guestAccess = !!r.guest_access_enabled;
  }

  async function onToggleGuest(v) {
    guestAccess = v;
    const r = await setSettings({ guest_access_enabled: v });
    signupEnabled = !!r.signup_enabled;
    guestAccess = !!r.guest_access_enabled;
  }

  async function handleCreateInvite(e) {
    e.preventDefault();
    inviteBusy = true; inviteError = ''; inviteLink = '';
    try {
      const r = await createInvite(inviteEmail, inviteIsAdmin);
      inviteLink = r.link;
      inviteEmail = '';
      inviteIsAdmin = false;
      // refresh invite list
      invites = await listInvites();
    } catch (err) {
      inviteError = err.message || 'failed to create invite';
    } finally {
      inviteBusy = false;
    }
  }

  async function copyLink() {
    await navigator.clipboard.writeText(inviteLink);
    copied = true;
    setTimeout(() => { copied = false; }, 2000);
  }

  async function handleRevokeInvite(id) {
    const r = await deleteInvite(id);
    if (!r.ok) {
      const e = await r.json().catch(() => ({}));
      error = e.error || 'failed to revoke invite';
      return;
    }
    invites = await listInvites();
  }

  async function handleDeleteUser(id) {
    userError = '';
    const r = await deleteUser(id);
    if (!r.ok) {
      const e = await r.json().catch(() => ({}));
      userError = e.error || 'failed to delete user';
      return;
    }
    users = await listUsers();
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="overlay" role="dialog" aria-modal="true" aria-label="Admin settings">
  <div role="button" tabindex="-1" aria-label="Close" class="backdrop" onclick={onClose} onkeydown={(e) => { if (e.key === 'Escape') onClose(); }}></div>
  <div class="panel">
    <div class="panel-header">
      <span class="panel-title">Settings</span>
      <button class="close-btn" onclick={onClose} aria-label="Close">✕</button>
    </div>

    {#if loading}
      <p class="muted">Loading…</p>
    {:else if error}
      <p class="danger">{error}</p>
    {:else}
      <!-- Toggles -->
      <section class="section">
        <h2 class="section-title">Access</h2>
        <label class="toggle-row">
          <span class="toggle-label">Signup enabled</span>
          <Switch checked={signupEnabled} onChange={onToggleSignup} tone="cyan" />
        </label>
        <label class="toggle-row">
          <span class="toggle-label">Guest access</span>
          <Switch checked={guestAccess} onChange={onToggleGuest} tone="cyan" />
        </label>
      </section>

      <!-- Invite creator -->
      <section class="section">
        <h2 class="section-title">Create invite</h2>
        <form class="invite-form" onsubmit={handleCreateInvite}>
          <input type="email" placeholder="Email (optional)" bind:value={inviteEmail} autocomplete="off" />
          <label class="check-row">
            <input type="checkbox" bind:checked={inviteIsAdmin} />
            <span>Admin</span>
          </label>
          <button type="submit" disabled={inviteBusy} class="btn-primary">
            {inviteBusy ? 'Creating…' : 'Create invite'}
          </button>
        </form>
        {#if inviteError}<p class="danger">{inviteError}</p>{/if}
        {#if inviteLink}
          <div class="link-row">
            <input type="text" readonly value={inviteLink} class="link-input" />
            <button type="button" class="btn-copy" onclick={copyLink}>{copied ? '✓ Copied' : 'Copy'}</button>
          </div>
        {/if}
      </section>

      <!-- Invites list -->
      <section class="section">
        <h2 class="section-title">Invites</h2>
        {#if invites.length === 0}
          <p class="muted">No invites.</p>
        {:else}
          <ul class="list">
            {#each invites as inv (inv.id)}
              <li class="list-row">
                <span class="list-email">{inv.email || '(no email)'}</span>
                <span class="badge badge-{inv.status}">{inv.status}</span>
                {#if inv.is_admin}<span class="badge badge-admin">admin</span>{/if}
                <button class="btn-danger" onclick={() => handleRevokeInvite(inv.id)}>Revoke</button>
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <!-- Users list -->
      <section class="section">
        <h2 class="section-title">Users</h2>
        {#if userError}<p class="danger">{userError}</p>{/if}
        {#if users.length === 0}
          <p class="muted">No users.</p>
        {:else}
          <ul class="list">
            {#each users as u (u.id)}
              <li class="list-row">
                <span class="list-email">{u.display_name || u.email}</span>
                <span class="list-meta">{u.email}</span>
                {#if u.is_admin}<span class="badge badge-admin">admin</span>{/if}
                <button class="btn-danger" onclick={() => handleDeleteUser(u.id)}>Delete</button>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    {/if}
  </div>
</div>

<style>
  .overlay { position: fixed; inset: 0; z-index: 90; display: flex; align-items: flex-start; justify-content: flex-end; }
  .backdrop { position: absolute; inset: 0; background: rgba(6,6,11,0.6); backdrop-filter: blur(4px); }
  .panel { position: relative; z-index: 91; width: 420px; max-width: 100vw; height: 100vh; background: var(--bg-surface); background-image: var(--scanline); border-left: 1.5px solid var(--neon-magenta); overflow-y: auto; display: flex; flex-direction: column; gap: 0; padding: 0; box-sizing: border-box; }
  .panel-header { display: flex; align-items: center; justify-content: space-between; padding: 18px 20px; border-bottom: 1px solid var(--border-default); flex: none; }
  .panel-title { font-family: var(--font-display); font-weight: 700; font-size: 15px; letter-spacing: 0.08em; text-transform: uppercase; color: var(--text-strong); }
  .close-btn { background: none; border: none; color: var(--text-muted); font-size: 18px; cursor: pointer; padding: 4px 8px; border-radius: var(--radius-sm); }
  .close-btn:hover { color: var(--text-strong); }
  .section { padding: 16px 20px; border-bottom: 1px solid var(--border-subtle); }
  .section-title { font-family: var(--font-mono); font-size: 10px; letter-spacing: 0.2em; text-transform: uppercase; color: var(--text-faint); margin: 0 0 12px 0; }
  .toggle-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 10px; cursor: pointer; }
  .toggle-label { font-family: var(--font-sans); font-size: 14px; color: var(--text-body); }
  .invite-form { display: flex; flex-direction: column; gap: 8px; }
  input[type="email"], input[type="text"] { padding: 8px 10px; background: var(--bg-inset); border: 1px solid var(--border-strong); border-radius: var(--radius-md); color: var(--text-body); font-family: var(--font-sans); font-size: 13px; width: 100%; box-sizing: border-box; }
  input[type="email"]:focus, input[type="text"]:focus { outline: none; border-color: var(--neon-cyan); }
  .check-row { display: flex; align-items: center; gap: 8px; font-family: var(--font-sans); font-size: 13px; color: var(--text-body); cursor: pointer; }
  .check-row input[type="checkbox"] { width: auto; }
  .btn-primary { padding: 8px 14px; background: var(--neon-magenta); color: var(--text-on-accent); border: none; border-radius: var(--radius-md); font-family: var(--font-display); font-weight: 700; font-size: 12px; letter-spacing: 0.06em; cursor: pointer; }
  .btn-primary:disabled { opacity: 0.5; cursor: default; }
  .link-row { display: flex; gap: 8px; margin-top: 8px; }
  .link-input { flex: 1; font-size: 11px; font-family: var(--font-mono); color: var(--neon-cyan); background: var(--bg-inset); border: 1px solid var(--border-default); border-radius: var(--radius-md); padding: 6px 8px; }
  .btn-copy { padding: 6px 12px; background: rgba(31,224,255,0.1); border: 1px solid var(--neon-cyan); border-radius: var(--radius-md); color: var(--neon-cyan); font-family: var(--font-mono); font-size: 11px; cursor: pointer; white-space: nowrap; }
  .list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
  .list-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; padding: 6px 0; border-bottom: 1px solid var(--border-subtle); }
  .list-row:last-child { border-bottom: none; }
  .list-email { font-family: var(--font-sans); font-size: 13px; color: var(--text-body); flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .list-meta { font-family: var(--font-mono); font-size: 10px; color: var(--text-faint); }
  .badge { font-family: var(--font-mono); font-size: 9px; letter-spacing: 0.1em; text-transform: uppercase; padding: 2px 6px; border-radius: var(--radius-sm); }
  .badge-pending { background: rgba(255,176,46,0.15); color: var(--neon-amber); border: 1px solid var(--neon-amber-deep); }
  .badge-accepted { background: rgba(61,245,155,0.1); color: var(--status-success); border: 1px solid var(--status-success-deep); }
  .badge-expired { background: rgba(255,77,94,0.1); color: var(--status-danger); border: 1px solid var(--status-danger-deep); }
  .badge-admin { background: rgba(138,108,255,0.15); color: var(--neon-violet); border: 1px solid var(--neon-violet-deep); }
  .btn-danger { margin-left: auto; padding: 4px 10px; background: transparent; border: 1px solid var(--status-danger-deep); border-radius: var(--radius-sm); color: var(--status-danger); font-family: var(--font-mono); font-size: 10px; cursor: pointer; white-space: nowrap; }
  .btn-danger:hover { background: rgba(255,77,94,0.1); }
  .muted { color: var(--text-faint); font-size: 13px; font-family: var(--font-sans); }
  .danger { color: var(--status-danger); font-size: 13px; font-family: var(--font-sans); }
</style>
