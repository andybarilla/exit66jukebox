<script>
  let { checked = false, onChange, tone = 'magenta' } = $props();
  const T = {
    magenta: { c: 'var(--neon-magenta)', glow: 'var(--glow-soft-magenta)' },
    cyan: { c: 'var(--neon-cyan)', glow: 'var(--glow-soft-cyan)' },
    amber: { c: 'var(--neon-amber)', glow: 'var(--glow-amber)' },
  };
  const t = $derived(T[tone] || T.magenta);
</script>
<span role="switch" aria-checked={checked} onclick={() => onChange && onChange(!checked)} tabindex="0"
  style="width:46px; height:26px; flex:none; border-radius:var(--radius-pill); background:{checked ? t.c : 'var(--ink-700)'}; border:1px solid {checked ? t.c : 'var(--border-strong)'}; box-shadow:{checked ? t.glow : 'inset 0 1px 2px rgba(0,0,0,0.5)'}; cursor:pointer; position:relative; transition:all var(--dur) var(--ease-out); display:inline-block;">
  <span style="position:absolute; top:2px; left:{checked ? '22px' : '2px'}; width:20px; height:20px; border-radius:50%; background:{checked ? 'var(--text-on-accent)' : 'var(--paper-300)'}; transition:left var(--dur) var(--ease-snap); box-shadow:0 1px 3px rgba(0,0,0,0.5);"></span>
</span>
