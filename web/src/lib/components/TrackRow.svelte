<script>
  import { toneGradient, keyActivate } from '../format.js';
  let { code = 'A6', title = 'Untitled', artist = 'Unknown', duration = '0:00',
        cover = null, gradient = null, tone = 'magenta', explicit = false,
        playing = false, onAdd, onClick } = $props();
  let artFailed = $state(false);
  const tile = $derived(gradient || toneGradient(tone));
</script>
<div class="tr" class:playing
  {...onClick ? { role: 'button', tabindex: 0, onclick: onClick, onkeydown: keyActivate(onClick) } : {}}
  style="display:flex; align-items:center; gap:14px; padding:10px 12px; border-radius:var(--radius-md); cursor:pointer; transition:background var(--dur) var(--ease-out); border:1px solid {playing ? 'rgba(255,46,136,0.35)' : 'transparent'}; background:{playing ? 'rgba(255,46,136,0.08)' : 'transparent'};">
  <div style="position:relative; width:46px; height:46px; flex:none; border-radius:var(--radius-sm); overflow:hidden; box-shadow:{playing ? 'var(--glow-magenta)' : 'inset 0 0 0 1px rgba(255,255,255,0.08)'}; background:{tile}; display:flex; align-items:flex-end; padding:5px; box-sizing:border-box;">
    {#if cover && !artFailed}
      <img src={cover} alt="" onerror={() => (artFailed = true)} style="position:absolute; inset:0; width:100%; height:100%; object-fit:cover;" />
    {:else}
      <span style="font-family:var(--font-mono); font-size:9px; font-weight:700; letter-spacing:0.1em; color:rgba(255,255,255,0.85);">{code}</span>
    {/if}
    {#if playing}<span style="position:absolute; top:5px; right:5px; width:6px; height:6px; border-radius:50%; background:var(--neon-magenta);"></span>{/if}
  </div>
  <div style="flex:1; min-width:0;">
    <div style="display:flex; align-items:center; gap:8px;">
      <span style="font-family:var(--font-sans); font-weight:600; font-size:15px; color:{playing ? 'var(--neon-magenta-bright)' : 'var(--text-strong)'}; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{title}</span>
      {#if explicit}<span style="font-family:var(--font-mono); font-size:9px; font-weight:700; color:var(--neon-amber); border:1px solid var(--neon-amber); border-radius:2px; padding:0 3px; line-height:13px; flex:none;">E</span>{/if}
    </div>
    <div style="font-family:var(--font-sans); font-size:13px; color:var(--text-muted); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{artist}</div>
  </div>
  <span style="font-family:var(--font-mono); font-size:13px; color:var(--text-faint); flex:none;">{duration}</span>
  {#if onAdd}
    <button class="add" aria-label="Add to queue" onclick={(e) => { e.stopPropagation(); onAdd(); }}
      style="width:34px; height:34px; flex:none; border-radius:var(--radius-sm); display:inline-flex; align-items:center; justify-content:center; background:var(--bg-surface-raised); color:var(--text-muted); border:1px solid var(--border-default); cursor:pointer; font-size:18px; line-height:1; transition:all var(--dur) var(--ease-out);">+</button>
  {/if}
</div>
<style>
  .tr:not(.playing):hover { background: var(--bg-surface-hover) !important; }
  .tr:hover .add { background: var(--neon-magenta); color: var(--text-on-accent); box-shadow: var(--glow-soft-magenta); }
</style>
