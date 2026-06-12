<script>
  import { fmt, toneGradient } from '../format.js';
  let { title = 'Nothing playing', artist = '—', code = 'A6', cover = null, gradient = null,
        tone = 'magenta', current = 0, duration = 0, playing = false, volume = 70,
        onPlayPause, onPrev, onNext, onSeek, onVolume } = $props();
  let artFailed = $state(false);
  let scrub;
  const pct = $derived(duration ? Math.min(100, (current / duration) * 100) : 0);
  const tile = $derived(gradient || toneGradient(tone));
  function seekFrom(clientX) {
    if (!scrub || !onSeek) return;
    const r = scrub.getBoundingClientRect();
    onSeek(Math.max(0, Math.min(1, (clientX - r.left) / r.width)));
  }
  function volFrom(e) {
    const r = e.currentTarget.getBoundingClientRect();
    onVolume && onVolume(Math.round(Math.max(0, Math.min(1, (e.clientX - r.left) / r.width)) * 100));
  }
</script>
<div style="display:flex; align-items:center; gap:20px; height:84px; padding:0 22px; background:var(--bg-surface-raised); background-image:var(--scanline); border-top:1px solid var(--border-strong); box-shadow:0 -8px 30px rgba(0,0,0,0.5);">
  <!-- track -->
  <div style="display:flex; align-items:center; gap:14px; width:280px; flex:none;">
    <div style="position:relative; width:54px; height:54px; flex:none; border-radius:var(--radius-sm); overflow:hidden; background:{tile}; display:flex; align-items:flex-end; padding:6px; box-sizing:border-box; box-shadow:{playing ? 'var(--glow-soft-magenta)' : 'none'};">
      {#if cover && !artFailed}
        <img src={cover} alt="" onerror={() => (artFailed = true)} style="position:absolute; inset:0; width:100%; height:100%; object-fit:cover;" />
      {:else}
        <span style="font-family:var(--font-mono); font-size:10px; font-weight:700; color:rgba(255,255,255,0.85);">{code}</span>
      {/if}
    </div>
    <div style="min-width:0;">
      <div style="font-family:var(--font-sans); font-weight:600; font-size:15px; color:var(--text-strong); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{title}</div>
      <div style="font-family:var(--font-sans); font-size:13px; color:var(--text-muted); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{artist}</div>
    </div>
  </div>
  <!-- transport + scrub -->
  <div style="flex:1; min-width:0; display:flex; flex-direction:column; gap:7px; align-items:center;">
    <div style="display:flex; align-items:center; gap:10px;">
      <button class="t" aria-label="Previous" onclick={onPrev} style="width:38px; height:38px;">⏮</button>
      <button class="t primary" aria-label={playing ? 'Pause' : 'Play'} onclick={onPlayPause} style="width:46px; height:46px;">{playing ? '❚❚' : '▶'}</button>
      <button class="t" aria-label="Next" onclick={onNext} style="width:38px; height:38px;">⏭</button>
    </div>
    <div style="display:flex; align-items:center; gap:12px; width:100%; max-width:520px;">
      <span style="font-family:var(--font-mono); font-size:11px; color:var(--text-faint); width:38px; text-align:right;">{fmt(current)}</span>
      <div bind:this={scrub} onmousedown={(e) => seekFrom(e.clientX)} role="slider" tabindex="0" aria-label="Seek" aria-valuenow={Math.round(pct)} style="position:relative; flex:1; height:14px; display:flex; align-items:center; cursor:pointer;">
        <div style="position:absolute; left:0; right:0; height:4px; border-radius:var(--radius-pill); background:var(--ink-700);"></div>
        <div style="position:absolute; left:0; width:{pct}%; height:4px; border-radius:var(--radius-pill); background:var(--neon-magenta);"></div>
        <div style="position:absolute; left:calc({pct}% - 6px); width:12px; height:12px; border-radius:50%; background:var(--paper-100); border:2px solid var(--neon-magenta);"></div>
      </div>
      <span style="font-family:var(--font-mono); font-size:11px; color:var(--text-faint); width:38px;">{fmt(duration)}</span>
    </div>
  </div>
  <!-- volume -->
  <div style="display:flex; align-items:center; gap:10px; width:160px; flex:none;">
    <span style="color:var(--text-muted); font-size:16px;">♪</span>
    <div onmousedown={volFrom} role="slider" tabindex="0" aria-label="Volume" aria-valuenow={volume} style="position:relative; flex:1; height:14px; display:flex; align-items:center; cursor:pointer;">
      <div style="position:absolute; left:0; right:0; height:4px; border-radius:var(--radius-pill); background:var(--ink-700);"></div>
      <div style="position:absolute; left:0; width:{volume}%; height:4px; border-radius:var(--radius-pill); background:var(--neon-cyan);"></div>
      <div style="position:absolute; left:calc({volume}% - 6px); width:12px; height:12px; border-radius:50%; background:var(--paper-100); border:2px solid var(--neon-cyan);"></div>
    </div>
  </div>
</div>
<style>
  .t { flex:none; border-radius:50%; cursor:pointer; display:inline-flex; align-items:center; justify-content:center; font-size:17px; line-height:1; background:transparent; color:var(--text-body); border:1px solid transparent; transition:all var(--dur) var(--ease-out); }
  .t.primary { font-size:20px; background:var(--neon-magenta); color:var(--text-on-accent); border:none; }
  .t.primary:hover { box-shadow: var(--glow-magenta); }
</style>
