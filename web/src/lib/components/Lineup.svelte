<script>
  import Switch from './Switch.svelte';
  import QueueItem from './QueueItem.svelte';
  let { streamLabel = 'House', listeners = 0, shuffle = false, onToggleShuffle,
        np = null, npPct = '0%', queue = [], isPhone = false, onClose, onRemove } = $props();
</script>
<div style="display:flex; flex-direction:column; gap:13px; height:100%; min-height:0; box-sizing:border-box;">
  <div style="display:flex; align-items:flex-start; justify-content:space-between; gap:12px;">
    <div style="min-width:0;">
      <div style="font-family:var(--font-display); font-weight:700; font-size:16px; letter-spacing:0.08em; text-transform:uppercase; color:var(--text-strong); white-space:nowrap;">The Lineup</div>
      <div style="font-family:var(--font-mono); font-size:10px; letter-spacing:0.14em; text-transform:uppercase; color:var(--text-faint); margin-top:4px;">{streamLabel} stream · {listeners} listening</div>
    </div>
    <div style="display:flex; align-items:center; gap:11px; flex:none;">
      <span style="display:inline-flex; align-items:center; gap:8px; font-family:var(--font-mono); font-size:10px; letter-spacing:0.14em; text-transform:uppercase; color:var(--text-faint); white-space:nowrap;">Shuffle<Switch checked={shuffle} onChange={onToggleShuffle} tone="magenta" /></span>
      {#if isPhone}
        <button aria-label="Close lineup" onclick={onClose} style="width:32px; height:32px; flex:none; border:1px solid var(--border-default); background:var(--bg-surface-raised); color:var(--text-muted); border-radius:var(--radius-sm); cursor:pointer; font-size:15px; line-height:1;">✕</button>
      {/if}
    </div>
  </div>

  {#if np}
    <div style="border:1.5px solid var(--neon-cyan); box-shadow:var(--glow-cyan); border-radius:var(--radius-md); background:var(--bg-inset); padding:12px; display:flex; flex-direction:column; gap:10px; flex:none;">
      <div style="display:flex; align-items:center; gap:12px;">
        <div style="width:46px; height:46px; flex:none; border-radius:var(--radius-sm); background:{np.gradient}; display:flex; align-items:flex-end; padding:5px; box-sizing:border-box; box-shadow:var(--glow-soft-cyan);"><span style="font-family:var(--font-mono); font-size:9px; font-weight:700; color:rgba(255,255,255,0.88);">{np.code}</span></div>
        <div style="flex:1; min-width:0;">
          <div style="font-family:var(--font-mono); font-size:9px; letter-spacing:0.18em; text-transform:uppercase; color:var(--neon-magenta); margin-bottom:3px; display:flex; align-items:center; gap:6px;"><span style="width:6px; height:6px; border-radius:50%; background:var(--neon-magenta);"></span>Now playing</div>
          <div style="font-family:var(--font-sans); font-weight:600; font-size:14px; color:var(--text-strong); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{np.title}</div>
          <div style="font-family:var(--font-sans); font-size:12px; color:var(--text-muted); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{np.artistName} · {np.albumName}</div>
        </div>
        <div style="display:flex; align-items:flex-end; gap:3px; height:20px; flex:none;">
          <span class="eq" style="animation-duration:.5s;"></span>
          <span class="eq" style="animation-duration:.7s;"></span>
          <span class="eq" style="animation-duration:.42s;"></span>
          <span class="eq" style="animation-duration:.62s;"></span>
        </div>
      </div>
      <div style="position:relative; height:4px; border-radius:var(--radius-pill); background:var(--ink-700);"><div style="position:absolute; left:0; top:0; bottom:0; width:{npPct}; border-radius:var(--radius-pill); background:var(--neon-magenta);"></div></div>
    </div>
  {/if}

  <div style="font-family:var(--font-mono); font-size:10px; letter-spacing:0.18em; text-transform:uppercase; color:var(--text-faint); flex:none;">Up next · {queue.length}</div>

  {#if queue.length === 0}
    <div style="flex:1; min-height:0; display:flex; flex-direction:column; align-items:center; justify-content:center; gap:10px; text-align:center; padding:24px 16px; border:1px dashed var(--border-strong); border-radius:var(--radius-md);">
      <div style="font-family:var(--font-display); font-weight:700; font-size:15px; letter-spacing:0.04em; text-transform:uppercase; color:var(--text-muted);">The floor is yours</div>
      <div style="font-family:var(--font-sans); font-size:13px; color:var(--text-faint); max-width:200px;">Nothing queued. Head to the crate and request the next track.</div>
    </div>
  {:else}
    <div style="flex:1; min-height:0; overflow-y:auto; display:flex; flex-direction:column; gap:8px; margin-right:-6px; padding-right:6px;">
      {#each queue as q, i (q.uid)}
        <QueueItem position={i + 1} code={q.code} title={q.title} artist={q.artistName}
          requester={q.requester} tone={q.tone} onRemove={() => onRemove(q)} />
      {/each}
    </div>
  {/if}
</div>
<style>
  .eq { width:3px; height:20px; background:var(--neon-cyan); transform-origin:bottom; animation-name:e66-eq; animation-timing-function:var(--ease-in-out); animation-iteration-count:infinite; animation-direction:alternate; }
</style>
