<script>
  import { keyActivate } from '../format.js';
  let { rows = [], onOpen, onRequest } = $props();
</script>
<div style="display:flex; flex-direction:column; gap:8px;">
  {#each rows as ar (ar.id)}
    <div class="row" role="button" tabindex="0" onclick={() => onOpen(ar)} onkeydown={keyActivate(() => onOpen(ar))} style="display:flex; align-items:center; gap:14px; padding:12px 14px; border:1px solid var(--border-default); border-radius:var(--radius-md); background:var(--bg-surface); cursor:pointer; transition:border-color var(--dur) var(--ease-out), background var(--dur) var(--ease-out);">
      <div style="width:46px; height:46px; flex:none; border-radius:50%; background:{ar.gradient}; display:flex; align-items:center; justify-content:center; font-family:var(--font-display); font-weight:700; font-size:18px; color:rgba(255,255,255,0.9); box-shadow:inset 0 0 0 1px rgba(255,255,255,0.1);">{ar.initial}</div>
      <div style="flex:1; min-width:0;">
        <div style="font-family:var(--font-sans); font-weight:600; font-size:15px; color:var(--text-strong); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{ar.name}</div>
        <div style="font-family:var(--font-mono); font-size:11px; letter-spacing:0.06em; color:var(--text-faint); margin-top:3px; text-transform:uppercase; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{ar.meta}</div>
      </div>
      <button class="qa" onclick={(e) => { e.stopPropagation(); onRequest(ar); }}
        style="flex:none; height:34px; padding:0 14px; border:1px solid var(--neon-cyan); background:transparent; color:var(--neon-cyan); border-radius:var(--radius-md); cursor:pointer; font-family:var(--font-display); font-weight:600; font-size:12px; letter-spacing:0.08em; text-transform:uppercase; white-space:nowrap;">+ Queue all</button>
    </div>
  {/each}
</div>
<style>
  .row:hover { border-color: var(--neon-magenta); background: var(--bg-surface-hover); }
  .qa:hover { background: rgba(31,224,255,0.10); }
</style>
