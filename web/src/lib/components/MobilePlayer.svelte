<script>
  let { np = null, npPct = '0%', playing = true, onPlayPause, onNext } = $props();
</script>
<div style="flex:none; position:relative; border-top:1px solid var(--border-strong); background:var(--bg-surface-raised); background-image:var(--scanline);">
  <div style="position:absolute; top:0; left:0; right:0; height:3px; background:var(--ink-700);"><div style="height:100%; width:{npPct}; background:var(--neon-magenta);"></div></div>
  <div style="display:flex; align-items:center; gap:12px; padding:11px 14px;">
    <div style="width:44px; height:44px; flex:none; border-radius:var(--radius-sm); background:{np ? np.gradient : 'var(--ink-700)'}; display:flex; align-items:flex-end; padding:5px; box-sizing:border-box;"><span style="font-family:var(--font-mono); font-size:9px; font-weight:700; color:rgba(255,255,255,0.85);">{np ? np.code : '··'}</span></div>
    <div style="flex:1; min-width:0;">
      <div style="font-family:var(--font-sans); font-weight:600; font-size:14px; color:var(--text-strong); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{np ? np.title : 'Nothing playing'}</div>
      <div style="font-family:var(--font-sans); font-size:12px; color:var(--text-muted); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{np ? `${np.artistName} · ${np.albumName}` : '—'}</div>
    </div>
    <button aria-label="Play / pause" onclick={onPlayPause} style="width:42px; height:42px; flex:none; border-radius:50%; border:none; background:var(--neon-magenta); color:var(--text-on-accent); font-size:18px; cursor:pointer;">{playing ? '❚❚' : '▶'}</button>
    <button aria-label="Next" onclick={onNext} style="width:38px; height:38px; flex:none; border-radius:50%; border:1px solid var(--border-strong); background:transparent; color:var(--text-body); font-size:16px; cursor:pointer;">⏭</button>
  </div>
</div>
