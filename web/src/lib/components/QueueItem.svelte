<script>
  import { toneGradient } from '../format.js';
  let { position = 1, title = 'Untitled', artist = 'Unknown', code = 'A6',
        requester = '', tone = 'magenta', onRemove } = $props();
  const initials = $derived(requester.split(' ').map((w) => w[0]).filter(Boolean).slice(0, 2).join('').toUpperCase());
</script>
<div class="qi" style="display:flex; align-items:center; gap:12px; padding:10px 12px; border-radius:var(--radius-md); background:var(--bg-surface); border:1px solid var(--border-default); transition:background var(--dur) var(--ease-out);">
  <span style="color:var(--text-disabled); font-size:14px; letter-spacing:-2px; flex:none;">⋮⋮</span>
  <span style="font-family:var(--font-mono); font-size:15px; font-weight:700; color:var(--neon-cyan); width:22px; text-align:center; flex:none;">{position}</span>
  <div style="width:38px; height:38px; flex:none; border-radius:var(--radius-sm); background:{toneGradient(tone)}; display:flex; align-items:flex-end; padding:4px; box-sizing:border-box;">
    <span style="font-family:var(--font-mono); font-size:8px; font-weight:700; color:rgba(255,255,255,0.85);">{code}</span>
  </div>
  <div style="flex:1; min-width:0;">
    <div style="font-family:var(--font-sans); font-weight:600; font-size:14px; color:var(--text-strong); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{title}</div>
    <div style="font-family:var(--font-sans); font-size:12px; color:var(--text-muted); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{artist}</div>
  </div>
  {#if requester}
    <div title={requester} style="width:28px; height:28px; flex:none; border-radius:50%; background:linear-gradient(140deg, var(--ink-700), var(--ink-850)); display:inline-flex; align-items:center; justify-content:center; font-family:var(--font-mono); font-size:11px; font-weight:700; color:var(--paper-200);">{initials}</div>
  {/if}
  {#if onRemove}<button class="rm" onclick={onRemove} aria-label="Remove" style="background:none; border:none; color:var(--text-faint); cursor:pointer; font-size:15px; flex:none;">✕</button>{/if}
</div>
<style>
  .qi:hover { background: var(--bg-surface-hover) !important; }
  .rm { opacity: 0; transition: opacity var(--dur) var(--ease-out); }
  .qi:hover .rm { opacity: 1; }
</style>
