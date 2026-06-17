<script>
  import { onMount } from 'svelte';
  import { getSettings, setSettings, getLibraries, setLibraries, createInvite, listInvites, deleteInvite, listUsers, deleteUser } from '../auth.js';
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

  let libraries = $state([]);
  let libraryWarnings = $state([]);
  let libraryBusy = $state(false);
  let libraryMessage = $state('');
  let libraryError = $state('');
  let federation = $state({ enabled: false, role: '', hub_addr: '', listen: '', token: '', peer_id: '', token_configured: false, restart_required: false });

  onMount(async () => {
    try {
      const [settings, librarySettings, invList, userList] = await Promise.all([getSettings(), getLibraries(), listInvites(), listUsers()]);
      signupEnabled = !!settings.signup_enabled;
      guestAccess = !!settings.guest_access_enabled;
      libraries = librarySettings.local_libraries || [];
      libraryWarnings = librarySettings.warnings || [];
      federation = { ...federation, ...(librarySettings.federation || {}), token: '' };
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

  function addLibrary() {
    libraries = [...libraries, { name: '', path: '', enabled: true }];
  }

  function removeLibrary(index) {
    libraries = libraries.filter((_, i) => i !== index);
  }

  function setLibraryField(index, field, value) {
    libraries = libraries.map((lib, i) => i === index ? { ...lib, [field]: value } : lib);
  }

  function warningFor(path) {
    return libraryWarnings.find((w) => w.path === path)?.message || '';
  }

  async function saveLibraries(saveAndScan = false) {
    libraryBusy = true;
    libraryError = '';
    libraryMessage = '';
    try {
      const r = await setLibraries({ local_libraries: libraries, federation, save_and_scan: saveAndScan });
      libraries = r.local_libraries || [];
      libraryWarnings = r.warnings || [];
      federation = { ...federation, ...(r.federation || {}), token: '' };
      libraryMessage = saveAndScan ? 'Saved. Scan started.' : 'Saved.';
    } catch (err) {
      libraryError = err.message || 'failed to save libraries';
    } finally {
      libraryBusy = false;
    }
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

      <section class="section library-section">
        <h2 class="section-title">Libraries</h2>
        <p class="muted">Enabled paths feed the scanner. Disabled paths stay saved but are skipped.</p>
        <div class="library-list">
          {#each libraries as lib, i}
            <div class="library-card">
              <label class="check-row library-enabled">
                <input type="checkbox" checked={!!lib.enabled} onchange={(e) => setLibraryField(i, 'enabled', e.currentTarget.checked)} />
                <span>Enabled</span>
              </label>
              <input type="text" placeholder="Display name" value={lib.name || ''} oninput={(e) => setLibraryField(i, 'name', e.currentTarget.value)} />
              <input type="text" placeholder="/path/to/music" value={lib.path || ''} oninput={(e) => setLibraryField(i, 'path', e.currentTarget.value)} />
              {#if warningFor(lib.path)}<p class="warning">{warningFor(lib.path)}</p>{/if}
              <button type="button" class="btn-danger" onclick={() => removeLibrary(i)}>Remove</button>
            </div>
          {/each}
        </div>
        <button type="button" class="btn-copy" onclick={addLibrary}>Add path</button>

        <div class="federation-box">
          <h3>Federation</h3>
          <label class="check-row">
            <input type="checkbox" checked={!!federation.enabled} onchange={(e) => federation = { ...federation, enabled: e.currentTarget.checked }} />
            <span>Enable federation after restart</span>
          </label>
          <select value={federation.role || ''} onchange={(e) => federation = { ...federation, role: e.currentTarget.value }}>
            <option value="">Off</option>
            <option value="hub">Hub</option>
            <option value="member">Member</option>
          </select>
          <input type="text" placeholder="Hub address (members)" value={federation.hub_addr || ''} oninput={(e) => federation = { ...federation, hub_addr: e.currentTarget.value }} />
          <input type="text" placeholder="Listen address (hub)" value={federation.listen || ''} oninput={(e) => federation = { ...federation, listen: e.currentTarget.value }} />
          <input type="password" placeholder={federation.token_configured ? 'Token saved; leave blank to keep' : 'Token'} value={federation.token || ''} oninput={(e) => federation = { ...federation, token: e.currentTarget.value }} />
          <input type="text" placeholder="Peer ID" value={federation.peer_id || ''} oninput={(e) => federation = { ...federation, peer_id: e.currentTarget.value }} />
          {#if federation.restart_required}<p class="warning">Federation changes require a restart.</p>{/if}
        </div>

        <div class="library-actions">
          <button type="button" class="btn-primary" disabled={libraryBusy} onclick={() => saveLibraries(false)}>{libraryBusy ? 'Saving…' : 'Save'}</button>
          <button type="button" class="btn-copy" disabled={libraryBusy} onclick={() => saveLibraries(true)}>Save and scan now</button>
        </div>
        {#if libraryMessage}<p class="success">{libraryMessage}</p>{/if}
        {#if libraryError}<p class="danger">{libraryError}</p>{/if}
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
  input[type="email"], input[type="text"], input[type="password"], select { padding: 8px 10px; background: var(--bg-inset); border: 1px solid var(--border-strong); border-radius: var(--radius-md); color: var(--text-body); font-family: var(--font-sans); font-size: 13px; width: 100%; box-sizing: border-box; }
  input[type="email"]:focus, input[type="text"]:focus, input[type="password"]:focus, select:focus { outline: none; border-color: var(--neon-cyan); }
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
  .library-section { background: linear-gradient(135deg, rgba(31,224,255,0.05), rgba(255,42,166,0.06)); }
  .library-list { display: flex; flex-direction: column; gap: 10px; margin: 10px 0; }
  .library-card { display: grid; gap: 8px; padding: 12px; border: 1px solid var(--border-default); border-radius: var(--radius-lg); background: rgba(0,0,0,0.18); box-shadow: 0 10px 30px rgba(0,0,0,0.18); }
  .library-enabled { justify-content: flex-start; margin: 0; }
  .federation-box { display: grid; gap: 8px; margin-top: 16px; padding-top: 14px; border-top: 1px solid var(--border-subtle); }
  .federation-box h3 { margin: 0; font-family: var(--font-display); font-size: 13px; letter-spacing: 0.08em; color: var(--neon-cyan); text-transform: uppercase; }
  .library-actions { display: flex; gap: 8px; margin-top: 12px; flex-wrap: wrap; }
  .warning { color: var(--neon-amber); font-size: 12px; font-family: var(--font-sans); margin: 0; }
  .success { color: var(--status-success); font-size: 13px; font-family: var(--font-sans); }
</style>
