<script>
  import SearchInput from './SearchInput.svelte';
  import Avatar from './Avatar.svelte';
  let { isPhone = false, query = '', onSearch, streamChipLabel = '', onToggleStream } = $props();
</script>
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
      <button onclick={onToggleStream} style="display:inline-flex; align-items:center; gap:7px; padding:6px 11px; border:1px solid var(--border-strong); border-radius:var(--radius-sm); background:var(--bg-surface); font-family:var(--font-mono); font-size:10px; letter-spacing:0.1em; text-transform:uppercase; color:var(--text-body); cursor:pointer; white-space:nowrap;"><span style="width:6px; height:6px; border-radius:50%; background:var(--neon-cyan);"></span>{streamChipLabel}</button>
    </div>
    <SearchInput value={query} onInput={onSearch} placeholder="Search the crate…" />
  </header>
{/if}
