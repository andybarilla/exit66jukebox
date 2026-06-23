<script>
  import { onMount } from 'svelte';
  import { listProfiles, createProfile, selectProfile } from '../auth.js';

  let { onLoggedIn } = $props();
  let profiles = $state([]);
  let displayName = $state('');
  let loading = $state(true);
  let busy = $state(false);
  let error = $state('');

  onMount(async () => {
    try {
      profiles = await listProfiles();
    } catch (err) {
      error = err.message || 'failed to load profiles';
    } finally {
      loading = false;
    }
  });

  async function chooseProfile(profile) {
    busy = true;
    error = '';
    try {
      const user = await selectProfile(profile.id);
      onLoggedIn?.(user);
    } catch (err) {
      error = err.message || 'failed to select profile';
    } finally {
      busy = false;
    }
  }

  async function submitProfile(event) {
    event.preventDefault();
    if (!displayName.trim()) {
      error = 'name is required';
      return;
    }

    busy = true;
    error = '';
    try {
      const profile = await createProfile(displayName.trim());
      profiles = [...profiles, profile];
      displayName = '';
      const user = await selectProfile(profile.id);
      onLoggedIn?.(user);
    } catch (err) {
      error = err.message || 'failed to create profile';
    } finally {
      busy = false;
    }
  }
</script>

<section class="profile-shell">
  <div class="profile-card">
    <p class="eyebrow">Household jukebox</p>
    <h1>Choose your profile</h1>
    <p class="muted">Pick a household profile before using the jukebox.</p>

    {#if error}<p class="danger">{error}</p>{/if}

    {#if loading}
      <p class="muted">Loading profiles…</p>
    {:else}
      <div class="profile-list">
        {#each profiles as profile (profile.id)}
          <button type="button" disabled={busy} class="profile-option" onclick={() => chooseProfile(profile)}>{profile.display_name}</button>
        {/each}
      </div>

      <form class="profile-form" onsubmit={submitProfile}>
        <input bind:value={displayName} placeholder="New profile name" autocomplete="name" />
        <button type="submit" disabled={busy}>{busy ? 'Working…' : 'Create'}</button>
      </form>
    {/if}
  </div>
</section>

<style>
  .profile-shell { min-height: 100vh; display: grid; place-items: center; padding: 24px; background: var(--grid-glow), var(--bg-base); color: var(--text-body); box-sizing: border-box; }
  .profile-card { width: min(560px, 100%); display: grid; gap: 16px; padding: 24px; border: 1.5px solid var(--neon-cyan); border-radius: var(--radius-lg); background: var(--bg-surface); background-image: var(--scanline); box-shadow: var(--shadow-xl), var(--glow-soft-cyan); box-sizing: border-box; }
  .eyebrow { margin: 0; font-family: var(--font-mono); font-size: 10px; letter-spacing: 0.22em; text-transform: uppercase; color: var(--neon-cyan); }
  h1 { margin: 0; font-family: var(--font-display); letter-spacing: 0.06em; text-transform: uppercase; color: var(--text-strong); }
  .muted { margin: 0; color: var(--text-muted); font-family: var(--font-sans); font-size: 14px; }
  .danger { margin: 0; color: var(--neon-magenta); font-family: var(--font-sans); font-size: 14px; }
  .profile-list { display: grid; gap: 10px; }
  .profile-option { padding: 14px 16px; text-align: left; border: 1px solid var(--border-strong); border-radius: var(--radius-md); background: var(--bg-surface-raised); color: var(--text-body); font-family: var(--font-display); cursor: pointer; }
  .profile-option:hover { border-color: var(--neon-cyan); color: var(--text-strong); box-shadow: var(--glow-soft-cyan); }
  .profile-option:disabled { opacity: 0.55; cursor: default; }
  .profile-form { display: flex; gap: 10px; }
  .profile-form input { flex: 1; min-width: 0; padding: 12px; border-radius: var(--radius-sm); border: 1px solid var(--border-strong); background: var(--bg-base); color: var(--text-body); }
  .profile-form button { padding: 0 16px; border: none; border-radius: var(--radius-sm); background: var(--neon-cyan); color: var(--text-on-accent); font-family: var(--font-display); font-weight: 700; cursor: pointer; }
  .profile-form button:disabled { opacity: 0.55; cursor: default; }
</style>
