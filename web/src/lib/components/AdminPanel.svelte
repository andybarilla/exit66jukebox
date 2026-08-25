<script>
  import { onMount } from 'svelte';
  import { getSettings, setSettings, getLibraries, setLibraries, getFederationPeers, addFederationPeer, approveFederationPeer, getFederationGroups, createFederationGroup, deleteFederationGroup, addFederationGroupMember, removeFederationGroupMember, createInvite, listInvites, deleteInvite, listUsers, deleteUser, listLibraryPaths, createPasswordReset, createEmailVerification, beginMfaEnrollment, confirmMfaEnrollment, disableMfa, regenerateRecoveryCodes } from '../auth.js';
  import { beforeUnloadIfDirty, buildEditableSettingsSnapshot, hasEditableSettingsChanges, loadPathBrowserLocation } from '../settingsPanelState.js';
  import Switch from './Switch.svelte';

  let { onClose } = $props();

  let signupEnabled = $state(false);
  let guestAccess = $state(false);
  let securityMode = $state('full_login');
  let adminMfaRequired = $state(false);
  let invites = $state([]);
  let users = $state([]);
  let loading = $state(true);
  let error = $state('');
  let accessError = $state('');

  // invite creator
  let inviteEmail = $state('');
  let inviteIsAdmin = $state(false);
  let inviteBusy = $state(false);
  let inviteLink = $state('');
  let inviteError = $state('');
  let copied = $state(false);

  // user delete error
  let userError = $state('');
  let resetLink = $state('');
  let verificationLink = $state('');

  let mfaEnrollment = $state(null);
  let mfaConfirmCode = $state('');
  let mfaRecoveryCodes = $state([]);
  let mfaSecurityError = $state('');
  let mfaSecurityMessage = $state('');
  let mfaSecurityBusy = $state(false);
  let mfaDisablePassword = $state('');
  let mfaDisableCode = $state('');
  let mfaDisableUseRecovery = $state(false);
  let mfaRegeneratePassword = $state('');
  let mfaRegenerateCode = $state('');
  let mfaRegenerateUseRecovery = $state(false);

  let libraries = $state([]);
  let libraryWarnings = $state([]);
  let libraryBusy = $state(false);
  let libraryMessage = $state('');
  let libraryError = $state('');
  let federation = $state({ enabled: false, role: '', hub_addr: '', listen: '', token: '', peer_id: '', token_configured: false, restart_required: false });
  let scan = $state({ assume_same_title_folder_compilations: false });
  let federationPeers = $state([]);
  let peerDraft = $state({ peer_id: '', display_name: '', address: '' });
  let peerBusy = $state(false);
  let peerError = $state('');
  let federationGroups = $state([]);
  let groupDraft = $state('');
  let groupMemberDraft = $state({});
  let groupBusy = $state(false);
  let groupError = $state('');
  let cleanEditableSettingsState = $state(null);
  let cleanSettingsSnapshot = $state('');
  let hasUnsavedChanges = $state(false);
  let removeBeforeUnload = null;
  let pathBrowser = $state({ open: false, row: -1, path: '', parent: '', directories: [], loading: false, error: '', requestedError: '' });

  function currentEditableSettingsState() {
    return { signupEnabled, guestAccess, securityMode, adminMfaRequired, libraries, federation, scan };
  }

  function refreshCleanSettingsSnapshot() {
    cleanEditableSettingsState = currentEditableSettingsState();
    cleanSettingsSnapshot = buildEditableSettingsSnapshot(cleanEditableSettingsState);
    updateUnsavedState();
  }

  function updateCleanSettingsSnapshot(savedSettingsState) {
    if (!cleanEditableSettingsState) {
      refreshCleanSettingsSnapshot();
      return;
    }

    cleanEditableSettingsState = { ...cleanEditableSettingsState, ...savedSettingsState };
    cleanSettingsSnapshot = buildEditableSettingsSnapshot(cleanEditableSettingsState);
    updateUnsavedState();
  }

  function handleBeforeUnload(event) {
    beforeUnloadIfDirty(hasUnsavedChanges, event);
  }

  function setBeforeUnloadGuard(isDirty) {
    if (!isDirty && removeBeforeUnload) {
      removeBeforeUnload();
      removeBeforeUnload = null;
      return;
    }

    if (!isDirty || removeBeforeUnload) return;

    window.addEventListener('beforeunload', handleBeforeUnload);
    removeBeforeUnload = () => window.removeEventListener('beforeunload', handleBeforeUnload);
  }

  function updateUnsavedState() {
    if (!cleanSettingsSnapshot) {
      hasUnsavedChanges = false;
      setBeforeUnloadGuard(false);
      return;
    }

    hasUnsavedChanges = hasEditableSettingsChanges(cleanSettingsSnapshot, currentEditableSettingsState());
    setBeforeUnloadGuard(hasUnsavedChanges);
  }

  function requestCloseSettings() {
    if (!hasUnsavedChanges) {
      onClose?.();
      return;
    }

    if (!confirm('Discard unsaved settings changes?')) return;

    hasUnsavedChanges = false;
    setBeforeUnloadGuard(false);
    onClose?.();
  }

  onMount(() => {
    async function loadSettings() {
      try {
        const [settings, librarySettings, peerSettings, groupSettings, invList, userList] = await Promise.all([getSettings(), getLibraries(), getFederationPeers(), getFederationGroups(), listInvites(), listUsers()]);
        signupEnabled = !!settings.signup_enabled;
        guestAccess = !!settings.guest_access_enabled;
        securityMode = settings.security_mode || 'full_login';
        adminMfaRequired = !!settings.admin_mfa_required;
        libraries = librarySettings.local_libraries || [];
        libraryWarnings = librarySettings.warnings || [];
        federation = { ...federation, ...(librarySettings.federation || {}), token: '' };
        scan = { ...scan, ...(librarySettings.scan || {}) };
        federationPeers = peerSettings.peers || [];
        federationGroups = groupSettings.groups || [];
        invites = invList;
        users = userList;
        refreshCleanSettingsSnapshot();
      } catch (e) {
        error = e.message || 'failed to load settings';
      } finally {
        loading = false;
      }
    }

    loadSettings();

    return () => {
      if (removeBeforeUnload) removeBeforeUnload();
    };
  });

  async function onToggleSignup(v) {
    accessError = '';
    signupEnabled = v;
    updateUnsavedState();
    const r = await setSettings({ signup_enabled: v });
    signupEnabled = !!r.signup_enabled;
    updateCleanSettingsSnapshot({ signupEnabled });
  }

  async function onToggleGuest(v) {
    accessError = '';
    guestAccess = v;
    updateUnsavedState();
    const r = await setSettings({ guest_access_enabled: v });
    guestAccess = !!r.guest_access_enabled;
    updateCleanSettingsSnapshot({ guestAccess });
  }

  async function onChangeSecurityMode(v) {
    accessError = '';
    securityMode = v;
    updateUnsavedState();
    try {
      const r = await setSettings({ security_mode: v });
      securityMode = r.security_mode || v;
      guestAccess = !!r.guest_access_enabled;
      signupEnabled = !!r.signup_enabled;
      updateCleanSettingsSnapshot({ securityMode, guestAccess, signupEnabled });
    } catch (err) {
      accessError = err.message || 'failed to update security mode';
      updateUnsavedState();
    }
  }

  async function onToggleAdminMFA(v) {
    accessError = '';
    adminMfaRequired = v;
    updateUnsavedState();
    try {
      const r = await setSettings({ admin_mfa_required: v });
      adminMfaRequired = !!r.admin_mfa_required;
      updateCleanSettingsSnapshot({ adminMfaRequired });
    } catch (err) {
      accessError = err.message || 'failed to update admin MFA setting';
      updateUnsavedState();
    }
  }

  function resetMfaSecretState() {
    mfaEnrollment = null;
    mfaConfirmCode = '';
    mfaRecoveryCodes = [];
  }

  async function handleBeginMfaEnrollment() {
    mfaSecurityBusy = true;
    mfaSecurityError = '';
    mfaSecurityMessage = '';
    resetMfaSecretState();
    try {
      mfaEnrollment = await beginMfaEnrollment();
    } catch (err) {
      mfaSecurityError = err.message || 'failed to begin MFA enrollment';
    } finally {
      mfaSecurityBusy = false;
    }
  }

  async function handleConfirmMfaEnrollment(e) {
    e.preventDefault();
    mfaSecurityBusy = true;
    mfaSecurityError = '';
    mfaRecoveryCodes = [];
    try {
      const r = await confirmMfaEnrollment(mfaConfirmCode);
      mfaRecoveryCodes = r.recovery_codes || [];
      mfaEnrollment = null;
      mfaConfirmCode = '';
      mfaSecurityMessage = 'MFA enabled.';
      users = await listUsers();
    } catch (err) {
      mfaSecurityError = err.message || 'failed to confirm MFA enrollment';
    } finally {
      mfaSecurityBusy = false;
    }
  }

  async function handleDisableMfa(e) {
    e.preventDefault();
    mfaSecurityBusy = true;
    mfaSecurityError = '';
    mfaSecurityMessage = '';
    mfaRecoveryCodes = [];
    try {
      await disableMfa(mfaDisablePassword, mfaDisableCode, mfaDisableUseRecovery);
      mfaDisablePassword = '';
      mfaDisableCode = '';
      mfaDisableUseRecovery = false;
      mfaSecurityMessage = 'MFA disabled.';
      users = await listUsers();
    } catch (err) {
      mfaSecurityError = err.message || 'failed to disable MFA';
    } finally {
      mfaSecurityBusy = false;
    }
  }

  async function handleRegenerateRecoveryCodes(e) {
    e.preventDefault();
    mfaSecurityBusy = true;
    mfaSecurityError = '';
    mfaSecurityMessage = '';
    mfaRecoveryCodes = [];
    try {
      const r = await regenerateRecoveryCodes(mfaRegeneratePassword, mfaRegenerateCode, mfaRegenerateUseRecovery);
      mfaRecoveryCodes = r.recovery_codes || [];
      mfaRegeneratePassword = '';
      mfaRegenerateCode = '';
      mfaRegenerateUseRecovery = false;
      mfaSecurityMessage = 'Recovery codes regenerated.';
    } catch (err) {
      mfaSecurityError = err.message || 'failed to regenerate recovery codes';
    } finally {
      mfaSecurityBusy = false;
    }
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

  async function handleCreatePasswordReset(id) {
    userError = ''; resetLink = ''; verificationLink = '';
    try {
      const r = await createPasswordReset(id);
      resetLink = r.link;
    } catch (err) {
      userError = err.message || 'failed to create reset link';
    }
  }

  async function generateVerificationLink(u) {
    userError = ''; resetLink = ''; verificationLink = '';
    try {
      const r = await createEmailVerification(u.id);
      verificationLink = r.link;
      users = await listUsers();
    } catch (err) {
      userError = err.message || 'failed to create verification link';
    }
  }

  function addLibrary() {
    libraries = [...libraries, { name: '', path: '', enabled: true }];
    updateUnsavedState();
  }

  function removeLibrary(index) {
    libraries = libraries.filter((_, i) => i !== index);
    updateUnsavedState();
  }

  function setLibraryField(index, field, value) {
    libraries = libraries.map((lib, i) => i === index ? { ...lib, [field]: value } : lib);
    updateUnsavedState();
  }

  function setFederationField(field, value) {
    federation = { ...federation, [field]: value };
    updateUnsavedState();
  }

  function setScanField(field, value) {
    scan = { ...scan, [field]: value };
    updateUnsavedState();
  }

  async function openPathBrowser(row) {
    const library = libraries[row] || {};
    pathBrowser = { ...pathBrowser, open: true, row, path: library.path || '', parent: '', directories: [], loading: true, error: '', requestedError: '' };
    await loadLibraryPath(library.path || '', true);
  }

  async function loadLibraryPath(path = '', allowFallback = false) {
    pathBrowser = { ...pathBrowser, path, loading: true, error: '', requestedError: '' };
    const location = await loadPathBrowserLocation(listLibraryPaths, path, allowFallback);
    pathBrowser = { ...pathBrowser, ...location, loading: false };
  }

  function choosePathBrowserFolder() {
    if (pathBrowser.row < 0) return;

    setLibraryField(pathBrowser.row, 'path', pathBrowser.path);
    closePathBrowser();
  }

  function closePathBrowser() {
    pathBrowser = { open: false, row: -1, path: '', parent: '', directories: [], loading: false, error: '', requestedError: '' };
  }

  function handleSettingsKeydown(e) {
    if (e.key !== 'Escape') return;

    if (pathBrowser.open) {
      closePathBrowser();
      return;
    }

    requestCloseSettings();
  }

  function warningFor(path) {
    return libraryWarnings.find((w) => w.path === path)?.message || '';
  }

  async function saveLibraries(saveAndScan = false) {
    libraryBusy = true;
    libraryError = '';
    libraryMessage = '';
    try {
      const r = await setLibraries({ local_libraries: libraries, federation, scan, save_and_scan: saveAndScan });
      libraries = r.local_libraries || [];
      libraryWarnings = r.warnings || [];
      federation = { ...federation, ...(r.federation || {}), token: '' };
      scan = { ...scan, ...(r.scan || {}) };
      libraryMessage = saveAndScan ? 'Saved. Scan started.' : 'Saved.';
      updateCleanSettingsSnapshot({ libraries, federation, scan });
    } catch (err) {
      libraryError = err.message || 'failed to save libraries';
    } finally {
      libraryBusy = false;
    }
  }

  async function saveManualPeer() {
    peerBusy = true;
    peerError = '';
    try {
      const r = await addFederationPeer(peerDraft);
      federationPeers = r.peers || [];
      peerDraft = { peer_id: '', display_name: '', address: '' };
    } catch (err) {
      peerError = err.message || 'failed to save peer';
    } finally {
      peerBusy = false;
    }
  }

  // Every group endpoint answers with the full group list, so each action just
  // replaces local state rather than refetching.
  async function runGroupAction(action) {
    groupBusy = true;
    groupError = '';
    try {
      const r = await action();
      federationGroups = r.groups || [];
    } catch (err) {
      groupError = err.message || 'failed to update listening groups';
    } finally {
      groupBusy = false;
    }
  }

  async function addGroup() {
    const name = groupDraft.trim();
    if (!name) return;
    await runGroupAction(() => createFederationGroup(name));
    groupDraft = '';
  }

  async function addGroupMember(groupID) {
    const peerID = (groupMemberDraft[groupID] || '').trim();
    if (!peerID) return;
    await runGroupAction(() => addFederationGroupMember(groupID, peerID));
    groupMemberDraft = { ...groupMemberDraft, [groupID]: '' };
  }

  async function approvePeer(peerID) {
    peerBusy = true;
    peerError = '';
    try {
      const r = await approveFederationPeer(peerID);
      federationPeers = r.peers || [];
    } catch (err) {
      peerError = err.message || 'failed to approve peer';
    } finally {
      peerBusy = false;
    }
  }
</script>

<svelte:window onkeydown={handleSettingsKeydown} />

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="overlay" role="dialog" aria-modal="true" aria-label="Admin settings">
  <div role="button" tabindex="-1" aria-label="Close" class="backdrop" onclick={requestCloseSettings} onkeydown={handleSettingsKeydown}></div>
  <div class="panel">
    <div class="panel-header">
      <span class="panel-title">Settings</span>
      <button class="close-btn" onclick={requestCloseSettings} aria-label="Close">✕</button>
    </div>

    {#if loading}
      <p class="muted">Loading…</p>
    {:else if error}
      <p class="danger">{error}</p>
    {:else}
      <!-- Toggles -->
      <section class="section">
        <h2 class="section-title">Access</h2>
        <section class="mode-section">
          <h3>Security mode</h3>
          <div class="mode-list">
            {#each [
              { value: 'open', label: 'Open', help: 'Frictionless trusted household jukebox. Normal queue controls are open.' },
              { value: 'open_admin_locked', label: 'Open, admin locked', help: 'Public jukebox playback with settings and protected house controls behind /admin.' },
              { value: 'household_profiles', label: 'Household profiles', help: 'Visitors choose or create a passwordless profile before using the jukebox.' },
              { value: 'full_login', label: 'Full login', help: 'Password accounts are required before app access.' },
            ] as option}
              <label class="mode-card">
                <input type="radio" name="security-mode" value={option.value} checked={securityMode === option.value} onchange={() => onChangeSecurityMode(option.value)} />
                <span class="mode-copy">
                  <strong>{option.label}</strong>
                  <span>{option.help}</span>
                </span>
              </label>
            {/each}
          </div>
          {#if securityMode !== 'full_login'}
            <p class="warning">Open, admin-locked, and household profile modes are intended for trusted/private networks. Use full_login for public exposure.</p>
          {/if}
        </section>
        <label class="toggle-row">
          <span class="toggle-label">Signup enabled <small>(full_login accounts only)</small></span>
          <Switch checked={signupEnabled} onChange={onToggleSignup} tone="cyan" />
        </label>
        {#if securityMode !== 'full_login'}<p class="muted">Signup applies only to full_login mode.</p>{/if}
        <label class="toggle-row">
          <span class="toggle-label">Require MFA for admin access</span>
          <Switch checked={adminMfaRequired} onChange={onToggleAdminMFA} tone="cyan" />
        </label>
        {#if accessError}<p class="danger">{accessError}</p>{/if}
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
              <div class="path-input-row">
                <input type="text" placeholder="/path/to/music" value={lib.path || ''} oninput={(e) => setLibraryField(i, 'path', e.currentTarget.value)} />
                <button type="button" class="btn-copy" onclick={() => openPathBrowser(i)}>Browse</button>
              </div>
              {#if warningFor(lib.path)}<p class="warning">{warningFor(lib.path)}</p>{/if}
              <button type="button" class="btn-danger" onclick={() => removeLibrary(i)}>Remove</button>
            </div>
          {/each}
        </div>
        <button type="button" class="btn-copy" onclick={addLibrary}>Add path</button>

        <div class="scan-box">
          <h3>Scan behavior</h3>
          <label class="check-row">
            <input type="checkbox" checked={!!scan.assume_same_title_folder_compilations} onchange={(e) => setScanField('assume_same_title_folder_compilations', e.currentTarget.checked)} />
            <span>Assume same-title albums in one folder are compilations</span>
          </label>
        </div>

        <div class="federation-box">
          <h3>Federation</h3>
          <label class="check-row">
            <input type="checkbox" checked={!!federation.enabled} onchange={(e) => setFederationField('enabled', e.currentTarget.checked)} />
            <span>Enable federation after restart</span>
          </label>
          <select value={federation.role || ''} onchange={(e) => setFederationField('role', e.currentTarget.value)}>
            <option value="">Off</option>
            <option value="hub">Hub</option>
            <option value="member">Member</option>
            <option value="peer">Peer</option>
          </select>
          <input type="text" placeholder="Hub address (members)" value={federation.hub_addr || ''} oninput={(e) => setFederationField('hub_addr', e.currentTarget.value)} />
          <input type="text" placeholder="Listen address (hub or peer)" value={federation.listen || ''} oninput={(e) => setFederationField('listen', e.currentTarget.value)} />
          <input type="password" placeholder={federation.token_configured ? 'Token saved; leave blank to keep' : 'Token'} value={federation.token || ''} oninput={(e) => setFederationField('token', e.currentTarget.value)} />
          <input type="text" placeholder="Peer ID" value={federation.peer_id || ''} oninput={(e) => setFederationField('peer_id', e.currentTarget.value)} />
          {#if federation.restart_required}<p class="warning">Federation changes require a restart.</p>{/if}

          {#if federation.role === 'peer'}
            <div class="peer-box">
              <h4>Direct peers</h4>
              <div class="peer-draft">
                <input type="text" placeholder="Peer ID" value={peerDraft.peer_id} oninput={(e) => peerDraft = { ...peerDraft, peer_id: e.currentTarget.value }} />
                <input type="text" placeholder="Display name" value={peerDraft.display_name} oninput={(e) => peerDraft = { ...peerDraft, display_name: e.currentTarget.value }} />
                <input type="text" placeholder="host:port" value={peerDraft.address} oninput={(e) => peerDraft = { ...peerDraft, address: e.currentTarget.value }} />
                <button type="button" class="btn-copy" disabled={peerBusy} onclick={saveManualPeer}>Add manual peer</button>
              </div>
              {#if federationPeers.length === 0}
                <p class="muted">No manual or discovered peers yet.</p>
              {:else}
                <ul class="peer-list">
                  {#each federationPeers as peer (peer.id || `${peer.peer_id}-${peer.address}`)}
                    <li class="peer-row">
                      <span class="peer-name">{peer.display_name || peer.peer_id}</span>
                      <span class="peer-address">{peer.address}</span>
                      <span class="badge badge-{peer.status}">{peer.status}</span>
                      {#if peer.status === 'pending' && peer.token_authenticated}
                        <button type="button" class="btn-copy" disabled={peerBusy} onclick={() => approvePeer(peer.peer_id)}>Approve</button>
                      {/if}
                    </li>
                  {/each}
                </ul>
              {/if}
              {#if peerError}<p class="danger">{peerError}</p>{/if}
            </div>
          {/if}

          {#if federation.role === 'peer' || federation.role === 'hub'}
            <div class="peer-box">
              <h4>Listening groups</h4>
              <p class="muted">
                Groups decide whose music shows up in browse. A peer only sees the libraries of
                peers it shares a group with. They are not a playback restriction: a peer that
                already has a track's link can still play it, whatever group it is in. With no
                groups at all, every approved peer sees every other.
              </p>
              <div class="peer-draft">
                <input type="text" placeholder="Group name" value={groupDraft} oninput={(e) => groupDraft = e.currentTarget.value} />
                <button type="button" class="btn-copy" disabled={groupBusy} onclick={addGroup}>Add group</button>
              </div>
              {#if federationGroups.length === 0}
                <p class="muted">No groups yet.</p>
              {:else}
                <ul class="peer-list">
                  {#each federationGroups as group (group.id)}
                    <li class="peer-row">
                      <span class="peer-name">{group.name}</span>
                      <span class="peer-address">{group.members.join(', ') || 'no members'}</span>
                      <input type="text" placeholder="Peer ID" value={groupMemberDraft[group.id] || ''} oninput={(e) => groupMemberDraft = { ...groupMemberDraft, [group.id]: e.currentTarget.value }} />
                      <button type="button" class="btn-copy" disabled={groupBusy} onclick={() => addGroupMember(group.id)}>Add member</button>
                      {#each group.members as member (member)}
                        <button type="button" class="btn-copy" disabled={groupBusy} onclick={() => runGroupAction(() => removeFederationGroupMember(group.id, member))}>Remove {member}</button>
                      {/each}
                      <button type="button" class="btn-copy" disabled={groupBusy} onclick={() => runGroupAction(() => deleteFederationGroup(group.id))}>Delete group</button>
                    </li>
                  {/each}
                </ul>
              {/if}
              {#if groupError}<p class="danger">{groupError}</p>{/if}
            </div>
          {/if}
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

      <section class="section security-section">
        <h2 class="section-title">Account security</h2>
        <p class="muted">Manage MFA for the current authenticated account.</p>
        <button type="button" class="btn-copy" disabled={mfaSecurityBusy} onclick={handleBeginMfaEnrollment}>Begin MFA enrollment</button>

        {#if mfaEnrollment}
          <div class="secret-box">
            <p class="muted">Add this secret to your authenticator app, or use the otpauth URI.</p>
            <code>{mfaEnrollment.secret}</code>
            <code>{mfaEnrollment.otpauth_uri}</code>
            <form class="mfa-form" onsubmit={handleConfirmMfaEnrollment}>
              <input type="text" inputmode="numeric" maxlength="6" placeholder="Confirmation code" bind:value={mfaConfirmCode} autocomplete="one-time-code" required />
              <button type="submit" class="btn-primary" disabled={mfaSecurityBusy}>Confirm enrollment</button>
            </form>
          </div>
        {/if}

        {#if mfaRecoveryCodes.length > 0}
          <div class="recovery-box">
            <p class="warning">Save these recovery codes now. They will not be shown again.</p>
            <ul class="recovery-list">
              {#each mfaRecoveryCodes as code}
                <li><code>{code}</code></li>
              {/each}
            </ul>
          </div>
        {/if}

        <form class="mfa-form" onsubmit={handleDisableMfa}>
          <h3>Disable MFA</h3>
          <input type="password" placeholder="Password" bind:value={mfaDisablePassword} autocomplete="current-password" required />
          <input type="text" placeholder={mfaDisableUseRecovery ? 'Recovery code' : 'Authenticator code'} bind:value={mfaDisableCode} autocomplete="one-time-code" required />
          <label class="check-row">
            <input type="checkbox" bind:checked={mfaDisableUseRecovery} />
            <span>Use recovery code</span>
          </label>
          <button type="submit" class="btn-danger" disabled={mfaSecurityBusy}>Disable MFA</button>
        </form>

        <form class="mfa-form" onsubmit={handleRegenerateRecoveryCodes}>
          <h3>Regenerate recovery codes</h3>
          <input type="password" placeholder="Password" bind:value={mfaRegeneratePassword} autocomplete="current-password" required />
          <input type="text" placeholder={mfaRegenerateUseRecovery ? 'Recovery code' : 'Authenticator code'} bind:value={mfaRegenerateCode} autocomplete="one-time-code" required />
          <label class="check-row">
            <input type="checkbox" bind:checked={mfaRegenerateUseRecovery} />
            <span>Use recovery code</span>
          </label>
          <button type="submit" class="btn-primary" disabled={mfaSecurityBusy}>Regenerate recovery codes</button>
        </form>

        {#if mfaSecurityError}<p class="danger">{mfaSecurityError}</p>{/if}
        {#if mfaSecurityMessage}<p class="success">{mfaSecurityMessage}</p>{/if}
      </section>

      <!-- Users list -->
      <section class="section">
        <h2 class="section-title">Users</h2>
        {#if userError}<p class="danger">{userError}</p>{/if}
        {#if resetLink}
          <div class="link-row">
            <input type="text" readonly value={resetLink} class="link-input" />
            <button type="button" class="btn-copy" onclick={async () => navigator.clipboard.writeText(resetLink)}>Copy</button>
          </div>
        {/if}
        {#if verificationLink}
          <div class="link-row">
            <input type="text" readonly value={verificationLink} class="link-input" />
            <button type="button" class="btn-copy" onclick={async () => navigator.clipboard.writeText(verificationLink)}>Copy</button>
          </div>
        {/if}
        {#if users.length === 0}
          <p class="muted">No users.</p>
        {:else}
          <ul class="list">
            {#each users as u (u.id)}
              <li class="list-row">
                <span class="list-email">{u.display_name || u.email}</span>
                <span class="list-meta">{u.email}</span>
                {#if u.is_admin}<span class="badge badge-admin">admin</span>{/if}
                {#if u.mfa_enabled}<span class="badge badge-mfa">MFA</span>{/if}
                {#if u.email_verified}<span class="badge badge-verified">Verified</span>{:else}<span class="badge badge-unverified">Unverified</span>{/if}
                {#if !u.email_verified}<button class="btn-copy" onclick={() => generateVerificationLink(u)}>Generate verification link</button>{/if}
                <button class="btn-copy" onclick={() => handleCreatePasswordReset(u.id)}>Reset password</button>
                <button class="btn-danger" onclick={() => handleDeleteUser(u.id)}>Delete</button>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    {/if}
  </div>
</div>

{#if pathBrowser.open}
  <div class="path-browser-overlay" role="dialog" aria-modal="true" aria-label="Choose library folder">
    <div class="path-browser-modal">
      <div class="path-browser-header">
        <h2>Choose folder</h2>
        <button type="button" class="close-btn" onclick={closePathBrowser} aria-label="Cancel">✕</button>
      </div>
      <p class="muted">Current server path</p>
      <code class="path-browser-code">{pathBrowser.path || '/'}</code>

      {#if pathBrowser.requestedError}
        <p class="warning">Requested path unavailable: {pathBrowser.requestedError}</p>
      {/if}
      {#if pathBrowser.error}
        <p class="danger">{pathBrowser.error}</p>
      {/if}
      {#if pathBrowser.loading}
        <p class="muted">Loading folders…</p>
      {:else}
        <div class="path-browser-list">
          {#if pathBrowser.parent}
            <button type="button" class="path-browser-row parent-row" onclick={() => loadLibraryPath(pathBrowser.parent)}>../</button>
          {/if}
          {#each pathBrowser.directories as directory (directory.path)}
            <button type="button" class="path-browser-row" onclick={() => loadLibraryPath(directory.path)}>{directory.name || directory.path}</button>
          {/each}
          {#if !pathBrowser.parent && pathBrowser.directories.length === 0 && !pathBrowser.error}
            <p class="muted">No child folders.</p>
          {/if}
        </div>
      {/if}

      <div class="path-browser-actions">
        <button type="button" class="btn-primary" disabled={pathBrowser.loading || !pathBrowser.path} onclick={choosePathBrowserFolder}>Use this folder</button>
        <button type="button" class="btn-copy" onclick={closePathBrowser}>Cancel</button>
      </div>
    </div>
  </div>
{/if}

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
  .toggle-label small { display: block; color: var(--text-faint); font-size: 11px; margin-top: 2px; }
  .mode-section { display: grid; gap: 10px; margin-bottom: 14px; }
  .mode-section h3 { margin: 0; font-family: var(--font-display); font-size: 13px; letter-spacing: 0.08em; text-transform: uppercase; color: var(--neon-cyan); }
  .mode-list { display: grid; gap: 8px; }
  .mode-card { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 9px; padding: 10px; border: 1px solid var(--border-default); border-radius: var(--radius-sm); background: rgba(0,0,0,0.16); cursor: pointer; }
  .mode-card input { margin-top: 2px; }
  .mode-copy { display: grid; gap: 3px; }
  .mode-copy strong { font-family: var(--font-display); font-size: 13px; color: var(--text-strong); }
  .mode-copy span { font-family: var(--font-sans); font-size: 12px; color: var(--text-muted); }
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
  .badge-quarantined { background: rgba(255,77,94,0.1); color: var(--status-danger); border: 1px solid var(--status-danger-deep); }
  .badge-admin { background: rgba(138,108,255,0.15); color: var(--neon-violet); border: 1px solid var(--neon-violet-deep); }
  .badge-mfa { background: rgba(31,224,255,0.12); color: var(--neon-cyan); border: 1px solid var(--neon-cyan); }
  .badge-verified { background: rgba(61,245,155,0.12); color: var(--status-success); border: 1px solid var(--status-success-deep); }
  .badge-unverified { background: rgba(255,176,46,0.15); color: var(--neon-amber); border: 1px solid var(--neon-amber-deep); }
  .btn-danger { margin-left: auto; padding: 4px 10px; background: transparent; border: 1px solid var(--status-danger-deep); border-radius: var(--radius-sm); color: var(--status-danger); font-family: var(--font-mono); font-size: 10px; cursor: pointer; white-space: nowrap; }
  .btn-danger:hover { background: rgba(255,77,94,0.1); }
  .muted { color: var(--text-faint); font-size: 13px; font-family: var(--font-sans); }
  .danger { color: var(--status-danger); font-size: 13px; font-family: var(--font-sans); }
  .library-section { background: linear-gradient(135deg, rgba(31,224,255,0.05), rgba(255,42,166,0.06)); }
  .security-section { display: grid; gap: 12px; background: radial-gradient(circle at top right, rgba(138,108,255,0.12), transparent 50%); }
  .secret-box, .recovery-box, .mfa-form { display: grid; gap: 8px; padding: 10px; border: 1px solid var(--border-subtle); border-radius: var(--radius-lg); background: rgba(0,0,0,0.18); }
  .secret-box code, .recovery-list code { display: block; padding: 7px 8px; border: 1px solid var(--border-default); border-radius: var(--radius-sm); background: var(--bg-inset); color: var(--neon-cyan); font-family: var(--font-mono); font-size: 11px; overflow-wrap: anywhere; }
  .mfa-form h3 { margin: 0; font-family: var(--font-display); font-size: 12px; letter-spacing: 0.08em; color: var(--text-strong); text-transform: uppercase; }
  .recovery-list { list-style: none; margin: 0; padding: 0; display: grid; gap: 5px; }
  .library-list { display: flex; flex-direction: column; gap: 10px; margin: 10px 0; }
  .library-card { display: grid; gap: 8px; padding: 12px; border: 1px solid var(--border-default); border-radius: var(--radius-lg); background: rgba(0,0,0,0.18); box-shadow: 0 10px 30px rgba(0,0,0,0.18); }
  .library-enabled { justify-content: flex-start; margin: 0; }
  .path-input-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 8px; align-items: center; }
   .scan-box, .federation-box { display: grid; gap: 8px; margin-top: 16px; padding-top: 14px; border-top: 1px solid var(--border-subtle); }
   .scan-box { padding: 12px; border: 1px solid rgba(255,42,166,0.2); border-radius: var(--radius-lg); background: linear-gradient(135deg, rgba(255,42,166,0.08), rgba(31,224,255,0.05)); }
   .scan-box h3, .federation-box h3 { margin: 0; font-family: var(--font-display); font-size: 13px; letter-spacing: 0.08em; color: var(--neon-cyan); text-transform: uppercase; }
  .peer-box { display: grid; gap: 10px; margin-top: 8px; padding: 12px; border: 1px solid rgba(31,224,255,0.25); border-radius: var(--radius-lg); background: radial-gradient(circle at top right, rgba(31,224,255,0.12), rgba(0,0,0,0.18) 52%); }
  .peer-box h4 { margin: 0; font-family: var(--font-display); color: var(--neon-cyan); font-size: 12px; letter-spacing: 0.08em; text-transform: uppercase; }
  .peer-draft { display: grid; gap: 8px; }
  .peer-list { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
  .peer-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 4px 8px; align-items: center; padding: 8px; border: 1px solid var(--border-subtle); border-radius: var(--radius-md); background: rgba(0,0,0,0.16); }
  .peer-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: var(--font-sans); color: var(--text-body); font-size: 13px; }
  .peer-address { grid-column: 1 / -1; font-family: var(--font-mono); color: var(--text-faint); font-size: 10px; }
  .library-actions { display: flex; gap: 8px; margin-top: 12px; flex-wrap: wrap; }
  .warning { color: var(--neon-amber); font-size: 12px; font-family: var(--font-sans); margin: 0; }
  .success { color: var(--status-success); font-size: 13px; font-family: var(--font-sans); }
  .path-browser-overlay { position: fixed; inset: 0; z-index: 120; display: grid; place-items: center; padding: 18px; background: rgba(4,4,9,0.72); backdrop-filter: blur(6px); }
  .path-browser-modal { width: min(520px, 100%); max-height: min(680px, 86vh); overflow: hidden; display: flex; flex-direction: column; gap: 12px; padding: 18px; border: 1.5px solid var(--neon-cyan); border-radius: var(--radius-lg); background: radial-gradient(circle at top left, rgba(31,224,255,0.14), transparent 46%), linear-gradient(145deg, rgba(13,16,27,0.98), rgba(8,8,14,0.98)); box-shadow: 0 24px 80px rgba(0,0,0,0.55), 0 0 30px rgba(31,224,255,0.18); }
  .path-browser-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  .path-browser-header h2 { margin: 0; font-family: var(--font-display); font-size: 15px; letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-strong); }
  .path-browser-code { display: block; padding: 10px 12px; border: 1px solid var(--border-default); border-radius: var(--radius-md); background: var(--bg-inset); color: var(--neon-cyan); font-family: var(--font-mono); font-size: 12px; overflow: auto; }
  .path-browser-list { min-height: 0; flex: 1 1 auto; overflow: auto; display: grid; align-content: start; gap: 6px; padding: 2px; }
  .path-browser-row { width: 100%; padding: 9px 10px; text-align: left; border: 1px solid var(--border-subtle); border-radius: var(--radius-md); background: rgba(255,255,255,0.03); color: var(--text-body); font-family: var(--font-sans); font-size: 13px; cursor: pointer; }
  .path-browser-row:hover { border-color: var(--neon-cyan); background: rgba(31,224,255,0.1); color: var(--text-strong); }
  .parent-row { color: var(--neon-amber); font-family: var(--font-mono); }
  .path-browser-actions { flex: none; display: flex; justify-content: flex-end; gap: 8px; flex-wrap: wrap; padding-top: 4px; }
</style>
