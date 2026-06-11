<script>
  let { tone = 'magenta', title = '', message = '', onClose } = $props();
  const C = {
    magenta: 'var(--neon-magenta)', cyan: 'var(--neon-cyan)', amber: 'var(--neon-amber)',
    success: 'var(--status-success)', danger: 'var(--status-danger)',
  };
</script>
<div role="status" style="display:flex; align-items:flex-start; gap:12px; min-width:280px; max-width:420px; padding:14px 16px; background:var(--bg-surface-raised); border-radius:var(--radius-md); border:1px solid var(--border-default); border-left:3px solid {C[tone] || C.magenta}; box-shadow:var(--shadow-lg);">
  <div style="flex:1; min-width:0;">
    {#if title}<div style="font-family:var(--font-display); font-weight:600; font-size:14px; letter-spacing:0.04em; text-transform:uppercase; color:var(--text-strong); margin-bottom:{message ? '3px' : '0'};">{title}</div>{/if}
    {#if message}<div style="font-family:var(--font-sans); font-size:13px; line-height:1.5; color:var(--text-muted);">{message}</div>{/if}
  </div>
  {#if onClose}<button onclick={onClose} aria-label="Dismiss" style="background:none; border:none; color:var(--text-faint); cursor:pointer; font-size:16px; line-height:1; padding:2px;">✕</button>{/if}
</div>
