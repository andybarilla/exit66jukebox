<script>
  import IconNext from './icons/IconNext.svelte';
  import IconPlay from './icons/IconPlay.svelte';
  import IconPause from './icons/IconPause.svelte';
  let { np = null, npPct = '0%', playing = true, onPlayPause, onNext, onOpenAlbum } = $props();
  let artFailed = $state(false);
  // Reset the fallback when the now-playing track changes.
  $effect(() => { np?.id; artFailed = false; });
  const canOpenAlbum = $derived(!!np?.albumId && !!onOpenAlbum);
</script>
<div style="flex:none; position:relative; border-top:1px solid var(--border-strong); background:var(--bg-surface-raised); background-image:var(--scanline);">
  <div style="position:absolute; top:0; left:0; right:0; height:3px; background:var(--ink-700);"><div style="height:100%; width:{npPct}; background:var(--neon-magenta);"></div></div>
  <div style="display:flex; align-items:center; gap:12px; padding:11px 14px;">
    {#snippet art()}
      {#if np?.cover && !artFailed}
        <img src={np.cover} alt="" onerror={() => (artFailed = true)} style="position:absolute; inset:0; width:100%; height:100%; object-fit:cover;" />
      {:else}
        <span style="font-family:var(--font-mono); font-size:9px; font-weight:700; color:rgba(255,255,255,0.85);">{np ? np.code : '··'}</span>
      {/if}
    {/snippet}
    {#if canOpenAlbum}
      <button type="button" class="cover-btn" onclick={onOpenAlbum} aria-label="Open album" title="Open album" style="position:relative; width:44px; height:44px; flex:none; border:none; border-radius:var(--radius-sm); overflow:hidden; background:{np.gradient}; display:flex; align-items:flex-end; padding:5px; box-sizing:border-box; cursor:pointer;">{@render art()}</button>
    {:else}
      <div style="position:relative; width:44px; height:44px; flex:none; border-radius:var(--radius-sm); overflow:hidden; background:{np ? np.gradient : 'var(--ink-700)'}; display:flex; align-items:flex-end; padding:5px; box-sizing:border-box;">{@render art()}</div>
    {/if}
    <div style="flex:1; min-width:0;">
      <div style="font-family:var(--font-sans); font-weight:600; font-size:14px; color:var(--text-strong); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{np ? np.title : 'Nothing playing'}</div>
      <div style="font-family:var(--font-sans); font-size:12px; color:var(--text-muted); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{np ? `${np.artistName} · ${np.albumName}` : '—'}</div>
    </div>
    <button aria-label="Play / pause" onclick={onPlayPause} style="width:42px; height:42px; flex:none; border-radius:50%; border:none; background:var(--neon-magenta); color:var(--text-on-accent); cursor:pointer; display:inline-flex; align-items:center; justify-content:center;">{#if playing}<IconPause size={20} />{:else}<IconPlay size={20} />{/if}</button>
    <button aria-label="Next" onclick={onNext} style="width:38px; height:38px; flex:none; border-radius:50%; border:1px solid var(--border-strong); background:transparent; color:var(--text-body); cursor:pointer; display:inline-flex; align-items:center; justify-content:center;"><IconNext size={18} /></button>
  </div>
</div>
<style>
  .cover-btn { transition: box-shadow var(--dur) var(--ease-out), transform var(--dur) var(--ease-out); }
  .cover-btn:hover { transform: scale(1.05); }
  .cover-btn:focus-visible { outline: 2px solid var(--neon-cyan); outline-offset: 2px; }
</style>
