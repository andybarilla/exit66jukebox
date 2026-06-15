<script>
  import SearchInput from './SearchInput.svelte';
  import Avatar from './Avatar.svelte';
  import CastPanel from './CastPanel.svelte';
  import { scanIndicator } from '../scan.js';
  let {
    isPhone = false, query = '', onSearch, streamChipLabel = '', onToggleStream,
    scan = null, onToast = () => {}, onCastActive = () => {},
    isAdmin = true, adminRequired = false, onLogin = async () => {}, onLogout = () => {},
  } = $props();
  let ind = $derived(scanIndicator(scan));

  // Admin lock control state — only meaningful when a password gate is configured.
  let lockOpen = $state(false);
  let password = $state('');
  let submitting = $state(false);
  let loginError = $state('');

  async function submitLogin() {
    if (!password || submitting) return;
    submitting = true; loginError = '';
    try {
      await onLogin(password);
      password = ''; lockOpen = false;
    } catch (_) {
      loginError = 'Incorrect password.';
    } finally {
      submitting = false;
    }
  }
</script>

{#snippet scanChip()}
  {#if ind.visible}
    <span title="Library scan in progress" style="display:inline-flex; align-items:center; gap:8px; padding:7px 12px; border:1px solid var(--neon-cyan); border-radius:var(--radius-sm); font-family:var(--font-mono); font-size:11px; letter-spacing:0.08em; color:var(--neon-cyan); white-space:nowrap;">
      <span class="scan-pulse" style="width:7px; height:7px; border-radius:50%; background:var(--neon-cyan);"></span>{ind.text}
    </span>
  {/if}
{/snippet}

{#snippet adminControl()}
  {#if adminRequired}
    <div style="position:relative; flex:none;">
      {#if isAdmin}
        <button onclick={onLogout} title="Admin mode active — click to lock"
          style="display:inline-flex; align-items:center; gap:7px; padding:7px 12px; border:1px solid var(--neon-cyan); border-radius:var(--radius-sm); background:rgba(34,224,238,0.06); font-family:var(--font-mono); font-size:11px; letter-spacing:0.1em; text-transform:uppercase; color:var(--neon-cyan); cursor:pointer; white-space:nowrap;">
          <span aria-hidden="true">🔓</span>Admin
        </button>
      {:else}
        <button onclick={() => (lockOpen = !lockOpen)} aria-label="Unlock admin mode" aria-expanded={lockOpen}
          style="display:inline-flex; align-items:center; gap:7px; padding:7px 12px; border:1px solid var(--border-default); border-radius:var(--radius-sm); background:transparent; font-family:var(--font-mono); font-size:11px; letter-spacing:0.1em; text-transform:uppercase; color:var(--text-muted); cursor:pointer; white-space:nowrap;">
          <span aria-hidden="true">🔒</span>Admin
        </button>
        {#if lockOpen}
          <div role="button" tabindex="-1" aria-label="Close" onclick={() => (lockOpen = false)} onkeydown={(e) => { if (e.key === 'Escape') lockOpen = false; }} style="position:fixed; inset:0; z-index:80;"></div>
          <form onsubmit={(e) => { e.preventDefault(); submitLogin(); }} style="position:absolute; right:0; top:calc(100% + 8px); z-index:81; width:248px; max-width:calc(100vw - 28px); background:var(--bg-surface); background-image:var(--scanline); border:1.5px solid var(--neon-cyan); border-radius:var(--radius-md); box-shadow:var(--shadow-lg); padding:14px; box-sizing:border-box; display:flex; flex-direction:column; gap:10px;">
            <span style="font-family:var(--font-display); font-weight:700; font-size:13px; letter-spacing:0.08em; text-transform:uppercase; color:var(--text-strong);">Unlock admin</span>
            <!-- svelte-ignore a11y_autofocus -->
            <input bind:value={password} type="password" autocomplete="current-password" autofocus placeholder="Admin password" aria-label="Admin password"
              style="width:100%; box-sizing:border-box; padding:8px 10px; border:1px solid var(--border-default); border-radius:var(--radius-sm); background:var(--ink-950); color:var(--text-body); font-family:var(--font-mono); font-size:12px;" />
            {#if loginError}<span style="font-family:var(--font-mono); font-size:10px; color:var(--neon-magenta-bright);">{loginError}</span>{/if}
            <button type="submit" disabled={submitting || !password}
              style="padding:8px 0; border:1px solid var(--neon-cyan); border-radius:var(--radius-sm); background:rgba(34,224,238,0.08); color:var(--neon-cyan); font-family:var(--font-mono); font-size:11px; letter-spacing:0.08em; text-transform:uppercase; cursor:pointer;">{submitting ? 'Unlocking…' : 'Unlock'}</button>
          </form>
        {/if}
      {/if}
    </div>
  {/if}
{/snippet}
{#if !isPhone}
  <header style="height:62px; flex:none; display:flex; align-items:center; gap:24px; padding:0 24px; border-bottom:1px solid var(--border-default); background:var(--ink-950); z-index:5;">
    <div style="display:flex; align-items:center; gap:11px; flex:none;">
      <div style="width:34px; height:34px; flex:none; border-radius:var(--radius-md); border:1.5px solid var(--neon-magenta); box-shadow:0 0 0 2px var(--ink-950),0 0 0 3.5px var(--neon-magenta); display:flex; align-items:center; justify-content:center; font-family:var(--font-display); font-weight:700; font-size:17px; color:var(--neon-magenta-bright); background:rgba(255,46,136,0.05);">66</div>
      <div style="font-family:var(--font-display); font-weight:700; font-size:16px; letter-spacing:0.06em; color:var(--text-strong);">EXIT&nbsp;<span style="color:var(--neon-cyan);">66</span></div>
    </div>
    <div style="flex:1; max-width:480px; margin:0 auto;">
      <SearchInput value={query} onInput={onSearch} placeholder="Search the crate — artist, album, track, or slot code…" />
    </div>
    <div style="display:flex; align-items:center; gap:14px; flex:none;">
      {@render scanChip()}
      {@render adminControl()}
      {#if isAdmin}<CastPanel {onToast} {onCastActive} />{/if}
      <span style="display:inline-flex; align-items:center; gap:8px; padding:7px 12px; border:1px solid var(--border-default); border-radius:var(--radius-sm); font-family:var(--font-mono); font-size:11px; letter-spacing:0.1em; text-transform:uppercase; color:var(--text-muted); white-space:nowrap;"><span style="width:6px; height:6px; border-radius:50%; background:var(--neon-cyan);"></span>{streamChipLabel} listening</span>
      <Avatar name="You" ring="cyan" size="sm" />
    </div>
  </header>
{:else}
  <header style="flex:none; display:flex; flex-direction:column; gap:10px; padding:12px 14px; border-bottom:1px solid var(--border-default); background:var(--ink-950); z-index:5;">
    <div style="display:flex; align-items:center; justify-content:space-between; gap:12px;">
      <div style="display:flex; align-items:center; gap:9px;">
        <div style="width:30px; height:30px; flex:none; border-radius:var(--radius-md); border:1.5px solid var(--neon-magenta); box-shadow:0 0 0 2px var(--ink-950),0 0 0 3px var(--neon-magenta); display:flex; align-items:center; justify-content:center; font-family:var(--font-display); font-weight:700; font-size:15px; color:var(--neon-magenta-bright);">66</div>
        <div style="font-family:var(--font-display); font-weight:700; font-size:15px; letter-spacing:0.06em; color:var(--text-strong);">EXIT&nbsp;<span style="color:var(--neon-cyan);">66</span></div>
      </div>
      <div style="display:flex; align-items:center; gap:9px;">
        {@render adminControl()}
        {#if isAdmin}<CastPanel {onToast} {onCastActive} />{/if}
        <button onclick={onToggleStream} style="display:inline-flex; align-items:center; gap:7px; padding:6px 11px; border:1px solid var(--border-strong); border-radius:var(--radius-sm); background:var(--bg-surface); font-family:var(--font-mono); font-size:10px; letter-spacing:0.1em; text-transform:uppercase; color:var(--text-body); cursor:pointer; white-space:nowrap;"><span style="width:6px; height:6px; border-radius:50%; background:var(--neon-cyan);"></span>{streamChipLabel}</button>
      </div>
    </div>
    <SearchInput value={query} onInput={onSearch} placeholder="Search the crate…" />
    {@render scanChip()}
  </header>
{/if}

<style>
  .scan-pulse { animation: scan-pulse 1.1s ease-in-out infinite; }
  @keyframes scan-pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.25; } }
</style>
