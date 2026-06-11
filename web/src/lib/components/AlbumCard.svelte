<script>
  let { title, artist, meta, initial, cover = null, gradient = null, onOpen, onRequest } = $props();
  let artFailed = $state(false);
</script>
<div class="card" onclick={onOpen} style="position:relative; background:var(--bg-surface); background-image:var(--scanline); border:1px solid var(--border-default); border-radius:var(--radius-lg); overflow:hidden; cursor:pointer; transition:border-color var(--dur) var(--ease-out), transform var(--dur) var(--ease-out);">
  <div style="position:relative; aspect-ratio:1/1; overflow:hidden; background:{gradient};">
    {#if cover && !artFailed}
      <img src={cover} alt="" onerror={() => (artFailed = true)} style="position:absolute; inset:0; width:100%; height:100%; object-fit:cover;" />
    {/if}
    <span style="position:absolute; top:9px; left:10px; font-family:var(--font-mono); font-size:9px; letter-spacing:0.18em; color:rgba(246,243,235,0.62); text-transform:uppercase;">Art</span>
    {#if !cover || artFailed}
      <span style="position:absolute; left:50%; top:48%; transform:translate(-50%,-50%); font-family:var(--font-display); font-weight:700; font-size:72px; line-height:1; color:rgba(255,255,255,0.15);">{initial}</span>
    {/if}
    <button class="req" aria-label="Request album" onclick={(e) => { e.stopPropagation(); onRequest(); }}
      style="position:absolute; right:10px; bottom:10px; width:32px; height:32px; border-radius:var(--radius-md); border:1.5px solid var(--neon-magenta); background:var(--neon-magenta); color:var(--text-on-accent); font-size:19px; line-height:1; cursor:pointer; display:inline-flex; align-items:center; justify-content:center; box-shadow:var(--shadow-sm);">+</button>
  </div>
  <div style="padding:11px 13px 13px;">
    <div style="font-family:var(--font-sans); font-weight:600; font-size:14px; color:var(--text-strong); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{title}</div>
    <div style="font-family:var(--font-sans); font-size:13px; color:var(--text-muted); white-space:nowrap; overflow:hidden; text-overflow:ellipsis; margin-top:2px;">{artist}</div>
    <div style="font-family:var(--font-mono); font-size:10px; letter-spacing:0.13em; color:var(--text-faint); margin-top:9px; text-transform:uppercase;">{meta}</div>
  </div>
</div>
<style>
  .card:hover { border-color: var(--neon-magenta); transform: translateY(-2px); }
  .req:hover { background: var(--neon-magenta-bright); }
</style>
